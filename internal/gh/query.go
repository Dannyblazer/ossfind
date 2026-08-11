package gh

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Search runs a single GitHub issue-search query and returns the matched issues.
func (c *Client) Search(query string, sortBy string, perPage int) ([]Issue, error) {
	if perPage <= 0 || perPage > 100 {
		perPage = 30
	}

	q := url.Values{}
	q.Set("q", query)
	if sortBy != "" {
		q.Set("sort", sortBy)
		q.Set("order", "desc")
	}
	q.Set("per_page", strconv.Itoa(perPage))

	body, _, err := c.doGet(searchURL + "?" + q.Encode())
	if err != nil {
		return nil, err
	}

	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return sr.Items, nil
}

// SearchCount runs a search query and returns only GitHub's total_count,
// fetching a single result (per_page=1) to keep the request cheap. Useful
// when you need "how many" without needing the items themselves.
func (c *Client) SearchCount(query string) (int, error) {
	q := url.Values{}
	q.Set("q", query)
	q.Set("per_page", "1")

	body, _, err := c.doGet(searchURL + "?" + q.Encode())
	if err != nil {
		return 0, err
	}

	var sr searchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return 0, fmt.Errorf("decoding response: %w", err)
	}
	return sr.TotalCount, nil
}
