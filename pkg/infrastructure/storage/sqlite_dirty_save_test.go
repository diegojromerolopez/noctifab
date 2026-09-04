package storage

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

func newDirtyTestRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "noctifab-sqlite-dirty-test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	repo, err := NewSQLiteRepository(context.Background(), filepath.Join(tmpDir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func newDirtyTestState(id string) *domain.State {
	now := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	return &domain.State{
		ID:          id,
		ProjectPath: "/workspace",
		BuildStatus: domain.BuildUnknown,
		StoryStatus: domain.StoryRunning,
		Tasks: []domain.Task{
			{ID: id + "-t1", Title: "Task 1", Description: "d1", Status: domain.TaskPending, ChangeType: domain.ChangeTypeFix, DependsOn: []string{}, CreatedAt: now, UpdatedAt: now},
			{ID: id + "-t2", Title: "Task 2", Description: "d2", Status: domain.TaskSuccess, ChangeType: domain.ChangeTypeFix, DependsOn: []string{}, CreatedAt: now, UpdatedAt: now.Add(time.Minute)},
		},
		Clarifications:     []domain.Clarification{{Question: "Q?", Answer: "A", Resolved: true, AskedAt: now}},
		LastActions:        []domain.Action{{Timestamp: now, Tool: "write", Args: map[string]any{"k": "v"}, Reasoning: "r", Result: "ok", Success: true}},
		Files:              []domain.FileInfo{{Path: "main.go", Size: 42, LastModified: now}},
		ValidationCriteria: []domain.ValidationCriterion{{ID: id + "-c1", Type: domain.ValidationCommand, Expression: "go test", Description: "tests"}},
		ActiveAgents:       []domain.Agent{{ID: id + "-a1", Name: "gen", Role: domain.AgentRoleGenerator, Status: domain.AgentIdle}},
	}
}

// taskRowID returns the sqlite rowid of a task row. Rowids change when a row
// is deleted and re-inserted, so their stability across saves proves the
// relation group was not rewritten.
func taskRowID(t *testing.T, repo *SQLiteRepository, taskID string) int64 {
	t.Helper()
	var rowid int64
	err := repo.DB().QueryRow("SELECT rowid FROM tasks WHERE id = ?", taskID).Scan(&rowid)
	require.NoError(t, err)
	return rowid
}

// saveShieldState persists an extra state AFTER the state under test so the
// relation tables hold higher rowids. SQLite allocates max(rowid)+1 on
// insert, so without this shield a DELETE+INSERT rewrite could silently
// reuse the same rowid and make rowid-based rewrite detection unreliable.
func saveShieldState(t *testing.T, repo *SQLiteRepository) {
	t.Helper()
	require.NoError(t, repo.Save(context.Background(), newDirtyTestState("state-shield")))
}

