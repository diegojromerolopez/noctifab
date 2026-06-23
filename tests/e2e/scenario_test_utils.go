package e2e

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func scanWorkspaceFiles(dir string) ([]domain.FileInfo, error) {
	var files []domain.FileInfo
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".noctifab" || name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, domain.FileInfo{
			Path:         rel,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
		return nil
	})
	return files, err
}

func resolveDependencies(tasks []domain.Task) ([]string, error) {
	idMap := make(map[string]string)
	for _, t := range tasks {
		idMap[t.Title] = t.ID
	}

	adj := make(map[string][]string)
	for _, t := range tasks {
		var deps []string
		for _, dep := range t.DependsOn {
			if id, exists := idMap[dep]; exists {
				deps = append(deps, id)
			} else {
				deps = append(deps, dep)
			}
		}
		adj[t.ID] = deps
	}

	visited := make(map[string]int)
	var order []string
	var dfs func(node string) error
	dfs = func(node string) error {
		visited[node] = 1
		for _, dep := range adj[node] {
			if visited[dep] == 1 {
				return errors.New("cycle detected in task DAG: circular reference")
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
	return order, nil
}
