package services

import (
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ResolveAndSortTasks validates dependencies, checks for cycles, and returns tasks in topological order.
func ResolveAndSortTasks(tasks []domain.Task) ([]domain.Task, error) {
	idMap := make(map[string]string)
	titleMap := make(map[string]string)
	for _, t := range tasks {
		if existingID, exists := titleMap[t.Title]; exists && existingID != t.ID {
			return nil, fmt.Errorf("duplicate task title found: %s", t.Title)
		}
		titleMap[t.Title] = t.ID
		idMap[t.ID] = t.ID
	}

	adj := make(map[string][]string)
	taskMap := make(map[string]domain.Task)
	for _, t := range tasks {
		taskMap[t.ID] = t
		var deps []string
		for _, dep := range t.DependsOn {
			if id, exists := titleMap[dep]; exists {
				deps = append(deps, id)
			} else if id, exists := idMap[dep]; exists {
				deps = append(deps, id)
			} else if isTaskReference(dep) && ExtractStoryID(dep) != "" {
				// External cross-story task dependency; resolved by global task scheduler.
				continue
			} else {
				return nil, fmt.Errorf("task '%s' depends on unresolved prerequisite '%s'", t.Title, dep)
			}
		}
		adj[t.ID] = deps
	}

	visited := make(map[string]int) // 0 = unvisited, 1 = visiting, 2 = visited
	var order []string
	var dfs func(node string) error
	dfs = func(node string) error {
		visited[node] = 1
		for _, dep := range adj[node] {
			if visited[dep] == 1 {
				return fmt.Errorf("cycle detected in task DAG: circular reference containing %s", node)
			}
			if visited[dep] == 0 {
				if err := dfs(dep); err != nil {
					return err
				}
			}
		}
		visited[node] = 2
		order = append(order, node)
		return nil
	}

	for _, t := range tasks {
		if visited[t.ID] == 0 {
			if err := dfs(t.ID); err != nil {
				return nil, err
			}
		}
	}

	// Reconstruct sorted task slice
	sorted := make([]domain.Task, 0, len(tasks))
	for _, id := range order {
		sorted = append(sorted, taskMap[id])
	}
	return sorted, nil
}
