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

func writeJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "error encoding json: %v\n", err)
		os.Exit(1)
	}
}

func printTable(issues []gh.Issue) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "REPO\tTITLE\tLABELS\tUPDATED\tURL")
	for _, iss := range issues {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			iss.Repo(),
			truncate(iss.Title, 60),
			strings.Join(iss.LabelNames(), ","),
			iss.UpdatedAt.Format(time.RFC3339)[:10],
			iss.HTMLURL,
		)
	}
	w.Flush()
}

func printScoredTable(issues []gh.ScoredIssue) {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "DIFFICULTY\tSTARS\tREPO\tTITLE\tUPDATED\tURL")
	for _, iss := range issues {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\n",
			iss.Difficulty,
			iss.RepoStars,
			iss.Repo(),
			truncate(iss.Title, 50),
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
