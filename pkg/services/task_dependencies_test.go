package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
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
	roadmapDir := filepath.Join(tmpDir, "roadmap")
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
