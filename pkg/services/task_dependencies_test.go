package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
)

func TestResolveTaskDependencies_IntraStoryDependenciesPreserved(t *testing.T) {
	tasks := []domain.Task{
		{ID: "task-1", Title: "Scaffold", DependsOn: nil},
		{ID: "task-2", Title: "Implementation", DependsOn: []string{"task-1"}},
	}

	resolved, err := ResolveTaskDependencies(tasks, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved[1].DependsOn) != 1 || resolved[1].DependsOn[0] != "task-1" {
		t.Errorf("expected task-1 dependency preserved, got: %v", resolved[1].DependsOn)
	}
}

func TestResolveTaskDependencies_ExistingStoryDependencySatisfied(t *testing.T) {
	tmpDir := t.TempDir()
	roadmapDir := filepath.Join(tmpDir, "roadmap", "user-stories")
	if err := os.MkdirAll(roadmapDir, 0755); err != nil {
		t.Fatalf("failed to create roadmap dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roadmapDir, "US-001.md"), []byte("# US-001"), 0644); err != nil {
		t.Fatalf("failed to write US-001.md: %v", err)
	}

	tasks := []domain.Task{
		{ID: "task-1", Title: "Core counting", DependsOn: []string{"US-001"}},
	}

	resolved, err := ResolveTaskDependencies(tasks, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved[0].DependsOn) != 0 {
		t.Errorf("expected US-001 cross-story dependency satisfied and removed, got: %v", resolved[0].DependsOn)
	}
}

func TestResolveTaskDependencies_NonExistentStoryDependencyPruned(t *testing.T) {
	tmpDir := t.TempDir()
	tasks := []domain.Task{
		{ID: "task-1", Title: "Core counting", DependsOn: []string{"US-999"}},
	}

	resolved, err := ResolveTaskDependencies(tasks, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved[0].DependsOn) != 0 {
		t.Errorf("expected non-existent story dependency pruned, got: %v", resolved[0].DependsOn)
	}
}

func TestResolveTaskDependencies_UnknownTaskDependencyPruned(t *testing.T) {
	tmpDir := t.TempDir()
	tasks := []domain.Task{
		{ID: "task-1", Title: "Core counting", DependsOn: []string{"non-existent-task-id"}},
	}

	resolved, err := ResolveTaskDependencies(tasks, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved[0].DependsOn) != 0 {
		t.Errorf("expected unknown task dependency pruned, got: %v", resolved[0].DependsOn)
	}
}

func TestResolveTaskDependencies_OverlappingBuildFilesAutoSerialized(t *testing.T) {
	tasks := []domain.Task{
		{ID: "task-1", Title: "Task 1", TargetFiles: []string{"Makefile", "src/foo.c"}},
		{ID: "task-2", Title: "Task 2", TargetFiles: []string{"Makefile", "src/bar.c"}},
		{ID: "task-3", Title: "Task 3", TargetFiles: []string{"Makefile", "src/baz.c"}},
	}

	resolved, err := ResolveTaskDependencies(tasks, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved[0].DependsOn) != 0 {
		t.Errorf("expected task-1 to have 0 deps, got %v", resolved[0].DependsOn)
	}
	if len(resolved[1].DependsOn) != 1 || resolved[1].DependsOn[0] != "task-1" {
		t.Errorf("expected task-2 to depend on task-1, got %v", resolved[1].DependsOn)
	}
	if len(resolved[2].DependsOn) != 1 || resolved[2].DependsOn[0] != "task-2" {
		t.Errorf("expected task-3 to depend on task-2, got %v", resolved[2].DependsOn)
	}
}

func TestResolveTaskDependencies_IndependentSourceFilesRemainParallel(t *testing.T) {
	tasks := []domain.Task{
		{ID: "task-1", Title: "Task 1", TargetFiles: []string{"src/foo.c", "src/foo.h"}},
		{ID: "task-2", Title: "Task 2", TargetFiles: []string{"src/bar.c", "src/bar.h"}},
	}

	resolved, err := ResolveTaskDependencies(tasks, t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved[0].DependsOn) != 0 {
		t.Errorf("expected task-1 to have 0 deps, got %v", resolved[0].DependsOn)
	}
	if len(resolved[1].DependsOn) != 0 {
		t.Errorf("expected task-2 to have 0 deps (parallel execution), got %v", resolved[1].DependsOn)
	}
}

func TestIsSharedRootBuildFile(t *testing.T) {
	tests := []struct {
		path   string
		expect bool
	}{
		{"Makefile", true},
		{"makefile", true},
		{"./Makefile", true},
		{"pyproject.toml", true},
		{"package.json", true},
		{"CMakeLists.txt", true},
		{"src/Makefile", false},
		{"pkg/services/handler.go", false},
	}

	for _, tt := range tests {
		got := IsSharedRootBuildFile(tt.path)
		if got != tt.expect {
			t.Errorf("IsSharedRootBuildFile(%q) = %v, want %v", tt.path, got, tt.expect)
		}
	}
}

func TestValidatePlannedTasks_CycleDetection(t *testing.T) {
	tasks := []domain.Task{
		{
			ID:          "task-1",
			Title:       "Task 1 Implementation",
			Description: "This is a detailed description of task 1 that is sufficiently long",
			TargetFiles: []string{"pkg/foo.go"},
			DependsOn:   []string{"task-2"},
		},
		{
			ID:          "task-2",
			Title:       "Task 2 Implementation",
			Description: "This is a detailed description of task 2 that is sufficiently long",
			TargetFiles: []string{"pkg/bar.go"},
			DependsOn:   []string{"task-1"},
		},
	}

	err := ValidatePlannedTasks(tasks, t.TempDir())
	if err == nil {
		t.Fatal("expected error due to circular dependencies, got nil")
	}
}

func TestResolveTaskDependencies_CrossStoryTaskDependencyPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	roadmapDir := filepath.Join(tmpDir, "roadmap", "user-stories")
	if err := os.MkdirAll(roadmapDir, 0755); err != nil {
		t.Fatalf("failed to create roadmap dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roadmapDir, "US-001.md"), []byte("# US-001"), 0644); err != nil {
		t.Fatalf("failed to write US-001.md: %v", err)
	}

	tasks := []domain.Task{
		{ID: "US-002-TASK-001", Title: "CLI Implementation", DependsOn: []string{"US-001-TASK-001"}},
	}

	resolved, err := ResolveTaskDependencies(tasks, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resolved[0].DependsOn) != 1 || resolved[0].DependsOn[0] != "US-001-TASK-001" {
		t.Errorf("expected cross-story task dependency US-001-TASK-001 preserved, got: %v", resolved[0].DependsOn)
	}
}

func TestValidatePlannedTasks_CrossStoryDependencyValidates(t *testing.T) {
	tmpDir := t.TempDir()
	roadmapDir := filepath.Join(tmpDir, "roadmap", "user-stories")
	if err := os.MkdirAll(roadmapDir, 0755); err != nil {
		t.Fatalf("failed to create roadmap dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roadmapDir, "US-001.md"), []byte("# US-001"), 0644); err != nil {
		t.Fatalf("failed to write US-001.md: %v", err)
	}

	tasks := []domain.Task{
		{
			ID:          "US-002-TASK-001",
			Title:       "CLI Implementation",
			Description: "Implement CLI argument parsing and commands with sufficient detail",
			TargetFiles: []string{"cmd/cli.go"},
			DependsOn:   []string{"US-001-TASK-001"},
			StoryID:     "US-002",
		},
		{
			ID:          "US-002-TASK-002",
			Title:       "CLI Integration Tests",
			Description: "Integration tests for the CLI commands with sufficient detail",
			TargetFiles: []string{"cmd/cli_test.go"},
			DependsOn:   []string{"US-002-TASK-001"},
			StoryID:     "US-002",
		},
	}

	err := ValidatePlannedTasks(tasks, tmpDir)
	if err != nil {
		t.Fatalf("expected ValidatePlannedTasks to succeed with cross-story dependency, got: %v", err)
	}
}

func TestValidatePlannedTasks_InputValidationErrors(t *testing.T) {
	t.Run("returns error on empty task slice", func(t *testing.T) {
		err := ValidatePlannedTasks(nil, t.TempDir())
		if err == nil {
			t.Fatal("expected error on empty tasks slice, got nil")
		}
		if !strings.Contains(err.Error(), "no tasks were generated") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("returns error when task title is empty", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-1", Title: "", Description: "A valid description of sufficient length"},
		}
		err := ValidatePlannedTasks(tasks, t.TempDir())
		if err == nil {
			t.Fatal("expected error on empty title, got nil")
		}
		if !strings.Contains(err.Error(), "does not have a detailed title or description") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("returns error when task description is too short", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-1", Title: "Valid Title", Description: "Too short"},
		}
		err := ValidatePlannedTasks(tasks, t.TempDir())
		if err == nil {
			t.Fatal("expected error on short description, got nil")
		}
		if !strings.Contains(err.Error(), "does not have a detailed title or description") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("defaults TargetFiles to .gitignore when empty", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-1", Title: "Valid Title", Description: "A valid description of sufficient length", TargetFiles: nil},
		}
		err := ValidatePlannedTasks(tasks, t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tasks[0].TargetFiles) != 1 || tasks[0].TargetFiles[0] != ".gitignore" {
			t.Errorf("expected TargetFiles to default to [.gitignore], got: %v", tasks[0].TargetFiles)
		}
	})
}

func TestResolveTaskDependencies_InputValidationAndEdgeCases(t *testing.T) {
	t.Run("empty task list returns empty list without error", func(t *testing.T) {
		resolved, err := ResolveTaskDependencies(nil, t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resolved) != 0 {
			t.Errorf("expected 0 resolved tasks, got %d", len(resolved))
		}
	})

	t.Run("prunes whitespace and empty dependencies", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-1", Title: "Task 1", DependsOn: []string{"", "   ", "\t"}},
		}
		resolved, err := ResolveTaskDependencies(tasks, t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resolved[0].DependsOn) != 0 {
			t.Errorf("expected empty DependsOn, got: %v", resolved[0].DependsOn)
		}
	})

	t.Run("serializes tasks touching package.json, Cargo.toml, and go.mod", func(t *testing.T) {
		tasks := []domain.Task{
			{ID: "task-1", Title: "Node Config", TargetFiles: []string{"package.json"}},
			{ID: "task-2", Title: "Node Dep", TargetFiles: []string{"package.json"}},
			{ID: "task-3", Title: "Go Config", TargetFiles: []string{"go.mod"}},
			{ID: "task-4", Title: "Go Dep", TargetFiles: []string{"go.mod"}},
		}
		resolved, err := ResolveTaskDependencies(tasks, t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// task-2 must depend on task-1
		if len(resolved[1].DependsOn) != 1 || resolved[1].DependsOn[0] != "task-1" {
			t.Errorf("expected task-2 to depend on task-1, got: %v", resolved[1].DependsOn)
		}
		// task-4 must depend on task-3
		if len(resolved[3].DependsOn) != 1 || resolved[3].DependsOn[0] != "task-3" {
			t.Errorf("expected task-4 to depend on task-3, got: %v", resolved[3].DependsOn)
		}
	})
}

func TestIsReferenceClassifiers(t *testing.T) {
	// isTaskReference and isStoryReference classify dependency syntax
	t.Run("classifies task references", func(t *testing.T) {
		assert.True(t, isTaskReference("US-001-TASK-001"))
		assert.True(t, isTaskReference("TASK-001"))
		assert.True(t, isTaskReference("task-1"))
		assert.False(t, isTaskReference("US-001"))
		assert.False(t, isTaskReference("roadmap/US-001.md"))
		assert.False(t, isTaskReference(""))
	})

	t.Run("classifies story references", func(t *testing.T) {
		assert.True(t, isStoryReference("US-001"))
		assert.True(t, isStoryReference("US-002.md"))
		assert.True(t, isStoryReference("roadmap/user-stories/US-001.md"))
		assert.False(t, isStoryReference("US-001-TASK-001"))
		assert.False(t, isStoryReference("TASK-001"))
		assert.False(t, isStoryReference("arbitrary-feature"))
		assert.False(t, isStoryReference(""))
	})
}
