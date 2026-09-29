package services_test

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAndSortTasks_ValidGraphs(t *testing.T) {
	t.Run("empty task slice returns empty slice without error", func(t *testing.T) {
		sorted, err := services.ResolveAndSortTasks(nil)
		require.NoError(t, err)
		assert.Empty(t, sorted)

		sorted, err = services.ResolveAndSortTasks([]domain.Task{})
		require.NoError(t, err)
		assert.Empty(t, sorted)
	})

	t.Run("single task with no dependencies is preserved", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-1", Title: "Scaffold project"},
		}
		sorted, err := services.ResolveAndSortTasks(tasks)
		require.NoError(t, err)
		require.Len(t, sorted, 1)
		assert.Equal(t, "task-1", sorted[0].ID)
	})

	t.Run("linear dependency chain A -> B -> C sorts topologically", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-C", Title: "End-to-End Tests", DependsOn: []string{"task-B"}},
			{ID: "task-A", Title: "Database Setup", DependsOn: nil},
			{ID: "task-B", Title: "CRUD Handlers", DependsOn: []string{"task-A"}},
		}
		sorted, err := services.ResolveAndSortTasks(tasks)
		require.NoError(t, err)
		require.Len(t, sorted, 3)

		assert.Equal(t, "task-A", sorted[0].ID)
		assert.Equal(t, "task-B", sorted[1].ID)
		assert.Equal(t, "task-C", sorted[2].ID)
	})

	t.Run("diamond dependency graph A -> B, A -> C, B & C -> D", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-D", Title: "Final Verification", DependsOn: []string{"task-B", "task-C"}},
			{ID: "task-B", Title: "Service Layer", DependsOn: []string{"task-A"}},
			{ID: "task-C", Title: "CLI Interface", DependsOn: []string{"task-A"}},
			{ID: "task-A", Title: "Domain Entities", DependsOn: nil},
		}
		sorted, err := services.ResolveAndSortTasks(tasks)
		require.NoError(t, err)
		require.Len(t, sorted, 4)

		pos := make(map[string]int)
		for i, tsk := range sorted {
			pos[tsk.ID] = i
		}

		assert.True(t, pos["task-A"] < pos["task-B"])
		assert.True(t, pos["task-A"] < pos["task-C"])
		assert.True(t, pos["task-B"] < pos["task-D"])
		assert.True(t, pos["task-C"] < pos["task-D"])
	})

	t.Run("resolves dependency by Title when ID is not used", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-2", Title: "Implement Core Logic", DependsOn: []string{"Setup Repository"}},
			{ID: "task-1", Title: "Setup Repository", DependsOn: nil},
		}
		sorted, err := services.ResolveAndSortTasks(tasks)
		require.NoError(t, err)
		require.Len(t, sorted, 2)
		assert.Equal(t, "task-1", sorted[0].ID)
		assert.Equal(t, "task-2", sorted[1].ID)
	})

	t.Run("preserves external cross-story task dependency without error", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "US-002-TASK-001", Title: "API Endpoint", DependsOn: []string{"US-001-TASK-001"}},
			{ID: "US-002-TASK-002", Title: "API Tests", DependsOn: []string{"US-002-TASK-001"}},
		}
		sorted, err := services.ResolveAndSortTasks(tasks)
		require.NoError(t, err)
		require.Len(t, sorted, 2)
		assert.Equal(t, "US-002-TASK-001", sorted[0].ID)
		assert.Equal(t, "US-002-TASK-002", sorted[1].ID)
	})
}

func TestResolveAndSortTasks_InputValidationErrors(t *testing.T) {
	t.Run("returns error when duplicate task titles exist", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-1", Title: "Setup Infrastructure"},
			{ID: "task-2", Title: "Setup Infrastructure"},
		}
		_, err := services.ResolveAndSortTasks(tasks)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate task title found")
	})

	t.Run("returns error when task depends on unresolved non-existent prerequisite", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-1", Title: "Feature Implementation", DependsOn: []string{"missing-task-id"}},
		}
		_, err := services.ResolveAndSortTasks(tasks)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "depends on unresolved prerequisite")
	})

	t.Run("returns error when task has self-referencing dependency", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-1", Title: "Looping Task", DependsOn: []string{"task-1"}},
		}
		_, err := services.ResolveAndSortTasks(tasks)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected in task DAG")
	})

	t.Run("returns error on two-node circular dependency", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-1", Title: "Task 1", DependsOn: []string{"task-2"}},
			{ID: "task-2", Title: "Task 2", DependsOn: []string{"task-1"}},
		}
		_, err := services.ResolveAndSortTasks(tasks)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected in task DAG")
	})

	t.Run("returns error on three-node circular dependency", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-1", Title: "Task 1", DependsOn: []string{"task-2"}},
			{ID: "task-2", Title: "Task 2", DependsOn: []string{"task-3"}},
			{ID: "task-3", Title: "Task 3", DependsOn: []string{"task-1"}},
		}
		_, err := services.ResolveAndSortTasks(tasks)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cycle detected in task DAG")
	})
}
