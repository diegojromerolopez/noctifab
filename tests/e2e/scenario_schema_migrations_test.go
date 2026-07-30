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

func TestScenario_SchemaMigration(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when a task adds a migration, it executes validation successfully", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "migration", "migration-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}
		workspace := filepath.Join(tempDir, "migration")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Migration spec"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "migration-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:  "markdown",
				InputPath:    "requirements.md",
				FeatureName:  "migration feature",
				TotalCostUSD: "0.0000",
			},
			Tasks: []domain.Task{
				{
					ID:         "task-migration",
					Title:      "Task Migration",
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
				Reasoning: "Creating migration files",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    "migrations/0001_add_age.sql",
							"content": "ALTER TABLE users ADD COLUMN age INT;\n",
						},
					},
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    "models.py",
							"content": "class User:\n    age = int\n",
						},
					},
				},
			}, nil
		}

		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 100000)
		require.NoError(t, err)

		finalState, err := repo.Load(ctx)
		require.NoError(t, err)

		// Assert task succeeded and required files exist
		assert.Equal(t, domain.TaskSuccess, finalState.Tasks[0].Status)
		assert.Equal(t, domain.BuildPassing, finalState.BuildStatus)

		_, err = os.Stat(filepath.Join(workspace, "migrations/0001_add_age.sql"))
		require.NoError(t, err)
	})
}

func TestScenario_ShutdownResumption(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when context is cancelled, the orchestrator shuts down gracefully and resumes on restart", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "shutdown", "shutdown-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}
		workspace := filepath.Join(tempDir, "shutdown")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Shutdown spec"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "shutdown-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:  "markdown",
				InputPath:    "requirements.md",
				FeatureName:  "shutdown feature",
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

		// Create a cancelled context to trigger immediate shutdown
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // cancel immediately

		client.completeFn = func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{
				Reasoning: "Fails if called",
			}, nil
		}

		err = runSimulatedOrchestrator(cancelCtx, repo, client, workspace, 100000)
		assert.ErrorIs(t, err, context.Canceled)

		midState, err := repo.Load(ctx)
		require.NoError(t, err)

		// Verify graceful shutdown action was logged
		var shutdownAction *domain.Action
		for _, act := range midState.LastActions {
			if act.Tool == "graceful_shutdown" {
				shutdownAction = &act
				break
			}
		}
		require.NotNil(t, shutdownAction)
		assert.True(t, shutdownAction.Success)

		// Now resume execution with active context
		client.completeFn = func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{
				Reasoning: "Resuming and writing main.go",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    "main.go",
							"content": "package main\n",
						},
					},
				},
			}, nil
		}

		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 100000)
		require.NoError(t, err)

		finalState, err := repo.Load(ctx)
		require.NoError(t, err)

		// Assert task completed successfully on resumption
		assert.Equal(t, domain.TaskSuccess, finalState.Tasks[0].Status)
		assert.Equal(t, domain.BuildPassing, finalState.BuildStatus)
	})
}
