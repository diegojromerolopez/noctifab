package storage

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSQLiteRepository(t *testing.T) {
	ctx := context.Background()

	newTempSQLiteDSN := func(t *testing.T) string {
		tmpDir, err := os.MkdirTemp("", "noctifab-sqlite-test")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(tmpDir)
		})
		return filepath.Join(tmpDir, "test.db")
	}

	t.Run("when initializing repository with invalid DSN, it returns error", func(t *testing.T) {
		// A directory path cannot be opened as a SQLite database file for writing
		dsn := "/"
		repo, err := NewSQLiteRepository(ctx, dsn)
		assert.Error(t, err)
		assert.Nil(t, repo)
	})

	t.Run("when initializing repository, it runs migrations successfully", func(t *testing.T) {
		dsn := newTempSQLiteDSN(t)
		repo, err := NewSQLiteRepository(ctx, dsn)
		require.NoError(t, err)
		defer func() { _ = repo.Close() }()

		var version int
		err = repo.db.QueryRowContext(ctx, "SELECT version FROM schema_migrations WHERE version = 1").Scan(&version)
		assert.NoError(t, err)
		assert.Equal(t, 1, version)
	})

	t.Run("when saving and loading a populated state, it reconstructs the domain model exactly", func(t *testing.T) {
		dsn := newTempSQLiteDSN(t)
		repo, err := NewSQLiteRepository(ctx, dsn)
		require.NoError(t, err)
		defer func() { _ = repo.Close() }()

		stateID := "state-123"
		now := time.Now().Truncate(time.Second)
		initialState := &domain.State{
			ID:          stateID,
			ProjectPath: "/workspace",
			Version:     0,
			BuildStatus: domain.BuildPassing,
			Clarifications: []domain.Clarification{
				{
					Question: "What is the auth key?",
					Answer:   "12345",
					Resolved: true,
					AskedAt:  now,
				},
				{
					Question: "Unresolved question?",
					Answer:   "",
					Resolved: false,
					AskedAt:  now,
				},
			},
			ValidationCriteria: []domain.ValidationCriterion{
				{
					ID:          "crit-1",
					Type:        domain.ValidationCommand,
					Expression:  "go test ./...",
					Description: "Run unit tests",
					Passed:      true,
					ErrorLog:    "",
				},
			},
			Tasks: []domain.Task{
				{
					ID:               "task-1",
					Title:            "Implement Auth",
					Description:      "Add auth endpoint",
					Status:           domain.TaskSuccess,
					ChangeType:       domain.ChangeTypeFeature,
					AssignedTo:       "agent-1",
					DependsOn:        []string{},
					TargetFiles:      []string{"auth.go"},
					PartialChangelog: []string{"Added auth endpoint"},
					Retries:          0,
					MaxRetries:       3,
					CreatedAt:        now,
					UpdatedAt:        now,
				},
			},
			ActiveAgents: []domain.Agent{
				{
					ID:          "agent-1",
					Name:        "Developer Agent",
					Role:        domain.AgentRoleGenerator,
					Status:      domain.AgentWorking,
					TaskID:      "task-1",
					StartedAt:   now,
					CompletedAt: time.Time{},
					TokensUsed:  150,
					LastError:   "",
				},
			},
			Files: []domain.FileInfo{
				{
					Path:         "auth.go",
					Size:         1024,
					LastModified: now,
				},
			},
			LastActions: []domain.Action{
				{
					Timestamp: now,
					Tool:      "write_file",
					Args:      map[string]any{"path": "auth.go", "content": "package main"},
					Reasoning: "Create basic entry file",
					Result:    "success",
					Success:   true,
				},
			},
			Metadata: domain.StateMetadata{
				InputSource:       "markdown",
				InputPath:         "spec.md",
				IntegrationBranch: "feature/auth",
				FeatureName:       "Authentication",
				BaseBranch:        "main",
				ProjectVersion:    "1.0.0",
				TotalTokensUsed:   500,
				TotalCostUSD:      "0.01500",
			},
		}

		err = repo.Save(ctx, initialState)
		assert.NoError(t, err)
		assert.Equal(t, 1, initialState.Version)

		loadedState, err := repo.Load(ctx)
		assert.NoError(t, err)
		require.NotNil(t, loadedState)

		assert.Equal(t, initialState.ID, loadedState.ID)
		assert.Equal(t, initialState.ProjectPath, loadedState.ProjectPath)
		assert.Equal(t, initialState.Version, loadedState.Version)
		assert.Equal(t, initialState.BuildStatus, loadedState.BuildStatus)
		assert.Equal(t, initialState.Metadata, loadedState.Metadata)

		require.Len(t, loadedState.Clarifications, 2)
		assert.Equal(t, initialState.Clarifications[0].Question, loadedState.Clarifications[0].Question)
		assert.Equal(t, initialState.Clarifications[0].Answer, loadedState.Clarifications[0].Answer)
		assert.Equal(t, initialState.Clarifications[0].Resolved, loadedState.Clarifications[0].Resolved)
		assert.True(t, initialState.Clarifications[0].AskedAt.Equal(loadedState.Clarifications[0].AskedAt))
		assert.False(t, loadedState.Clarifications[1].Resolved)

		require.Len(t, loadedState.ValidationCriteria, 1)
		assert.Equal(t, initialState.ValidationCriteria[0], loadedState.ValidationCriteria[0])

		require.Len(t, loadedState.Tasks, 1)
		assert.Equal(t, initialState.Tasks[0].ID, loadedState.Tasks[0].ID)
		assert.Equal(t, initialState.Tasks[0].Title, loadedState.Tasks[0].Title)
		assert.Equal(t, initialState.Tasks[0].DependsOn, loadedState.Tasks[0].DependsOn)
		assert.Equal(t, initialState.Tasks[0].TargetFiles, loadedState.Tasks[0].TargetFiles)
		assert.Equal(t, initialState.Tasks[0].PartialChangelog, loadedState.Tasks[0].PartialChangelog)
		assert.True(t, initialState.Tasks[0].CreatedAt.Equal(loadedState.Tasks[0].CreatedAt))

		require.Len(t, loadedState.ActiveAgents, 1)
		assert.Equal(t, initialState.ActiveAgents[0].ID, loadedState.ActiveAgents[0].ID)
		assert.Equal(t, initialState.ActiveAgents[0].TokensUsed, loadedState.ActiveAgents[0].TokensUsed)
		assert.True(t, initialState.ActiveAgents[0].StartedAt.Equal(loadedState.ActiveAgents[0].StartedAt))
		assert.True(t, loadedState.ActiveAgents[0].CompletedAt.IsZero())

		require.Len(t, loadedState.Files, 1)
		assert.Equal(t, initialState.Files[0].Path, loadedState.Files[0].Path)
		assert.Equal(t, initialState.Files[0].Size, loadedState.Files[0].Size)
		assert.True(t, initialState.Files[0].LastModified.Equal(loadedState.Files[0].LastModified))

		require.Len(t, loadedState.LastActions, 1)
		assert.Equal(t, initialState.LastActions[0].Tool, loadedState.LastActions[0].Tool)
		assert.Equal(t, initialState.LastActions[0].Reasoning, loadedState.LastActions[0].Reasoning)
		assert.Equal(t, initialState.LastActions[0].Success, loadedState.LastActions[0].Success)
		assert.Equal(t, initialState.LastActions[0].Args["path"], loadedState.LastActions[0].Args["path"])
	})

	t.Run("when updating state with OCC version mismatch, it returns ErrVersionConflict", func(t *testing.T) {
		dsn := newTempSQLiteDSN(t)
		repo, err := NewSQLiteRepository(ctx, dsn)
		require.NoError(t, err)
		defer func() { _ = repo.Close() }()

		state := &domain.State{
			ID:          "state-occ",
			ProjectPath: "/work",
			Version:     0,
		}

		err = repo.Save(ctx, state)
		assert.NoError(t, err)
		assert.Equal(t, 1, state.Version)

		staleState := &domain.State{
			ID:          "state-occ",
			ProjectPath: "/work",
			Version:     0,
		}
		err = repo.Save(ctx, staleState)
		assert.ErrorIs(t, err, domain.ErrVersionConflict)

		loaded, err := repo.Load(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, loaded.Version)
	})

	t.Run("when a transaction fails, it rolls back state changes cleanly", func(t *testing.T) {
		dsn := newTempSQLiteDSN(t)
		repo, err := NewSQLiteRepository(ctx, dsn)
		require.NoError(t, err)
		defer func() { _ = repo.Close() }()

		state := &domain.State{
			ID:          "state-rollback",
			ProjectPath: "/work",
			Version:     0,
		}

		err = repo.Save(ctx, state)
		assert.NoError(t, err)

		state.Version = 1
		state.Tasks = []domain.Task{
			{ID: "dup-task", Title: "t1"},
			{ID: "dup-task", Title: "t2"},
		}

		err = repo.Save(ctx, state)
		assert.Error(t, err)

		loaded, err := repo.Load(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 1, loaded.Version)
		assert.Empty(t, loaded.Tasks)
	})

	t.Run("when saving action with unmarshallable args, it returns JSON error", func(t *testing.T) {
		dsn := newTempSQLiteDSN(t)
		repo, err := NewSQLiteRepository(ctx, dsn)
		require.NoError(t, err)
		defer func() { _ = repo.Close() }()

		state := &domain.State{
			ID:          "state-json-err",
			ProjectPath: "/work",
			Version:     0,
			LastActions: []domain.Action{
				{
					Timestamp: time.Now(),
					Tool:      "test",
					Args:      map[string]any{"invalid": make(chan int)}, // Channels cannot be serialized to JSON
				},
			},
		}

		err = repo.Save(ctx, state)
		assert.Error(t, err)
	})

	t.Run("when loading database with corrupted JSON fields, it returns error", func(t *testing.T) {
		dsn := newTempSQLiteDSN(t)
		repo, err := NewSQLiteRepository(ctx, dsn)
		require.NoError(t, err)
		defer func() { _ = repo.Close() }()

		state := &domain.State{
			ID:          "state-corrupt",
			ProjectPath: "/work",
			Version:     0,
			Tasks: []domain.Task{
				{ID: "task-1", Title: "Title"},
			},
		}

		err = repo.Save(ctx, state)
		require.NoError(t, err)

		// Corrupt the depends_on field in database
		_, err = repo.db.ExecContext(ctx, "UPDATE tasks SET depends_on = '{corrupted_json'")
		require.NoError(t, err)

		_, err = repo.Load(ctx)
		assert.Error(t, err)
	})

	t.Run("when repository is closed, operations return error", func(t *testing.T) {
		dsn := newTempSQLiteDSN(t)
		repo, err := NewSQLiteRepository(ctx, dsn)
		require.NoError(t, err)
		_ = repo.Close()

		state := &domain.State{
			ID:      "state-closed",
			Version: 0,
		}
		err = repo.Save(ctx, state)
		assert.Error(t, err)

		_, err = repo.Load(ctx)
		assert.Error(t, err)
	})

	t.Run("when loaded concurrently, the serialized write mutex prevents write locks and races", func(t *testing.T) {
		dsn := newTempSQLiteDSN(t)
		repo, err := NewSQLiteRepository(ctx, dsn)
		require.NoError(t, err)
		defer func() { _ = repo.Close() }()

		state := &domain.State{
			ID:          "state-concurrent",
			ProjectPath: "/work",
			Version:     0,
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)

		var wg sync.WaitGroup
		concurrency := 10
		wg.Add(concurrency)

		for i := 0; i < concurrency; i++ {
			go func() {
				defer wg.Done()
				for {
					localState, err := repo.Load(ctx)
					if err != nil {
						continue
					}
					localState.ProjectPath = "/work-updated"
					time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
					err = repo.Save(ctx, localState)
					if err == nil {
						break
					}
					if !errors.Is(err, domain.ErrVersionConflict) {
						t.Errorf("unexpected error under concurrency: %v", err)
						return
					}
				}
			}()
		}

		wg.Wait()

		loaded, err := repo.Load(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 11, loaded.Version)
		assert.Equal(t, "/work-updated", loaded.ProjectPath)
	})
}
