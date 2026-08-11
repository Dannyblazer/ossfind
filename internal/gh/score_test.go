package gh

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

type rewriteTransport struct {
	base *url.URL
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = t.base.Scheme
	req.URL.Host = t.base.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestScore(t *testing.T) {
	repoCalls := map[string]int{}

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/", func(w http.ResponseWriter, r *http.Request) {
		repoCalls[r.URL.Path]++
		var stars int
		switch r.URL.Path {
		case "/repos/octo/small-repo":
			stars = 50
		case "/repos/octo/big-repo":
			stars = 10000
		}
		json.NewEncoder(w).Encode(RepoInfo{
			FullName:        r.URL.Path[len("/repos/"):],
			StargazersCount: stars,
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()
	base, _ := url.Parse(srv.URL)

	c := NewClient("")
	c.HTTPClient = &http.Client{Transport: rewriteTransport{base: base}, Timeout: 5 * time.Second}

	issues := []Issue{
		{
			Title:    "quiet issue in small repo",
			HTMLURL:  "https://github.com/octo/small-repo/issues/1",
			Comments: 1,
			RepoURL:  "https://api.github.com/repos/octo/small-repo",
		},
		{
			Title:    "busy issue in big repo",
			HTMLURL:  "https://github.com/octo/big-repo/issues/2",
			Comments: 12,
			RepoURL:  "https://api.github.com/repos/octo/big-repo",
		},
		{
			Title:    "second issue, same small repo",
			HTMLURL:  "https://github.com/octo/small-repo/issues/3",
			Comments: 0,
			RepoURL:  "https://api.github.com/repos/octo/small-repo",
		},
	}

	scored, err := c.Score(issues)
	if err != nil {
		t.Fatalf("Score returned error: %v", err)
	}
	if len(scored) != 3 {
		t.Fatalf("expected 3 scored issues, got %d", len(scored))
	}

	// Dedup check: small-repo appears twice among issues but should only be fetched once.
	if got := repoCalls["/repos/octo/small-repo"]; got != 1 {
		t.Errorf("expected 1 call for small-repo (dedup), got %d", got)
	}
	if got := repoCalls["/repos/octo/big-repo"]; got != 1 {
		t.Errorf("expected 1 call for big-repo, got %d", got)
	}

	byTitle := map[string]ScoredIssue{}
	for _, s := range scored {
		byTitle[s.Title] = s
	}

	quiet := byTitle["quiet issue in small repo"]
	if quiet.Difficulty != "Easy" {
		t.Errorf("expected quiet/small-repo issue to be Easy, got %s (score=%d)", quiet.Difficulty, quiet.Score)
	}
	if quiet.RepoStars != 50 {
		t.Errorf("expected 50 stars, got %d", quiet.RepoStars)
	}

	busy := byTitle["busy issue in big repo"]
	if busy.Difficulty != "Hard" {
		t.Errorf("expected busy/big-repo issue to be Hard, got %s (score=%d)", busy.Difficulty, busy.Score)
	}

	// SortByDifficulty should put Easy before Hard.
	SortByDifficulty(scored)
	if scored[0].Difficulty == "Hard" {
		t.Errorf("expected Easy/Medium issues to sort before Hard, got %s first", scored[0].Difficulty)
	}
	if scored[len(scored)-1].Title != "busy issue in big repo" {
		t.Errorf("expected hardest issue last after sort, got %q", scored[len(scored)-1].Title)
	}
}
