package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMaintenanceTestRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "noctifab-sqlite-maint-test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	repo, err := NewSQLiteRepository(context.Background(), filepath.Join(tmpDir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

// newMaintenanceState builds a state with one task per relation group so
// relation-row cascade deletion is observable.
func newMaintenanceState(id string, status domain.StoryStatus, updatedAt time.Time) *domain.State {
	return &domain.State{
		ID:          id,
		ProjectPath: "/workspace",
		BuildStatus: domain.BuildPassing,
		StoryStatus: status,
		Tasks: []domain.Task{
			{ID: id + "-t1", Title: "T", Description: "d", Status: domain.TaskSuccess, ChangeType: domain.ChangeTypeFix, DependsOn: []string{}, CreatedAt: updatedAt.Add(-time.Hour), UpdatedAt: updatedAt},
		},
		Clarifications:     []domain.Clarification{{Question: "Q?", AskedAt: updatedAt}},
		LastActions:        []domain.Action{{Timestamp: updatedAt, Tool: "write", Args: map[string]any{}, Reasoning: "r", Result: "ok", Success: true}},
		Files:              []domain.FileInfo{{Path: id + ".go", Size: 1, LastModified: updatedAt}},
		ValidationCriteria: []domain.ValidationCriterion{{ID: id + "-c1", Type: domain.ValidationCommand, Expression: "e", Description: "d"}},
		ActiveAgents:       []domain.Agent{{ID: id + "-a1", Name: "n", Role: domain.AgentRoleGenerator, Status: domain.AgentCompleted}},
	}
}

func countRows(t *testing.T, repo *SQLiteRepository, table, stateID string) int {
	t.Helper()
	var n int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE state_id = ?", table)
	require.NoError(t, repo.DB().QueryRow(query, stateID).Scan(&n))
	return n
}

func TestSQLiteRepositoryPruneFinishedStates(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 8, 4, 8, 0, 0, 0, time.UTC)

	seed := func(t *testing.T, repo *SQLiteRepository) (finished []string, running string) {
		t.Helper()
		// 5 finished states (oldest to newest by task updated_at) + 1 running.
		for i := 0; i < 5; i++ {
			status := domain.StorySuccess
			if i%2 == 1 {
				status = domain.StoryFailed
			}
			id := fmt.Sprintf("state-fin-%d", i)
			state := newMaintenanceState(id, status, base.Add(time.Duration(i)*time.Hour))
			require.NoError(t, repo.Save(ctx, state))
			finished = append(finished, id)
		}
		running = "state-running"
		require.NoError(t, repo.Save(ctx, newMaintenanceState(running, domain.StoryRunning, base.Add(10*time.Hour))))
		return finished, running
	}

	t.Run("when pruning with keepLast=2, it deletes the 3 oldest finished states and their relation rows", func(t *testing.T) {
		repo := newMaintenanceTestRepo(t)
		finished, running := seed(t, repo)

		pruned, err := repo.PruneFinishedStates(ctx, 2)
		require.NoError(t, err)
		assert.Equal(t, 3, pruned)

		var stateCount int
		require.NoError(t, repo.DB().QueryRow("SELECT COUNT(*) FROM state").Scan(&stateCount))
		assert.Equal(t, 3, stateCount, "expected 2 finished survivors + 1 running")

		// The 3 oldest finished states are gone, relation rows included.
		for _, id := range finished[:3] {
			for _, table := range stateRelationTables {
				assert.Equal(t, 0, countRows(t, repo, table, id), "expected %s rows of %s to be deleted", table, id)
			}
			_, err := repo.LoadByID(ctx, id)
			assert.Error(t, err, "expected pruned state %s to be gone", id)
		}

		// Survivors (2 newest finished + running) are intact and loadable.
		for _, id := range []string{finished[3], finished[4], running} {
			loaded, err := repo.LoadByID(ctx, id)
			require.NoError(t, err, "expected survivor %s to load", id)
			assert.Len(t, loaded.Tasks, 1)
			assert.Len(t, loaded.LastActions, 1)
			assert.Len(t, loaded.Files, 1)
		}
	})

	t.Run("when fewer finished states exist than keepLast, it prunes nothing", func(t *testing.T) {
		repo := newMaintenanceTestRepo(t)
		seed(t, repo)
		pruned, err := repo.PruneFinishedStates(ctx, 10)
		require.NoError(t, err)
		assert.Equal(t, 0, pruned)
	})

	t.Run("when keepLast is zero, it prunes all finished states but never the running one", func(t *testing.T) {
		repo := newMaintenanceTestRepo(t)
		_, running := seed(t, repo)
		pruned, err := repo.PruneFinishedStates(ctx, 0)
		require.NoError(t, err)
		assert.Equal(t, 5, pruned)

		loaded, err := repo.LoadByID(ctx, running)
		require.NoError(t, err)
		assert.Equal(t, domain.StoryRunning, loaded.StoryStatus)
	})

	t.Run("when keepLast is negative, it is treated as zero", func(t *testing.T) {
		repo := newMaintenanceTestRepo(t)
		seed(t, repo)
		pruned, err := repo.PruneFinishedStates(ctx, -3)
		require.NoError(t, err)
		assert.Equal(t, 5, pruned)
	})

	t.Run("when a pruned state is re-saved, its fingerprint cache no longer applies", func(t *testing.T) {
		repo := newMaintenanceTestRepo(t)
		state := newMaintenanceState("state-recycled", domain.StorySuccess, base)
		require.NoError(t, repo.Save(ctx, state))

		pruned, err := repo.PruneFinishedStates(ctx, 0)
		require.NoError(t, err)
		require.Equal(t, 1, pruned)
		assert.Nil(t, repo.fingerprints.get("state-recycled"), "expected fingerprint cache cleared for pruned state")

		// Re-saving the same content must write rows again (fresh state, version 0).
		recreated := newMaintenanceState("state-recycled", domain.StorySuccess, base)
		require.NoError(t, repo.Save(ctx, recreated))
		assert.Equal(t, 1, countRows(t, repo, "tasks", "state-recycled"))
	})
}

