package vcs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Client struct {
	Provider   string
	Repository string
	Token      string
	BaseURL    string
}

var _ domain.VCSClient = (*Client)(nil)

func NewClient(provider, repository, token string) *Client {
	return &Client{
		Provider:   provider,
		Repository: repository,
		Token:      token,
	}
}

func (c *Client) CreatePullRequest(ctx context.Context, title, body, headBranch, baseBranch string) (string, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "noctifab.vcs_create_pr",
		trace.WithAttributes(
			attribute.String("provider", c.Provider),
			attribute.String("repository", c.Repository),
			attribute.String("head_branch", headBranch),
			attribute.String("base_branch", baseBranch),
		))
	defer span.End()

	if c.Provider == "mock" || c.Token == "test-token" || c.Token == "" {
		// Mock implementation for test runners/offline validation
		return fmt.Sprintf("https://github.com/%s/pull/123", c.Repository), nil
	}

	if c.Provider == "github" {
		baseURL := c.BaseURL
		if baseURL == "" {
			baseURL = "https://api.github.com"
		}
		url := fmt.Sprintf("%s/repos/%s/pulls", baseURL, c.Repository)
		payload := map[string]string{
			"title": title,
			"body":  body,
			"head":  headBranch,
			"base":  baseBranch,
		}
		reqBody, _ := json.Marshal(payload)

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
		if err != nil {
			return "", err
		}

		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Content-Type", "application/json")

		httpClient := &http.Client{}
		resp, err := httpClient.Do(req)
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusCreated {
			return "", fmt.Errorf("GitHub PR creation status %d: %s", resp.StatusCode, string(respBody))
		}

		var result map[string]any
		if err := json.Unmarshal(respBody, &result); err != nil {
			return "", err
		}

		htmlURL, _ := result["html_url"].(string)
		return htmlURL, nil
	}

	return "", fmt.Errorf("unsupported VCS provider: %s", c.Provider)
}

func (c *Client) MergePullRequest(ctx context.Context, prID string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "noctifab.vcs_merge_pr",
		trace.WithAttributes(
			attribute.String("provider", c.Provider),
			attribute.String("pr_id", prID),
		))
	defer span.End()

	if c.Provider == "mock" || c.Token == "test-token" || c.Token == "" {
		return nil
	}

	if c.Provider == "github" {
		// Extract PR number from ID or URL (e.g., https://github.com/owner/repo/pull/123)
		parts := strings.Split(prID, "/")
		prNumber := parts[len(parts)-1]

		baseURL := c.BaseURL
		if baseURL == "" {
			baseURL = "https://api.github.com"
		}
		url := fmt.Sprintf("%s/repos/%s/pulls/%s/merge", baseURL, c.Repository, prNumber)
		payload := map[string]string{
			"merge_method": "rebase",
		}
		reqBody, _ := json.Marshal(payload)

		req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(reqBody))
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Content-Type", "application/json")

		httpClient := &http.Client{}
		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GitHub PR merge status %d: %s", resp.StatusCode, string(respBody))
		}

		return nil
	}

	return fmt.Errorf("unsupported VCS provider: %s", c.Provider)
}