func TestSQLiteRepositoryDirtyGroupSaves(t *testing.T) {
	ctx := context.Background()

	t.Run("when only one group changes, it does not rewrite untouched groups", func(t *testing.T) {
		repo := newDirtyTestRepo(t)
		state := newDirtyTestState("state-dirty")
		require.NoError(t, repo.Save(ctx, state))
		saveShieldState(t, repo)

		taskRowIDBefore := taskRowID(t, repo, "state-dirty-t1")
		var fileRowIDBefore int64
		require.NoError(t, repo.DB().QueryRow("SELECT rowid FROM workspace_files WHERE path = 'main.go'").Scan(&fileRowIDBefore))

		// Mutate ONLY the actions group.
		state.LastActions = append(state.LastActions, domain.Action{
			Timestamp: time.Now(), Tool: "evaluate", Args: map[string]any{}, Reasoning: "r2", Result: "ok2", Success: true,
		})
		require.NoError(t, repo.Save(ctx, state))

		assert.Equal(t, taskRowIDBefore, taskRowID(t, repo, "state-dirty-t1"),
			"tasks row was rewritten despite being untouched")
		var fileRowIDAfter int64
		require.NoError(t, repo.DB().QueryRow("SELECT rowid FROM workspace_files WHERE path = 'main.go'").Scan(&fileRowIDAfter))
		assert.Equal(t, fileRowIDBefore, fileRowIDAfter,
			"workspace_files row was rewritten despite being untouched")

		var actionCount int
		require.NoError(t, repo.DB().QueryRow("SELECT COUNT(*) FROM actions WHERE state_id = 'state-dirty'").Scan(&actionCount))
		assert.Equal(t, 2, actionCount, "actions group must have been rewritten")
	})

	t.Run("when the changed group is rewritten, load returns the identical state after partial saves", func(t *testing.T) {
		repo := newDirtyTestRepo(t)
		state := newDirtyTestState("state-load")
		require.NoError(t, repo.Save(ctx, state))

		state.LastActions = append(state.LastActions, domain.Action{
			Timestamp: time.Date(2026, 8, 4, 11, 0, 0, 0, time.UTC), Tool: "evaluate", Args: map[string]any{"n": 1.0}, Reasoning: "r2", Result: "ok2", Success: false,
		})
		require.NoError(t, repo.Save(ctx, state))

		loaded, err := repo.LoadByID(ctx, "state-load")
		require.NoError(t, err)
		assert.Equal(t, state.Version, loaded.Version)
		assert.Len(t, loaded.LastActions, 2)
		assert.Equal(t, state.Tasks, loaded.Tasks)
		assert.Equal(t, state.Clarifications, loaded.Clarifications)
		assert.Equal(t, state.ValidationCriteria, loaded.ValidationCriteria)
		assert.Equal(t, "ok2", loaded.LastActions[1].Result)
	})

	t.Run("when nothing changes between saves, untouched groups keep stable rowids", func(t *testing.T) {
		repo := newDirtyTestRepo(t)
		state := newDirtyTestState("state-noop")
		require.NoError(t, repo.Save(ctx, state))
		saveShieldState(t, repo)
		before := taskRowID(t, repo, "state-noop-t1")

		require.NoError(t, repo.Save(ctx, state))
		assert.Equal(t, before, taskRowID(t, repo, "state-noop-t1"))
	})

	t.Run("when a version conflict occurs, the cache is invalidated and the next save rewrites everything", func(t *testing.T) {
		repo := newDirtyTestRepo(t)
		state := newDirtyTestState("state-conflict")
		require.NoError(t, repo.Save(ctx, state))
		saveShieldState(t, repo)
		taskRowBefore := taskRowID(t, repo, "state-conflict-t1")
		var actionIDBefore int64
		require.NoError(t, repo.DB().QueryRow("SELECT id FROM actions WHERE state_id = 'state-conflict'").Scan(&actionIDBefore))

		// Simulate a concurrent writer bumping the version behind our back.
		_, err := repo.DB().Exec("UPDATE state SET version = version + 1 WHERE id = 'state-conflict'")
		require.NoError(t, err)

		stale := newDirtyTestState("state-conflict")
		stale.Version = state.Version - 1 // deliberately stale
		err = repo.Save(ctx, stale)
		require.ErrorIs(t, err, domain.ErrVersionConflict)

		// Reload the authoritative state and save again with NO group changes:
		// because the cache was invalidated, every group is re-saved.
		fresh, err := repo.LoadByID(ctx, "state-conflict")
		require.NoError(t, err)
		require.NoError(t, repo.Save(ctx, fresh))

		taskRowAfter := taskRowID(t, repo, "state-conflict-t1")
		assert.Equal(t, taskRowBefore, taskRowAfter,
			"in-place upsert preserves stable task rowid even when re-persisted")

		var actionIDAfter int64
		require.NoError(t, repo.DB().QueryRow("SELECT id FROM actions WHERE state_id = 'state-conflict'").Scan(&actionIDAfter))
		assert.NotEqual(t, actionIDBefore, actionIDAfter,
			"actions group must be rewritten after cache invalidation (id should change)")
	})

	t.Run("when tasks are mutated, upsert updates fields in-place and preserves stable rowid", func(t *testing.T) {
		repo := newDirtyTestRepo(t)
		state := newDirtyTestState("state-upsert")
		require.NoError(t, repo.Save(ctx, state))
		saveShieldState(t, repo)

		rowBefore := taskRowID(t, repo, "state-upsert-t1")

		// Update progress and title on task 1
		state.Tasks[0].Progress = 75
		state.Tasks[0].Title = "Task 1 Updated"
		require.NoError(t, repo.Save(ctx, state))

		rowAfter := taskRowID(t, repo, "state-upsert-t1")
		assert.Equal(t, rowBefore, rowAfter, "in-place upsert must keep stable rowid")

		loaded, err := repo.LoadByID(ctx, "state-upsert")
		require.NoError(t, err)
		assert.Equal(t, 75, loaded.Tasks[0].Progress)
		assert.Equal(t, "Task 1 Updated", loaded.Tasks[0].Title)
	})

	t.Run("when a task is added or removed, upsert inserts and selective delete prunes", func(t *testing.T) {
		repo := newDirtyTestRepo(t)
		state := newDirtyTestState("state-add-del")
		require.NoError(t, repo.Save(ctx, state))

		// Add task 3, remove task 2
		state.Tasks = []domain.Task{
			state.Tasks[0],
			{ID: "state-add-del-t3", Title: "Task 3", Description: "d3", Status: domain.TaskPending, ChangeType: domain.ChangeTypeFeature, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		}
		require.NoError(t, repo.Save(ctx, state))

		loaded, err := repo.LoadByID(ctx, "state-add-del")
		require.NoError(t, err)
		require.Len(t, loaded.Tasks, 2)
		assert.Equal(t, "state-add-del-t1", loaded.Tasks[0].ID)
		assert.Equal(t, "state-add-del-t3", loaded.Tasks[1].ID)

		// Verify task 2 was deleted from database
		var count int
		require.NoError(t, repo.DB().QueryRow("SELECT COUNT(*) FROM tasks WHERE id = 'state-add-del-t2'").Scan(&count))
		assert.Equal(t, 0, count, "pruned task must be deleted from DB")
	})

	t.Run("when the state has unserializable actions, save fails before touching the database", func(t *testing.T) {
		repo := newDirtyTestRepo(t)
		state := newDirtyTestState("state-badjson")
		state.LastActions[0].Args = map[string]any{"bad": make(chan int)}
		err := repo.Save(ctx, state)
		assert.Error(t, err)
	})
}
