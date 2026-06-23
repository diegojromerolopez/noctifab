package usecase

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestScheduler_ClarificationBlocking(t *testing.T) {
	t.Run("global clarification blocks everything", func(t *testing.T) {
		state := &domain.State{
			Tasks: []domain.Task{
				{ID: "task-1", Title: "Task 1", Status: domain.TaskPending},
			},
			Clarifications: []domain.Clarification{
				{ID: "c1", Question: "Global question", Resolved: false},
			},
		}

		scheduler := NewScheduler(NewFileLockRegistry())
		ready := scheduler.GetReadyTasks(state, 3)
		if len(ready) != 0 {
			t.Errorf("expected no ready tasks when global clarification is unresolved, got %d", len(ready))
		}
	})

	t.Run("task-specific clarification blocks only downstream", func(t *testing.T) {
		// DAG: A -> B (B depends on A), and C is independent.
		// A has an unresolved clarification.
		state := &domain.State{
			Tasks: []domain.Task{
				{ID: "task-A", Title: "Task A", Status: domain.TaskPending},
				{ID: "task-B", Title: "Task B", Status: domain.TaskPending, DependsOn: []string{"task-A"}},
				{ID: "task-C", Title: "Task C", Status: domain.TaskPending},
			},
			Clarifications: []domain.Clarification{
				{ID: "c-task-A", Question: "Task A detail?", Resolved: false, TaskID: "task-A"},
			},
		}

		scheduler := NewScheduler(NewFileLockRegistry())

		// Get ready tasks
		ready := scheduler.GetReadyTasks(state, 3)

		// Task A is blocked directly by clarification.
		// Task B is blocked transitively because it depends on Task A.
		// Task C should be ready to execute!
		if len(ready) != 1 {
			t.Fatalf("expected exactly 1 ready task (Task C), got %d", len(ready))
		}

		if ready[0].ID != "task-C" {
			t.Errorf("expected ready task to be task-C, got %s", ready[0].ID)
		}
	})
}
