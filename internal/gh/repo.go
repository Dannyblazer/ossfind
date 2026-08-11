package gh

import (
	"encoding/json"
	"fmt"
)

// RepoInfo is the subset of GitHub's repo response used for difficulty scoring.
type RepoInfo struct {
	FullName        string `json:"full_name"`
	StargazersCount int    `json:"stargazers_count"`
	OpenIssuesCount int    `json:"open_issues_count"`
}

// GetRepo fetches basic metadata for a single "owner/name" repo.
func (c *Client) GetRepo(fullName string) (RepoInfo, error) {
	body, _, err := c.doGet(reposURL + fullName)
	if err != nil {
		return RepoInfo{}, fmt.Errorf("fetching repo %s: %w", fullName, err)
	}
	var info RepoInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return RepoInfo{}, fmt.Errorf("decoding repo %s: %w", fullName, err)
	}
	return info, nil
}
