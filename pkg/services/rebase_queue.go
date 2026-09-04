package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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

func (g *GitClient) Dir() string {
	if g == nil {
		return ""
	}
	return g.dir
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

const defaultStaleLockThreshold = 60 * time.Second

// isProcessAlive checks whether a process with the given PID is actively running.
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}

// isLockFileStale determines if a git lock file is stale. If the lock contains a PID,
// it checks whether the process is alive; if dead, the lock is stale.
// If the lock contains no PID or cannot be parsed, it falls back to the threshold (default 60s).
func isLockFileStale(path string, info os.FileInfo, fallbackThreshold time.Duration) bool {
	if info == nil {
		return false
	}
	// Check if the lock file contains a valid process PID
	if data, err := os.ReadFile(path); err == nil {
		trimmed := strings.TrimSpace(string(data))
		if pid, parseErr := strconv.Atoi(trimmed); parseErr == nil && pid > 0 {
			if !isProcessAlive(pid) {
				return true
			}
			// Process is actively running; the lock is definitely active.
			return false
		}
	}

	if fallbackThreshold <= 0 {
		fallbackThreshold = defaultStaleLockThreshold
	}
	return time.Since(info.ModTime()) > fallbackThreshold
}

// CleanStaleLocks purges stale Git lock files (.git/index.lock, .git/worktrees/*/*.lock)
// using process liveness detection and a 60-second fallback threshold to prevent
// race conditions during active git commands while clearing orphaned locks.
func (g *GitClient) CleanStaleLocks(ctx context.Context) {
	g.CleanStaleLocksWithThreshold(ctx, defaultStaleLockThreshold)
}

// CleanStaleLocksWithThreshold purges stale Git lock files with a custom fallback age threshold.
func (g *GitClient) CleanStaleLocksWithThreshold(ctx context.Context, fallbackThreshold time.Duration) {
	if g == nil || g.dir == "" {
		return
	}
	gitDir := filepath.Join(g.dir, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		idxLock := filepath.Join(gitDir, "index.lock")
		if lockInfo, err := os.Stat(idxLock); err == nil {
			if isLockFileStale(idxLock, lockInfo, fallbackThreshold) {
				_ = os.Remove(idxLock)
			}
		}
		wtDir := filepath.Join(gitDir, "worktrees")
		_ = filepath.Walk(wtDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".lock") {
				if isLockFileStale(path, info, fallbackThreshold) {
					_ = os.Remove(path)
				}
			}
			return nil
		})
	}
}

// OptimisticUnionMerge combines unique lines from baseContent and workerContent preserving ordering.
func OptimisticUnionMerge(baseContent, workerContent string) string {
	baseLines := strings.Split(baseContent, "\n")
	workerLines := strings.Split(workerContent, "\n")
	seen := make(map[string]bool)
	var merged []string
	for _, l := range baseLines {
		if !seen[l] || strings.TrimSpace(l) == "" {
			seen[l] = true
			merged = append(merged, l)
		}
	}
	for _, l := range workerLines {
		if !seen[l] {
			seen[l] = true
			merged = append(merged, l)
		}
	}
	return strings.Join(merged, "\n")
}

