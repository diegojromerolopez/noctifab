package services

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestValidateTaskCohesion_ValidCohesiveTasks(t *testing.T) {
	tasks := []domain.Task{
		{
			ID:          "task-1",
			Title:       "Define user repository interface and memory implementation",
			Description: "Create user interface and implement memory storage",
			TargetFiles: []string{"pkg/domain/user_interface.go", "pkg/infrastructure/memory/user_repo.go"},
		},
		{
			ID:          "task-2",
			Title:       "Implement HTTP handlers",
			Description: "Add HTTP endpoint handlers for user management",
			TargetFiles: []string{"pkg/services/handler.go"},
		},
	}

	if err := ValidateTaskCohesion(tasks); err != nil {
		t.Fatalf("expected valid task cohesion, got error: %v", err)
	}
}

func TestValidateTaskCohesion_InvalidInterfaceOnlyTask(t *testing.T) {
	tasks := []domain.Task{
		{
			ID:          "task-1",
			Title:       "Define user interface stubs",
			Description: "Create interface definitions for user repo",
			TargetFiles: []string{"pkg/domain/user_interface.go"},
		},
	}

	err := ValidateTaskCohesion(tasks)
	if err == nil {
		t.Fatalf("expected task cohesion validation error for interface-only task, got nil")
	}
}
