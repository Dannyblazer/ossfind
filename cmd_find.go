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
		languages    = fs.String("languages", "Go", "comma-separated GitHub language names (e.g. Go,Python)")
		level        = fs.String("level", "beginner", "beginner | intermediate | advanced (sets a default label set)")
		labelsFlag   = fs.String("labels", "", "comma-separated labels; overrides -level's default labels")
		sinceDays    = fs.Int("since", 7, "only include issues created in the last N days (0 = no limit)")
		limit        = fs.Int("limit", 20, "max issues to print after merging/deduping")
		perQuery     = fs.Int("per-query", 20, "max results fetched per individual search query")
		sortBy       = fs.String("sort", "updated", "updated | created | difficulty")
		score        = fs.Bool("score", false, "attach a rough difficulty estimate (comments + repo stars) to each issue")
		scorePool    = fs.Int("score-pool", 40, "when -sort difficulty, how many of the most recent candidates to score before ranking (bounds API calls)")
		token        = fs.String("token", os.Getenv("GITHUB_TOKEN"), "GitHub token (defaults to $GITHUB_TOKEN; raises your rate limit)")
		includeAll   = fs.Bool("include-assigned", false, "include issues that already have an assignee")
		asJSON       = fs.Bool("json", false, "print raw JSON instead of a table")
		verbose      = fs.Bool("v", false, "log each query and rate-limit status to stderr")
		cooldown     = fs.Int("cooldown", 7, "days to hide an issue after it's been shown once (0 = never hide repeats)")
		showRepeats  = fs.Bool("show-repeats", false, "ignore the cooldown for this run (shows previously-seen issues too)")
		noState      = fs.Bool("no-state", false, "don't read or write local history at all (stateless run)")
		statePath    = fs.String("state-path", "", "override the local state file location (default: ~/.config/ossfind/state.json)")
		auto         = fs.Bool("auto", false, "auto-detect -languages and -level from a GitHub user's own repo languages + merged PR count")
		githubUser   = fs.String("github-user", "", "GitHub username to auto-detect from (default: the token's own user, if a token is set)")
		health       = fs.Bool("health", false, "filter out issues from stale or unresponsive repos (~2 extra API calls per unique repo)")
		maxStaleDays = fs.Int("max-stale-days", 180, "with -health, filter out repos with no push in this many days")
		minMergeRate = fs.Float64("min-merge-rate", 0.2, "with -health, filter out repos whose recent PR merge rate is below this (0.0-1.0)")
		prSample     = fs.Int("pr-sample", 10, "with -health, how many recently-closed PRs to sample for merge rate")
		healthPool   = fs.Int("health-pool", 30, "with -health, how many of the most recent candidates to health-check (bounds API calls)")
	)
	fs.Parse(args)

	// Track which flags were set explicitly, so -auto only fills in the
	// ones the user didn't already specify by hand.
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	client := gh.NewClient(*token)
	if *verbose {
		client.LogFunc = func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		}
	}
	if *token == "" && *verbose {
		fmt.Fprintln(os.Stderr, "note: no token set — limited to 10 search req/min and 60 repo lookups/hour. Set GITHUB_TOKEN to raise both.")
	}

	effectiveLanguages := *languages
	effectiveLevel := *level

	if *auto {
		user := *githubUser
		if user == "" {
			resolved, err := client.CurrentUsername()
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: -auto couldn't resolve a username (%v) — pass -github-user or set GITHUB_TOKEN. Falling back to -languages/-level as given.\n", err)
			} else {
				user = resolved
			}
		}
		if user != "" {
			profile, err := client.DetectProfile(user, 3)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: -auto profile detection failed (%v) — falling back to -languages/-level as given.\n", err)
			} else {
				if !explicit["languages"] {
					effectiveLanguages = strings.Join(profile.TopLanguages, ",")
				}
				if !explicit["level"] && !explicit["labels"] {
					effectiveLevel = profile.Level
				}
				fmt.Fprintf(os.Stderr, "auto-detected profile for %s: top languages [%s], %d merged PR(s) to other repos → level=%s\n",
					profile.Username, strings.Join(profile.TopLanguages, ", "), profile.ExternalMergedPRCount, profile.Level)
			}
		}
	}

	langs := splitAndTrim(effectiveLanguages)
	if len(langs) == 0 {
		fmt.Fprintln(os.Stderr, "error: -languages must not be empty")
		os.Exit(1)
	}

	var labels []string
	if *labelsFlag != "" {
		labels = splitAndTrim(*labelsFlag)
	} else {
		defaults, ok := gh.LevelLabels[strings.ToLower(effectiveLevel)]
		if !ok {
			fmt.Fprintf(os.Stderr, "error: unknown -level %q (want beginner, intermediate, or advanced)\n", effectiveLevel)
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

	// Filter out issues from stale/unresponsive repos, before truncating to
	// -limit. Bounded to a pool so a large candidate set doesn't turn into
	// hundreds of extra API calls.
	skippedForHealth := 0
	if *health && len(issues) > 0 {
		pool := *limit
		if sortByDifficulty && *scorePool > pool {
			pool = *scorePool
		}
		if *healthPool > pool {
			pool = *healthPool
		}
		candidates := issues
		if len(candidates) > pool {
			candidates = candidates[:pool]
		}

		kept, dropped, healthByRepo, err := client.FilterHealthy(candidates, gh.HealthOptions{
			MaxStaleDays: *maxStaleDays,
			MinMergeRate: *minMergeRate,
			PRSample:     *prSample,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: -health check failed (%v) — continuing without it\n", err)
		} else {
			skippedForHealth = len(dropped)
			issues = kept
			if *verbose {
				for repo, h := range healthByRepo {
					if !h.Healthy {
						fmt.Fprintf(os.Stderr, "  filtered %s: %s\n", repo, h.Reason)
					}
				}
			}
		}
	}

	if len(issues) == 0 {
		switch {
		case skippedForHealth > 0 && skippedForCooldown > 0:
			fmt.Printf("No matching issues right now — %d hidden by the %d-day cooldown, %d filtered for stale/unresponsive repos.\n", skippedForCooldown, *cooldown, skippedForHealth)
		case skippedForHealth > 0:
			fmt.Printf("No healthy-repo issues right now — %d matched but were filtered (inactive repo or low PR merge rate). Try -max-stale-days, -min-merge-rate, or turn off -health.\n", skippedForHealth)
		case skippedForCooldown > 0:
			fmt.Printf("No fresh issues right now — %d matched but were already shown within the last %d day(s). Try -show-repeats or -cooldown 0.\n", skippedForCooldown, *cooldown)
		default:
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
	if skippedForHealth > 0 {
		fmt.Fprintf(os.Stderr, "(%d match(es) filtered for stale/unresponsive repos — use -v to see why, or adjust -max-stale-days/-min-merge-rate)\n", skippedForHealth)
	}
}
