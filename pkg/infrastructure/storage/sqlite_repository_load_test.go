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

func TestSQLiteRepository_LoadByID_And_LoadAll(t *testing.T) {
	ctx := context.Background()

	newTempSQLiteDSN := func(t *testing.T) string {
		tmpDir, err := os.MkdirTemp("", "noctifab-sqlite-test-load")
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = os.RemoveAll(tmpDir)
		})
		return filepath.Join(tmpDir, "test.db")
	}

	t.Run("when loading states, it retrieves by ID and retrieves all states with progress", func(t *testing.T) {
		dsn := newTempSQLiteDSN(t)
		repo, err := NewSQLiteRepository(ctx, dsn)
		require.NoError(t, err)
		defer func() { _ = repo.Close() }()

		state1 := &domain.State{
			ID:          "story-1",
			ProjectPath: "/work/1",
			Version:     0,
			StoryStatus: domain.StoryRunning,
			Tasks: []domain.Task{
				{
					ID:          "t1",
					Title:       "Task 1",
					Description: "Desc 1",
					Status:      domain.TaskInProgress,
					Progress:    45,
					DependsOn:   []string{},
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
			},
		}

		state2 := &domain.State{
			ID:          "story-2",
			ProjectPath: "/work/2",
			Version:     0,
			StoryStatus: domain.StoryPaused,
			Tasks: []domain.Task{
				{
					ID:          "t2",
					Title:       "Task 2",
					Description: "Desc 2",
					Status:      domain.TaskPending,
					Progress:    10,
					DependsOn:   []string{},
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				},
			},
		}

		err = repo.Save(ctx, state1)
		require.NoError(t, err)

		err = repo.Save(ctx, state2)
		require.NoError(t, err)

		// 1. Test LoadByID
		loaded1, err := repo.LoadByID(ctx, "story-1")
		require.NoError(t, err)
		assert.Equal(t, "story-1", loaded1.ID)
		assert.Equal(t, "/work/1", loaded1.ProjectPath)
		require.Len(t, loaded1.Tasks, 1)
		assert.Equal(t, 45, loaded1.Tasks[0].Progress)

		// 2. Test LoadAll
		all, err := repo.LoadAll(ctx)
		require.NoError(t, err)
		// Should find both states
		require.Len(t, all, 2)
		// LoadAll order is: story_status = 'RUNNING' first, then ID descending
		assert.Equal(t, "story-1", all[0].ID) // running story is first
		assert.Equal(t, "story-2", all[1].ID)
	})
}
