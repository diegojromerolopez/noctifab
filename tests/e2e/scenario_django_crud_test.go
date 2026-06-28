package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenario_DjangoCRUD(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when daily budget is too low, pre-flight budget safeguarding halts run", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "budget", "django-crud-budget-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}

		// Initial requirements file
		workspace := filepath.Join(tempDir, "budget")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)
		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Django contact CRUD notebook spec"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "django-crud-budget-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:  "markdown",
				InputPath:    "requirements.md",
				FeatureName:  "Django Contact CRUD Notebook",
				TotalCostUSD: "0.0000",
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		// Low budget limit of $0.01 ($0.0375 is required for Planner)
		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 0.01)
		assert.ErrorIs(t, err, domain.ErrBudgetExhausted)

		// Assert no tasks were planned because budget was exceeded pre-flight
		finalState, err := repo.Load(ctx)
		require.NoError(t, err)
		assert.Len(t, finalState.Tasks, 0)
	})

	t.Run("when cyclic dependencies are returned, planning fails validation", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "cyclic", "django-crud-cyclic-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}

		workspace := filepath.Join(tempDir, "cyclic")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)
		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Django contact CRUD notebook spec"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "django-crud-cyclic-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:  "markdown",
				InputPath:    "requirements.md",
				FeatureName:  "cyclic", // Triggers cyclic tasks in mock LLM
				TotalCostUSD: "0.0000",
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 10.0)
		assert.ErrorContains(t, err, "cycle detected in task DAG")

		// Verify cycle validation failure action was logged in state
		finalState, err := repo.Load(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, finalState.LastActions)
		lastAction := finalState.LastActions[len(finalState.LastActions)-1]
		assert.Equal(t, "validate_dag", lastAction.Tool)
		assert.False(t, lastAction.Success)
		assert.Contains(t, lastAction.Result, "cycle detected")
	})

	t.Run("when requirement is to create a Django Contact CRUD Notebook", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "normal", "django-crud-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}

		workspace := filepath.Join(tempDir, "normal")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		// Create project requirements file
		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("# Django contact CRUD notebook spec\nMust implement user CRUD notebook."), 0644)
		require.NoError(t, err)

		// Create ignored folders to verify filesystem scanner exclusion filters
		err = os.MkdirAll(filepath.Join(workspace, "node_modules"), 0755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(workspace, "node_modules/package.json"), []byte("{}"), 0644)
		require.NoError(t, err)

		err = os.MkdirAll(filepath.Join(workspace, ".noctifab"), 0755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(workspace, ".noctifab/config.yaml"), []byte("config: details"), 0644)
		require.NoError(t, err)

		err = os.MkdirAll(filepath.Join(workspace, ".git"), 0755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(workspace, ".git/config"), []byte("[core]\nrepositoryformatversion = 0"), 0644)
		require.NoError(t, err)

		// Initial state
		state := &domain.State{
			ID:          "django-crud-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource:       "markdown",
				InputPath:         "requirements.md",
				IntegrationBranch: "feature/django-contact-crud",
				FeatureName:       "Django Contact CRUD Notebook",
				BaseBranch:        "main",
				ProjectVersion:    "0.1.0",
				TotalCostUSD:      "0.0000",
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		// 1. Run 1: Clarification block
		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 10.0)
		require.NoError(t, err)

		midState1, err := repo.Load(ctx)
		require.NoError(t, err)
		assert.Len(t, midState1.Clarifications, 1)
		assert.False(t, midState1.Clarifications[0].Resolved)
		assert.Len(t, midState1.Tasks, 0)

		// Verify scanner ignored node_modules, .noctifab, and .git folders
		for _, f := range midState1.Files {
			assert.NotContains(t, f.Path, "node_modules")
			assert.NotContains(t, f.Path, ".noctifab")
			assert.NotContains(t, f.Path, ".git")
		}

		// Operator resolves clarification
		midState1.Clarifications[0].Answer = "SQLite"
		midState1.Clarifications[0].Resolved = true
		err = repo.Save(ctx, midState1)
		require.NoError(t, err)

		// 2. Run 2: planning and execution to completion
		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 10.0)
		require.NoError(t, err)

		finalState, err := repo.Load(ctx)
		require.NoError(t, err)

		// 3. Verify OCC version save check
		assert.Equal(t, 18, finalState.Version, "Expected exactly 18 updates (final version 18)")

		// 4. Verify token and cost accumulation
		assert.Greater(t, finalState.Metadata.TotalTokensUsed, int64(4000))
		costVal, err := strconv.ParseFloat(finalState.Metadata.TotalCostUSD, 64)
		require.NoError(t, err)
		assert.InDelta(t, 0.0930, costVal, 0.0001)

		// 5. Verify Git branch sandboxing checkouts & commits logged
		gitCheckoutCount := 0
		gitCommitCount := 0
		createPRCount := 0
		flakyWarningCount := 0

		for _, act := range finalState.LastActions {
			switch act.Tool {
			case "git_checkout":
				gitCheckoutCount++
				assert.Contains(t, act.Args["branch"].(string), "noctifab/task-")
			case "git_commit":
				gitCommitCount++
			case "create_pr":
				createPRCount++
			case "validate_task_majority_vote":
				if strings.Contains(act.Result, "Warning: Potentially Flaky Build") {
					flakyWarningCount++
				}
			}
		}

		assert.Equal(t, 4, gitCheckoutCount, "Expected 4 sandboxed checkouts (1 for setup, 1 for model, 2 for views)")
		assert.Equal(t, 4, gitCommitCount, "Expected 4 commits (3 task succeeds + 1 version bump)")
		assert.Equal(t, 1, createPRCount, "Expected 1 pull request action")
		assert.Equal(t, 1, flakyWarningCount, "Expected 1 flaky build warning flagged in task-setup evaluation")

		// 6. Verify agents completed their work
		assert.Len(t, finalState.ActiveAgents, 3)
		assert.Equal(t, domain.AgentCompleted, finalState.ActiveAgents[0].Status) // Planner
		assert.Equal(t, domain.AgentIdle, finalState.ActiveAgents[1].Status)      // Generator
		assert.Equal(t, domain.AgentCompleted, finalState.ActiveAgents[2].Status) // Tester

		// 7. Verify all planned tasks succeeded
		assert.Len(t, finalState.Tasks, 3)
		for _, tTask := range finalState.Tasks {
			assert.Equal(t, domain.TaskSuccess, tTask.Status)
		}

		// 8. Verify validation criteria passed
		assert.Equal(t, domain.BuildPassing, finalState.BuildStatus)
		require.Len(t, finalState.ValidationCriteria, 1)
		assert.True(t, finalState.ValidationCriteria[0].Passed)

		// 9. Verify physical files exist
		versionContent, err := os.ReadFile(filepath.Join(workspace, "VERSION"))
		require.NoError(t, err)
		assert.Equal(t, "0.1.1", string(versionContent))

		changelogContent, err := os.ReadFile(filepath.Join(workspace, "CHANGELOG.md"))
		require.NoError(t, err)
		assert.Contains(t, string(changelogContent), "- Added Django contact CRUD notebook")
	})
}

