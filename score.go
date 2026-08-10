package gh

import (
	"sort"
	"time"
)

// ScoredIssue pairs an Issue with a rough, heuristic difficulty estimate
// based on discussion volume (comments) and repo popularity (stars) --
// neither is a real measure of complexity, just a cheap proxy: quiet
// issues in smaller repos tend to be easier to land a first PR on than
// heavily-discussed issues in large, high-traffic repos.
type ScoredIssue struct {
	Issue
	RepoStars  int    `json:"repo_stars"`
	Difficulty string `json:"difficulty"` // "Easy", "Medium", or "Hard"
	Score      int    `json:"score"`      // 0-4, lower = easier
}

// Score fetches repo metadata for each unique repo among the given issues
// and attaches a difficulty estimate to each. It dedupes repo lookups, so
// the number of API calls is bounded by the number of *distinct* repos,
// not the number of issues.
func (c *Client) Score(issues []Issue) ([]ScoredIssue, error) {
	starsByRepo := map[string]int{}

	for _, iss := range issues {
		repo := iss.Repo()
		if _, ok := starsByRepo[repo]; ok {
			continue
		}
		info, err := c.GetRepo(repo)
		if err != nil {
			return nil, err
		}
		starsByRepo[repo] = info.StargazersCount
		time.Sleep(300 * time.Millisecond) // be polite to the core rate limit
	}

	scored := make([]ScoredIssue, 0, len(issues))
	for _, iss := range issues {
		stars := starsByRepo[iss.Repo()]
		score := commentScore(iss.Comments) + starScore(stars)
		scored = append(scored, ScoredIssue{
			Issue:      iss,
			RepoStars:  stars,
			Score:      score,
			Difficulty: difficultyLabel(score),
		})
	}
	return scored, nil
}

func commentScore(comments int) int {
	switch {
	case comments <= 2:
		return 0
	case comments <= 6:
		return 1
	default:
		return 2
	}
}

func starScore(stars int) int {
	switch {
	case stars < 200:
		return 0
	case stars < 5000:
		return 1
	default:
		return 2
	}
}

func difficultyLabel(score int) string {
	switch {
	case score <= 1:
		return "Easy"
	case score <= 3:
		return "Medium"
	default:
		return "Hard"
	}
}

// SortByDifficulty sorts a slice of ScoredIssue ascending by Score (easiest first).
func SortByDifficulty(scored []ScoredIssue) {
	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].Score < scored[j].Score
	})
}
