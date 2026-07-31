package services

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestrator_BreadthFirstExecutionMode(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	state := &domain.State{
		ID:          "state-breadth-first",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-bfg-1", Title: "Breadth First Task", TargetFiles: []string{"bfg.go"}, Status: domain.TaskPending, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: "noctifab/feature-state-breadth-first",
		},
	}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&mockTool{name: "read_file"})
	reg.Register(&mockTool{name: "write_file"})
	reg.Register(&mockTool{name: "run_tests"})

	llmClient := &testMockLLM{
		responses: []*domain.LLMResponse{
			{Actions: []domain.LLMAction{{Tool: "noop"}}}, // Generator action
			{Actions: []domain.LLMAction{{Tool: "noop"}}}, // Tester action
		},
	}

	validator := NewPolicyValidator([]string{"go", "git"}, "main", nil)
	sched := NewScheduler(NewFileLockRegistry())
	gitClient := NewGitClient(repoDir)
	rebaseQueue := NewRebaseQueue(gitClient)
	evaluator := NewTestValidator(&mockSandbox{Out: "PASS"}, false, llmClient, reg.Tools())

	cfg := OrchestratorConfig{
		Architecture: "breadth_first",
		Concurrency:  1,
		UseWorktrees: false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go rebaseQueue.Start(ctx)

	orch := NewOrchestrator(repo, reg, llmClient, validator, sched, gitClient, rebaseQueue, evaluator, nil, cfg, nil, nil)

	orch.executeTask(ctx, state.ID, "task-bfg-1")

	st, err := repo.Load(context.Background())
	require.NoError(t, err)

	for _, task := range st.Tasks {
		if task.ID == "task-bfg-1" {
			assert.Equal(t, domain.TaskSuccess, task.Status)
		}
	}
}
