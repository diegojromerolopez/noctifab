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

func TestScenario_FlakyQuarantine(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when a task encounters a flaky build, the quarantine majority vote logs warnings but succeeds", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "flaky", "django-flaky-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}
		workspace := filepath.Join(tempDir, "flaky")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Django CRUD spec"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "django-flaky-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:  "markdown",
				InputPath:    "requirements.md",
				FeatureName:  "Flaky Feature",
				TotalCostUSD: "0.0000",
			},
			Tasks: []domain.Task{
				{
					ID:          "task-setup",
					Title:       "Setup Django project",
					Description: "Initialize base files",
					Status:      domain.TaskPending,
					ChangeType:  domain.ChangeTypeFeature,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		// Set client mock callback to simulate generation for task-setup
		client.completeFn = func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{
				Reasoning: "Generating setup files",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    "manage.py",
							"content": "#!/usr/bin/env python\n",
						},
					},
				},
			}, nil
		}

		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 100000)
		require.NoError(t, err)

		finalState, err := repo.Load(ctx)
		require.NoError(t, err)

		// Verify task succeeded
		require.Len(t, finalState.Tasks, 1)
		assert.Equal(t, domain.TaskSuccess, finalState.Tasks[0].Status)

		// Assert quarantine warning actions exist in history
		var run1Action, voteAction *domain.Action
		for _, act := range finalState.LastActions {
			if act.Tool == "validate_task_run_1" {
				run1Action = &act
			}
			if act.Tool == "validate_task_majority_vote" {
				voteAction = &act
			}
		}
		require.NotNil(t, run1Action)
		assert.False(t, run1Action.Success)
		assert.Equal(t, "flaky mock error on run 1", run1Action.Result)

		require.NotNil(t, voteAction)
		assert.True(t, voteAction.Success)
		assert.Contains(t, voteAction.Result, "Warning: Potentially Flaky Build")
	})
}

func TestScenario_BudgetExceededMidExecution(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when cost accumulates mid-execution, budget limit halts run gracefully", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "budget-mid", "budget-mid-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}
		workspace := filepath.Join(tempDir, "budget-mid")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Budget spec"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "budget-mid-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:  "markdown",
				InputPath:    "requirements.md",
				FeatureName:  "Budget Feature",
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
				{
					ID:         "task-2",
					Title:      "Task 2",
					Status:     domain.TaskPending,
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
				Reasoning: "Executing task",
				Actions:   nil, // no actions
			}, nil
		}

		// Low token limit of 1200.
		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 1200)
		assert.ErrorIs(t, err, domain.ErrBudgetExhausted)

		finalState, err := repo.Load(ctx)
		require.NoError(t, err)

		// Assert task 1 succeeded, but task 2 remained pending
		var task1, task2 *domain.Task
		for i := range finalState.Tasks {
			if finalState.Tasks[i].ID == "task-1" {
				task1 = &finalState.Tasks[i]
			}
			if finalState.Tasks[i].ID == "task-2" {
				task2 = &finalState.Tasks[i]
			}
		}
		require.NotNil(t, task1)
		require.NotNil(t, task2)

		assert.Equal(t, domain.TaskSuccess, task1.Status)
		assert.Equal(t, domain.TaskPending, task2.Status)

		// Verify total tokens registered is 800
		assert.Equal(t, int64(800), finalState.Metadata.TotalTokensUsed)
	})
}
