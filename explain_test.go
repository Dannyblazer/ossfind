package main

import (
	"strings"
	"testing"

	"ossfind/internal/gh"
)

func TestMatchedLabelsCaseInsensitive(t *testing.T) {
	got := matchedLabels([]string{"Good First Issue", "wontfix"}, []string{"good first issue"})
	if len(got) != 1 || got[0] != "Good First Issue" {
		t.Errorf("expected case-insensitive match preserving issue's own casing, got %v", got)
	}
}

func TestMatchedLabelsNoSearchedLabels(t *testing.T) {
	got := matchedLabels([]string{"good first issue"}, nil)
	if got != nil {
		t.Errorf("expected nil when no labels were searched (advanced level), got %v", got)
	}
}

func TestMatchReasonFallsBackWhenNoLabelMatch(t *testing.T) {
	iss := gh.Issue{}
	reason := matchReason(iss, nil)
	if !strings.Contains(reason, "language + recency") {
		t.Errorf("expected fallback reason for no-label case, got %q", reason)
	}
}

func TestWhyIncludesHealthWhenPresent(t *testing.T) {
	iss := gh.Issue{RepoURL: "https://api.github.com/repos/octo/repo"}
	healthByRepo := map[string]gh.RepoHealth{
		"octo/repo": {Reason: "last push 3d ago; 8/10 recent PRs merged (80%)"},
	}
	got := why(iss, nil, healthByRepo)
	if !strings.Contains(got, "repo: last push 3d ago") {
		t.Errorf("expected health reason folded into why(), got %q", got)
	}
}

func TestWhyOmitsHealthWhenAbsent(t *testing.T) {
	iss := gh.Issue{RepoURL: "https://api.github.com/repos/octo/repo"}
	got := why(iss, nil, nil)
	if strings.Contains(got, "repo:") {
		t.Errorf("expected no health segment when -health wasn't used, got %q", got)
	}
}
