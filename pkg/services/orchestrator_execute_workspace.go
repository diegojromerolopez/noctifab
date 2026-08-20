package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ensureIntegrationBranch ensures the integration branch exists in the repository.
// If it does not exist, it creates it from baseBranch.
// If it already exists (e.g. from prior user stories), its commit history is preserved.
func (o *Orchestrator) ensureIntegrationBranch(ctx context.Context, baseBranch, integrationBranch string) error {
	_, err := o.git.Run(ctx, false, "show-ref", "--verify", "--quiet", "refs/heads/"+integrationBranch)
	if err == nil {
		// Integration branch already exists — preserve all existing commits across stories
		return nil
	}

	effectiveBase := ResolveBaseBranch(ctx, o.git, baseBranch)
	if _, err := o.git.Run(ctx, true, "checkout", effectiveBase); err != nil {
		// Fallback self-healing: if base branch was deleted or unavailable, create from HEAD
		if _, errHead := o.git.Run(ctx, true, "checkout", "-b", integrationBranch, "HEAD"); errHead == nil {
			return nil
		}
		return fmt.Errorf("failed to checkout base branch %s: %w", effectiveBase, err)
	}
	if _, err := o.git.Run(ctx, true, "checkout", "-b", integrationBranch); err != nil {
		if _, errVerify := o.git.Run(ctx, false, "show-ref", "--verify", "--quiet", "refs/heads/"+integrationBranch); errVerify != nil {
			return fmt.Errorf("failed to create integration branch %s: %w", integrationBranch, err)
		}
	}
	return nil
}

// setupTaskWorkspace prepares the working directory and GitClient for a task.
// If UseWorktrees is enabled, it creates an isolated worktree branched from integrationBranch.
// Otherwise, it checks out integrationBranch and creates a dedicated worker branch.
func (o *Orchestrator) setupTaskWorkspace(
	ctx context.Context,
	state *domain.State,
	task *domain.Task,
	taskID string,
	baseBranch string,
	integrationBranch string,
	branchName string,
) (worktreeDir string, taskGit *GitClient, cleanup func(), err error) {
	if !o.cfg.UseWorktrees {
		_, _ = o.git.Run(ctx, true, "reset", "--hard")
		_, _ = o.git.Run(ctx, true, "clean", "-fd")
	}

	if err := o.ensureIntegrationBranch(ctx, baseBranch, integrationBranch); err != nil {
		return "", nil, nil, err
	}

	if o.cfg.UseWorktrees {
		worktreeDir = filepath.Join(state.ProjectPath, ".noctifab", "worktrees", fmt.Sprintf("task-%s", taskID))
		_, _ = o.git.Run(ctx, true, "worktree", "remove", "--force", worktreeDir)
		_ = os.RemoveAll(worktreeDir)
		_ = os.MkdirAll(filepath.Dir(worktreeDir), 0755)

		branchExists := false
		if _, err := o.git.Run(ctx, false, "show-ref", "--verify", "--quiet", "refs/heads/"+branchName); err == nil {
			branchExists = true
		}

		if branchExists && task.Retries > 0 {
			_, _ = o.git.Run(ctx, true, "worktree", "add", worktreeDir, branchName)
		} else {
			if branchExists {
				_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
			}
			_, err = o.git.Run(ctx, true, "worktree", "add", "-b", branchName, worktreeDir, integrationBranch)
			if err != nil {
				_, _ = o.git.Run(ctx, true, "worktree", "add", worktreeDir, branchName)
			}
		}
		taskGit = NewGitClient(worktreeDir)
		syncRootManifests(state.ProjectPath, worktreeDir)

		var cleanupOnce sync.Once
		cleanup = func() {
			cleanupOnce.Do(func() {
				_, _ = o.git.Run(ctx, true, "worktree", "remove", "--force", worktreeDir)
				_ = os.RemoveAll(worktreeDir)
				_, _ = o.git.Run(ctx, true, "worktree", "prune")
			})
		}
		return worktreeDir, taskGit, cleanup, nil
	}

	taskGit = o.git
	if _, err := o.git.Run(ctx, true, "checkout", integrationBranch); err != nil {
		return "", nil, nil, fmt.Errorf("failed to checkout integration branch %s: %w", integrationBranch, err)
	}

	branchExists := false
	if _, err := o.git.Run(ctx, false, "show-ref", "--verify", "--quiet", "refs/heads/"+branchName); err == nil {
		branchExists = true
	}

	if branchExists && task.Retries > 0 {
		if _, err := o.git.Run(ctx, true, "checkout", branchName); err != nil {
			return "", nil, nil, fmt.Errorf("failed to checkout existing worker branch %s: %w", branchName, err)
		}
	} else {
		_, _ = o.git.Run(ctx, true, "branch", "-D", branchName)
		if _, err := o.git.Run(ctx, true, "checkout", "-b", branchName); err != nil {
			return "", nil, nil, fmt.Errorf("failed to create worker branch %s: %w", branchName, err)
		}
	}

	cleanup = func() {}
	return state.ProjectPath, taskGit, cleanup, nil
}

// CleanConflictMarkers resolves standard Git conflict markers (<<<<<<<, =======, >>>>>>>)
// by deterministically preserving the incoming worker changes (between ======= and >>>>>>>).
func CleanConflictMarkers(content string) string {
	lines := strings.Split(content, "\n")
	var cleaned []string
	inOurs := false
	inTheirs := false
	for _, line := range lines {
		if strings.HasPrefix(line, "<<<<<<<") {
			inOurs = true
			inTheirs = false
			continue
		}
		if inOurs && strings.HasPrefix(line, "=======") {
			inOurs = false
			inTheirs = true
			continue
		}
		if inTheirs && strings.HasPrefix(line, ">>>>>>>") {
			inOurs = false
			inTheirs = false
			continue
		}
		if inOurs {
			// Skip 'ours' (base) block in favor of incoming 'theirs' (worker)
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.Join(cleaned, "\n")
}

// resolveGitRebaseConflict automatically inspects and resolves Git merge conflicts during rebase.
func (o *Orchestrator) resolveGitRebaseConflict(ctx context.Context, branch, base string) error {
	if o.git == nil {
		return fmt.Errorf("git client is nil")
	}
	out, err := o.git.Run(ctx, false, "diff", "--name-only", "--diff-filter=U")
	if err != nil || strings.TrimSpace(out) == "" {
		return nil
	}

	conflictedFiles := strings.Split(strings.TrimSpace(out), "\n")
	for _, file := range conflictedFiles {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		fullPath, err := resolveSandboxPath(o.git.dir, file)
		if err != nil {
			continue
		}
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		cleaned := CleanConflictMarkers(string(content))
		_ = os.WriteFile(fullPath, []byte(cleaned), 0644)
	}
	_, _ = o.git.Run(ctx, true, "add", "--all")
	return nil
}
