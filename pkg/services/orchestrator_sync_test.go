package services

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestAllTasksFinished(t *testing.T) {
	o := &Orchestrator{}

	t.Run("empty state returns false", func(t *testing.T) {
		if o.allTasksFinished(&domain.State{}) {
			t.Error("expected false for empty state")
		}
	})

	t.Run("pending task returns false", func(t *testing.T) {
		s := &domain.State{Tasks: []domain.Task{
			{ID: "a", Status: domain.TaskSuccess},
			{ID: "b", Status: domain.TaskPending},
		}}
		if o.allTasksFinished(s) {
			t.Error("expected false with a pending task")
		}
	})

	t.Run("all success or failed returns true", func(t *testing.T) {
		s := &domain.State{Tasks: []domain.Task{
			{ID: "a", Status: domain.TaskSuccess},
			{ID: "b", Status: domain.TaskFailed},
		}}
		if !o.allTasksFinished(s) {
			t.Error("expected true when all tasks are success/failed")
		}
	})
}

func TestAllTasksSucceeded(t *testing.T) {
	o := &Orchestrator{}

	t.Run("empty state returns false", func(t *testing.T) {
		if o.allTasksSucceeded(&domain.State{}) {
			t.Error("expected false for empty state")
		}
	})

	t.Run("all success returns true", func(t *testing.T) {
		s := &domain.State{Tasks: []domain.Task{
			{ID: "a", Status: domain.TaskSuccess},
			{ID: "b", Status: domain.TaskSuccess},
		}}
		if !o.allTasksSucceeded(s) {
			t.Error("expected true when all tasks succeeded")
		}
	})

	t.Run("one failed returns false", func(t *testing.T) {
		s := &domain.State{Tasks: []domain.Task{
			{ID: "a", Status: domain.TaskSuccess},
			{ID: "b", Status: domain.TaskFailed},
		}}
		if o.allTasksSucceeded(s) {
			t.Error("expected false when any task failed")
		}
	})

	t.Run("one pending returns false", func(t *testing.T) {
		s := &domain.State{Tasks: []domain.Task{
			{ID: "a", Status: domain.TaskSuccess},
			{ID: "b", Status: domain.TaskPending},
		}}
		if o.allTasksSucceeded(s) {
			t.Error("expected false when any task is pending")
		}
	})
}
