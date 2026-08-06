package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ResolveTaskDependencies validates and resolves dependency identifiers in a planned task DAG.
// Intra-story dependencies matching other task IDs/titles in the DAG are preserved.
// Cross-story dependencies matching valid, existing user story files (e.g., US-001) are marked as satisfied and removed.
// Dependencies referencing non-existent tasks or story files return a descriptive error.
func ResolveTaskDependencies(tasks []domain.Task, projectPath string) ([]domain.Task, error) {
	if len(tasks) == 0 {
		return tasks, nil
	}

	// Build lookup set of current DAG task IDs and titles
	existingTaskIDs := make(map[string]bool)
	for _, t := range tasks {
		if t.ID != "" {
			existingTaskIDs[t.ID] = true
		}
		if t.Title != "" {
			existingTaskIDs[t.Title] = true
		}
	}

	resolvedTasks := make([]domain.Task, len(tasks))
	copy(resolvedTasks, tasks)

	for i := range resolvedTasks {
		task := &resolvedTasks[i]
		var cleanDeps []string

		for _, dep := range task.DependsOn {
			depClean := strings.TrimSpace(dep)
			if depClean == "" {
				continue
			}

			// 1. Direct match with a task in the current DAG
			if existingTaskIDs[depClean] {
				cleanDeps = append(cleanDeps, depClean)
				continue
			}

			// 2. Check if dependency is a user story reference (e.g., "US-001", "US-001.md", "roadmap/US-001.md")
			if isStoryReference(depClean) {
				if storyExists(projectPath, depClean) {
					// Referenced user story exists; prerequisite is satisfied. Omit from active task DAG dependencies.
					continue
				}
				return nil, fmt.Errorf("task '%s' (%s) depends on non-existent user story '%s'", task.ID, task.Title, depClean)
			}

			// 3. Dependency is neither a current task nor a valid story file
			return nil, fmt.Errorf("task '%s' (%s) depends on unknown task or story '%s'", task.ID, task.Title, depClean)
		}

		task.DependsOn = cleanDeps
	}

	return resolvedTasks, nil
}

// isStoryReference checks if a dependency string looks like a user story identifier.
func isStoryReference(dep string) bool {
	upper := strings.ToUpper(dep)
	return strings.HasPrefix(upper, "US-") ||
		strings.Contains(upper, "US-") ||
		strings.HasPrefix(strings.ToLower(dep), "roadmap/")
}

// storyExists checks if the specified user story exists on disk relative to projectPath.
func storyExists(projectPath, dep string) bool {
	clean := strings.TrimSpace(dep)
	candidates := []string{
		clean,
		clean + ".md",
		filepath.Join("roadmap", clean),
		filepath.Join("roadmap", clean+".md"),
	}

	for _, cand := range candidates {
		absPath := cand
		if !filepath.IsAbs(cand) {
			absPath = filepath.Join(projectPath, cand)
		}
		info, err := os.Stat(absPath)
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}
