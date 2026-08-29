package services

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

func TestScheduler_FuzzyDependencyMatching(t *testing.T) {
	t.Run("resolves dependency with minor naming variations", func(t *testing.T) {
		// task-B depends on "Implement JSON Repository"
		// but the actual task title is "Implement JSON Repository and Unit Test"
		state := &domain.State{
			Tasks: []domain.Task{
				{ID: "task-A", Title: "Implement JSON Repository and Unit Test", Status: domain.TaskSuccess},
				{ID: "task-B", Title: "Implement CLI Add Command Logic", Status: domain.TaskPending, DependsOn: []string{"Implement JSON Repository"}},
			},
		}

		scheduler := NewScheduler(NewFileLockRegistry())
		ready := scheduler.GetReadyTasks(state, 3)

		if len(ready) != 1 {
			t.Fatalf("expected exactly 1 ready task (task-B), got %d", len(ready))
		}

		if ready[0].ID != "task-B" {
			t.Errorf("expected ready task to be task-B, got %s", ready[0].ID)
		}
	})
}

func TestScheduler_StoryMilestoneBarrier(t *testing.T) {
	t.Run("when upstream story US-001 has incomplete tasks, downstream US-002 tasks are blocked", func(t *testing.T) {
		state := &domain.State{
			Tasks: []domain.Task{
				{ID: "task-1", Title: "Core Server Socket", Status: domain.TaskPending, StoryID: "US-001"},
				{ID: "task-2", Title: "Expiration Engine", Status: domain.TaskPending, StoryID: "US-002"},
				{ID: "task-3", Title: "CI Workflow", Status: domain.TaskPending, StoryID: "US-006"},
			},
		}

		scheduler := NewScheduler(NewFileLockRegistry())
		ready := scheduler.GetReadyTasks(state, 5)

		// Only US-001 task should be ready
		if len(ready) != 1 {
			t.Fatalf("expected exactly 1 ready task from US-001, got %d", len(ready))
		}
		if ready[0].ID != "task-1" {
			t.Errorf("expected task-1 to be ready, got %s", ready[0].ID)
		}

		// When task-1 succeeds, US-002 task becomes ready
		state.Tasks[0].Status = domain.TaskSuccess
		ready = scheduler.GetReadyTasks(state, 5)
		if len(ready) != 1 {
			t.Fatalf("expected exactly 1 ready task from US-002, got %d", len(ready))
		}
		if ready[0].ID != "task-2" {
			t.Errorf("expected task-2 to be ready, got %s", ready[0].ID)
		}
	})
}
