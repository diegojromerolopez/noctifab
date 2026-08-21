package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fileWritingMockTool struct {
	name string
}

func (t *fileWritingMockTool) Name() string        { return t.name }
func (t *fileWritingMockTool) Description() string { return "mock description" }
func (t *fileWritingMockTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	if t.name == "write_file" {
		path, _ := args["path"].(string)
		if path == "" {
			path = "artifact.txt"
		}
		fullPath := filepath.Join(state.ProjectPath, path)
		_ = os.MkdirAll(filepath.Dir(fullPath), 0755)
		content, _ := args["content"].(string)
		if content == "" {
			content = "dummy content\n"
		}
		_ = os.WriteFile(fullPath, []byte(content), 0644)
	}
	return "ok", nil
}

type alwaysGeneratingMockLLM struct {
	mu        sync.Mutex
	callCount int
}

func (m *alwaysGeneratingMockLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	filename := fmt.Sprintf("file_%d.txt", m.callCount)
	return &domain.LLMResponse{
		Actions: []domain.LLMAction{
			{
				Tool: "write_file",
				Args: map[string]any{
					"path":    filename,
					"content": fmt.Sprintf("content for call %d\n", m.callCount),
				},
			},
		},
	}, nil
}

func TestOrchestrator_CumulativeBranching_SequentialStories(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	integrationBranch := "noctifab/implementation"

	reg := NewToolRegistry()
	reg.Register(&fileWritingMockTool{name: "read_file"})
	reg.Register(&fileWritingMockTool{name: "write_file"})
	reg.Register(&fileWritingMockTool{name: "run_tests"})

	llmClient := &alwaysGeneratingMockLLM{}
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
		Concurrency:  1,
		UseWorktrees: true,
	}

	// --- STORY 1 (US-001) ---
	state1 := &domain.State{
		ID:          "state-story-1",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-story-1", Title: "Create parser", TargetFiles: []string{"parser.go"}, Status: domain.TaskPending, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: integrationBranch,
			FeatureName:       "US-001",
		},
	}
	repo1 := &mockRepo{state: state1}
	orch1 := NewOrchestrator(repo1, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)

	orch1.executeTask(ctx, "state-story-1", "task-story-1")

	st1, err := repo1.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskSuccess, st1.Tasks[0].Status, "Story 1 task should succeed")

	// Finalize Story 1
	err = orch1.FinalizeUserStory(ctx, st1)
	require.NoError(t, err)

	// Verify Story 1 commit on integration branch
	log1, err := git.Run(ctx, false, "log", "--oneline", integrationBranch)
	require.NoError(t, err)
	assert.Contains(t, log1, "feat(core): implement minimal functionality for task task-story-1")

	// --- STORY 2 (US-002) ---
	// Story 2 starts fresh with all tasks in TaskPending status (no TaskSuccess yet)
	state2 := &domain.State{
		ID:          "state-story-2",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-story-2", Title: "Create C ABI", TargetFiles: []string{"abi.c"}, Status: domain.TaskPending, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: integrationBranch,
			FeatureName:       "US-002",
		},
	}
	repo2 := &mockRepo{state: state2}
	orch2 := NewOrchestrator(repo2, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)

	orch2.executeTask(ctx, "state-story-2", "task-story-2")

	st2, err := repo2.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskSuccess, st2.Tasks[0].Status, "Story 2 task should succeed")

	// Finalize Story 2
	err = orch2.FinalizeUserStory(ctx, st2)
	require.NoError(t, err)

	// Verify Story 2 did NOT overwrite Story 1! Both commits MUST exist on integration branch
	log2, err := git.Run(ctx, false, "log", "--oneline", integrationBranch)
	require.NoError(t, err)
	assert.Contains(t, log2, "feat(core): implement minimal functionality for task task-story-1", "Story 1 commits must be retained on integration branch")
	assert.Contains(t, log2, "feat(core): implement minimal functionality for task task-story-2", "Story 2 commits must be appended onto integration branch")

	// --- STORY 3 (US-003) ---
	state3 := &domain.State{
		ID:          "state-story-3",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-story-3", Title: "Create Python driver", TargetFiles: []string{"driver.py"}, Status: domain.TaskPending, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: integrationBranch,
			FeatureName:       "US-003",
		},
	}
	repo3 := &mockRepo{state: state3}
	orch3 := NewOrchestrator(repo3, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)

	orch3.executeTask(ctx, "state-story-3", "task-story-3")

	st3, err := repo3.Load(ctx)
	require.NoError(t, err)
	assert.Equal(t, domain.TaskSuccess, st3.Tasks[0].Status, "Story 3 task should succeed")

	// Finalize Story 3
	err = orch3.FinalizeUserStory(ctx, st3)
	require.NoError(t, err)

	// Verify continuous cumulative history containing Story 1, Story 2, and Story 3
	log3, err := git.Run(ctx, false, "log", "--oneline", integrationBranch)
	require.NoError(t, err)
	assert.Contains(t, log3, "feat(core): implement minimal functionality for task task-story-1")
	assert.Contains(t, log3, "feat(core): implement minimal functionality for task task-story-2")
	assert.Contains(t, log3, "feat(core): implement minimal functionality for task task-story-3")
	assert.Contains(t, log3, "chore(release): bump version to 0.0.2 for story US-001")
	assert.Contains(t, log3, "chore(release): bump version to 0.0.3 for story US-002")
	assert.Contains(t, log3, "chore(release): bump version to 0.0.4 for story US-003")
}

