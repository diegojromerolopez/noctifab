package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func (o *Orchestrator) syncWorkspaceFiles(ctx context.Context, state *domain.State) error {
	var files []domain.FileInfo

	// Try git-aware scanning first
	out, err := o.git.Run(ctx, false, "ls-files", "-co", "--exclude-standard")
	if err == nil {
		lines := strings.Split(out, "\n")
		for _, rel := range lines {
			rel = strings.TrimSpace(rel)
			if rel == "" {
				continue
			}
			parts := strings.Split(rel, string(filepath.Separator))
			ignored := false
			for _, part := range parts {
				if part == ".noctifab" || part == ".git" || part == "node_modules" || part == "vendor" {
					ignored = true
					break
				}
			}
			if ignored {
				continue
			}
			abs := filepath.Join(state.ProjectPath, rel)
			info, err := os.Stat(abs)
			if err == nil && !info.IsDir() {
				files = append(files, domain.FileInfo{
					Path:         rel,
					Size:         info.Size(),
					LastModified: info.ModTime(),
				})
			}
		}
		state.Files = files
		return o.repo.Save(ctx, state)
	}

	// Fallback to WalkDir if git command fails or is not a repo
	err = filepath.WalkDir(state.ProjectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(state.ProjectPath, path)
		if err != nil {
			return nil
		}
		if rel == "." || rel == ".." {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		for _, part := range parts {
			if part == ".noctifab" || part == ".git" || part == "node_modules" || part == "vendor" {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				files = append(files, domain.FileInfo{
					Path:         rel,
					Size:         info.Size(),
					LastModified: info.ModTime(),
				})
			}
		}
		return nil
	})

	if err != nil {
		return err
	}
	state.Files = files
	return o.repo.Save(ctx, state)
}

func (o *Orchestrator) allTasksFinished(state *domain.State) bool {
	if len(state.Tasks) == 0 {
		return false
	}
	for _, t := range state.Tasks {
		if t.Status != domain.TaskSuccess && t.Status != domain.TaskFailed {
			return false
		}
	}
	return true
}

// allTasksSucceeded reports whether every task in the state has a TaskSuccess
// status. It is used to decide whether the story build is PASSING (all tasks
// passed test validation) or FAILING (one or more tasks exhausted their
// retries without a passing build).
func (o *Orchestrator) allTasksSucceeded(state *domain.State) bool {
	if len(state.Tasks) == 0 {
		return false
	}
	for _, t := range state.Tasks {
		if t.Status != domain.TaskSuccess {
			return false
		}
	}
	return true
}