func TestScenario_GitConflict(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when two agents edit the same lines of the same file, a git conflict is detected and the task is blocked", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "conflict", "django-crud-conflict-session")
		defer cleanup()

		client := &mockLLMClient{repo: repo}

		// Initial requirements file
		workspace := filepath.Join(tempDir, "conflict")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)
		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("Conflicting edits requirements spec"), 0644)
		require.NoError(t, err)

		// Set initial state
		initialState := &domain.State{
			ID:          "django-crud-conflict-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildPassing,
			Metadata: domain.StateMetadata{
				InputSource:  "local",
				InputPath:    "requirements.md",
				FeatureName:  "conflict-edits",
				BaseBranch:   "main",
				TotalCostUSD: "0.0000",
			},
		}
		err = repo.Save(ctx, initialState)
		require.NoError(t, err)

		// Run the simulated orchestrator loop
		// We expect the orchestrator to succeed because the Conflict Resolver agent resolves the conflict.
		err = runSimulatedOrchestrator(ctx, repo, client, workspace, 10.0)
		require.NoError(t, err)

		// Load final state and assert task statuses
		finalState, err := repo.Load(ctx)
		require.NoError(t, err)

		// Both tasks should have succeeded
		var task1, task2 *domain.Task
		for i := range finalState.Tasks {
			switch finalState.Tasks[i].ID {
			case "task-agent-1":
				task1 = &finalState.Tasks[i]
			case "task-agent-2":
				task2 = &finalState.Tasks[i]
			}
		}

		require.NotNil(t, task1, "task-agent-1 should exist")
		require.NotNil(t, task2, "task-agent-2 should exist")

		assert.Equal(t, domain.TaskSuccess, task1.Status, "Agent 1 changes should be successfully merged")
		assert.Equal(t, domain.TaskSuccess, task2.Status, "Agent 2 changes should be successfully resolved and completed")

		// Verify that a failed git_merge action was logged
		var gitMergeAction *domain.Action
		var resolveConflictAction *domain.Action
		for _, act := range finalState.LastActions {
			if act.Tool == "git_merge" {
				gitMergeAction = &act
			}
			if act.Tool == "resolve_conflict" {
				resolveConflictAction = &act
			}
		}
		require.NotNil(t, gitMergeAction, "git_merge action should be logged in history")
		assert.False(t, gitMergeAction.Success, "git_merge should have failed")
		assert.Contains(t, gitMergeAction.Result, "Merge Conflict")

		require.NotNil(t, resolveConflictAction, "resolve_conflict action should be logged in history")
		assert.True(t, resolveConflictAction.Success, "resolve_conflict should have succeeded")
		assert.Equal(t, "common.py", resolveConflictAction.Args["file"])

		// Verify resolver agent was spawned and completed
		var resolverAgent *domain.Agent
		for i := range finalState.ActiveAgents {
			if finalState.ActiveAgents[i].Role == domain.AgentRoleResolver {
				resolverAgent = &finalState.ActiveAgents[i]
			}
		}
		require.NotNil(t, resolverAgent, "resolver agent should be spawned")
		assert.Equal(t, "Conflict Resolver", resolverAgent.Name)
		assert.Equal(t, domain.AgentCompleted, resolverAgent.Status)

		// Verify workspace final file contents
		commonPath := filepath.Join(workspace, "common.py")
		content, err := os.ReadFile(commonPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "line 1: content from agent 1 and agent 2 combined")

		// Verify validation passed and build status is passing
		assert.Equal(t, domain.BuildPassing, finalState.BuildStatus)
		require.Len(t, finalState.ValidationCriteria, 1)
		assert.True(t, finalState.ValidationCriteria[0].Passed)
	})
}