func TestOrchestrator_CumulativeBranching_DirectMode(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	integrationBranch := "noctifab/implementation"

	reg := NewToolRegistry()
	reg.Register(&fileWritingMockTool{name: "read_file"})
	reg.Register(&fileWritingMockTool{name: "write_file"})
	reg.Register(&fileWritingMockTool{name: "run_tests"})

	llmClient := &alwaysGeneratingMockLLM{}
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
		Concurrency:  1,
		UseWorktrees: false, // Direct mode (no worktrees)
	}

	state1 := &domain.State{
		ID:          "state-direct-1",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-direct-1", Title: "Task 1", TargetFiles: []string{"t1.txt"}, Status: domain.TaskPending, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: integrationBranch,
			FeatureName:       "US-001",
		},
	}
	repo1 := &mockRepo{state: state1}
	orch1 := NewOrchestrator(repo1, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)
	orch1.executeTask(ctx, "state-direct-1", "task-direct-1")

	state2 := &domain.State{
		ID:          "state-direct-2",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-direct-2", Title: "Task 2", TargetFiles: []string{"t2.txt"}, Status: domain.TaskPending, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: integrationBranch,
			FeatureName:       "US-002",
		},
	}
	repo2 := &mockRepo{state: state2}
	orch2 := NewOrchestrator(repo2, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)
	orch2.executeTask(ctx, "state-direct-2", "task-direct-2")

	log, err := git.Run(ctx, false, "log", "--oneline", integrationBranch)
	require.NoError(t, err)
	assert.Contains(t, log, "feat(core): implement minimal functionality for task task-direct-1")
	assert.Contains(t, log, "feat(core): implement minimal functionality for task task-direct-2")
}

func TestOrchestrator_MergeFailure_PreservesWorkerBranchAndCode(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	integrationBranch := "noctifab/implementation"

	reg := NewToolRegistry()
	reg.Register(&fileWritingMockTool{name: "read_file"})
	reg.Register(&fileWritingMockTool{name: "write_file"})
	reg.Register(&fileWritingMockTool{name: "run_tests"})

	llmClient := &alwaysGeneratingMockLLM{}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(repoDir)
	// Create a RebaseQueue that is deliberately NOT started to simulate a merge queue failure
	queue := NewRebaseQueue(git)

	evaluator := NewTestValidator(&mockSandbox{Out: "ok"}, false, llmClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{
		Concurrency:  1,
		UseWorktrees: true,
	}

	state := &domain.State{
		ID:          "state-merge-fail",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-fail-merge", Title: "Task Fail Merge", TargetFiles: []string{"fail.txt"}, Status: domain.TaskPending, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: integrationBranch,
			FeatureName:       "US-001",
		},
	}
	repo := &mockRepo{state: state}
	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)
	orch.executeTask(context.Background(), "state-merge-fail", "task-fail-merge")

	st, err := repo.Load(context.Background())
	require.NoError(t, err)

	// Task must NOT be marked TaskSuccess if the merge failed!
	assert.NotEqual(t, domain.TaskSuccess, st.Tasks[0].Status, "Task must not succeed if merge-back fails")
	assert.Contains(t, st.Tasks[0].FailureLog, "Failed to merge task branch into integration branch")

	// The worker branch must STILL EXIST in Git so code is never lost!
	branchName := "noctifab/task-task-fail-merge-worker"
	_, showErr := git.Run(context.Background(), false, "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
	assert.NoError(t, showErr, "Worker branch must be preserved in Git when merge fails")
}

