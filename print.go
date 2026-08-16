package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"ossfind/internal/gh"
)

// jsonIssue/jsonScoredIssue wrap the gh package's plain API types with the
// computed "why" explanation for JSON output, without polluting gh.Issue /
// gh.ScoredIssue (which map directly to GitHub's API shape) with
// ossfind-specific display fields.
type jsonIssue struct {
	gh.Issue
	Why string `json:"why"`
}

type jsonScoredIssue struct {
	gh.ScoredIssue
	Why string `json:"why"`
}

func writeJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "error encoding json: %v\n", err)
		os.Exit(1)
	}
}

func writeIssuesJSON(issues []gh.Issue, searchedLabels []string, healthByRepo map[string]gh.RepoHealth) {
	out := make([]jsonIssue, len(issues))
	for i, iss := range issues {
		out[i] = jsonIssue{Issue: iss, Why: why(iss, searchedLabels, healthByRepo)}
	}
	writeJSON(out)
}

func writeScoredJSON(issues []gh.ScoredIssue, searchedLabels []string, healthByRepo map[string]gh.RepoHealth) {
	out := make([]jsonScoredIssue, len(issues))
	for i, iss := range issues {
		out[i] = jsonScoredIssue{ScoredIssue: iss, Why: whyScored(iss, searchedLabels, healthByRepo)}
	}
	writeJSON(out)
}

func printTable(issues []gh.Issue, searchedLabels []string, healthByRepo map[string]gh.RepoHealth) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "REPO\tTITLE\tWHY\tUPDATED\tURL")
	for _, iss := range issues {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			iss.Repo(),
			truncate(iss.Title, 50),
			truncate(why(iss, searchedLabels, healthByRepo), 60),
			iss.UpdatedAt.Format(time.RFC3339)[:10],
			iss.HTMLURL,
		)
	}
	w.Flush()
}

func printScoredTable(issues []gh.ScoredIssue, searchedLabels []string, healthByRepo map[string]gh.RepoHealth) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "DIFFICULTY\tREPO\tTITLE\tWHY\tUPDATED\tURL")
	for _, iss := range issues {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			iss.Difficulty,
			iss.Repo(),
			truncate(iss.Title, 40),
			truncate(whyScored(iss, searchedLabels, healthByRepo), 70),
			iss.UpdatedAt.Format(time.RFC3339)[:10],
			iss.HTMLURL,
		)
	}
	w.Flush()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

func splitAndTrim(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
