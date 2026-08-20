package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// defaultGitCommandTimeout bounds each individual git invocation so a hung
// git process (stale index lock, credential prompt, etc.) cannot hold the
// GitClient mutex indefinitely and block all git operations.
const defaultGitCommandTimeout = 2 * time.Minute

// GitClient wraps shell git commands with a centralized RWMutex.
// Every command runs with a per-command timeout (default 2 minutes) and with
// GIT_TERMINAL_PROMPT=0 so git never blocks waiting for credentials.
type GitClient struct {
	mu             sync.RWMutex
	dir            string
	commandTimeout time.Duration
}

// GitClientOption customizes a GitClient created via NewGitClient.
type GitClientOption func(*GitClient)

// WithGitCommandTimeout overrides the per-command timeout. Non-positive
// values keep the 2-minute default.
func WithGitCommandTimeout(d time.Duration) GitClientOption {
	return func(g *GitClient) {
		g.commandTimeout = d
	}
}

func NewGitClient(dir string, opts ...GitClientOption) *GitClient {
	g := &GitClient{
		dir: dir,
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

func (g *GitClient) Run(ctx context.Context, isWrite bool, args ...string) (string, error) {
	if g == nil {
		return "", errors.New("git client is nil")
	}
	if isWrite {
		g.mu.Lock()
		defer g.mu.Unlock()
	} else {
		g.mu.RLock()
		defer g.mu.RUnlock()
	}

	timeout := g.commandTimeout
	if timeout <= 0 {
		timeout = defaultGitCommandTimeout
	}
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := runCommandWithTimeout(ctx, timeout, g.dir, env, "git", args...)
	if err != nil {
		return out, fmt.Errorf("git command failed: %w (output: %s)", err, out)
	}
	return out, nil
}

// runCommandWithTimeout executes a command bounded by both the caller's ctx
// and a per-command timeout, returning combined stdout/stderr output.
func runCommandWithTimeout(ctx context.Context, timeout time.Duration, dir string, env []string, name string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil && cmdCtx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("command timed out after %s: %w", timeout, cmdCtx.Err())
	}
	return string(out), err
}

// RebaseJob represents a request to rebase a branch sequentially
type RebaseJob struct {
	Ctx    context.Context
	Branch string
	Base   string
	Result chan error
}

// GeneratorConflictResolverFunc defines a callback to resolve Git merge/rebase conflicts via the Generator agent.
type GeneratorConflictResolverFunc func(ctx context.Context, branch, base string) error

// RebaseQueue serializes git rebases to prevent index lock contention
type RebaseQueue struct {
	git      *GitClient
	jobs     chan RebaseJob
	started  atomic.Bool
	resolver GeneratorConflictResolverFunc
}

func NewRebaseQueue(git *GitClient) *RebaseQueue {
	return &RebaseQueue{
		git:  git,
		jobs: make(chan RebaseJob, 100),
	}
}

// SetConflictResolver configures the conflict resolver callback (invoked on rebase conflicts).
func (q *RebaseQueue) SetConflictResolver(resolver GeneratorConflictResolverFunc) {
	q.resolver = resolver
}

// Start spawns the rebase queue consumer loop
func (q *RebaseQueue) Start(ctx context.Context) {
	q.started.Store(true)
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

// ErrRebaseQueueNotStarted is returned by Push when the consumer loop
// (Start) was never launched, so the job would never be processed.
var ErrRebaseQueueNotStarted = errors.New("rebase queue not started")

func (q *RebaseQueue) Push(ctx context.Context, branch, base string) error {
	if !q.started.Load() {
		return ErrRebaseQueueNotStarted
	}
	res := make(chan error, 1)
	select {
	case <-ctx.Done():
		return fmt.Errorf("rebase queue send cancelled: %w", ctx.Err())
	case q.jobs <- RebaseJob{
		Ctx:    ctx,
		Branch: branch,
		Base:   base,
		Result: res,
	}:
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("rebase queue cancelled or timed out waiting for rebase result on branch %s: %w", branch, ctx.Err())
	case err := <-res:
		return err
	}
}

func (q *RebaseQueue) executeRebase(ctx context.Context, branch, base string) error {
	// Stash any uncommitted work first
	stashed := false
	out, err := q.git.Run(ctx, true, "stash", "--include-untracked")
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
		fmt.Printf("⚠️  [Git Rebase Conflict] Branch %q encountered conflict with %q. Invoking Generator Agent...\n", branch, base)
		resolved := false
		if q.resolver != nil {
			if resolveErr := q.resolver(ctx, branch, base); resolveErr == nil {
				_, _ = q.git.Run(ctx, true, "add", "--all")
				if _, contErr := q.git.Run(ctx, true, "rebase", "--continue"); contErr == nil {
					resolved = true
					fmt.Printf("✨ [Conflict Resolved] Generator Agent successfully resolved merge conflicts between %q and %q\n", branch, base)
				}
			}
		}

		if !resolved {
			// Conflict could not be resolved; abort rebase cleanly
			_, _ = q.git.Run(ctx, true, "rebase", "--abort")
			_, _ = q.git.Run(ctx, true, "checkout", base) // revert to safe base
			if stashed {
				_, _ = q.git.Run(ctx, true, "stash", "pop")
			}
			return fmt.Errorf("rebase conflict detected: %w", err)
		}
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
