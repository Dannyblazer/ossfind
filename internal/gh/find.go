package gh

import (
	"fmt"
	"sort"
	"time"
)

// Level maps a rough experience level to the label(s) GitHub repos
// conventionally use for issues suited to that level. These are just
// sensible defaults -- override with explicit labels on the CLI.
var LevelLabels = map[string][]string{
	"beginner":     {"good first issue"},
	"intermediate": {"help wanted"},
	"advanced":     {}, // no label constraint; relies on language + recency only
}

type FinderOptions struct {
	Languages   []string
	Labels      []string // if empty, no label qualifier is added
	SinceDays   int      // only issues created in the last N days
	Unassigned  bool
	SortBy      string // "updated" or "created"
	PerLanguage int    // max results fetched per (language, label) query
	Delay       time.Duration
}

// Find runs one search per (language, label) combination (GitHub's search
// syntax can't OR across language: qualifiers), merges results, dedupes by
// issue URL, and returns them sorted by UpdatedAt descending.
func (c *Client) Find(opts FinderOptions) ([]Issue, error) {
	if len(opts.Languages) == 0 {
		return nil, fmt.Errorf("at least one language is required")
	}
	if opts.SortBy == "" {
		opts.SortBy = "updated"
	}
	if opts.PerLanguage <= 0 {
		opts.PerLanguage = 20
	}
	if opts.Delay <= 0 {
		opts.Delay = 1500 * time.Millisecond
	}

	labels := opts.Labels
	if len(labels) == 0 {
		labels = []string{""} // one pass, no label qualifier
	}

	seen := map[string]Issue{}
	first := true

	for _, lang := range opts.Languages {
		for _, label := range labels {
			if !first {
				time.Sleep(opts.Delay)
			}
			first = false

			q := buildQuery(lang, label, opts.SinceDays, opts.Unassigned)
			issues, err := c.Search(q, opts.SortBy, opts.PerLanguage)
			if err != nil {
				return nil, fmt.Errorf("searching (lang=%s label=%q): %w", lang, label, err)
			}
			for _, iss := range issues {
				seen[iss.HTMLURL] = iss
			}
		}
	}

	result := make([]Issue, 0, len(seen))
	for _, iss := range seen {
		result = append(result, iss)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})

	return result, nil
}

func buildQuery(language, label string, sinceDays int, unassigned bool) string {
	q := fmt.Sprintf(`is:issue is:open language:%s`, language)
	if unassigned {
		q += " no:assignee"
	}
	if sinceDays > 0 {
		since := time.Now().AddDate(0, 0, -sinceDays).Format("2006-01-02")
		q += fmt.Sprintf(" created:>=%s", since)
	}
	if label != "" {
		q += fmt.Sprintf(" label:%q", label)
	}
	return q
}
