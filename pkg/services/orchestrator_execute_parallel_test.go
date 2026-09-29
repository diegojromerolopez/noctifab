package services

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type parallelMockLLM struct {
	calls int32
}

func (m *parallelMockLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	atomic.AddInt32(&m.calls, 1)
	role, _ := ctx.Value(AgentRoleKey).(string)
	if role == "tester" {
		return &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "write_file", Args: map[string]any{"path": "sample_test.go", "content": "package sample\nimport \"testing\"\nfunc TestSample(t *testing.T) {}\n"}},
			},
		}, nil
	}
	return &domain.LLMResponse{
		Actions: []domain.LLMAction{
			{Tool: "write_file", Args: map[string]any{"path": "sample.go", "content": "package sample\nfunc Foo() int { return 42 }\n"}},
		},
	}, nil
}

func TestOrchestrator_ParallelCoSynthesisMode(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	state := &domain.State{
		ID:          "state-parallel-co-synth",
		ProjectPath: repoDir,
		Tasks: []domain.Task{
			{ID: "task-pcs-1", Title: "Parallel Co-Synthesis Task", TargetFiles: []string{"sample.go"}, Status: domain.TaskPending, MaxRetries: 3},
		},
		Metadata: domain.StateMetadata{
			BaseBranch:        "main",
			IntegrationBranch: "noctifab/feature-state-parallel-co-synth",
		},
	}

	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	reg.Register(&mockTool{name: "read_file"})
	reg.Register(&mockTool{name: "write_file"})
	reg.Register(&mockTool{name: "run_tests"})

	llmClient := &parallelMockLLM{}

	validator := NewPolicyValidator([]string{"go", "git"}, "main", nil)
	sched := NewScheduler(NewFileLockRegistry())
	gitClient := NewGitClient(repoDir)
	rebaseQueue := NewRebaseQueue(gitClient)
	evaluator := NewTestValidator(&mockSandbox{Out: "PASS"}, false, nil, reg.Tools())

	cfg := OrchestratorConfig{
		Architecture:       "code_first",
		TaskExecutionOrder: "parallel",
		Concurrency:        1,
		UseWorktrees:       false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go rebaseQueue.Start(ctx)

	orch := NewOrchestrator(repo, reg, nil, validator, sched, gitClient, rebaseQueue, evaluator, nil, cfg, nil, nil, nil)
	orch.llmClient = llmClient

	orch.executeTask(ctx, state.ID, "task-pcs-1")

	st, err := repo.Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, domain.TaskSuccess, st.Tasks[0].Status)
	// Both tester and generator agents were invoked in parallel during Turn 1
	assert.GreaterOrEqual(t, atomic.LoadInt32(&llmClient.calls), int32(2))
}
