// Repo-health filtering: skip issues that live in repos that look dead or
// unresponsive, so -score's difficulty estimate never sends you into a
// repo where a PR would just sit unmerged. Two signals, both free of any
// extra API cost beyond one repo lookup + one PR-list lookup per repo:
//
//   - staleness: days since the repo's last push
//   - responsiveness: merge rate across its most recently closed PRs
package gh

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// HealthOptions configures the thresholds used to judge a repo healthy.
type HealthOptions struct {
	MaxStaleDays int     // repo is "Stale" if its last push is older than this
	MinMergeRate float64 // repo is "Unresponsive" if recent PR merge rate is below this (0.0-1.0)
	PRSample     int     // how many recently-closed PRs to sample for merge rate
}

// minMergeSampleSize is the minimum number of closed PRs needed before the
// merge-rate signal is trusted. Below this, a repo isn't penalized for a
// thin sample (e.g. a small repo with only 1-2 closed PRs ever).
const minMergeSampleSize = 3

// RepoHealth is the computed health verdict for a single repo.
type RepoHealth struct {
	Repo             string    `json:"repo"`
	LastPushed       time.Time `json:"last_pushed"`
	DaysSincePush    int       `json:"days_since_push"`
	ClosedPRsSampled int       `json:"closed_prs_sampled"`
	MergedPRs        int       `json:"merged_prs"`
	MergeRate        float64   `json:"merge_rate"` // -1 if sample too small to trust
	Healthy          bool      `json:"healthy"`
	Status           string    `json:"status"` // "Healthy", "Stale", "Unresponsive", "Stale+Unresponsive"
	Reason           string    `json:"reason"` // short human-readable explanation
}

type pullSummary struct {
	MergedAt *string `json:"merged_at"` // null if closed-without-merge
}

// recentClosedPRs fetches up to `sample` most-recently-updated closed PRs.
func (c *Client) recentClosedPRs(repoFullName string, sample int) ([]pullSummary, error) {
	if sample <= 0 || sample > 100 {
		sample = 10
	}
	url := fmt.Sprintf("%s%s/pulls?state=closed&per_page=%d&sort=updated&direction=desc", reposURL, repoFullName, sample)
	body, _, err := c.doGet(url)
	if err != nil {
		return nil, fmt.Errorf("fetching closed PRs for %s: %w", repoFullName, err)
	}
	var prs []pullSummary
	if err := json.Unmarshal(body, &prs); err != nil {
		return nil, fmt.Errorf("decoding closed PRs for %s: %w", repoFullName, err)
	}
	return prs, nil
}

// CheckRepoHealth fetches a repo's last-push date and recent merge rate and
// judges it against opts.
func (c *Client) CheckRepoHealth(repoFullName string, opts HealthOptions) (RepoHealth, error) {
	info, err := c.GetRepo(repoFullName)
	if err != nil {
		return RepoHealth{}, err
	}
	prs, err := c.recentClosedPRs(repoFullName, opts.PRSample)
	if err != nil {
		return RepoHealth{}, err
	}

	merged := 0
	for _, pr := range prs {
		if pr.MergedAt != nil {
			merged++
		}
	}

	mergeRate := -1.0
	if len(prs) >= minMergeSampleSize {
		mergeRate = float64(merged) / float64(len(prs))
	}

	daysSincePush := int(time.Since(info.PushedAt).Hours() / 24)

	var reasons []string
	stale := daysSincePush > opts.MaxStaleDays
	if stale {
		reasons = append(reasons, fmt.Sprintf("no push in %d days (limit %d)", daysSincePush, opts.MaxStaleDays))
	}
	unresponsive := mergeRate >= 0 && mergeRate < opts.MinMergeRate
	if unresponsive {
		reasons = append(reasons, fmt.Sprintf("merge rate %.0f%% (%d/%d recent PRs) below %.0f%% threshold",
			mergeRate*100, merged, len(prs), opts.MinMergeRate*100))
	}

	healthy := !stale && !unresponsive
	status := "Healthy"
	switch {
	case stale && unresponsive:
		status = "Stale+Unresponsive"
	case stale:
		status = "Stale"
	case unresponsive:
		status = "Unresponsive"
	}
	reason := "active and responsive"
	if len(reasons) > 0 {
		reason = strings.Join(reasons, "; ")
	}

	return RepoHealth{
		Repo:             repoFullName,
		LastPushed:       info.PushedAt,
		DaysSincePush:    daysSincePush,
		ClosedPRsSampled: len(prs),
		MergedPRs:        merged,
		MergeRate:        mergeRate,
		Healthy:          healthy,
		Status:           status,
		Reason:           reason,
	}, nil
}

// FilterHealthy checks every unique repo among issues (deduped -- one
// health check per repo, not per issue) and splits them into kept
// (healthy-repo issues) and dropped (unhealthy-repo issues), along with
// the full per-repo health verdicts for reporting.
func (c *Client) FilterHealthy(issues []Issue, opts HealthOptions) (kept, dropped []Issue, healthByRepo map[string]RepoHealth, err error) {
	healthByRepo = map[string]RepoHealth{}

	for _, iss := range issues {
		repo := iss.Repo()
		if _, ok := healthByRepo[repo]; ok {
			continue
		}
		h, err := c.CheckRepoHealth(repo, opts)
		if err != nil {
			return nil, nil, nil, err
		}
		healthByRepo[repo] = h
		time.Sleep(300 * time.Millisecond) // be polite to the core rate limit
	}

	for _, iss := range issues {
		if healthByRepo[iss.Repo()].Healthy {
			kept = append(kept, iss)
		} else {
			dropped = append(dropped, iss)
		}
	}
	return kept, dropped, healthByRepo, nil
}
