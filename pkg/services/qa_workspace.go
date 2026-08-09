package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// GitReviewWorkspaceFactory creates three worktrees transactionally.
type GitReviewWorkspaceFactory struct {
	git   ReviewGitRunner
	fs    QAFileSystem
	clock QAClock
	mu    sync.Mutex
	owned map[string]*reviewWorkspaceOwnership
}

type reviewWorkspaceOwnership struct {
	repository   string
	root         string
	worktreeDone bool
	pathDone     bool
	branchDone   bool
}

var _ ReviewWorkspaceFactory = (*GitReviewWorkspaceFactory)(nil)

func NewGitReviewWorkspaceFactory(git ReviewGitRunner, fsys QAFileSystem, clock QAClock) *GitReviewWorkspaceFactory {
	return &GitReviewWorkspaceFactory{git: git, fs: fsys, clock: clock,
		owned: make(map[string]*reviewWorkspaceOwnership)}
}

func (f *GitReviewWorkspaceFactory) Create(ctx context.Context, repositoryPath, sourceCommit string) (
	ReviewWorkspace, ReviewWorkspace, ReviewWorkspace, error,
) {
	if f.git == nil || f.fs == nil || f.clock == nil {
		return ReviewWorkspace{}, ReviewWorkspace{}, ReviewWorkspace{}, errors.New("review workspace: missing dependency")
	}
	repositoryPath, err := f.fs.Abs(repositoryPath)
	if err != nil || strings.TrimSpace(sourceCommit) == "" {
		return ReviewWorkspace{}, ReviewWorkspace{}, ReviewWorkspace{}, fmt.Errorf("review workspace: invalid source: %w", err)
	}
	resolvedCommit, err := f.git.Run(ctx, repositoryPath, "rev-parse", "--verify", sourceCommit+"^{commit}")
	if err != nil {
		return ReviewWorkspace{}, ReviewWorkspace{}, ReviewWorkspace{}, fmt.Errorf("review workspace: resolve commit: %w", err)
	}
	resolvedCommit = strings.TrimSpace(resolvedCommit)
	expectedManifest, err := f.git.Run(ctx, repositoryPath, "ls-tree", "-r", "--full-tree", resolvedCommit)
	if err != nil {
		return ReviewWorkspace{}, ReviewWorkspace{}, ReviewWorkspace{}, fmt.Errorf("review workspace: source manifest unavailable: %w", err)
	}
	stamp := f.clock.Now().UTC().Format("20060102T150405.000000000")
	shortCommit := resolvedCommit
	if len(shortCommit) > 12 {
		shortCommit = shortCommit[:12]
	}
	root := filepath.Join(filepath.Dir(repositoryPath), ".noctifab-review-"+
		filepath.Base(repositoryPath)+"-"+shortCommit+"-"+stamp)
	if nestedPath(repositoryPath, root) || nestedPath(root, repositoryPath) || root == repositoryPath {
		return ReviewWorkspace{}, ReviewWorkspace{}, ReviewWorkspace{}, errors.New("review workspace: review root must be distinct and non-nested from repository")
	}
	if err := f.fs.MkdirAll(root, 0o700); err != nil {
		return ReviewWorkspace{}, ReviewWorkspace{}, ReviewWorkspace{}, fmt.Errorf("review workspace: create root: %w", err)
	}
	roles := []string{"build", "tester", "qa"}
	created := make([]ReviewWorkspace, 0, len(roles))
	for _, role := range roles {
		workspace := ReviewWorkspace{
			Path:   filepath.Join(root, role),
			Branch: fmt.Sprintf("noctifab/review/%s-%s-%s", shortCommit, strings.ReplaceAll(stamp, ".", ""), role),
		}
		f.remember(workspace.Path, repositoryPath, root)
		created = append(created, workspace)
		_, addErr := f.git.Run(ctx, repositoryPath, "worktree", "add", "--detach", workspace.Path, resolvedCommit)
		if addErr == nil {
			_, addErr = f.git.Run(ctx, workspace.Path, "switch", "-c", workspace.Branch)
		}
		if addErr == nil {
			addErr = f.verifyWorkspace(ctx, workspace, resolvedCommit, expectedManifest)
		}
		if addErr != nil {
			if cleanErr := f.Cleanup(ctx, created...); cleanErr != nil {
				addErr = errors.Join(addErr, fmt.Errorf("cleanup after create %s: %w", role, cleanErr))
			}
			var partial [3]ReviewWorkspace
			copy(partial[:], created)
			return partial[0], partial[1], partial[2], fmt.Errorf("review workspace: create %s: %w", role, addErr)
		}
	}
	return created[0], created[1], created[2], nil
}

