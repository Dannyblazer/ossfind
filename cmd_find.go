package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"ossfind/internal/gh"
	"ossfind/internal/state"
)

func runFind(args []string) {
	fs := flag.NewFlagSet("find", flag.ExitOnError)
	var (
		languages   = fs.String("languages", "Go", "comma-separated GitHub language names (e.g. Go,Python)")
		level       = fs.String("level", "beginner", "beginner | intermediate | advanced (sets a default label set)")
		labelsFlag  = fs.String("labels", "", "comma-separated labels; overrides -level's default labels")
		sinceDays   = fs.Int("since", 7, "only include issues created in the last N days (0 = no limit)")
		limit       = fs.Int("limit", 20, "max issues to print after merging/deduping")
		perQuery    = fs.Int("per-query", 20, "max results fetched per individual search query")
		sortBy      = fs.String("sort", "updated", "updated | created | difficulty")
		score       = fs.Bool("score", false, "attach a rough difficulty estimate (comments + repo stars) to each issue")
		scorePool   = fs.Int("score-pool", 40, "when -sort difficulty, how many of the most recent candidates to score before ranking (bounds API calls)")
		token       = fs.String("token", os.Getenv("GITHUB_TOKEN"), "GitHub token (defaults to $GITHUB_TOKEN; raises your rate limit)")
		includeAll  = fs.Bool("include-assigned", false, "include issues that already have an assignee")
		asJSON      = fs.Bool("json", false, "print raw JSON instead of a table")
		verbose     = fs.Bool("v", false, "log each query and rate-limit status to stderr")
		cooldown    = fs.Int("cooldown", 7, "days to hide an issue after it's been shown once (0 = never hide repeats)")
		showRepeats = fs.Bool("show-repeats", false, "ignore the cooldown for this run (shows previously-seen issues too)")
		noState     = fs.Bool("no-state", false, "don't read or write local history at all (stateless run)")
		statePath   = fs.String("state-path", "", "override the local state file location (default: ~/.config/ossfind/state.json)")
	)
	fs.Parse(args)

	langs := splitAndTrim(*languages)
	if len(langs) == 0 {
		fmt.Fprintln(os.Stderr, "error: -languages must not be empty")
		os.Exit(1)
	}

	var labels []string
	if *labelsFlag != "" {
		labels = splitAndTrim(*labelsFlag)
	} else {
		defaults, ok := gh.LevelLabels[strings.ToLower(*level)]
		if !ok {
			fmt.Fprintf(os.Stderr, "error: unknown -level %q (want beginner, intermediate, or advanced)\n", *level)
			os.Exit(1)
		}
		labels = defaults
	}

	sortByDifficulty := *sortBy == "difficulty"
	if sortByDifficulty {
		*score = true // ranking by difficulty implies computing it
	}
	apiSort := *sortBy
	if sortByDifficulty {
		apiSort = "updated" // difficulty isn't a GitHub-side sort; fetch by recency, rank client-side
	} else if apiSort != "updated" && apiSort != "created" {
		fmt.Fprintf(os.Stderr, "error: unknown -sort %q (want updated, created, or difficulty)\n", *sortBy)
		os.Exit(1)
	}

	client := gh.NewClient(*token)
	if *verbose {
		client.LogFunc = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}
	if *token == "" && *verbose {
		fmt.Fprintln(os.Stderr, "note: no token set — limited to 10 search req/min and 60 repo lookups/hour. Set GITHUB_TOKEN to raise both.")
	}

	// --- local state (seen-issue history) ---
	var st *state.State
	if !*noState {
		path := *statePath
		if path == "" {
			p, err := state.DefaultPath()
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: couldn't resolve state path (%v) — continuing without local history\n", err)
			} else {
				path = p
			}
		}
		if path != "" {
			loaded, err := state.Load(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: couldn't load local state (%v) — continuing without local history\n", err)
			} else {
				st = loaded
			}
		}
	}

	issues, err := client.Find(gh.FinderOptions{
		Languages:   langs,
		Labels:      labels,
		SinceDays:   *sinceDays,
		Unassigned:  !*includeAll,
		SortBy:      apiSort,
		PerLanguage: *perQuery,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	now := time.Now()

	// Filter out issues shown within the cooldown window, before truncating
	// to -limit, so a fresh run actually gets fresh issues where possible.
	skippedForCooldown := 0
	if st != nil && !*showRepeats && *cooldown > 0 {
		fresh := issues[:0:0]
		for _, iss := range issues {
			if st.IsCool(iss.HTMLURL, *cooldown, now) {
				fresh = append(fresh, iss)
			} else {
				skippedForCooldown++
			}
		}
		issues = fresh
	}

	if len(issues) == 0 {
		if skippedForCooldown > 0 {
			fmt.Printf("No fresh issues right now — %d matched but were already shown within the last %d day(s). Try -show-repeats or -cooldown 0.\n", skippedForCooldown, *cooldown)
		} else {
			fmt.Println("No matching issues found right now — try a wider -since window, a different -level, or add -include-assigned.")
		}
		return
	}

	var finalIssues []gh.Issue
	var finalScored []gh.ScoredIssue

	if !*score {
		if len(issues) > *limit {
			issues = issues[:*limit]
		}
		finalIssues = issues
	} else {
		// Scoring path: bound how many issues we fetch repo metadata for.
		pool := *limit
		if sortByDifficulty && *scorePool > pool {
			pool = *scorePool
		}
		if len(issues) > pool {
			issues = issues[:pool]
		}

		scored, err := client.Score(issues)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error scoring issues: %v\n", err)
			os.Exit(1)
		}
		if sortByDifficulty {
			gh.SortByDifficulty(scored)
		}
		if len(scored) > *limit {
			scored = scored[:*limit]
		}
		finalScored = scored
	}

	// Record what we're about to show, then persist.
	if st != nil {
		if finalIssues != nil {
			for _, iss := range finalIssues {
				st.MarkShown(iss.HTMLURL, iss.Repo(), iss.Title, now)
			}
		}
		for _, iss := range finalScored {
			st.MarkShown(iss.HTMLURL, iss.Repo(), iss.Title, now)
		}
		st.RecordRun(now)
		if err := st.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: couldn't save local state: %v\n", err)
		}
	}

	if *asJSON {
		if finalIssues != nil {
			writeJSON(finalIssues)
		} else {
			writeJSON(finalScored)
		}
		return
	}

	if finalIssues != nil {
		printTable(finalIssues)
	} else {
		printScoredTable(finalScored)
	}

	if skippedForCooldown > 0 {
		fmt.Fprintf(os.Stderr, "(%d previously-seen match(es) hidden by the %d-day cooldown — use -show-repeats to include them)\n", skippedForCooldown, *cooldown)
	}
}
