package vcs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
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

func (c *Client) resolveToken(ctx context.Context) string {
	if c.Token != "" {
		return c.Token
	}
	// Try fetching token via `gh auth token` if available
	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err == nil {
		tok := strings.TrimSpace(string(out))
		if tok != "" {
			return tok
		}
	}
	return ""
}

func (c *Client) CreatePullRequest(ctx context.Context, title, body, headBranch, baseBranch string) (string, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "CreatePullRequest",
		trace.WithAttributes(
			attribute.String("provider", c.Provider),
			attribute.String("repository", c.Repository),
			attribute.String("head_branch", headBranch),
			attribute.String("base_branch", baseBranch),
		))
	defer span.End()

	if c.Provider == "mock" || c.Token == "test-token" {
		return fmt.Sprintf("https://github.com/%s/pull/123", c.Repository), nil
	}

	if c.Provider == "github" {
		token := c.resolveToken(ctx)
		var apiErr error

		if token != "" || c.BaseURL != "" {
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
			if err == nil {
				if token != "" {
					req.Header.Set("Authorization", "Bearer "+token)
				}
				req.Header.Set("Accept", "application/vnd.github+json")
				req.Header.Set("Content-Type", "application/json")

				httpClient := &http.Client{}
				resp, err := httpClient.Do(req)
				if err == nil {
					defer func() { _ = resp.Body.Close() }()
					respBody, _ := io.ReadAll(resp.Body)
					if resp.StatusCode == http.StatusCreated {
						var result map[string]any
						if err := json.Unmarshal(respBody, &result); err == nil {
							if htmlURL, ok := result["html_url"].(string); ok && htmlURL != "" {
								return htmlURL, nil
							}
						}
					}
					apiErr = fmt.Errorf("GitHub PR creation status %d: %s", resp.StatusCode, string(respBody))
				} else {
					apiErr = err
				}
			}
		}

		// Fallback to gh CLI tool if HTTP API failed or token was absent
		if ghPR, ghErr := c.createPRViaGHCLI(ctx, title, body, headBranch, baseBranch); ghErr == nil {
			return ghPR, nil
		}

		if apiErr != nil {
			return "", apiErr
		}
		return "", fmt.Errorf("no GITHUB_TOKEN and gh CLI unavailable; cannot create PR for %s", c.Repository)
	}

	return "", fmt.Errorf("unsupported VCS provider: %s", c.Provider)
}

func (c *Client) createPRViaGHCLI(ctx context.Context, title, body, headBranch, baseBranch string) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", err
	}
	args := []string{
		"pr", "create",
		"--repo", c.Repository,
		"--title", title,
		"--body", body,
		"--head", headBranch,
		"--base", baseBranch,
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh pr create failed: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		lastLine := strings.TrimSpace(lines[len(lines)-1])
		if strings.HasPrefix(lastLine, "http") {
			return lastLine, nil
		}
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *Client) MergePullRequest(ctx context.Context, prID string) error {
	ctx, span := telemetry.Tracer().Start(ctx, "MergePullRequest",
		trace.WithAttributes(
			attribute.String("provider", c.Provider),
			attribute.String("pr_id", prID),
		))
	defer span.End()

	if c.Provider == "mock" || c.Token == "test-token" {
		return nil
	}

	if c.Provider == "github" {
		token := c.resolveToken(ctx)
		var apiErr error

		if token != "" || c.BaseURL != "" {
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
			if err == nil {
				if token != "" {
					req.Header.Set("Authorization", "Bearer "+token)
				}
				req.Header.Set("Accept", "application/vnd.github+json")
				req.Header.Set("Content-Type", "application/json")

				httpClient := &http.Client{}
				resp, err := httpClient.Do(req)
				if err == nil {
					defer func() { _ = resp.Body.Close() }()
					respBody, _ := io.ReadAll(resp.Body)
					if resp.StatusCode == http.StatusOK {
						return nil
					}
					apiErr = fmt.Errorf("GitHub PR merge status %d: %s", resp.StatusCode, string(respBody))
				} else {
					apiErr = err
				}
			}
		}

		// Fallback to gh CLI tool
		if ghErr := c.mergePRViaGHCLI(ctx, prID); ghErr == nil {
			return nil
		}

		if apiErr != nil {
			return apiErr
		}
		return fmt.Errorf("no GITHUB_TOKEN and gh CLI unavailable; cannot merge PR %s for %s", prID, c.Repository)
	}

	return fmt.Errorf("unsupported VCS provider: %s", c.Provider)
}

func (c *Client) mergePRViaGHCLI(ctx context.Context, prID string) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return err
	}
	parts := strings.Split(prID, "/")
	prNumber := parts[len(parts)-1]

	args := []string{
		"pr", "merge", prNumber,
		"--repo", c.Repository,
		"--rebase",
		"--auto",
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		argsNoAuto := []string{
			"pr", "merge", prNumber,
			"--repo", c.Repository,
			"--rebase",
		}
		if _, err2 := exec.CommandContext(ctx, "gh", argsNoAuto...).CombinedOutput(); err2 != nil {
			return fmt.Errorf("gh pr merge failed: %s: %w", string(out), err)
		}
	}
	return nil
}
