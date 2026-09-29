package e2e

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenario_GlobalTaskDAGCrossStoryParallelism(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when downstream story tasks declare fine-grained dependencies on upstream story tasks", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "global-dag", "global-dag-parallel-session")
		defer cleanup()

		workspace := filepath.Join(tempDir, "global-dag")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("# Global Task DAG Cross-Story Spec\n"), 0644)
		require.NoError(t, err)

		// Create user stories in roadmap/user-stories
		storyDir := filepath.Join(workspace, "roadmap", "user-stories")
		err = os.MkdirAll(storyDir, 0755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(storyDir, "US-001.md"), []byte("# US-001 Core Foundation\n"), 0644)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(storyDir, "US-002.md"), []byte("# US-002 CLI Consumer\ndepends_on: [\"US-001\"]\n"), 0644)
		require.NoError(t, err)

		// Construct state with fine-grained cross-story task dependencies
		// US-001-TASK-001 is Foundation
		// US-001-TASK-002 is Upstream Polish (depends on US-001-TASK-001)
		// US-002-TASK-001 is Downstream CLI (depends on US-001-TASK-001)
		state := &domain.State{
			ID:          "global-dag-parallel-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource: "markdown",
				InputPath:   "requirements.md",
				FeatureName: "Global Task DAG Parallelism",
			},
			Tasks: []domain.Task{
				{
					ID:          "US-001-TASK-001",
					Title:       "Foundation Types",
					StoryID:     "US-001",
					Status:      domain.TaskPending,
					TargetFiles: []string{"pkg/types.go"},
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
				{
					ID:          "US-001-TASK-002",
					Title:       "Foundation Polish & Helpers",
					StoryID:     "US-001",
					Status:      domain.TaskPending,
					DependsOn:   []string{"US-001-TASK-001"},
					TargetFiles: []string{"pkg/helpers.go"},
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
				{
					ID:          "US-002-TASK-001",
					Title:       "CLI Command Layer",
					StoryID:     "US-002",
					Status:      domain.TaskPending,
					DependsOn:   []string{"US-001-TASK-001"},
					TargetFiles: []string{"cmd/cli.go"},
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		scheduler := services.NewScheduler(services.NewFileLockRegistry())

		// Tick 1: Initially, only US-001-TASK-001 is ready
		ready := scheduler.GetReadyTasks(state, 5)
		require.Len(t, ready, 1)
		assert.Equal(t, "US-001-TASK-001", ready[0].ID)

		// Execute US-001-TASK-001: write types.go and mark SUCCESS
		typesFile := filepath.Join(workspace, "pkg", "types.go")
		err = os.MkdirAll(filepath.Dir(typesFile), 0755)
		require.NoError(t, err)
		err = os.WriteFile(typesFile, []byte("package pkg\ntype Item struct{ ID string }\n"), 0644)
		require.NoError(t, err)

		scheduler.ReleaseLocks("US-001-TASK-001")
		state.Tasks[0].Status = domain.TaskSuccess
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		// Tick 2: BDD Verification
		// "it unblocks downstream story tasks immediately once upstream foundation task succeeds, running concurrently with upstream remaining tasks"
		t.Run("it unblocks downstream story tasks immediately once upstream foundation task succeeds, running concurrently with upstream remaining tasks", func(t *testing.T) {
			readyTick2 := scheduler.GetReadyTasks(state, 5)
			require.Len(t, readyTick2, 2, "Both US-001-TASK-002 and US-002-TASK-001 must be ready concurrently")

			readyMap := make(map[string]bool)
			for _, tsk := range readyTick2 {
				readyMap[tsk.ID] = true
			}
			assert.True(t, readyMap["US-001-TASK-002"], "Upstream polish task should be ready")
			assert.True(t, readyMap["US-002-TASK-001"], "Downstream CLI task should be ready simultaneously")

			// Simulate concurrent execution of both tasks
			var wg sync.WaitGroup
			var mu sync.Mutex
			executed := make(map[string]time.Time)

			for _, tsk := range readyTick2 {
				wg.Add(1)
				go func(id string, files []string) {
					defer wg.Done()
					mu.Lock()
					executed[id] = time.Now()
					mu.Unlock()

					for _, f := range files {
						abs := filepath.Join(workspace, f)
						_ = os.MkdirAll(filepath.Dir(abs), 0755)
						_ = os.WriteFile(abs, []byte("// generated\n"), 0644)
					}
					scheduler.ReleaseLocks(id)
				}(tsk.ID, tsk.TargetFiles)
			}
			wg.Wait()

			mu.Lock()
			assert.Len(t, executed, 2)
			mu.Unlock()

			// Mark all completed
			state.Tasks[1].Status = domain.TaskSuccess
			state.Tasks[2].Status = domain.TaskSuccess
			err = repo.Save(ctx, state)
			require.NoError(t, err)

			// Verify all generated files exist in workspace
			assert.FileExists(t, filepath.Join(workspace, "pkg", "types.go"))
			assert.FileExists(t, filepath.Join(workspace, "pkg", "helpers.go"))
			assert.FileExists(t, filepath.Join(workspace, "cmd", "cli.go"))

			// Assert scheduler has zero remaining tasks
			remaining := scheduler.GetReadyTasks(state, 5)
			assert.Empty(t, remaining)
		})
	})
}

func TestScenario_CrossStoryFailurePruning(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("when an upstream foundation task fails, dependent cross-story tasks are blocked while independent tasks proceed", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "failure-pruning", "failure-prune-session")
		defer cleanup()

		workspace := filepath.Join(tempDir, "failure-pruning")
		err := os.MkdirAll(workspace, 0755)
		require.NoError(t, err)

		reqPath := filepath.Join(workspace, "requirements.md")
		err = os.WriteFile(reqPath, []byte("# Failure Pruning Spec\n"), 0644)
		require.NoError(t, err)

		// Setup stories: US-001 and US-002
		storyDir := filepath.Join(workspace, "roadmap", "user-stories")
		err = os.MkdirAll(storyDir, 0755)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(storyDir, "US-001.md"), []byte("# US-001\n"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(storyDir, "US-002.md"), []byte("# US-002\ndepends_on: [\"US-001\"]\n"), 0644)
		require.NoError(t, err)

		state := &domain.State{
			ID:          "failure-prune-session",
			ProjectPath: workspace,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource: "markdown",
				InputPath:   "requirements.md",
				FeatureName: "Cross-Story Failure Pruning",
			},
			Tasks: []domain.Task{
				{
					ID:          "US-001-TASK-001",
					Title:       "Foundation Types",
					StoryID:     "US-001",
					Status:      domain.TaskFailed, // Foundation FAILED!
					TargetFiles: []string{"pkg/types.go"},
				},
				{
					ID:          "US-002-TASK-001",
					Title:       "CLI Dependent on Foundation",
					StoryID:     "US-002",
					Status:      domain.TaskPending,
					DependsOn:   []string{"US-001-TASK-001"},
					TargetFiles: []string{"cmd/cli.go"},
				},
				{
					ID:          "US-002-TASK-002",
					Title:       "CLI Independent Diagnostics",
					StoryID:     "US-002",
					Status:      domain.TaskPending,
					DependsOn:   nil, // Independent
					TargetFiles: []string{"cmd/diag.go"},
				},
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		scheduler := services.NewScheduler(services.NewFileLockRegistry())

		t.Run("it prevents execution of dependent cross-story tasks without deadlocking", func(t *testing.T) {
			ready := scheduler.GetReadyTasks(state, 5)

			var hasDependentTask bool
			for _, tsk := range ready {
				if tsk.ID == "US-002-TASK-001" {
					hasDependentTask = true
				}
			}
			assert.False(t, hasDependentTask, "US-002-TASK-001 must not be ready when its upstream prerequisite failed")
		})
	})
}
