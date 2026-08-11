package gh

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	searchURL = "https://api.github.com/search/issues"
	reposURL  = "https://api.github.com/repos/"
)

type Client struct {
	Token      string
	HTTPClient *http.Client
	// LogFunc receives progress/rate-limit messages. Defaults to a no-op.
	LogFunc func(format string, args ...any)
}

func NewClient(token string) *Client {
	return &Client{
		Token:      token,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		LogFunc:    func(string, ...any) {},
	}
}

// doGet performs an authenticated GET, transparently waiting and retrying
// once if GitHub reports the relevant quota (search or core) is exhausted.
func (c *Client) doGet(rawURL string) ([]byte, http.Header, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	remaining := resp.Header.Get("X-RateLimit-Remaining")
	reset := resp.Header.Get("X-RateLimit-Reset")

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		wait := waitDuration(reset)
		c.LogFunc("rate limited (remaining=%s), waiting %s before retry: %s", remaining, wait, rawURL)
		time.Sleep(wait)
		return c.doGet(rawURL) // retry once after waiting
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("github api error %d: %s", resp.StatusCode, string(body))
	}

	c.LogFunc("GET %s -> 200 (remaining quota: %s)", rawURL, remaining)

	return body, resp.Header, nil
}

func waitDuration(resetHeader string) time.Duration {
	if resetHeader == "" {
		return 10 * time.Second
	}
	sec, err := strconv.ParseInt(resetHeader, 10, 64)
	if err != nil {
		return 10 * time.Second
	}
	d := time.Until(time.Unix(sec, 0))
	if d < 0 {
		return 2 * time.Second
	}
	return d + time.Second
}
