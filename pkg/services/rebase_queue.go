package services

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// GitClient wraps shell git commands with a centralized RWMutex
type GitClient struct {
	mu  sync.RWMutex
	dir string
}

func NewGitClient(dir string) *GitClient {
	return &GitClient{
		dir: dir,
	}
}

func (g *GitClient) Run(ctx context.Context, isWrite bool, args ...string) (string, error) {
	if isWrite {
		g.mu.Lock()
		defer g.mu.Unlock()
	} else {
		g.mu.RLock()
		defer g.mu.RUnlock()
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git command failed: %w (output: %s)", err, string(out))
	}
	return string(out), nil
}

// RebaseJob represents a request to rebase a branch sequentially
type RebaseJob struct {
	Ctx    context.Context
	Branch string
	Base   string
	Result chan error
}

// RebaseQueue serializes git rebases to prevent index lock contention
type RebaseQueue struct {
	git  *GitClient
	jobs chan RebaseJob
}

func NewRebaseQueue(git *GitClient) *RebaseQueue {
	return &RebaseQueue{
		git:  git,
		jobs: make(chan RebaseJob, 100),
	}
}

// Start spawns the rebase queue consumer loop
func (q *RebaseQueue) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-q.jobs:
			err := q.executeRebase(job.Ctx, job.Branch, job.Base)
			job.Result <- err
		}
	}
}

func (q *RebaseQueue) Push(ctx context.Context, branch, base string) error {
	res := make(chan error, 1)
	q.jobs <- RebaseJob{
		Ctx:    ctx,
		Branch: branch,
		Base:   base,
		Result: res,
	}
	return <-res
}

func (q *RebaseQueue) executeRebase(ctx context.Context, branch, base string) error {
	// Stash any uncommitted work first
	stashed := false
	out, err := q.git.Run(ctx, true, "stash")
	if err == nil && !strings.Contains(out, "No local changes to save") {
		stashed = true
	}

	// Checkout branch
	_, err = q.git.Run(ctx, true, "checkout", branch)
	if err != nil {
		if stashed {
			_, _ = q.git.Run(ctx, true, "stash", "pop")
		}
		return err
	}

	// Rebase onto base
	_, err = q.git.Run(ctx, true, "rebase", base)
	if err != nil {
		fmt.Printf("⚠️  [Git Rebase Conflict] Branch %q encountered conflict with %q. Aborting rebase...\n", branch, base)
		// Conflict occurred! Abort rebase
		_, _ = q.git.Run(ctx, true, "rebase", "--abort")
		_, _ = q.git.Run(ctx, true, "checkout", base) // revert to safe base
		if stashed {
			_, _ = q.git.Run(ctx, true, "stash", "pop")
		}
		return fmt.Errorf("rebase conflict detected: %w", err)
	}

	// Checkout base and merge branch to fast-forward local base branch
	_, err = q.git.Run(ctx, true, "checkout", base)
	if err != nil {
		return fmt.Errorf("failed to checkout base: %w", err)
	}
	_, err = q.git.Run(ctx, true, "merge", branch)
	if err != nil {
		return fmt.Errorf("failed to merge branch into base: %w", err)
	}

	fmt.Printf("🔀 [Git Integration Merged] Branch %q merged cleanly into %q\n", branch, base)

	// Pop stash if we stashed
	if stashed {
		_, err = q.git.Run(ctx, true, "stash", "pop")
		if err != nil {
			return fmt.Errorf("failed to pop stash after rebase: %w", err)
		}
	}

	return nil
}

// MockVCSClient stubs remote VCS operations (e.g. creating PRs)
type MockVCSClient struct {
	Repo string
}

func NewMockVCSClient(repo string) *MockVCSClient {
	return &MockVCSClient{Repo: repo}
}

func (c *MockVCSClient) CreatePullRequest(ctx context.Context, title, body, base, head string) (string, error) {
	if title == "" || head == "" {
		return "", errors.New("invalid pull request parameters")
	}
	// Return a simulated PR url
	return fmt.Sprintf("https://github.com/%s/pull/42", c.Repo), nil
}
