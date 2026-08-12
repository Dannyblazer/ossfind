package gh

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func mergedAtPtr(t time.Time) *string {
	s := t.Format(time.RFC3339)
	return &s
}

func TestCheckRepoHealth(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name        string
		pushedAt    time.Time
		mergedCount int // out of 10 PRs
		wantHealthy bool
		wantStatus  string
	}{
		{
			name:        "active and responsive",
			pushedAt:    now.AddDate(0, 0, -5),
			mergedCount: 8,
			wantHealthy: true,
			wantStatus:  "Healthy",
		},
		{
			name:        "stale but would-be responsive",
			pushedAt:    now.AddDate(0, 0, -400),
			mergedCount: 8,
			wantHealthy: false,
			wantStatus:  "Stale",
		},
		{
			name:        "active but unresponsive",
			pushedAt:    now.AddDate(0, 0, -5),
			mergedCount: 1,
			wantHealthy: false,
			wantStatus:  "Unresponsive",
		},
		{
			name:        "stale and unresponsive",
			pushedAt:    now.AddDate(0, 0, -400),
			mergedCount: 0,
			wantHealthy: false,
			wantStatus:  "Stale+Unresponsive",
		},
	}

	opts := HealthOptions{MaxStaleDays: 180, MinMergeRate: 0.2, PRSample: 10}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/repos/octo/repo", func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(RepoInfo{FullName: "octo/repo", PushedAt: tc.pushedAt})
			})
			mux.HandleFunc("/repos/octo/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
				prs := make([]pullSummary, 10)
				for i := 0; i < 10; i++ {
					if i < tc.mergedCount {
						prs[i] = pullSummary{MergedAt: mergedAtPtr(now)}
					} else {
						prs[i] = pullSummary{MergedAt: nil}
					}
				}
				json.NewEncoder(w).Encode(prs)
			})

			srv := httptest.NewServer(mux)
			defer srv.Close()
			base, _ := url.Parse(srv.URL)

			c := NewClient("")
			c.HTTPClient = &http.Client{Transport: rewriteTransport{base: base}, Timeout: 5 * time.Second}

			h, err := c.CheckRepoHealth("octo/repo", opts)
			if err != nil {
				t.Fatalf("CheckRepoHealth returned error: %v", err)
			}
			if h.Healthy != tc.wantHealthy {
				t.Errorf("Healthy = %v, want %v (reason: %s)", h.Healthy, tc.wantHealthy, h.Reason)
			}
			if h.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", h.Status, tc.wantStatus)
			}
		})
	}
}

func TestCheckRepoHealthThinSampleNotPenalized(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/octo/tiny", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(RepoInfo{FullName: "octo/tiny", PushedAt: now.AddDate(0, 0, -5)})
	})
	mux.HandleFunc("/repos/octo/tiny/pulls", func(w http.ResponseWriter, r *http.Request) {
		// Only 2 closed PRs ever, both unmerged -- below minMergeSampleSize,
		// so the merge-rate signal should NOT be trusted/applied.
		json.NewEncoder(w).Encode([]pullSummary{{MergedAt: nil}, {MergedAt: nil}})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	c := NewClient("")
	c.HTTPClient = &http.Client{Transport: rewriteTransport{base: base}, Timeout: 5 * time.Second}

	h, err := c.CheckRepoHealth("octo/tiny", HealthOptions{MaxStaleDays: 180, MinMergeRate: 0.2, PRSample: 10})
	if err != nil {
		t.Fatalf("CheckRepoHealth returned error: %v", err)
	}
	if !h.Healthy {
		t.Errorf("expected a thin PR sample to NOT be penalized, got Healthy=false (reason: %s)", h.Reason)
	}
	if h.MergeRate != -1 {
		t.Errorf("expected MergeRate=-1 (untrusted) for a thin sample, got %v", h.MergeRate)
	}
}

func TestFilterHealthyDedupesAndSplits(t *testing.T) {
	now := time.Now()
	calls := map[string]int{}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/octo/good", func(w http.ResponseWriter, r *http.Request) {
		calls["repo:good"]++
		json.NewEncoder(w).Encode(RepoInfo{FullName: "octo/good", PushedAt: now.AddDate(0, 0, -1)})
	})
	mux.HandleFunc("/repos/octo/good/pulls", func(w http.ResponseWriter, r *http.Request) {
		calls["pulls:good"]++
		prs := make([]pullSummary, 5)
		for i := range prs {
			prs[i] = pullSummary{MergedAt: mergedAtPtr(now)}
		}
		json.NewEncoder(w).Encode(prs)
	})
	mux.HandleFunc("/repos/octo/dead", func(w http.ResponseWriter, r *http.Request) {
		calls["repo:dead"]++
		json.NewEncoder(w).Encode(RepoInfo{FullName: "octo/dead", PushedAt: now.AddDate(0, 0, -900)})
	})
	mux.HandleFunc("/repos/octo/dead/pulls", func(w http.ResponseWriter, r *http.Request) {
		calls["pulls:dead"]++
		json.NewEncoder(w).Encode([]pullSummary{})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	c := NewClient("")
	c.HTTPClient = &http.Client{Transport: rewriteTransport{base: base}, Timeout: 5 * time.Second}

	issues := []Issue{
		{Title: "a", HTMLURL: "https://github.com/octo/good/issues/1", RepoURL: "https://api.github.com/repos/octo/good"},
		{Title: "b", HTMLURL: "https://github.com/octo/good/issues/2", RepoURL: "https://api.github.com/repos/octo/good"}, // same repo, should dedupe
		{Title: "c", HTMLURL: "https://github.com/octo/dead/issues/3", RepoURL: "https://api.github.com/repos/octo/dead"},
	}

	kept, dropped, healthByRepo, err := c.FilterHealthy(issues, HealthOptions{MaxStaleDays: 180, MinMergeRate: 0.2, PRSample: 10})
	if err != nil {
		t.Fatalf("FilterHealthy returned error: %v", err)
	}

	if calls["repo:good"] != 1 || calls["pulls:good"] != 1 {
		t.Errorf("expected exactly 1 health check for octo/good (dedup), got repo=%d pulls=%d", calls["repo:good"], calls["pulls:good"])
	}
	if len(kept) != 2 {
		t.Errorf("expected 2 kept issues (both from octo/good), got %d", len(kept))
	}
	if len(dropped) != 1 {
		t.Errorf("expected 1 dropped issue (from octo/dead), got %d", len(dropped))
	}
	if healthByRepo["octo/dead"].Healthy {
		t.Errorf("expected octo/dead to be unhealthy")
	}
	if !healthByRepo["octo/good"].Healthy {
		t.Errorf("expected octo/good to be healthy")
	}
}
