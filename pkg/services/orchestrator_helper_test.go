package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type customTool struct {
	name      string
	executeFn func(ctx context.Context, state *domain.State, args map[string]any) (string, error)
}

func (t *customTool) Name() string        { return t.name }
func (t *customTool) Description() string { return "custom tool" }
func (t *customTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	if t.executeFn != nil {
		return t.executeFn(ctx, state, args)
	}
	return "", nil
}

type trackingMockLLM struct {
	responses              []*domain.LLMResponse
	callCount              int
	testerAgentInvokeCount int
}

func (m *trackingMockLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if strings.Contains(prompt, "Feedback from generator agent:") {
		m.testerAgentInvokeCount++
	}

	if m.callCount >= len(m.responses) {
		return &domain.LLMResponse{Actions: []domain.LLMAction{{Tool: "noop"}}}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

func TestRunReaderPhase_HeuristicSkip(t *testing.T) {
	tempDir := t.TempDir()
	state := &domain.State{
		ProjectPath: tempDir,
	}

	task := domain.Task{
		ID:          "task-reader",
		Title:       "Update Config Parser",
		TargetFiles: []string{"pkg/config/parser.go"},
	}

	err := os.MkdirAll(filepath.Join(tempDir, "pkg/config"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "pkg/config/parser.go"), []byte("package config\n"), 0644)
	require.NoError(t, err)

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&customTool{
		name: "read_file",
		executeFn: func(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
			path, _ := args["path"].(string)
			content, err := os.ReadFile(filepath.Join(state.ProjectPath, path))
			return string(content), err
		},
	})

	mockLLMClient := &testMockLLM{}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(tempDir)
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(nil, false, mockLLMClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{PollInterval: 10 * time.Millisecond}

	orch := NewOrchestrator(repo, reg, mockLLMClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)

	ctx := context.Background()
	gathered := orch.RunReaderPhase(ctx, "generator", task, state)

	assert.Len(t, gathered, 1)
	assert.Contains(t, gathered[0], "package config")
	assert.Equal(t, 0, mockLLMClient.callCount, "LLM should not be invoked when heuristic context is available")
}

func TestRunGeneratorAgent_NoopAutoExecutesRunTests(t *testing.T) {
	tempDir := t.TempDir()
	state := &domain.State{
		ProjectPath: tempDir,
	}

	task := domain.Task{ID: "task-noop-test", Title: "Noop Test"}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()

	testRunnerCalled := false
	reg.Register(&customTool{
		name: "run_tests",
		executeFn: func(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
			testRunnerCalled = true
			return "tests passed", nil
		},
	})

	mockLLMClient := &testMockLLM{
		responses: []*domain.LLMResponse{
			{Actions: []domain.LLMAction{{Tool: "noop"}}},
		},
	}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(tempDir)
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(nil, false, mockLLMClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{PollInterval: 10 * time.Millisecond}

	orch := NewOrchestrator(repo, reg, mockLLMClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)

	orch.RunGeneratorAgent(context.Background(), task, state, nil, "", "implement")

	assert.Equal(t, 1, mockLLMClient.callCount, "Agent should exit on turn 1 when returning noop")
	assert.True(t, testRunnerCalled, "run_tests must be auto-triggered on noop signal")
}

func TestRunGeneratorAgent_LimitRequestTestFix(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	state := &domain.State{
		ProjectPath:  repoDir,
		ActiveAgents: []domain.Agent{},
	}

	task := domain.Task{ID: "task-reentrant", Title: "Re-entrant Test"}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&customTool{
		name: "read_file",
		executeFn: func(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
			path, _ := args["path"].(string)
			content, err := os.ReadFile(filepath.Join(state.ProjectPath, path))
			return string(content), err
		},
	})
	reg.Register(&mockTool{name: "run_tests"})

	mockLLMClient := &trackingMockLLM{
		responses: []*domain.LLMResponse{
			{Actions: []domain.LLMAction{{Tool: "request_test_fix", Args: map[string]any{"feedback": "fix assertion"}}}},
			{Actions: []domain.LLMAction{{Tool: "request_test_fix", Args: map[string]any{"feedback": "fix syntax"}}}},
			{Actions: []domain.LLMAction{{Tool: "noop"}}},
		},
	}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(repoDir)
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(nil, false, mockLLMClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{PollInterval: 10 * time.Millisecond}

	orch := NewOrchestrator(repo, reg, mockLLMClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)

	orch.RunGeneratorAgent(context.Background(), task, state, nil, "", "implement")

	assert.Equal(t, 1, mockLLMClient.testerAgentInvokeCount, "request_test_fix should be limited to 1 call per task execution")
}
