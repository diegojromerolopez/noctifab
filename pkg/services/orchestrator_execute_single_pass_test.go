package services

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrchestrator_SinglePassExecutionMode(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	state := &domain.State{
		ID:          "state-single-pass",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-sp-1", Title: "Single Pass Task", TargetFiles: []string{"sp.go"}, Status: domain.TaskPending, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: "noctifab/feature-state-single-pass",
		},
	}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&mockTool{name: "read_file"})
	reg.Register(&mockTool{name: "write_file"})
	reg.Register(&mockTool{name: "run_tests"})

	llmClient := &testMockLLM{
		responses: []*domain.LLMResponse{
			{Actions: []domain.LLMAction{{Tool: "noop"}}}, // Single Generator action in single_pass_execution
		},
	}

	validator := NewPolicyValidator([]string{"go", "git"}, "main", nil)
	sched := NewScheduler(NewFileLockRegistry())
	gitClient := NewGitClient(repoDir)
	rebaseQueue := NewRebaseQueue(gitClient)
	evaluator := NewTestValidator(&mockSandbox{Out: "PASS"}, false, llmClient, reg.Tools())

	cfg := OrchestratorConfig{
		Architecture: "single_pass_execution",
		Concurrency:  1,
		UseWorktrees: false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go rebaseQueue.Start(ctx)

	orch := NewOrchestrator(repo, reg, llmClient, validator, sched, gitClient, rebaseQueue, evaluator, nil, cfg, nil, nil, nil)

	orch.executeTask(ctx, state.ID, "task-sp-1")

	st, err := repo.Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, domain.TaskSuccess, st.Tasks[0].Status)
	// Single pass should invoke Generator only 1 time on initial run (unlike default 3 times)
	assert.Equal(t, 1, llmClient.callCount)
}

func TestOrchestrator_SinglePassCoSynthesisMode(t *testing.T) {
	for _, archMode := range []string{"single_pass_co_synthesis", "co_synthesis", "spcs"} {
		t.Run("when architecture is "+archMode+", it completes successfully in unified single pass", func(t *testing.T) {
			repoDir, _, cleanup := setupTestGitRepo(t)
			defer cleanup()

			state := &domain.State{
				ID:          "state-co-synth-" + archMode,
				ProjectPath: repoDir,
				Tasks: []domain.Task{
					{ID: "task-cs-1", Title: "Co-Synthesis Task", TargetFiles: []string{"main.go"}, Status: domain.TaskPending, MaxRetries: 3},
				},
				Metadata: domain.StateMetadata{
					BaseBranch:        "main",
					IntegrationBranch: "noctifab/feature-state-co-synth-" + archMode,
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
				},
			}

			validator := NewPolicyValidator([]string{"go", "git"}, "main", nil)
			sched := NewScheduler(NewFileLockRegistry())
			gitClient := NewGitClient(repoDir)
			rebaseQueue := NewRebaseQueue(gitClient)
			evaluator := NewTestValidator(&mockSandbox{Out: "PASS"}, false, llmClient, reg.Tools())

			cfg := OrchestratorConfig{
				Architecture: archMode,
				Concurrency:  1,
				UseWorktrees: false,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go rebaseQueue.Start(ctx)

			orch := NewOrchestrator(repo, reg, llmClient, validator, sched, gitClient, rebaseQueue, evaluator, nil, cfg, nil, nil, nil)
			orch.executeTask(ctx, state.ID, "task-cs-1")

			st, err := repo.Load(context.Background())
			require.NoError(t, err)
			assert.Equal(t, domain.TaskSuccess, st.Tasks[0].Status)
			assert.Equal(t, 1, llmClient.callCount)
		})
	}
}
