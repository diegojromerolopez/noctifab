package services

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testMockLLM struct {
	responses []*domain.LLMResponse
	callCount int
	mu        sync.Mutex
}

func (m *testMockLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callCount >= len(m.responses) {
		return &domain.LLMResponse{Actions: []domain.LLMAction{{Tool: "noop"}}}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

type mockTool struct {
	name string
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return "mock description" }
func (t *mockTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	return "mock tool output", nil
}

type mockSandbox struct {
	Out string
	Err error
}

func (m *mockSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	return m.Out, m.Err
}

func TestOrchestrator_ConcurrentWorktreeIsolation(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	state := &domain.State{
		ID:          "state-concurrent-worktree",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-A", Title: "Task A", TargetFiles: []string{"a.txt"}, Status: domain.TaskPending, MaxRetries: 3},
			{ID: "task-B", Title: "Task B", TargetFiles: []string{"b.txt"}, Status: domain.TaskPending, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: "noctifab/feature-state-concurrent-worktree",
		},
	}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&mockTool{name: "read_file"})
	reg.Register(&mockTool{name: "write_file"})
	reg.Register(&mockTool{name: "run_tests"})

	llmClient := &testMockLLM{
		responses: []*domain.LLMResponse{
			{Actions: []domain.LLMAction{{Tool: "noop"}}},
			{Actions: []domain.LLMAction{{Tool: "noop"}}},
			{Actions: []domain.LLMAction{{Tool: "noop"}}},
			{Actions: []domain.LLMAction{{Tool: "noop"}}},
		},
	}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(repoDir)
	queue := NewRebaseQueue(git)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go queue.Start(ctx)

	evaluator := NewTestValidator(&mockSandbox{Out: "ok"}, false, llmClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{
		Concurrency:  2,
		UseWorktrees: true,
	}

	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		orch.executeTask(context.Background(), "state-concurrent-worktree", "task-A")
	}()
	go func() {
		defer wg.Done()
		orch.executeTask(context.Background(), "state-concurrent-worktree", "task-B")
	}()
	wg.Wait()

	updatedState, err := repo.Load(context.Background())
	require.NoError(t, err)

	assert.Equal(t, domain.TaskSuccess, updatedState.Tasks[0].Status)
	assert.Equal(t, domain.TaskSuccess, updatedState.Tasks[1].Status)

	// Worktree directories should be cleaned up
	_, errA := os.Stat(filepath.Join(repoDir, ".noctifab", "worktrees", "task-task-A"))
	assert.True(t, os.IsNotExist(errA), "Worktree directory for task-A should be cleaned up")

	_, errB := os.Stat(filepath.Join(repoDir, ".noctifab", "worktrees", "task-task-B"))
	assert.True(t, os.IsNotExist(errB), "Worktree directory for task-B should be cleaned up")
}

func TestExecuteTask_FastAbortOnSandboxFailure(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	state := &domain.State{
		ID:          "state-sandbox-fail",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-env-fail", Title: "Env Fail", Retries: 0, MaxRetries: 3, Status: domain.TaskPending},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: "noctifab/feature-state-sandbox-fail",
		},
	}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&mockTool{name: "read_file"})
	reg.Register(&mockTool{name: "write_file"})
	reg.Register(&mockTool{name: "run_tests"})

	llmClient := &testMockLLM{
		responses: []*domain.LLMResponse{
			{Actions: []domain.LLMAction{{Tool: "noop"}}},
			{Actions: []domain.LLMAction{{Tool: "noop"}}},
		},
	}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(repoDir)
	queue := NewRebaseQueue(git)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go queue.Start(ctx)

	evaluator := NewTestValidator(&mockSandbox{Out: "ERROR: sandbox toolchain binary /usr/bin/gcc not found", Err: os.ErrNotExist}, false, llmClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{
		Concurrency:  1,
		UseWorktrees: true,
	}

	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil)
	orch.executeTask(context.Background(), "state-sandbox-fail", "task-env-fail")

	updatedState, err := repo.Load(context.Background())
	require.NoError(t, err)

	assert.Equal(t, domain.TaskFailed, updatedState.Tasks[0].Status)
	assert.Equal(t, 0, updatedState.Tasks[0].Retries, "Retries should not be wasted on sandbox failures")
}
