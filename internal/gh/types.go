package gh

import "time"

// Issue is the subset of GitHub's search/issues response we care about.
type Issue struct {
	Title     string     `json:"title"`
	HTMLURL   string     `json:"html_url"`
	Number    int        `json:"number"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Comments  int        `json:"comments"`
	RepoURL   string     `json:"repository_url"` // e.g. https://api.github.com/repos/owner/name
	Labels    []Label    `json:"labels"`
	Assignees []Assignee `json:"assignees"`
}

type Label struct {
	Name string `json:"name"`
}

type Assignee struct {
	Login string `json:"login"`
}

type searchResponse struct {
	TotalCount int     `json:"total_count"`
	Items      []Issue `json:"items"`
}

// Repo extracts "owner/name" from the repository_url field.
func (i Issue) Repo() string {
	// repository_url looks like: https://api.github.com/repos/owner/name
	const marker = "/repos/"
	idx := -1
	for j := 0; j+len(marker) <= len(i.RepoURL); j++ {
		if i.RepoURL[j:j+len(marker)] == marker {
			idx = j + len(marker)
			break
		}
	}
	if idx == -1 {
		return i.RepoURL
	}
	return i.RepoURL[idx:]
}

func (i Issue) LabelNames() []string {
	names := make([]string, 0, len(i.Labels))
	for _, l := range i.Labels {
		names = append(names, l.Name)
	}
	return names
}