func (q *RebaseQueue) executeRebase(ctx context.Context, branch, base string) error {
	q.git.CleanStaleLocks(ctx)

	// Stash any uncommitted work first
	stashed := false
	out, err := q.git.Run(ctx, true, "stash", "--include-untracked")
	if err == nil && !strings.Contains(out, "No local changes to save") {
		stashed = true
	}

	defer func() {
		if stashed {
			_, _ = q.git.Run(ctx, true, "stash", "pop")
		}
	}()

	// Checkout base
	_, err = q.git.Run(ctx, true, "checkout", base)
	if err != nil {
		return fmt.Errorf("failed to checkout base %s: %w", base, err)
	}

	// Tier 1: Non-interactive merge
	_, err = q.git.Run(ctx, true, "merge", "--no-ff", "-m", fmt.Sprintf("merge: incorporate worker branch %s", branch), branch)
	if err == nil {
		fmt.Printf("🔀 [Git Integration Merged] Branch %q merged cleanly into %q (Tier 1)\n", branch, base)
		return nil
	}

	// Merge conflict encountered: proceed through resilient tiers
	fmt.Printf("⚠️  [Git Merge Conflict] Branch %q encountered conflict merging into %q. Starting fallback tiers...\n", branch, base)

	// Tier 2: Deterministic conflict marker resolution
	outDiff, _ := q.git.Run(ctx, false, "diff", "--name-only", "--diff-filter=U")
	conflictedFiles := strings.Fields(strings.TrimSpace(outDiff))
	if len(conflictedFiles) > 0 {
		cleanedAll := true
		for _, file := range conflictedFiles {
			fullPath, err := resolveSandboxPath(q.git.dir, file)
			if err == nil {
				if content, err := os.ReadFile(fullPath); err == nil {
					cleaned := CleanConflictMarkers(string(content))
					_ = os.WriteFile(fullPath, []byte(cleaned), 0644)
				} else {
					cleanedAll = false
				}
			} else {
				cleanedAll = false
			}
		}
		if cleanedAll {
			_, _ = q.git.Run(ctx, true, "add", "--all")
			if _, commitErr := q.git.Run(ctx, true, "commit", "-m", fmt.Sprintf("merge: auto-resolved conflict markers for %s", branch)); commitErr == nil {
				fmt.Printf("✨ [Conflict Resolved] Deterministic marker cleaner resolved conflicts for %q (Tier 2)\n", branch)
				return nil
			}
		}
	}

	// Tier 3: Whole-File Dual Reimplementation by Generator Agent (LLM synthesis)
	if q.resolver != nil {
		fmt.Printf("🤖 [Conflict Resolver] Invoking Generator Agent for whole-file feature synthesis between %q and %q (Tier 3)...\n", branch, base)
		if resolveErr := q.resolver(ctx, branch, base); resolveErr == nil {
			_, _ = q.git.Run(ctx, true, "add", "--all")
			if _, contErr := q.git.Run(ctx, true, "commit", "-m", fmt.Sprintf("merge: synthesized dual-file features for %s", branch)); contErr == nil {
				fmt.Printf("✨ [Conflict Resolved] Generator Agent successfully synthesized features between %q and %q (Tier 3)\n", branch, base)
				return nil
			}
		}
	}

	// Tier 4: Optimistic Union Overwrite
	fmt.Printf("⚠️  [Conflict Fallback] Performing optimistic union overwrite for branch %q (Tier 4)...\n", branch)
	outDiff, _ = q.git.Run(ctx, false, "diff", "--name-only", "--diff-filter=U")
	for _, file := range strings.Fields(strings.TrimSpace(outDiff)) {
		fullPath, err := resolveSandboxPath(q.git.dir, file)
		if err == nil {
			baseContent, _ := q.git.Run(ctx, false, "show", fmt.Sprintf("HEAD:%s", file))
			workerContent, _ := q.git.Run(ctx, false, "show", fmt.Sprintf("%s:%s", branch, file))
			union := OptimisticUnionMerge(baseContent, workerContent)
			_ = os.WriteFile(fullPath, []byte(union), 0644)
		}
	}
	_, _ = q.git.Run(ctx, true, "add", "--all")
	if _, commitErr := q.git.Run(ctx, true, "commit", "-m", fmt.Sprintf("merge: optimistic union for %s", branch)); commitErr == nil {
		fmt.Printf("✨ [Conflict Resolved] Optimistic union overwrite resolved conflicts for %q (Tier 4)\n", branch)
		return nil
	}

	// Tier 5: Direct Diff Patch Overlay & Forced Commit (Last Resort)
	fmt.Printf("⚠️  [Conflict Last Resort] Applying direct overlay fallback for branch %q (Tier 5)...\n", branch)
	_, _ = q.git.Run(ctx, true, "merge", "--abort")
	_, _ = q.git.Run(ctx, true, "checkout", base)
	// Checkout files from branch directly
	_, _ = q.git.Run(ctx, true, "checkout", branch, "--", ".")
	_, _ = q.git.Run(ctx, true, "add", "-A")
	if _, forceCommitErr := q.git.Run(ctx, true, "commit", "-m", fmt.Sprintf("feat: forced overlay merge of branch %s [fallback tier 5]", branch)); forceCommitErr == nil {
		fmt.Printf("✨ [Conflict Resolved] Tier 5 Direct Overlay merge succeeded for %q\n", branch)
		return nil
	}

	fmt.Fprintf(os.Stderr, "⚠️ [RebaseQueue] All merge tiers failed for branch %q.\n", branch)
	return fmt.Errorf("all merge fallback tiers (1-5) failed to integrate branch %s into %s", branch, base)
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