func TestSQLiteRepositoryLoadAllSummaries(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 8, 4, 8, 0, 0, 0, time.UTC)

	t.Run("when states have tasks, it returns per-status counts without loading bodies", func(t *testing.T) {
		repo := newMaintenanceTestRepo(t)

		s1 := newMaintenanceState("state-a", domain.StoryRunning, base)
		s1.Tasks = []domain.Task{
			{ID: "a-t1", Title: "T1", Description: "d", Status: domain.TaskSuccess, ChangeType: domain.ChangeTypeFix, DependsOn: []string{}, CreatedAt: base.Add(-2 * time.Hour), UpdatedAt: base},
			{ID: "a-t2", Title: "T2", Description: "d", Status: domain.TaskSuccess, ChangeType: domain.ChangeTypeFix, DependsOn: []string{}, CreatedAt: base.Add(-time.Hour), UpdatedAt: base.Add(time.Minute)},
			{ID: "a-t3", Title: "T3", Description: "d", Status: domain.TaskPending, ChangeType: domain.ChangeTypeFix, DependsOn: []string{}, CreatedAt: base, UpdatedAt: base},
		}
		s1.Metadata = domain.StateMetadata{FeatureName: "feat-a", IntegrationBranch: "noctifab/a", BaseBranch: "main", InputPath: "a.md"}
		require.NoError(t, repo.Save(ctx, s1))

		s2 := newMaintenanceState("state-b", domain.StorySuccess, base)
		require.NoError(t, repo.Save(ctx, s2))

		summaries, err := repo.LoadAllSummaries(ctx)
		require.NoError(t, err)
		require.Len(t, summaries, 2)

		// RUNNING state ordered first.
		first := summaries[0]
		assert.Equal(t, "state-a", first.ID)
		assert.Equal(t, "RUNNING", first.StoryStatus)
		assert.Equal(t, "feat-a", first.FeatureName)
		assert.Equal(t, 1, first.Version)
		assert.Equal(t, 3, first.TotalTasks)
		assert.Equal(t, 2, first.TaskCounts["SUCCESS"])
		assert.Equal(t, 1, first.TaskCounts["PENDING"])
		assert.True(t, first.CreatedAt.Equal(base.Add(-2*time.Hour)), "expected earliest task CreatedAt, got %v", first.CreatedAt)
		assert.True(t, first.UpdatedAt.Equal(base.Add(time.Minute)), "expected latest task UpdatedAt, got %v", first.UpdatedAt)

		second := summaries[1]
		assert.Equal(t, "state-b", second.ID)
		assert.Equal(t, 1, second.TotalTasks)
		assert.Equal(t, 1, second.TaskCounts["SUCCESS"])
	})

	t.Run("when a summary is compared with SummarizeState of the loaded state, counts match", func(t *testing.T) {
		repo := newMaintenanceTestRepo(t)
		state := newMaintenanceState("state-x", domain.StoryFailed, base)
		require.NoError(t, repo.Save(ctx, state))

		summaries, err := repo.LoadAllSummaries(ctx)
		require.NoError(t, err)
		require.Len(t, summaries, 1)

		loaded, err := repo.LoadByID(ctx, "state-x")
		require.NoError(t, err)
		expected := domain.SummarizeState(loaded)
		assert.Equal(t, expected.TaskCounts, summaries[0].TaskCounts)
		assert.Equal(t, expected.TotalTasks, summaries[0].TotalTasks)
		assert.Equal(t, expected.StoryStatus, summaries[0].StoryStatus)
		assert.Equal(t, expected.Version, summaries[0].Version)
	})

	t.Run("when the database is empty, it returns an empty non-nil slice", func(t *testing.T) {
		repo := newMaintenanceTestRepo(t)
		summaries, err := repo.LoadAllSummaries(ctx)
		require.NoError(t, err)
		assert.NotNil(t, summaries)
		assert.Empty(t, summaries)
	})
}
