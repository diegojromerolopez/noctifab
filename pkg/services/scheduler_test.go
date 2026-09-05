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

func TestScheduler_CrossStoryPipelining(t *testing.T) {
	t.Run("downstream story task unblocks immediately when explicit prerequisite task succeeds even if other upstream tasks are pending", func(t *testing.T) {
		// US-001 has two tasks:
		// U1-T1 (Walking Skeleton / Shared Types)
		// U1-T2 (Internal Engine Implementation, depends on U1-T1)
		// US-002 has:
		// U2-T1 (CLI Subsystem, depends explicitly on U1-T1)
		//
		// When U1-T1 succeeds, BOTH U1-T2 and U2-T1 must become ready simultaneously!
		state := &domain.State{
			Tasks: []domain.Task{
				{ID: "US-001-TASK-001", Title: "Walking Skeleton & Types", Status: domain.TaskPending, StoryID: "US-001"},
				{ID: "US-001-TASK-002", Title: "Core Engine Logic", Status: domain.TaskPending, DependsOn: []string{"US-001-TASK-001"}, StoryID: "US-001"},
				{ID: "US-002-TASK-001", Title: "CLI Subsystem", Status: domain.TaskPending, DependsOn: []string{"US-001-TASK-001"}, StoryID: "US-002"},
			},
		}

		scheduler := NewScheduler(NewFileLockRegistry())

		// Initially, only US-001-TASK-001 is ready
		ready := scheduler.GetReadyTasks(state, 5)
		if len(ready) != 1 || ready[0].ID != "US-001-TASK-001" {
			t.Fatalf("expected only US-001-TASK-001 initially ready, got %v", ready)
		}

		// Mark US-001-TASK-001 as SUCCESS
		state.Tasks[0].Status = domain.TaskSuccess

		// Now, BOTH US-001-TASK-002 and US-002-TASK-001 must be ready!
		ready = scheduler.GetReadyTasks(state, 5)
		if len(ready) != 2 {
			t.Fatalf("expected exactly 2 ready tasks (US-001-TASK-002 and US-002-TASK-001), got %d", len(ready))
		}

		readyIDs := make(map[string]bool)
		for _, task := range ready {
			readyIDs[task.ID] = true
		}
		if !readyIDs["US-001-TASK-002"] || !readyIDs["US-002-TASK-001"] {
			t.Errorf("expected both US-001-TASK-002 and US-002-TASK-001 to be ready, got %v", ready)
		}
	})
}

func TestScheduler_FileLockContention(t *testing.T) {
	t.Run("two independent tasks targeting the same file cannot both be dispatched in the same tick", func(t *testing.T) {
		state := &domain.State{
			Tasks: []domain.Task{
				{ID: "task-1", Title: "Task 1", Status: domain.TaskPending, TargetFiles: []string{"pkg/server.go"}},
				{ID: "task-2", Title: "Task 2", Status: domain.TaskPending, TargetFiles: []string{"pkg/server.go"}},
			},
		}

		scheduler := NewScheduler(NewFileLockRegistry())
		ready := scheduler.GetReadyTasks(state, 5)

		// Only one task can acquire the lock for pkg/server.go
		if len(ready) != 1 {
			t.Fatalf("expected exactly 1 ready task due to file lock contention, got %d", len(ready))
		}
		if ready[0].ID != "task-1" {
			t.Errorf("expected task-1 to acquire lock first, got %s", ready[0].ID)
		}

		// Releasing locks allows the second task to proceed
		scheduler.ReleaseLocks("task-1")
		state.Tasks[0].Status = domain.TaskSuccess

		ready2 := scheduler.GetReadyTasks(state, 5)
		if len(ready2) != 1 || ready2[0].ID != "task-2" {
			t.Errorf("expected task-2 to acquire lock after task-1 release, got %v", ready2)
		}
	})
}

func TestScheduler_FailedUpstreamBlocksDownstream(t *testing.T) {
	t.Run("failed upstream task prevents dependent tasks from being dispatched", func(t *testing.T) {
		state := &domain.State{
			Tasks: []domain.Task{
				{ID: "task-1", Title: "Task 1", Status: domain.TaskFailed},
				{ID: "task-2", Title: "Task 2", Status: domain.TaskPending, DependsOn: []string{"task-1"}},
				{ID: "task-3", Title: "Task 3 (Independent)", Status: domain.TaskPending},
			},
		}

		scheduler := NewScheduler(NewFileLockRegistry())
		ready := scheduler.GetReadyTasks(state, 5)

		// task-2 must NOT be ready; only task-3 (and retry of task-1)
		var hasTask2 bool
		for _, task := range ready {
			if task.ID == "task-2" {
				hasTask2 = true
			}
		}
		if hasTask2 {
			t.Errorf("task-2 should not be ready when its upstream task-1 is failed")
		}
	})
}

func TestScheduler_ParseStoryIndex(t *testing.T) {
	tests := []struct {
		storyID  string
		expected int
	}{
		{"US-001", 1},
		{"US-002", 2},
		{"US-042", 42},
		{"US-999", 999},
		{"", 0},
		{"no-numbers", 0},
	}

	for _, tt := range tests {
		got := parseStoryIndex(tt.storyID)
		if got != tt.expected {
			t.Errorf("parseStoryIndex(%q) = %d, want %d", tt.storyID, got, tt.expected)
		}
	}
}
