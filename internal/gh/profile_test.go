package gh

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestDetectProfile(t *testing.T) {
	mux := http.NewServeMux()

	// Page 1: 100 repos worth of language signal (only a handful matter).
	mux.HandleFunc("/users/octocat/repos", func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "1" {
			repos := []repoSummary{
				{Language: "Go", Fork: false},
				{Language: "Go", Fork: false},
				{Language: "Go", Fork: false},
				{Language: "Python", Fork: false},
				{Language: "TypeScript", Fork: false},
				{Language: "TypeScript", Fork: false},
				{Language: "JavaScript", Fork: true}, // fork -- must be excluded
				{Language: "", Fork: false},          // no language -- must be excluded
			}
			json.NewEncoder(w).Encode(repos)
			return
		}
		// any later page: empty, signals end of pagination
		json.NewEncoder(w).Encode([]repoSummary{})
	})

	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q != "is:pr is:merged author:octocat -user:octocat" {
			t.Errorf("unexpected search query: %q", q)
		}
		json.NewEncoder(w).Encode(searchResponse{TotalCount: 14})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	c := NewClient("")
	c.HTTPClient = &http.Client{Transport: rewriteTransport{base: base}, Timeout: 5 * time.Second}

	profile, err := c.DetectProfile("octocat", 3)
	if err != nil {
		t.Fatalf("DetectProfile returned error: %v", err)
	}

	if len(profile.TopLanguages) != 3 {
		t.Fatalf("expected 3 top languages, got %v", profile.TopLanguages)
	}
	if profile.TopLanguages[0] != "Go" {
		t.Errorf("expected Go to rank first (3 repos), got %v", profile.TopLanguages)
	}
	// Python and TypeScript tie at... no, TypeScript has 2, Python has 1 -- TypeScript should rank 2nd.
	if profile.TopLanguages[1] != "TypeScript" {
		t.Errorf("expected TypeScript 2nd (2 repos vs Python's 1), got %v", profile.TopLanguages)
	}
	if profile.TopLanguages[2] != "Python" {
		t.Errorf("expected Python 3rd, got %v", profile.TopLanguages)
	}

	if profile.ExternalMergedPRCount != 14 {
		t.Errorf("expected ExternalMergedPRCount=14, got %d", profile.ExternalMergedPRCount)
	}
	if profile.Level != "intermediate" {
		t.Errorf("expected level=intermediate for 14 merged PRs, got %s", profile.Level)
	}
}

func TestLevelFromMergedPRs(t *testing.T) {
	cases := []struct {
		count int
		want  string
	}{
		{0, "beginner"},
		{5, "beginner"},
		{6, "intermediate"},
		{29, "intermediate"},
		{30, "advanced"},
		{500, "advanced"},
	}
	for _, tc := range cases {
		if got := levelFromMergedPRs(tc.count); got != tc.want {
			t.Errorf("levelFromMergedPRs(%d) = %s, want %s", tc.count, got, tc.want)
		}
	}
}

func TestCurrentUsernameNoToken(t *testing.T) {
	c := NewClient("")
	if _, err := c.CurrentUsername(); err == nil {
		t.Error("expected an error when no token is set, got nil")
	}
}
