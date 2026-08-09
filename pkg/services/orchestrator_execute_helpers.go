package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func collectTargetFilesRecursively(task domain.Task, tasks []domain.Task) []string {
	// Build map of ID/Title to Task
	taskMap := make(map[string]domain.Task)
	for _, t := range tasks {
		taskMap[t.ID] = t
		taskMap[t.Title] = t
	}

	visited := make(map[string]bool)
	var files []string
	var visit func(t domain.Task)
	visit = func(t domain.Task) {
		if visited[t.ID] {
			return
		}
		visited[t.ID] = true
		// Add target files of this task
		files = append(files, t.TargetFiles...)
		// Recurse on dependencies
		for _, dep := range t.DependsOn {
			if parent, exists := taskMap[dep]; exists {
				visit(parent)
			}
		}
	}

	visit(task)

	// Deduplicate
	uniqueFiles := make([]string, 0, len(files))
	seen := make(map[string]bool)
	for _, f := range files {
		if !seen[f] {
			seen[f] = true
			uniqueFiles = append(uniqueFiles, f)
		}
	}
	sort.Strings(uniqueFiles)
	return uniqueFiles
}

// syncRootManifests copies root project manifests into a worktree if missing.
func syncRootManifests(srcDir, dstDir string) {
	manifests := []string{
		"Cargo.toml", "Cargo.lock", "package.json", "package-lock.json", "go.mod", "go.sum",
		"Makefile", "CMakeLists.txt", "pyproject.toml", "requirements.txt",
	}
	for _, file := range manifests {
		srcPath, dstPath := filepath.Join(srcDir, file), filepath.Join(dstDir, file)
		if _, err := os.Stat(srcPath); err == nil {
			if _, errDst := os.Stat(dstPath); os.IsNotExist(errDst) {
				if data, readErr := os.ReadFile(srcPath); readErr == nil {
					_ = os.WriteFile(dstPath, data, 0o644)
				}
			}
		}
	}
}

func (o *Orchestrator) updateTaskProgress(ctx context.Context, taskID string, progress int) {
	err := o.updateStateWithRetry(ctx, func(st *domain.State) error {
		for i := range st.Tasks {
			if st.Tasks[i].ID == taskID {
				st.Tasks[i].Progress = progress
				st.Tasks[i].UpdatedAt = time.Now()
				return nil
			}
		}
		return fmt.Errorf("task %s not found in state", taskID)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: failed to persist progress %d%% for task %s: %v\n", progress, taskID, err)
	}
}
