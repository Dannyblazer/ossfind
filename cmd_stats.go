package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"ossfind/internal/state"
)

func runStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	statePath := fs.String("state-path", "", "override the local state file location (default: ~/.config/ossfind/state.json)")
	fs.Parse(args)

	path := *statePath
	if path == "" {
		p, err := state.DefaultPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error resolving state path: %v\n", err)
			os.Exit(1)
		}
		path = p
	}

	st, err := state.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading state: %v\n", err)
		os.Exit(1)
	}

	sum := st.Summarize(time.Now())

	if sum.TotalRuns == 0 {
		fmt.Println("No history yet — run `ossfind` a few times and check back here.")
		return
	}

	fmt.Printf("Runs:            %d (first: %s, last: %s)\n", sum.TotalRuns, sum.FirstRun, sum.LastRun)
	fmt.Printf("Current streak:  %d day(s)\n", sum.CurrentStreak)
	fmt.Printf("Issues surfaced: %d unique (%d total impressions) across %d repo(s)\n",
		sum.TotalUniqueIssues, sum.TotalTimesShown, sum.UniqueRepos)

	if len(sum.TopRepos) > 0 {
		fmt.Println("\nMost-surfaced repos:")
		for _, rc := range sum.TopRepos {
			fmt.Printf("  %-40s %d\n", rc.Repo, rc.Count)
		}
	}
}