func (f *GitReviewWorkspaceFactory) verifyWorkspace(ctx context.Context, workspace ReviewWorkspace,
	commit, expectedManifest string,
) error {
	head, err := f.git.Run(ctx, workspace.Path, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(head) != commit {
		return fmt.Errorf("verify commit: expected %s, got %s: %w", commit, strings.TrimSpace(head), err)
	}
	status, err := f.git.Run(ctx, workspace.Path, "status", "--porcelain", "--untracked-files=no")
	if err != nil || strings.TrimSpace(status) != "" {
		return fmt.Errorf("verify clean tree: %s: %w", strings.TrimSpace(status), err)
	}
	manifest, err := f.git.Run(ctx, workspace.Path, "ls-tree", "-r", "--full-tree", "HEAD")
	if err != nil {
		return fmt.Errorf("verify tracked manifest: read workspace: %w", err)
	}
	if manifest != expectedManifest {
		return errors.New("verify tracked manifest: workspace differs from source commit")
	}
	if err := f.verifyTrackedContent(ctx, workspace.Path, expectedManifest); err != nil {
		return err
	}
	return nil
}

func (f *GitReviewWorkspaceFactory) verifyTrackedContent(ctx context.Context, workspacePath, manifest string) error {
	for _, line := range strings.Split(strings.TrimSuffix(manifest, "\n"), "\n") {
		if line == "" {
			continue
		}
		metadata, path, ok := strings.Cut(line, "\t")
		fields := strings.Fields(metadata)
		if !ok || len(fields) != 3 {
			return errors.New("verify tracked manifest: malformed ls-tree output")
		}
		if fields[1] == "commit" {
			continue
		}
		hash, err := f.git.Run(ctx, workspacePath, "hash-object", "--", path)
		if err != nil || strings.TrimSpace(hash) != fields[2] {
			return fmt.Errorf("verify tracked manifest: content differs for %q: %w", path, err)
		}
	}
	return nil
}

func (f *GitReviewWorkspaceFactory) Cleanup(ctx context.Context, workspaces ...ReviewWorkspace) error {
	var errs []error
	repositories := make(map[string]struct{})
	roots := make(map[string]struct{})
	for _, workspace := range workspaces {
		ownership := f.ownership(workspace.Path)
		if ownership == nil {
			errs = append(errs, fmt.Errorf("review workspace: unknown workspace %q", workspace.Path))
			continue
		}
		repository := ownership.repository
		repositories[repository] = struct{}{}
		roots[ownership.root] = struct{}{}
		if !ownership.worktreeDone {
			if _, err := f.git.Run(ctx, repository, "worktree", "remove", "--force", workspace.Path); err != nil {
				errs = append(errs, fmt.Errorf("remove worktree %s: %w", workspace.Path, err))
			} else {
				f.markDone(workspace.Path, "worktree")
				ownership.worktreeDone = true
			}
		}
		if ownership.worktreeDone && !ownership.pathDone {
			if err := f.fs.RemoveAll(workspace.Path); err != nil {
				errs = append(errs, fmt.Errorf("remove workspace path %s: %w", workspace.Path, err))
			} else {
				f.markDone(workspace.Path, "path")
			}
		}
		if ownership.worktreeDone && workspace.Branch != "" && !ownership.branchDone {
			if _, err := f.git.Run(ctx, repository, "branch", "-D", workspace.Branch); err != nil {
				errs = append(errs, fmt.Errorf("remove branch %s: %w", workspace.Branch, err))
			} else {
				f.markDone(workspace.Path, "branch")
			}
		} else if ownership.worktreeDone && workspace.Branch == "" {
			f.markDone(workspace.Path, "branch")
		}
	}
	for repository := range repositories {
		if _, err := f.git.Run(ctx, repository, "worktree", "prune"); err != nil {
			errs = append(errs, fmt.Errorf("prune worktrees: %w", err))
		}
	}
	if len(errs) == 0 {
		for root := range roots {
			if err := f.fs.RemoveAll(root); err != nil {
				errs = append(errs, fmt.Errorf("remove review root %s: %w", root, err))
			}
		}
	}
	if len(errs) == 0 {
		for _, workspace := range workspaces {
			f.forget(workspace.Path)
		}
	}
	return errors.Join(errs...)
}

func (f *GitReviewWorkspaceFactory) remember(path, repository, root string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.owned[path] = &reviewWorkspaceOwnership{repository: repository, root: root}
}

func (f *GitReviewWorkspaceFactory) ownership(path string) *reviewWorkspaceOwnership {
	f.mu.Lock()
	defer f.mu.Unlock()
	if owned := f.owned[path]; owned != nil {
		copy := *owned
		return &copy
	}
	return nil
}

func (f *GitReviewWorkspaceFactory) markDone(path, operation string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	owned := f.owned[path]
	if owned == nil {
		return
	}
	switch operation {
	case "worktree":
		owned.worktreeDone = true
	case "path":
		owned.pathDone = true
	case "branch":
		owned.branchDone = true
	}
}

func (f *GitReviewWorkspaceFactory) forget(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.owned, path)
}
