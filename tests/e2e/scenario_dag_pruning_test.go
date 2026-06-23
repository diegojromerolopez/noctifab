package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenario_UpstreamFailurePruning(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when an upstream task fails, downstream dependent tasks are automatically pruned", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "pruning", "upstream-prune-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}
		workspace := filepath.Join(tempDir, "pruning")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Pruning spec"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "upstream-prune-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:  "markdown",
				InputPath:    "requirements.md",
				FeatureName:  "pruning feature",
				TotalCostUSD: "0.0000",
			},
			Tasks: []domain.Task{
				{
					ID:         "task-a",
					Title:      "Task A",
					Status:     domain.TaskPending,
					ChangeType: domain.ChangeTypeFeature,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				},
				{
					ID:         "task-b",
					Title:      "Task B",
					Status:     domain.TaskPending,
					DependsOn:  []string{"task-a"},
					ChangeType: domain.ChangeTypeFeature,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				},
				{
					ID:         "task-c",
					Title:      "Task C",
					Status:     domain.TaskPending,
					DependsOn:  []string{"task-b"},
					ChangeType: domain.ChangeTypeFeature,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				},
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		client.completeFn = func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{
				Reasoning: "Failing execution",
				Actions:   nil,
			}, nil
		}

		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 10.0)
		require.NoError(t, err)

		finalState, err := repo.Load(ctx)
		require.NoError(t, err)

		// Assert task-a is FAILED and task-b/task-c are pruned to CONFLICT_FAILED
		var taskA, taskB, taskC *domain.Task
		for i := range finalState.Tasks {
			switch finalState.Tasks[i].ID {
			case "task-a":
				taskA = &finalState.Tasks[i]
			case "task-b":
				taskB = &finalState.Tasks[i]
			case "task-c":
				taskC = &finalState.Tasks[i]
			}
		}
		require.NotNil(t, taskA)
		require.NotNil(t, taskB)
		require.NotNil(t, taskC)

		assert.Equal(t, domain.TaskFailed, taskA.Status)
		assert.Equal(t, domain.TaskConflictFailed, taskB.Status)
		assert.Equal(t, domain.TaskConflictFailed, taskC.Status)
	})
}

func TestScenario_RollbackOnBuildBreakage(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when a compilation build error occurs, the orchestrator rolls back and retries", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "rollback", "rollback-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}
		workspace := filepath.Join(tempDir, "rollback")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Rollback spec"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "rollback-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:  "markdown",
				InputPath:    "requirements.md",
				FeatureName:  "rollback feature",
				TotalCostUSD: "0.0000",
			},
			Tasks: []domain.Task{
				{
					ID:         "task-1",
					Title:      "Task 1",
					Status:     domain.TaskPending,
					ChangeType: domain.ChangeTypeFeature,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				},
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		// On retry (Retries > 0), the Generator succeeds
		client.completeFn = func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{
				Reasoning: "Generating working code",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    "main.go",
							"content": "package main\n\nfunc main() {}\n",
						},
					},
				},
			}, nil
		}

		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 10.0)
		require.NoError(t, err)

		finalState, err := repo.Load(ctx)
		require.NoError(t, err)

		// Verify task-1 succeeded on retry
		require.Len(t, finalState.Tasks, 1)
		assert.Equal(t, domain.TaskSuccess, finalState.Tasks[0].Status)
		assert.Equal(t, 1, finalState.Tasks[0].Retries)

		// Verify that a failed validate_task and a git_reset_hard rollback action were logged
		var validateFailAction, resetAction *domain.Action
		for _, act := range finalState.LastActions {
			if act.Tool == "validate_task" && !act.Success {
				validateFailAction = &act
			}
			if act.Tool == "git_reset_hard" {
				resetAction = &act
			}
		}
		require.NotNil(t, validateFailAction)
		assert.Equal(t, "Build breakage: syntax error in main.go", validateFailAction.Result)

		require.NotNil(t, resetAction)
		assert.True(t, resetAction.Success)
		assert.Equal(t, "HEAD", resetAction.Args["commit"])
	})
}
