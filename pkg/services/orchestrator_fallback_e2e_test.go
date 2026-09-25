package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockFallbackE2ESandbox struct {
	e2eFailCount int
	calls        int
}

func (m *mockFallbackE2ESandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	m.calls++
	if command == "make e2e" || command == "docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner" {
		if m.e2eFailCount > 0 {
			m.e2eFailCount--
			return "FAIL: E2E connection refused on port 6379", errors.New("exit status 1")
		}
		return "PASS: E2E all integration scenarios passed", nil
	}
	// Default unit test command passes
	return "PASS: 5 unit tests passed", nil
}

type promptRecordingLLM struct {
	prompts   []string
	responses []*domain.LLMResponse
	calls     int
}

func (m *promptRecordingLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	m.prompts = append(m.prompts, prompt)
	m.calls++
	if len(m.responses) >= m.calls {
		return m.responses[m.calls-1], nil
	}
	return &domain.LLMResponse{Reasoning: "noop"}, nil
}

func (m *promptRecordingLLM) GenerateTask(ctx context.Context, prompt string) (*domain.Task, error) {
	return nil, nil
}

func (m *promptRecordingLLM) GeneratePlan(ctx context.Context, prompt string) ([]domain.Task, error) {
	return nil, nil
}

func (m *promptRecordingLLM) Name() string {
	return "mock-recording-llm"
}

func (m *promptRecordingLLM) Provider() string {
	return "mock"
}

func (m *promptRecordingLLM) Model() string {
	return "mock-model"
}

func TestOrchestrator_RunFallbackAgent_E2EGate(t *testing.T) {
	tempDir := t.TempDir()

	// Create docker-compose.e2e.yml so DetectE2ECommand detects E2E suite
	e2eComposePath := filepath.Join(tempDir, "docker-compose.e2e.yml")
	require.NoError(t, os.WriteFile(e2eComposePath, []byte("version: '3'\nservices:\n  test-runner:\n    image: test\n"), 0o600))

	reg := NewToolRegistry()
	reg.Register(&WriteFileTool{})

	sandbox := &mockFallbackE2ESandbox{e2eFailCount: 1}

	mockLLM := &promptRecordingLLM{
		responses: []*domain.LLMResponse{
			{
				Reasoning: "Turn 1: Write server code",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    filepath.Join(tempDir, "server.py"),
							"content": "def run(): pass\n",
						},
					},
				},
			},
			{
				Reasoning: "Turn 2: Fix connection refused error in docker compose",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    filepath.Join(tempDir, "server.py"),
							"content": "def run(): print('listening on 6379')\n",
						},
					},
				},
			},
		},
	}

	evaluator := NewTestValidator(sandbox, false, mockLLM, reg.Tools())

	cfg := OrchestratorConfig{
		Fallback: config.FallbackAgentConfig{
			Enabled:  true,
			MaxTurns: 2,
		},
		E2E: config.E2EConfig{
			Mode: "docker",
		},
	}

	orch := &Orchestrator{
		cfg:            cfg,
		llmClient:      mockLLM,
		registry:       reg,
		evaluator:      evaluator,
		promptRenderer: prompts.NewDefaultRenderer(),
	}

	task := domain.Task{
		ID:          "sovereign-rescue-us-001",
		Title:       "Sovereign Rescue: Unblock Story US-001 QA & E2E Verification",
		Description: "Fix E2E container failures",
	}
	taskState := domain.State{
		ID:          "story-1",
		ProjectPath: tempDir,
	}

	passed, _ := orch.RunFallbackAgent(context.Background(), &task, &taskState, nil, "initial failure", "story_qa_persistent_e2e_failure")
	require.True(t, passed, "expected Fallback Agent to succeed after turn 2 fixes E2E")
	assert.Equal(t, 2, mockLLM.calls, "expected turn 1 to fail due to E2E failure and proceed to turn 2")

	// Verify that turn 2 received the E2E failure output
	require.GreaterOrEqual(t, len(mockLLM.prompts), 2)
	assert.Contains(t, mockLLM.prompts[1], "E2E connection refused on port 6379")
}

func TestOrchestrator_RunFallbackAgent_AntiStub(t *testing.T) {
	tempDir := t.TempDir()

	testsDir := filepath.Join(tempDir, "tests")
	require.NoError(t, os.MkdirAll(testsDir, 0o755))

	reg := NewToolRegistry()
	reg.Register(&WriteFileTool{})

	sandbox := &mockFallbackE2ESandbox{e2eFailCount: 0}

	relPath := "tests/test_app.py"

	mockLLM := &promptRecordingLLM{
		responses: []*domain.LLMResponse{
			{
				Reasoning: "Turn 1 writes a tautological test",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    relPath,
							"content": "import unittest\nclass TestApp(unittest.TestCase):\n    def test_tautology(self):\n        assert True\n",
						},
					},
				},
			},
			{
				Reasoning: "Turn 2 writes a genuine assertion test",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    relPath,
							"content": "import unittest\nclass TestApp(unittest.TestCase):\n    def test_valid(self):\n        self.assertEqual(1 + 1, 2)\n",
						},
					},
				},
			},
		},
	}

	evaluator := NewTestValidator(sandbox, false, mockLLM, reg.Tools())

	cfg := OrchestratorConfig{
		Fallback: config.FallbackAgentConfig{
			Enabled:  true,
			MaxTurns: 2,
		},
	}

	orch := &Orchestrator{
		cfg:            cfg,
		llmClient:      mockLLM,
		registry:       reg,
		evaluator:      evaluator,
		promptRenderer: prompts.NewDefaultRenderer(),
	}

	task := domain.Task{
		ID:          "sovereign-rescue-us-001",
		Title:       "Sovereign Rescue: Unblock Story US-001",
		Description: "Fix failing tests",
		TargetFiles: []string{relPath},
	}
	taskState := domain.State{
		ID:          "story-1",
		ProjectPath: tempDir,
	}

	passed, _ := orch.RunFallbackAgent(context.Background(), &task, &taskState, nil, "initial failure", "retries_exhausted")
	require.True(t, passed, "expected Fallback Agent to succeed on turn 2 after replacing tautological test")
	assert.Equal(t, 2, mockLLM.calls, "expected turn 1 to fail due to anti-stub violation")

	// Verify that turn 2 received the anti-stub violation details
	require.GreaterOrEqual(t, len(mockLLM.prompts), 2)
	assert.Contains(t, mockLLM.prompts[1], "Anti-stub / anti-gaming validation failed")
	assert.Contains(t, mockLLM.prompts[1], "tautological_test_assertion")
}
