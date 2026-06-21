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

func TestScenario_ParallelConflictResolution(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when concurrent agents produce conflicting changes, the resolver agent integrates them", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "parallel-conflict", "parallel-conflict-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}
		workspace := filepath.Join(tempDir, "parallel-conflict")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Conflict spec"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "parallel-conflict-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:  "markdown",
				InputPath:    "requirements.md",
				FeatureName:  "conflict feature",
				TotalCostUSD: "0.0000",
			},
			Tasks: []domain.Task{
				{
					ID:         "task-agent-1",
					Title:      "Agent 1 Edit",
					Status:     domain.TaskPending,
					ChangeType: domain.ChangeTypeFeature,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				},
				{
					ID:         "task-agent-2",
					Title:      "Agent 2 Edit",
					Status:     domain.TaskPending,
					ChangeType: domain.ChangeTypeFeature,
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
				},
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		// Set custom client callback to handle first commits and resolver call
		client.completeFn = func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			if prompt == "resolve git conflict on common.py" {
				return &domain.LLMResponse{
					Reasoning: "Integrating parallel edits",
					Actions: []domain.LLMAction{
						{
							Tool: "write_file",
							Args: map[string]any{
								"path":    "common.py",
								"content": "line 1: content from agent 1 and agent 2 combined\n",
							},
						},
					},
				}, nil
			}

			if prompt == "execute task task-agent-1" {
				return &domain.LLMResponse{
					Reasoning: "Writing agent 1 edits",
					Actions: []domain.LLMAction{
						{
							Tool: "write_file",
							Args: map[string]any{
								"path":    "common.py",
								"content": "line 1: content from agent 1\n",
							},
						},
					},
				}, nil
			}

			if prompt == "execute task task-agent-2" {
				return &domain.LLMResponse{
					Reasoning: "Writing agent 2 edits",
					Actions: []domain.LLMAction{
						{
							Tool: "write_file",
							Args: map[string]any{
								"path":    "common.py",
								"content": "line 1: conflicting content from agent 2\n",
							},
						},
					},
				}, nil
			}

			return &domain.LLMResponse{}, nil
		}

		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 10.0)
		require.NoError(t, err)

		finalState, err := repo.Load(ctx)
		require.NoError(t, err)

		// Assert both tasks completed successfully
		for _, task := range finalState.Tasks {
			assert.Equal(t, domain.TaskSuccess, task.Status)
		}

		// Verify workspace contains integrated changes
		commonPath := filepath.Join(workspace, "common.py")
		content, err := os.ReadFile(commonPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "line 1: content from agent 1 and agent 2 combined")
	})
}

func TestScenario_NonDestructiveRefactor(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when refactoring, the agent performs edits without destroying existing code", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "refactor", "non-destructive-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}
		workspace := filepath.Join(tempDir, "refactor")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		// Create pre-existing main.go file
		mainPath := filepath.Join(workspace, "main.go")
		originalCode := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello, World!\")\n}\n"
		err = os.WriteFile(mainPath, []byte(originalCode), 0644)
		require.NoError(t, err)

		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Refactor spec"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "non-destructive-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:  "markdown",
				InputPath:    "requirements.md",
				FeatureName:  "Refactor Feature",
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

		client.completeFn = func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
			// Append a greet function without altering original main
			updatedCode := originalCode + "\nfunc greet(name string) {\n\tfmt.Printf(\"Hello, %s!\\n\", name)\n}\n"
			return &domain.LLMResponse{
				Reasoning: "Adding greet function to main.go",
				Actions: []domain.LLMAction{
					{
						Tool: "write_file",
						Args: map[string]any{
							"path":    "main.go",
							"content": updatedCode,
						},
					},
				},
			}, nil
		}

		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 10.0)
		require.NoError(t, err)

		// Verify changes were non-destructive
		finalCode, err := os.ReadFile(mainPath)
		require.NoError(t, err)
		codeStr := string(finalCode)

		assert.Contains(t, codeStr, `fmt.Println("Hello, World!")`)
		assert.Contains(t, codeStr, `func greet(name string)`)
		assert.Contains(t, codeStr, `package main`)
	})
}
