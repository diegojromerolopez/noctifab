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

func TestResolveTaskDependencies_NonExistentStoryDependencyFails(t *testing.T) {
	tmpDir := t.TempDir()
	tasks := []domain.Task{
		{ID: "task-1", Title: "Core counting", DependsOn: []string{"US-999"}},
	}

	_, err := ResolveTaskDependencies(tasks, tmpDir)
	if err == nil {
		t.Fatal("expected error for non-existent story dependency, got nil")
	}
}

func TestResolveTaskDependencies_UnknownTaskDependencyFails(t *testing.T) {
	tmpDir := t.TempDir()
	tasks := []domain.Task{
		{ID: "task-1", Title: "Core counting", DependsOn: []string{"non-existent-task-id"}},
	}

	_, err := ResolveTaskDependencies(tasks, tmpDir)
	if err == nil {
		t.Fatal("expected error for unknown task dependency, got nil")
	}
}
