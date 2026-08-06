package services

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestDependencyIndexResolve(t *testing.T) {
	tasks := []domain.Task{
		{ID: "task-1", Title: "Implement JSON Repository and Unit Test"},
		{ID: "task-2", Title: "Implement CLI Add Command Logic"},
		{ID: "task-3", Title: "Wire the HTTP API"},
	}
	idx := newDependencyIndex(tasks)

	t.Run("when the dependency matches a task ID exactly, it resolves it", func(t *testing.T) {
		id, ok := idx.resolve("task-2")
		if !ok || id != "task-2" {
			t.Errorf("expected task-2, got %q (found=%v)", id, ok)
		}
	})

	t.Run("when the dependency matches a task title exactly, it resolves it", func(t *testing.T) {
		id, ok := idx.resolve("Wire the HTTP API")
		if !ok || id != "task-3" {
			t.Errorf("expected task-3, got %q (found=%v)", id, ok)
		}
	})

	t.Run("when the dependency differs only in case and punctuation, it resolves via normalization", func(t *testing.T) {
		id, ok := idx.resolve("wire-the-http-api!")
		if !ok || id != "task-3" {
			t.Errorf("expected task-3 via normalized match, got %q (found=%v)", id, ok)
		}
	})

	t.Run("when the dependency is a prefix of a task title, it resolves via substring fallback", func(t *testing.T) {
		id, ok := idx.resolve("Implement JSON Repository")
		if !ok || id != "task-1" {
			t.Errorf("expected task-1 via substring match, got %q (found=%v)", id, ok)
		}
	})

	t.Run("when the task title is contained in the dependency, it resolves via substring fallback", func(t *testing.T) {
		id, ok := idx.resolve("Please Wire the HTTP API for me")
		if !ok || id != "task-3" {
			t.Errorf("expected task-3 via reverse substring match, got %q (found=%v)", id, ok)
		}
	})

	t.Run("when the dependency matches nothing, it reports not found", func(t *testing.T) {
		if _, ok := idx.resolve("completely unrelated feature"); ok {
			t.Error("expected no match for unrelated dependency")
		}
	})

	t.Run("when the dependency normalizes to empty, it reports not found", func(t *testing.T) {
		if _, ok := idx.resolve("!!! ---"); ok {
			t.Error("expected no match for dependency that normalizes to empty")
		}
	})

	t.Run("when two tasks could match, the first task in order wins", func(t *testing.T) {
		dup := []domain.Task{
			{ID: "first", Title: "Shared Title"},
			{ID: "second", Title: "Shared Title"},
		}
		id, ok := newDependencyIndex(dup).resolve("Shared Title")
		if !ok || id != "first" {
			t.Errorf("expected first task to win, got %q (found=%v)", id, ok)
		}
	})
}

func TestScheduler_FuzzyMatchingThroughIndex(t *testing.T) {
	t.Run("when a dependency uses punctuation variations, GetReadyTasks still resolves it", func(t *testing.T) {
		state := &domain.State{
			Tasks: []domain.Task{
				{ID: "task-A", Title: "Setup Project Skeleton", Status: domain.TaskSuccess},
				{ID: "task-B", Title: "Add Feature", Status: domain.TaskPending, DependsOn: []string{"setup_project_skeleton"}},
			},
		}
		ready := NewScheduler(NewFileLockRegistry()).GetReadyTasks(state, 3)
		if len(ready) != 1 || ready[0].ID != "task-B" {
			t.Fatalf("expected task-B to be ready via normalized dependency, got %v", ready)
		}
	})

	t.Run("when a dependency cannot be resolved, the dependent task is not ready", func(t *testing.T) {
		state := &domain.State{
			Tasks: []domain.Task{
				{ID: "task-A", Title: "Setup", Status: domain.TaskSuccess},
				{ID: "task-B", Title: "Add Feature", Status: domain.TaskPending, DependsOn: []string{"nonexistent zebra dependency"}},
			},
		}
		ready := NewScheduler(NewFileLockRegistry()).GetReadyTasks(state, 3)
		for _, r := range ready {
			if r.ID == "task-B" {
				t.Error("task-B must not be ready when its dependency is unresolved")
			}
		}
	})
}
