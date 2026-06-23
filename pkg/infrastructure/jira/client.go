package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// IssueResponse represents the structure returned by Jira issue REST API (v3)
type IssueResponse struct {
	Key    string `json:"key"`
	Fields struct {
		Summary         string  `json:"summary"`
		Description     ADFNode `json:"description"`
		DescriptionText string  `json:"description_text,omitempty"`
	} `json:"fields"`
}

// Client connects to Jira Cloud REST API
type Client struct {
	BaseURL    string
	User       string
	Token      string
	MaxRetries int
	Backoff    time.Duration
}

func NewClient(baseURL, user, token string, maxRetries int, backoff time.Duration) *Client {
	if maxRetries <= 0 {
		maxRetries = 10
	}
	if backoff <= 0 {
		backoff = 100 * time.Millisecond
	}
	return &Client{
		BaseURL:    baseURL,
		User:       user,
		Token:      token,
		MaxRetries: maxRetries,
		Backoff:    backoff,
	}
}

// FetchIssueDescription gets the issue description and converts it to GFM markdown
func (c *Client) FetchIssueDescription(ctx context.Context, issueKey string) (string, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", c.BaseURL, issueKey)

	var respBody []byte
	var err error

	backoff := c.Backoff
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		respBody, err = c.doGet(ctx, url)
		if err == nil {
			break
		}

		if attempt == c.MaxRetries {
			return "", fmt.Errorf("jira API request failed after %d retries: %w", c.MaxRetries, err)
		}

		// Exponential backoff with jitter
		jitter := time.Duration(float64(backoff) * (1.0 + rand.Float64()))
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(jitter):
		}
		backoff *= 2
	}

	var issue IssueResponse
	if err := json.Unmarshal(respBody, &issue); err != nil {
		return "", fmt.Errorf("failed to unmarshal Jira response: %w", err)
	}

	// Try to walk ADF description
	gfm := WalkADF(issue.Fields.Description)
	gfm = strings.TrimSpace(gfm)

	// Fallback to text if ADF description parsing yielded empty results
	if gfm == "" {
		if issue.Fields.DescriptionText != "" {
			gfm = issue.Fields.DescriptionText
		} else {
			gfm = issue.Fields.Summary
		}
	}

	return gfm, nil
}

func (c *Client) doGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	if c.User != "" && c.Token != "" {
		req.SetBasicAuth(c.User, c.Token)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jira HTTP status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