func TestOrchestrator_TrollScenario_DeletedIntegrationBranch_SelfHeals(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	integrationBranch := "noctifab/implementation"

	reg := NewToolRegistry()
	reg.Register(&fileWritingMockTool{name: "read_file"})
	reg.Register(&fileWritingMockTool{name: "write_file"})
	reg.Register(&fileWritingMockTool{name: "run_tests"})

	llmClient := &alwaysGeneratingMockLLM{}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(repoDir)
	queue := NewRebaseQueue(git)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go queue.Start(ctx)

	evaluator := NewTestValidator(&mockSandbox{Out: "ok"}, false, llmClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{Concurrency: 1, UseWorktrees: true}

	// 1. Run Story 1
	state1 := &domain.State{
		ID:          "state-troll-1",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-troll-1", Title: "Task 1", TargetFiles: []string{"t1.txt"}, Status: domain.TaskPending, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{BaseBranch: "main", IntegrationBranch: integrationBranch, FeatureName: "US-001"},
	}
	repo1 := &mockRepo{state: state1}
	orch1 := NewOrchestrator(repo1, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)
	orch1.executeTask(ctx, "state-troll-1", "task-troll-1")
	st1, _ := repo1.Load(ctx)
	assert.Equal(t, domain.TaskSuccess, st1.Tasks[0].Status)

	// 2. User deletes integration branch externally to troll Noctifab!
	_, _ = git.Run(ctx, true, "checkout", "main")
	_, err := git.Run(ctx, true, "branch", "-D", integrationBranch)
	require.NoError(t, err, "Integration branch was deleted by user")

	// 3. Story 2 starts: Noctifab must self-heal, recreate integration branch, and complete Story 2 without crashing!
	state2 := &domain.State{
		ID:          "state-troll-2",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-troll-2", Title: "Task 2", TargetFiles: []string{"t2.txt"}, Status: domain.TaskPending, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{BaseBranch: "main", IntegrationBranch: integrationBranch, FeatureName: "US-002"},
	}
	repo2 := &mockRepo{state: state2}
	orch2 := NewOrchestrator(repo2, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)
	orch2.executeTask(ctx, "state-troll-2", "task-troll-2")

	st2, _ := repo2.Load(ctx)
	assert.Equal(t, domain.TaskSuccess, st2.Tasks[0].Status, "Task 2 should self-heal and succeed even if integration branch was deleted")

	// Check integration branch exists again
	_, showErr := git.Run(ctx, false, "show-ref", "--verify", "--quiet", "refs/heads/"+integrationBranch)
	assert.NoError(t, showErr, "Integration branch should be automatically recreated")
}

func TestOrchestrator_TrollScenario_DeletedWorkerBranchOnRetry_Recovers(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	integrationBranch := "noctifab/implementation"

	reg := NewToolRegistry()
	reg.Register(&fileWritingMockTool{name: "read_file"})
	reg.Register(&fileWritingMockTool{name: "write_file"})
	reg.Register(&fileWritingMockTool{name: "run_tests"})

	llmClient := &alwaysGeneratingMockLLM{}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(repoDir)
	queue := NewRebaseQueue(git)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go queue.Start(ctx)

	evaluator := NewTestValidator(&mockSandbox{Out: "ok"}, false, llmClient, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{Concurrency: 1, UseWorktrees: true}

	// Task is on Retry 2, but user externally deleted the worker branch!
	branchName := "noctifab/task-task-deleted-worker-worker"
	state := &domain.State{
		ID:          "state-deleted-worker",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-deleted-worker", Title: "Task Retry", TargetFiles: []string{"t.txt"}, Status: domain.TaskPending, Retries: 2, MaxRetries: 5},
		},
		Metadata: domain.StateMetadata{BaseBranch: "main", IntegrationBranch: integrationBranch, FeatureName: "US-001"},
	}
	repo := &mockRepo{state: state}
	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)

	// Make sure worker branch doesn't exist
	_, _ = git.Run(ctx, true, "branch", "-D", branchName)

	orch.executeTask(ctx, "state-deleted-worker", "task-deleted-worker")

	st, _ := repo.Load(ctx)
	assert.Equal(t, domain.TaskSuccess, st.Tasks[0].Status, "Task on retry should cleanly recreate missing worker branch and succeed")
}

func TestCleanConflictMarkers_DeterministicResolution(t *testing.T) {
	input := `func Example() string {
<<<<<<< HEAD
	return "base version"
=======
	return "worker version"
>>>>>>> branch
}`

	expected := `func Example() string {
	return "worker version"
}`

	resolved := CleanConflictMarkers(input)
	assert.Equal(t, expected, resolved, "CleanConflictMarkers must keep incoming changes deterministically without duplicating blocks")
}
