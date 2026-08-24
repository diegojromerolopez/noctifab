package services

import (
	"context"
	"errors"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestrator_PreMergeGate_RequeuesWhenRetriesAvailable(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	state := &domain.State{
		ID:          "state-premerge-retry",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-retryable", Title: "Retryable Failing Task", TargetFiles: []string{"file.go"}, Status: domain.TaskPending, Retries: 0, MaxRetries: 2},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: "noctifab/feature-state-premerge-retry",
		},
	}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&mockTool{name: "read_file"})
	reg.Register(&mockTool{name: "write_file"})
	reg.Register(&mockTool{name: "edit_file"})
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

	evaluator := NewTestValidator(&mockSandbox{Out: "FAIL: logic error", Err: errors.New("exit 1")}, false, llmClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{
		Concurrency:  1,
		UseWorktrees: true,
	}

	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)
	orch.executeTask(context.Background(), "state-premerge-retry", "task-retryable")

	updatedState, err := repo.Load(context.Background())
	require.NoError(t, err)

	// Verify task is re-queued as TaskPending with Retries = 1 and Progress = 0
	assert.Equal(t, domain.TaskPending, updatedState.Tasks[0].Status)
	assert.Equal(t, 1, updatedState.Tasks[0].Retries)
	assert.Equal(t, 0, updatedState.Tasks[0].Progress)
	assert.Equal(t, domain.BuildFailing, updatedState.BuildStatus)
}

func TestOrchestrator_PreMergeGate_SandboxFailureFastAborts(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	state := &domain.State{
		ID:          "state-sandbox-abort",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-sandbox-err", Title: "Sandbox Error Task", TargetFiles: []string{"file.go"}, Status: domain.TaskPending, Retries: 0, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: "noctifab/feature-state-sandbox-abort",
		},
	}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&mockTool{name: "read_file"})
	reg.Register(&mockTool{name: "write_file"})

	llmClient := &testMockLLM{
		responses: []*domain.LLMResponse{
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

	evaluator := NewTestValidator(&mockSandbox{Out: "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?", Err: errors.New("exit 1")}, false, llmClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{
		Concurrency:  1,
		UseWorktrees: true,
	}

	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)
	orch.executeTask(context.Background(), "state-sandbox-abort", "task-sandbox-err")

	updatedState, err := repo.Load(context.Background())
	require.NoError(t, err)

	// Verify unrecoverable environment failure fast-aborts to TaskFailed without retries
	assert.Equal(t, domain.TaskFailed, updatedState.Tasks[0].Status)
	assert.Equal(t, 0, updatedState.Tasks[0].Retries)
	assert.Contains(t, updatedState.Tasks[0].FailureLog, "Unrecoverable environment error")
	assert.Equal(t, domain.BuildFailing, updatedState.BuildStatus)
}

func TestOrchestrator_PreMergeGate_EffectiveMaxRetriesFallback(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	state := &domain.State{
		ID:          "state-fallback-retry",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			// MaxRetries: 0 (uninitialized)
			{ID: "task-zero-max-retries", Title: "Zero Max Retries Task", TargetFiles: []string{"file.go"}, Status: domain.TaskPending, Retries: 0, MaxRetries: 0},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: "noctifab/feature-state-fallback-retry",
		},
	}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&mockTool{name: "read_file"})
	reg.Register(&mockTool{name: "write_file"})

	llmClient := &testMockLLM{
		responses: []*domain.LLMResponse{
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

	evaluator := NewTestValidator(&mockSandbox{Out: "FAIL: logic error", Err: errors.New("exit 1")}, false, llmClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{
		Concurrency:  1,
		UseWorktrees: true,
		MaxRetries:   3, // Global fallback
	}

	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)
	orch.executeTask(context.Background(), "state-fallback-retry", "task-zero-max-retries")

	updatedState, err := repo.Load(context.Background())
	require.NoError(t, err)

	// Verify task used effectiveMaxRetries = 3 and was re-queued for retry 1
	assert.Equal(t, domain.TaskPending, updatedState.Tasks[0].Status)
	assert.Equal(t, 1, updatedState.Tasks[0].Retries)
	assert.Equal(t, 3, updatedState.Tasks[0].MaxRetries)
}
