package gh

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Profile is a rough skill fingerprint built from a GitHub user's own
// public activity, used to auto-set sensible -languages/-level defaults
// instead of asking the user to specify them by hand every time.
type Profile struct {
	Username              string   `json:"username"`
	TopLanguages          []string `json:"top_languages"`
	ExternalMergedPRCount int      `json:"external_merged_pr_count"` // merged PRs to repos they DON'T own
	Level                 string   `json:"level"`                    // beginner | intermediate | advanced
}

type repoSummary struct {
	Language string `json:"language"`
	Fork     bool   `json:"fork"`
}

// CurrentUsername resolves the login of the authenticated user (requires
// a token) via GET /user. Used when -github-user isn't passed explicitly
// but a token is available.
func (c *Client) CurrentUsername() (string, error) {
	if c.Token == "" {
		return "", fmt.Errorf("no token set — pass -github-user explicitly or set GITHUB_TOKEN")
	}
	body, _, err := c.doGet("https://api.github.com/user")
	if err != nil {
		return "", fmt.Errorf("fetching authenticated user: %w", err)
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return "", fmt.Errorf("decoding user: %w", err)
	}
	if u.Login == "" {
		return "", fmt.Errorf("token didn't resolve to a username")
	}
	return u.Login, nil
}

// DetectProfile builds a Profile for username from their own public repos
// (for language distribution) and how many of their pull requests have
// been merged into OTHER people's repos (as a rough open-source
// contribution-experience proxy -- PRs merged into their own repos don't
// count, since that's not the skill this tool is trying to match against).
// Forked repos are excluded from the language tally so the result
// reflects languages they've actually authored in, not just cloned.
func (c *Client) DetectProfile(username string, topN int) (Profile, error) {
	if topN <= 0 {
		topN = 3
	}

	langCounts := map[string]int{}
	const maxPages = 3 // caps at 300 repos; plenty for a language fingerprint
	for page := 1; page <= maxPages; page++ {
		url := fmt.Sprintf("https://api.github.com/users/%s/repos?per_page=100&page=%d&type=owner", username, page)
		body, _, err := c.doGet(url)
		if err != nil {
			return Profile{}, fmt.Errorf("fetching repos for %s: %w", username, err)
		}
		var repos []repoSummary
		if err := json.Unmarshal(body, &repos); err != nil {
			return Profile{}, fmt.Errorf("decoding repos for %s: %w", username, err)
		}
		for _, r := range repos {
			if r.Fork || r.Language == "" {
				continue
			}
			langCounts[r.Language]++
		}
		if len(repos) < 100 {
			break // last page
		}
	}

	if len(langCounts) == 0 {
		return Profile{}, fmt.Errorf("no language data found in %s's public, non-fork repos", username)
	}

	type kv struct {
		Lang  string
		Count int
	}
	ranked := make([]kv, 0, len(langCounts))
	for lang, count := range langCounts {
		ranked = append(ranked, kv{lang, count})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Count != ranked[j].Count {
			return ranked[i].Count > ranked[j].Count
		}
		return ranked[i].Lang < ranked[j].Lang // stable tie-break
	})

	top := make([]string, 0, topN)
	for i := 0; i < len(ranked) && i < topN; i++ {
		top = append(top, ranked[i].Lang)
	}

	// -user:username excludes repos OWNED by username, so this only counts
	// PRs merged into repos they contributed to but don't own.
	query := fmt.Sprintf("is:pr is:merged author:%s -user:%s", username, username)
	prCount, err := c.SearchCount(query)
	if err != nil {
		return Profile{}, fmt.Errorf("counting external merged PRs for %s: %w", username, err)
	}

	return Profile{
		Username:              username,
		TopLanguages:          top,
		ExternalMergedPRCount: prCount,
		Level:                 levelFromMergedPRs(prCount),
	}, nil
}

func levelFromMergedPRs(count int) string {
	switch {
	case count >= 30:
		return "advanced"
	case count >= 6:
		return "intermediate"
	default:
		return "beginner"
	}
}
