package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type mockStoryQALLM struct {
	response *domain.LLMResponse
	err      error
}

func (m *mockStoryQALLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return m.response, m.err
}

type mockStoryQASandbox struct {
	runCmdFunc func(ctx context.Context, projectPath, command, pkg string) (string, error)
}

func (m *mockStoryQASandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	if m.runCmdFunc != nil {
		return m.runCmdFunc(ctx, projectPath, command, pkg)
	}
	return "", nil
}

func TestStoryQAAuditor_AuditStoryCompleteness(t *testing.T) {
	tmpDir := t.TempDir()

	storyDir := filepath.Join(tmpDir, "roadmap", "user-stories")
	if err := os.MkdirAll(storyDir, 0755); err != nil {
		t.Fatalf("failed to create story dir: %v", err)
	}
	storyPath := filepath.Join(storyDir, "US-001.md")
	storyContent := `# US-001: Core Key-Value Commands
## Acceptance Criteria
1. PING returns PONG.
2. SET key value stores value.
3. GET key returns stored value.
`
	if err := os.WriteFile(storyPath, []byte(storyContent), 0644); err != nil {
		t.Fatalf("failed to write story file: %v", err)
	}

	state := &domain.State{
		ProjectPath: tmpDir,
		Metadata: domain.StateMetadata{
			FeatureName: "US-001",
			InputPath:   storyPath,
		},
		Tasks: []domain.Task{
			{ID: "task-1", Title: "Implement PING", Status: domain.TaskSuccess},
		},
	}

	t.Run("when all story requirements are met, audit passes", func(t *testing.T) {
		mockLLM := &mockStoryQALLM{
			response: &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "submit_story_qa_audit",
						Args: map[string]any{
							"passed":           true,
							"summary":          "All PING/SET/GET commands implemented and tested",
							"missing_features": []any{},
						},
					},
				},
			},
		}

		auditor := NewStoryQAAuditor(mockLLM)
		res, err := auditor.AuditStoryCompleteness(context.Background(), state, storyPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Passed {
			t.Errorf("expected audit to pass, got failed (summary: %s)", res.Summary)
		}
		if len(res.MissingFeatures) != 0 {
			t.Errorf("expected 0 missing features, got %d", len(res.MissingFeatures))
		}
	})

	t.Run("when features are missing, audit fails with missing feature list", func(t *testing.T) {
		mockLLM := &mockStoryQALLM{
			response: &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "submit_story_qa_audit",
						Args: map[string]any{
							"passed":           false,
							"summary":          "SET and GET commands are missing from server dispatcher",
							"missing_features": []any{"SET command", "GET command"},
						},
					},
				},
			},
		}

		auditor := NewStoryQAAuditor(mockLLM)
		res, err := auditor.AuditStoryCompleteness(context.Background(), state, storyPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Errorf("expected audit to fail when missing features exist")
		}
		if len(res.MissingFeatures) != 2 {
			t.Fatalf("expected 2 missing features, got %d", len(res.MissingFeatures))
		}
		if res.MissingFeatures[0] != "SET command" || res.MissingFeatures[1] != "GET command" {
			t.Errorf("unexpected missing features: %v", res.MissingFeatures)
		}
	})

	t.Run("when E2E test command fails, audit fails immediately", func(t *testing.T) {
		mockRunner := &mockStoryQASandbox{
			runCmdFunc: func(ctx context.Context, projectPath, command, pkg string) (string, error) {
				return "Error: Client connection refused on 6379", os.ErrInvalid
			},
		}

		auditor := NewStoryQAAuditor(&mockStoryQALLM{}, mockRunner)
		auditor.SetE2ECommand("make e2e")
		res, err := auditor.AuditStoryCompleteness(context.Background(), state, storyPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Passed {
			t.Errorf("expected audit to fail when E2E command fails")
		}
		if len(res.MissingFeatures) == 0 {
			t.Errorf("expected missing features explaining E2E failure")
		}
	})

	t.Run("when docker-compose.e2e.yml exists, detectE2ECommand discovers it", func(t *testing.T) {
		composePath := filepath.Join(tmpDir, "docker-compose.e2e.yml")
		_ = os.WriteFile(composePath, []byte("version: '3'"), 0o644)
		auditor := NewStoryQAAuditor(&mockStoryQALLM{})
		cmd := auditor.detectE2ECommand(tmpDir)
		if cmd != "docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner" {
			t.Errorf("unexpected discovered e2e command: %s", cmd)
		}
	})
}
