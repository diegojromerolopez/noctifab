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

func TestScenario_ContextCompaction(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when history reaches threshold, the orchestrator triggers conversation compaction", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "compaction", "compaction-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}
		workspace := filepath.Join(tempDir, "compaction")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Compaction spec"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "compaction-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:  "markdown",
				InputPath:    "requirements.md",
				FeatureName:  "compaction feature",
				TotalCostUSD: "0.0000",
			},
			Tasks: []domain.Task{
				{
					ID:          "task-1",
					Title:       "Task 1",
					Status:      domain.TaskPending,
					ChangeType:  domain.ChangeTypeFeature,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
			},
			LastActions: []domain.Action{
				{
					Timestamp: time.Now(),
					Tool:      "init",
					Success:   true,
				},
				{
					Timestamp: time.Now(),
					Tool:      "plan",
					Success:   true,
				},
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		client.completeFn = func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{
				Reasoning: "No actions needed",
			}, nil
		}

		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 10.0)
		require.NoError(t, err)

		finalState, err := repo.Load(ctx)
		require.NoError(t, err)

		// Assert task succeeded and compaction action is logged
		assert.Equal(t, domain.TaskSuccess, finalState.Tasks[0].Status)

		var compactAction *domain.Action
		for _, act := range finalState.LastActions {
			if act.Tool == "compact_history" {
				compactAction = &act
				break
			}
		}
		require.NotNil(t, compactAction)
		assert.True(t, compactAction.Success)
		assert.Contains(t, compactAction.Result, "Compacted history successfully")
	})
}

func TestScenario_SandboxViolation(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when a write action attempts to write outside the workspace jail, the orchestrator blocks it", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "sandbox-violation", "sandbox-violation-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}
		workspace := filepath.Join(tempDir, "sandbox-violation")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Sandbox spec"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "sandbox-violation-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:  "markdown",
				InputPath:    "requirements.md",
				FeatureName:  "Sandbox feature",
				TotalCostUSD: "0.0000",
			},
			Tasks: []domain.Task{
				{
					ID:          "task-1",
					Title:       "Task 1",
					Status:      domain.TaskPending,
					ChangeType:  domain.ChangeTypeFeature,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		// Client returns an action attempting to write to /etc/hosts (outside workspace)
		client.completeFn = func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			return &domain.LLMResponse{
				Reasoning: "Malicious writing attempt",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    "/etc/hosts",
							"content": "127.0.0.1 malicious-site.com\n",
						},
					},
				},
			}, nil
		}

		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 10.0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Sandbox violation: path resolves outside workspace")

		finalState, err := repo.Load(ctx)
		require.NoError(t, err)

		// Assert task status is FAILED
		assert.Equal(t, domain.TaskFailed, finalState.Tasks[0].Status)

		// Assert failed write_file action due to sandbox violation is recorded
		var sandboxFailAction *domain.Action
		for _, act := range finalState.LastActions {
			if act.Tool == "write_file" && !act.Success {
				sandboxFailAction = &act
				break
			}
		}
		require.NotNil(t, sandboxFailAction)
		assert.Contains(t, sandboxFailAction.Result, "Sandbox violation")
		assert.Contains(t, sandboxFailAction.Result, "resolves outside the workspace boundary")
	})
}
