package services_test

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeAndDeduplicateTasks(t *testing.T) {
	t.Run("rewrites aliased section numbers to active story ID and remaps intra-story dependencies", func(t *testing.T) {
		// Planner saw Section 5 and emitted US-005 task IDs while planning story US-002
		rawTasks := []domain.Task{
			{
				ID:          "US-005-TASK-001",
				Title:       "Define List Types",
				DependsOn:   []string{"US-001-TASK-001"}, // cross-story dependency preserved
				TargetFiles: []string{"src/lists.py"},
			},
			{
				ID:          "US-005-TASK-002",
				Title:       "Implement LPUSH",
				DependsOn:   []string{"US-005-TASK-001"}, // intra-story dependency remapped
				TargetFiles: []string{"src/commands/lpush.py"},
			},
		}

		existing := []domain.Task{
			{ID: "US-001-TASK-001", StoryID: "US-001", Title: "Walking Skeleton"},
		}

		normalized := services.NormalizeAndDeduplicateTasks(rawTasks, "US-002", existing)

		assert.Len(t, normalized, 2)
		assert.Equal(t, "US-002-TASK-001", normalized[0].ID)
		assert.Equal(t, "US-002", normalized[0].StoryID)
		assert.Equal(t, []string{"US-001-TASK-001"}, normalized[0].DependsOn)

		assert.Equal(t, "US-002-TASK-002", normalized[1].ID)
		assert.Equal(t, "US-002", normalized[1].StoryID)
		assert.Equal(t, []string{"US-002-TASK-001"}, normalized[1].DependsOn)
	})

	t.Run("resolves task ID collisions against existing database tasks", func(t *testing.T) {
		existing := []domain.Task{
			{ID: "US-002-TASK-001", StoryID: "US-002", Title: "Existing Task 1"},
		}

		rawTasks := []domain.Task{
			{
				ID:    "US-002-TASK-001",
				Title: "New Task with colliding ID",
			},
		}

		normalized := services.NormalizeAndDeduplicateTasks(rawTasks, "US-002", existing)

		assert.Len(t, normalized, 1)
		assert.Equal(t, "US-002-TASK-002", normalized[0].ID)
	})

	t.Run("disambiguates duplicate task IDs within the same batch", func(t *testing.T) {
		rawTasks := []domain.Task{
			{ID: "US-003-TASK-001", Title: "Task A"},
			{ID: "US-003-TASK-001", Title: "Task B duplicate"},
		}

		normalized := services.NormalizeAndDeduplicateTasks(rawTasks, "US-003", nil)

		assert.Len(t, normalized, 2)
		assert.Equal(t, "US-003-TASK-001", normalized[0].ID)
		assert.Equal(t, "US-003-TASK-002", normalized[1].ID)
	})

	t.Run("handles empty task slice", func(t *testing.T) {
		assert.Empty(t, services.NormalizeAndDeduplicateTasks(nil, "US-001", nil))
	})
}
