package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/trace"
)

const daemonBaseURL = "http://127.0.0.1:18080"

// DaemonClient is a thin HTTP client used by the foreground REPL (noctifab start)
// to communicate with the background daemon process (noctifab serve).
type DaemonClient struct {
	base   string
	client *http.Client
}

// NewDaemonClient creates a DaemonClient pointing to the standard loopback address.
func NewDaemonClient() *DaemonClient {
	return &DaemonClient{
		base: daemonBaseURL,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// NewDaemonClientWithBase creates a DaemonClient with a custom base URL (useful for tests).
func NewDaemonClientWithBase(base string) *DaemonClient {
	return &DaemonClient{
		base: base,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// IsAlive returns true if the daemon's /healthz endpoint responds successfully.
func (c *DaemonClient) IsAlive() bool {
	_, span := telemetry.Tracer().Start(context.Background(), "IsAlive")
	defer span.End()
	resp, err := c.client.Get(c.base + "/healthz")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// SendStartStory asks the daemon to enqueue a single user story file.
func (c *DaemonClient) SendStartStory(path string) error {
	_, span := telemetry.Tracer().Start(context.Background(), "SendStartStory",
		trace.WithAttributes(telemetry.Attr("path", path)))
	defer span.End()
	return c.postJSON("/api/v1/stories", map[string]any{
		"path":      path,
		"directory": false,
	})
}

// SendStartDirectory asks the daemon to enqueue all user stories in a directory.
func (c *DaemonClient) SendStartDirectory(dirPath string) error {
	_, span := telemetry.Tracer().Start(context.Background(), "SendStartDirectory",
		trace.WithAttributes(telemetry.Attr("dir_path", dirPath)))
	defer span.End()
	return c.postJSON("/api/v1/stories", map[string]any{
		"path":      dirPath,
		"directory": true,
	})
}

// PendingClarification is a clarification question waiting for a developer answer.
type PendingClarification struct {
	ID       string    `json:"id"`
	Question string    `json:"question"`
	Resolved bool      `json:"resolved"`
	Answer   string    `json:"answer"`
	AskedAt  time.Time `json:"asked_at"`
}

// GetPendingClarifications returns all unresolved clarification questions from the daemon.
func (c *DaemonClient) GetPendingClarifications() ([]PendingClarification, error) {
	resp, err := c.client.Get(c.base + "/api/v1/clarifications?pending=true")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch clarifications: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("daemon returned %d: %s", resp.StatusCode, string(body))
	}

	var result []PendingClarification
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode clarifications: %w", err)
	}
	return result, nil
}

// ResolveClarification sends an answer for a pending clarification to the daemon.
func (c *DaemonClient) ResolveClarification(id, answer string) error {
	return c.postJSON(fmt.Sprintf("/api/v1/clarifications/%s/resolve", id), map[string]any{
		"answer": answer,
	})
}

// GetStatus fetches the full current state from the daemon.
func (c *DaemonClient) GetStatus() (*domain.State, error) {
	_, span := telemetry.Tracer().Start(context.Background(), "GetStatus")
	defer span.End()
	resp, err := c.client.Get(c.base + "/statusz")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("daemon returned %d: %s", resp.StatusCode, string(body))
	}

	var state domain.State
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, fmt.Errorf("failed to decode state: %w", err)
	}
	return &state, nil
}

// postJSON is a helper that POSTs a JSON body to the given path and checks for a 2xx response.
func (c *DaemonClient) postJSON(path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.client.Post(c.base+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("request to daemon failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("daemon returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// GetStatusAll fetches the state of all user stories from the daemon.
func (c *DaemonClient) GetStatusAll(ctx context.Context) ([]*domain.State, error) {
	_, span := telemetry.Tracer().Start(ctx, "GetStatusAll")
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch all status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("daemon returned %d: %s", resp.StatusCode, string(body))
	}

	var states []*domain.State
	if err := json.NewDecoder(resp.Body).Decode(&states); err != nil {
		return nil, fmt.Errorf("failed to decode states: %w", err)
	}
	return states, nil
}

// PauseStory pauses the currently running user story.
func (c *DaemonClient) PauseStory(ctx context.Context) error {
	_, span := telemetry.Tracer().Start(ctx, "PauseStory")
	defer span.End()
	return c.postJSONWithContext(ctx, "/api/v1/pause", nil)
}

// ResumeStory resumes the paused user story.
func (c *DaemonClient) ResumeStory(ctx context.Context) error {
	_, span := telemetry.Tracer().Start(ctx, "ResumeStory")
	defer span.End()
	return c.postJSONWithContext(ctx, "/api/v1/resume", nil)
}

// CancelStory cancels the currently running user story.
func (c *DaemonClient) CancelStory(ctx context.Context) error {
	_, span := telemetry.Tracer().Start(ctx, "CancelStory")
	defer span.End()
	return c.postJSONWithContext(ctx, "/api/v1/cancel", nil)
}

// postJSONWithContext is a helper that POSTs a JSON body with a context.
func (c *DaemonClient) postJSONWithContext(ctx context.Context, path string, payload any) error {
	var bodyReader io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bodyReader)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request to daemon failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("daemon returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
