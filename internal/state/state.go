// Package state gives ossfind a small local memory: which issues you've
// already been shown (so repeat runs don't keep surfacing the same ones
// within a cooldown window) and which days you've run the tool (so it can
// report a simple contribution-habit streak). It's a single JSON file,
// no database, no server -- the CLI equivalent of CodeTriage's "don't
// overwhelm you" backoff, without needing an account or an inbox.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const dateLayout = "2006-01-02"

type SeenIssue struct {
	URL        string    `json:"url"`
	Repo       string    `json:"repo"`
	Title      string    `json:"title"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	TimesShown int       `json:"times_shown"`
}

type State struct {
	Issues map[string]*SeenIssue `json:"issues"` // key: issue URL
	Runs   []string              `json:"runs"`   // dates (YYYY-MM-DD) the tool was run

	path string
}

// DefaultPath returns ~/.config/ossfind/state.json (respecting XDG via
// os.UserConfigDir), creating the directory if needed.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving config dir: %w", err)
	}
	dir = filepath.Join(dir, "ossfind")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating config dir: %w", err)
	}
	return filepath.Join(dir, "state.json"), nil
}

// Load reads state from path, returning a fresh empty State if the file
// doesn't exist yet. A corrupt file is reported as an error rather than
// silently discarded, so you don't lose history without knowing why.
func Load(path string) (*State, error) {
	s := &State{Issues: map[string]*SeenIssue{}, path: path}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("parsing state file %s (corrupt? consider `ossfind reset-state -yes`): %w", path, err)
	}
	s.path = path
	if s.Issues == nil {
		s.Issues = map[string]*SeenIssue{}
	}
	return s, nil
}

// Save writes state atomically (temp file + rename) so a crash mid-write
// can't corrupt the existing file.
func (s *State) Save() error {
	if s.path == "" {
		return fmt.Errorf("state has no path set")
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding state: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing temp state file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("saving state file: %w", err)
	}
	return nil
}

// IsCool reports whether an issue was NOT shown within the last cooldownDays
// (i.e. it's safe/fresh to show again). cooldownDays <= 0 disables the check.
func (s *State) IsCool(url string, cooldownDays int, now time.Time) bool {
	if cooldownDays <= 0 {
		return true
	}
	seen, ok := s.Issues[url]
	if !ok {
		return true
	}
	return now.Sub(seen.LastSeen) >= time.Duration(cooldownDays)*24*time.Hour
}

// MarkShown records that an issue was surfaced to the user just now.
func (s *State) MarkShown(url, repo, title string, now time.Time) {
	if existing, ok := s.Issues[url]; ok {
		existing.LastSeen = now
		existing.TimesShown++
		return
	}
	s.Issues[url] = &SeenIssue{
		URL:        url,
		Repo:       repo,
		Title:      title,
		FirstSeen:  now,
		LastSeen:   now,
		TimesShown: 1,
	}
}

// RecordRun notes that the tool was run "today" (idempotent per day).
func (s *State) RecordRun(now time.Time) {
	today := now.Format(dateLayout)
	for _, d := range s.Runs {
		if d == today {
			return
		}
	}
	s.Runs = append(s.Runs, today)
	sort.Strings(s.Runs)
}

// Streak returns the number of consecutive days (ending today or yesterday
// -- so it doesn't reset to zero the moment you wake up) that the tool has
// been run at least once.
func (s *State) Streak(now time.Time) int {
	runDates := map[string]bool{}
	for _, d := range s.Runs {
		runDates[d] = true
	}
	cursor := now
	if !runDates[cursor.Format(dateLayout)] {
		cursor = cursor.AddDate(0, 0, -1) // allow "yesterday" as the streak anchor
		if !runDates[cursor.Format(dateLayout)] {
			return 0
		}
	}
	streak := 0
	for runDates[cursor.Format(dateLayout)] {
		streak++
		cursor = cursor.AddDate(0, 0, -1)
	}
	return streak
}

type Summary struct {
	TotalUniqueIssues int
	TotalTimesShown   int
	UniqueRepos       int
	FirstRun          string
	LastRun           string
	TotalRuns         int
	CurrentStreak     int
	TopRepos          []RepoCount
}

type RepoCount struct {
	Repo  string
	Count int
}

// Summarize computes stats for the `ossfind stats` subcommand.
func (s *State) Summarize(now time.Time) Summary {
	sum := Summary{}
	repoCounts := map[string]int{}

	for _, iss := range s.Issues {
		sum.TotalUniqueIssues++
		sum.TotalTimesShown += iss.TimesShown
		repoCounts[iss.Repo]++
	}
	sum.UniqueRepos = len(repoCounts)

	if len(s.Runs) > 0 {
		sum.FirstRun = s.Runs[0]
		sum.LastRun = s.Runs[len(s.Runs)-1]
	}
	sum.TotalRuns = len(s.Runs)
	sum.CurrentStreak = s.Streak(now)

	for repo, count := range repoCounts {
		sum.TopRepos = append(sum.TopRepos, RepoCount{Repo: repo, Count: count})
	}
	sort.Slice(sum.TopRepos, func(i, j int) bool {
		if sum.TopRepos[i].Count != sum.TopRepos[j].Count {
			return sum.TopRepos[i].Count > sum.TopRepos[j].Count
		}
		return sum.TopRepos[i].Repo < sum.TopRepos[j].Repo
	})
	if len(sum.TopRepos) > 5 {
		sum.TopRepos = sum.TopRepos[:5]
	}

	return sum
}
