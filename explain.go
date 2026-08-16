package main

import (
	"fmt"
	"strings"

	"ossfind/internal/gh"
)

// matchedLabels returns which of issueLabels are also in searched
// (case-insensitive), preserving the issue's own label casing for display.
func matchedLabels(issueLabels, searched []string) []string {
	if len(searched) == 0 {
		return nil
	}
	want := map[string]bool{}
	for _, s := range searched {
		want[strings.ToLower(s)] = true
	}
	var out []string
	for _, l := range issueLabels {
		if want[strings.ToLower(l)] {
			out = append(out, l)
		}
	}
	return out
}

// matchReason explains why an issue matched the language/label search --
// the base of the "why" transparency shown in every output mode.
func matchReason(iss gh.Issue, searchedLabels []string) string {
	if matched := matchedLabels(iss.LabelNames(), searchedLabels); len(matched) > 0 {
		return "label: " + strings.Join(matched, ", ")
	}
	return "language + recency match (no label filter)"
}

// scoreReason explains a -score difficulty rating in terms of its inputs.
func scoreReason(iss gh.ScoredIssue) string {
	return fmt.Sprintf("%d comment(s), %d star(s) → %s (score %d/4)",
		iss.Comments, iss.RepoStars, iss.Difficulty, iss.Score)
}

// healthReason explains a -health verdict for a specific repo, if one was computed.
func healthReason(repo string, healthByRepo map[string]gh.RepoHealth) string {
	h, ok := healthByRepo[repo]
	if !ok {
		return ""
	}
	return "repo: " + h.Reason
}

// why composes the full match explanation for a plain (unscored) issue.
func why(iss gh.Issue, searchedLabels []string, healthByRepo map[string]gh.RepoHealth) string {
	parts := []string{matchReason(iss, searchedLabels)}
	if h := healthReason(iss.Repo(), healthByRepo); h != "" {
		parts = append(parts, h)
	}
	return strings.Join(parts, "; ")
}

// whyScored composes the full match explanation for a scored issue.
func whyScored(iss gh.ScoredIssue, searchedLabels []string, healthByRepo map[string]gh.RepoHealth) string {
	parts := []string{matchReason(iss.Issue, searchedLabels), scoreReason(iss)}
	if h := healthReason(iss.Repo(), healthByRepo); h != "" {
		parts = append(parts, h)
	}
	return strings.Join(parts, "; ")
}
