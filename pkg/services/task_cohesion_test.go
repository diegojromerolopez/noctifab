package services

import (
	"strings"
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

func TestValidateTaskCohesion_InvalidMicroTask(t *testing.T) {
	tasks := []domain.Task{
		{
			ID:          "task-micro",
			Title:       "Define struct CountStats",
			Description: "Create struct",
			TargetFiles: []string{"src/stats.rs"},
		},
	}

	err := ValidateTaskCohesion(tasks)
	if err == nil {
		t.Fatalf("expected task cohesion error for micro-task, got nil")
	}
}

func TestValidateTaskCohesion_InvalidMegaTask_TooManyFiles(t *testing.T) {
	tasks := []domain.Task{
		{
			ID:          "task-mega-files",
			Title:       "Implement various handlers",
			Description: "Implement handlers across many modules with co-located tests",
			TargetFiles: []string{
				"src/cmd1.py", "src/cmd2.py", "src/cmd3.py",
				"src/cmd4.py", "src/cmd5.py", "src/cmd6.py", "src/cmd7.py",
				"tests/unit/test_cmd.py",
			},
		},
	}

	err := ValidateTaskCohesion(tasks)
	if err == nil {
		t.Fatalf("expected error for mega-task targeting 7 production files, got nil")
	}
	if !strings.Contains(err.Error(), "mega-task") || !strings.Contains(err.Error(), "7 production files") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateTaskCohesion_InvalidMegaTask_TooManyCommands(t *testing.T) {
	tasks := []domain.Task{
		{
			ID:          "task-mega-commands",
			Title:       "Implement connection and server commands",
			Description: "Implement AUTH, BGSAVE, BGREWRITEAOF, CLIENT, COMMAND, CONFIG, ECHO, HELLO, INFO, LASTSAVE, and PING in server.py.",
			TargetFiles: []string{"src/server.py", "tests/unit/test_server.py"},
		},
	}

	err := ValidateTaskCohesion(tasks)
	if err == nil {
		t.Fatalf("expected error for mega-task enumerating 11 commands, got nil")
	}
	if !strings.Contains(err.Error(), "mega-task") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestValidateTaskCohesion_ValidTaskWithMarkdownAndOptions(t *testing.T) {
	tasks := []domain.Task{
		{
			ID:    "US-003-TASK-001",
			Title: "Implement String Core Storage and Basic Key Access",
			Description: `### REQUIREMENTS & SCOPE
Implement SET, GET, and DEL command handlers.
Support EX, PX, NX, and XX options for SET command.
MUST return ERR on syntax error, or WRONGTYPE if target key holds a different type.
Co-located tests in tests/unit/commands/test_strings.py asserting EXPECTED output.`,
			TargetFiles: []string{
				"src/commands/strings.py",
				"tests/unit/commands/test_strings.py",
			},
		},
	}

	err := ValidateTaskCohesion(tasks)
	if err != nil {
		t.Fatalf("expected task with markdown headers and options to pass, but got error: %v", err)
	}
}
