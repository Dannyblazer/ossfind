package state

import (
	"path/filepath"
	"testing"
	"time"
)

func mustLoad(t *testing.T, path string) *State {
	t.Helper()
	s, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	return s
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := mustLoad(t, path)

	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	s.MarkShown("https://github.com/o/r/issues/1", "o/r", "fix thing", now)
	s.RecordRun(now)
	s.path = path
	if err := s.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	reloaded := mustLoad(t, path)
	if len(reloaded.Issues) != 1 {
		t.Fatalf("expected 1 issue after reload, got %d", len(reloaded.Issues))
	}
	if reloaded.Issues["https://github.com/o/r/issues/1"].TimesShown != 1 {
		t.Errorf("expected TimesShown=1 after reload")
	}
	if len(reloaded.Runs) != 1 || reloaded.Runs[0] != "2026-08-06" {
		t.Errorf("expected run log to contain 2026-08-06, got %v", reloaded.Runs)
	}
}

func TestIsCoolCooldown(t *testing.T) {
	s := mustLoad(t, filepath.Join(t.TempDir(), "state.json"))
	url := "https://github.com/o/r/issues/1"
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)

	if !s.IsCool(url, 7, now) {
		t.Error("never-seen issue should always be cool")
	}

	s.MarkShown(url, "o/r", "t", now)

	if s.IsCool(url, 7, now.Add(24*time.Hour)) {
		t.Error("issue seen yesterday should NOT be cool under a 7-day cooldown")
	}
	if !s.IsCool(url, 7, now.Add(8*24*time.Hour)) {
		t.Error("issue seen 8 days ago SHOULD be cool under a 7-day cooldown")
	}
	if !s.IsCool(url, 0, now.Add(time.Hour)) {
		t.Error("cooldown=0 should disable the check entirely")
	}
}

func TestMarkShownIncrementsCount(t *testing.T) {
	s := mustLoad(t, filepath.Join(t.TempDir(), "state.json"))
	url := "https://github.com/o/r/issues/1"
	t1 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)

	s.MarkShown(url, "o/r", "t", t1)
	s.MarkShown(url, "o/r", "t", t2)

	iss := s.Issues[url]
	if iss.TimesShown != 2 {
		t.Errorf("expected TimesShown=2, got %d", iss.TimesShown)
	}
	if !iss.FirstSeen.Equal(t1) {
		t.Errorf("FirstSeen should stay at first mark: got %v", iss.FirstSeen)
	}
	if !iss.LastSeen.Equal(t2) {
		t.Errorf("LastSeen should update to latest mark: got %v", iss.LastSeen)
	}
}

func TestStreak(t *testing.T) {
	s := mustLoad(t, filepath.Join(t.TempDir(), "state.json"))
	day := func(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 9, 0, 0, 0, time.UTC) }

	// No runs yet.
	if got := s.Streak(day(2026, 8, 6)); got != 0 {
		t.Errorf("expected streak 0 with no runs, got %d", got)
	}

	s.RecordRun(day(2026, 8, 4))
	s.RecordRun(day(2026, 8, 5))
	s.RecordRun(day(2026, 8, 6))

	if got := s.Streak(day(2026, 8, 6)); got != 3 {
		t.Errorf("expected streak 3 (Aug 4-6), got %d", got)
	}
	// "today" (Aug 7) hasn't run yet, but yesterday (Aug 6) did -- streak
	// should still count, so a morning check doesn't show 0 before you've
	// run the tool that day.
	if got := s.Streak(day(2026, 8, 7)); got != 3 {
		t.Errorf("expected streak still 3 when checked the next morning before running, got %d", got)
	}

	// A gap breaks the streak.
	s.RecordRun(day(2026, 8, 9)) // skipped Aug 7-8
	if got := s.Streak(day(2026, 8, 9)); got != 1 {
		t.Errorf("expected streak reset to 1 after a gap, got %d", got)
	}
}

func TestRecordRunIdempotentPerDay(t *testing.T) {
	s := mustLoad(t, filepath.Join(t.TempDir(), "state.json"))
	now := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)
	s.RecordRun(now)
	s.RecordRun(now.Add(3 * time.Hour)) // same day, later time
	if len(s.Runs) != 1 {
		t.Errorf("expected 1 run entry for repeated same-day calls, got %d: %v", len(s.Runs), s.Runs)
	}
}

func TestSummarize(t *testing.T) {
	s := mustLoad(t, filepath.Join(t.TempDir(), "state.json"))
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)

	s.MarkShown("u1", "octo/repo-a", "t1", now)
	s.MarkShown("u2", "octo/repo-a", "t2", now)
	s.MarkShown("u3", "octo/repo-b", "t3", now)
	s.RecordRun(now)

	sum := s.Summarize(now)
	if sum.TotalUniqueIssues != 3 {
		t.Errorf("expected 3 unique issues, got %d", sum.TotalUniqueIssues)
	}
	if sum.UniqueRepos != 2 {
		t.Errorf("expected 2 unique repos, got %d", sum.UniqueRepos)
	}
	if sum.CurrentStreak != 1 {
		t.Errorf("expected streak 1, got %d", sum.CurrentStreak)
	}
	if len(sum.TopRepos) == 0 || sum.TopRepos[0].Repo != "octo/repo-a" || sum.TopRepos[0].Count != 2 {
		t.Errorf("expected octo/repo-a to top the list with count 2, got %+v", sum.TopRepos)
	}
}
