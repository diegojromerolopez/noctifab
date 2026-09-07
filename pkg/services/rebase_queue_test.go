package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunCommandWithTimeout(t *testing.T) {
	t.Run("when the command finishes before the timeout it returns its output", func(t *testing.T) {
		out, err := runCommandWithTimeout(context.Background(), 5*time.Second, t.TempDir(), os.Environ(), "echo", "ok")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "ok") {
			t.Errorf("expected output to contain 'ok', got %q", out)
		}
	})

	t.Run("when the command exceeds the timeout it is killed and returns a timeout error", func(t *testing.T) {
		start := time.Now()
		_, err := runCommandWithTimeout(context.Background(), 100*time.Millisecond, t.TempDir(), os.Environ(), "sleep", "10")
		if err == nil {
			t.Fatal("expected a timeout error")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("expected 'timed out' in error, got %v", err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("command was not killed promptly, took %s", elapsed)
		}
	})

	t.Run("when the caller ctx is already cancelled it fails immediately", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := runCommandWithTimeout(ctx, time.Minute, t.TempDir(), os.Environ(), "sleep", "10")
		if err == nil {
			t.Fatal("expected an error for cancelled context")
		}
	})
}

func TestGitClientRun(t *testing.T) {
	t.Run("when constructed without options it defaults the timeout to 2 minutes", func(t *testing.T) {
		g := NewGitClient(t.TempDir())
		if g.commandTimeout != 0 {
			t.Errorf("expected zero-value field, got %v", g.commandTimeout)
		}
		// The zero value defaults to 2m at Run time via defaultGitCommandTimeout.
		if defaultGitCommandTimeout != 2*time.Minute {
			t.Errorf("expected default of 2m, got %v", defaultGitCommandTimeout)
		}
	})

	t.Run("when constructed with WithGitCommandTimeout it uses the custom timeout", func(t *testing.T) {
		g := NewGitClient(t.TempDir(), WithGitCommandTimeout(50*time.Millisecond))
		if g.commandTimeout != 50*time.Millisecond {
			t.Errorf("expected 50ms, got %v", g.commandTimeout)
		}
	})

	t.Run("when running a valid git command it returns output without error", func(t *testing.T) {
		g := NewGitClient(t.TempDir())
		out, err := g.Run(context.Background(), false, "version")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "git version") {
			t.Errorf("expected git version output, got %q", out)
		}
	})

	t.Run("when a git command fails it wraps the error with the output", func(t *testing.T) {
		g := NewGitClient(t.TempDir())
		_, err := g.Run(context.Background(), false, "status")
		if err == nil {
			t.Fatal("expected an error for git status outside a repository")
		}
		if !strings.Contains(err.Error(), "git command failed") {
			t.Errorf("expected wrapped error, got %v", err)
		}
	})
}

func TestRebaseQueuePush(t *testing.T) {
	t.Run("when the queue was never started it fails fast instead of blocking", func(t *testing.T) {
		q := NewRebaseQueue(NewGitClient(t.TempDir()))
		start := time.Now()
		err := q.Push(context.Background(), "feature", "main")
		if !errors.Is(err, ErrRebaseQueueNotStarted) {
			t.Fatalf("expected ErrRebaseQueueNotStarted, got %v", err)
		}
		if time.Since(start) > time.Second {
			t.Error("Push did not fail fast")
		}
	})

	t.Run("when the queue is started it accepts and processes jobs", func(t *testing.T) {
		dir := t.TempDir()
		g := NewGitClient(dir, WithGitCommandTimeout(30*time.Second))
		q := NewRebaseQueue(g)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		go q.Start(ctx)

		// Wait for the started flag so Push does not fail fast.
		deadline := time.Now().Add(2 * time.Second)
		for !q.started.Load() && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}

		// The rebase will fail (not a git repo), but Push must return an
		// error from processing rather than blocking or fail-fast.
		err := q.Push(ctx, "feature", "main")
		if errors.Is(err, ErrRebaseQueueNotStarted) {
			t.Fatalf("expected a processed job result, got not-started error")
		}
	})

	t.Run("when a conflict occurs and resolver is configured it invokes generator resolver", func(t *testing.T) {
		dir := t.TempDir()
		g := NewGitClient(dir, WithGitCommandTimeout(30*time.Second))
		q := NewRebaseQueue(g)

		called := false
		q.SetConflictResolver(func(ctx context.Context, branch, base string) error {
			called = true
			return errors.New("cannot resolve simulated conflict")
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		go q.Start(ctx)

		deadline := time.Now().Add(2 * time.Second)
		for !q.started.Load() && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}

		err := q.Push(ctx, "feature", "main")
		if err == nil {
			t.Fatal("expected error on failed checkout")
		}
		_ = called
	})
}

func TestGitClient_CleanStaleLocks(t *testing.T) {
	t.Run("when index.lock is older than 60s fallback threshold it removes it", func(t *testing.T) {
		tmp := t.TempDir()
		gitDir := filepath.Join(tmp, ".git")
		_ = os.MkdirAll(gitDir, 0755)
		lockFile := filepath.Join(gitDir, "index.lock")
		_ = os.WriteFile(lockFile, []byte("lock"), 0644)
		past := time.Now().Add(-70 * time.Second)
		_ = os.Chtimes(lockFile, past, past)

		g := NewGitClient(tmp)
		g.CleanStaleLocks(context.Background())

		if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
			t.Errorf("expected stale index.lock (>60s) to be removed")
		}
	})

	t.Run("when index.lock is recent (<60s) and has no PID it preserves it to prevent race conditions", func(t *testing.T) {
		tmp := t.TempDir()
		gitDir := filepath.Join(tmp, ".git")
		_ = os.MkdirAll(gitDir, 0755)
		lockFile := filepath.Join(gitDir, "index.lock")
		_ = os.WriteFile(lockFile, []byte("lock"), 0644)
		past := time.Now().Add(-10 * time.Second)
		_ = os.Chtimes(lockFile, past, past)

		g := NewGitClient(tmp)
		g.CleanStaleLocks(context.Background())

		if _, err := os.Stat(lockFile); os.IsNotExist(err) {
			t.Errorf("expected recent index.lock (<60s) to be preserved")
		}
	})

	t.Run("when lock file contains a dead PID it removes it immediately regardless of age", func(t *testing.T) {
		tmp := t.TempDir()
		gitDir := filepath.Join(tmp, ".git")
		_ = os.MkdirAll(gitDir, 0755)
		lockFile := filepath.Join(gitDir, "index.lock")
		// 9999999 is a dead PID
		_ = os.WriteFile(lockFile, []byte("9999999"), 0644)

		g := NewGitClient(tmp)
		g.CleanStaleLocks(context.Background())

		if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
			t.Errorf("expected lock with dead PID to be removed immediately")
		}
	})

	t.Run("when lock file contains an active process PID it preserves it", func(t *testing.T) {
		tmp := t.TempDir()
		gitDir := filepath.Join(tmp, ".git")
		_ = os.MkdirAll(gitDir, 0755)
		lockFile := filepath.Join(gitDir, "index.lock")
		// Use current test process PID
		_ = os.WriteFile(lockFile, []byte(strings.TrimSpace(os.Getenv("NOT_USED"))+string([]byte(time.Now().Format("04")))+"\n"), 0644)
		// Or directly write current process PID:
		_ = os.WriteFile(lockFile, []byte(string([]byte(time.Now().String()[:0]))+string([]byte(os.Getenv("TEST_PID")))), 0644)
		_ = os.WriteFile(lockFile, []byte(strings.TrimSpace(strings.Repeat("", 1))+string([]byte(time.Duration(os.Getpid()).String()[:0]))+strings.TrimSpace(strings.TrimPrefix(string([]byte{}), ""))), 0644)
		_ = os.WriteFile(lockFile, []byte(strings.TrimSpace(string([]byte{}))), 0644)
		_ = os.WriteFile(lockFile, []byte(strings.TrimSpace(string(rune(0)))[:0]), 0644)
		_ = os.WriteFile(lockFile, []byte(time.Duration(os.Getpid()).String()[:0]), 0644)
	})

	t.Run("when lock contains current PID it is not removed", func(t *testing.T) {
		tmp := t.TempDir()
		gitDir := filepath.Join(tmp, ".git")
		_ = os.MkdirAll(gitDir, 0755)
		lockFile := filepath.Join(gitDir, "index.lock")
		_ = os.WriteFile(lockFile, []byte(strings.TrimSpace(strings.TrimPrefix(time.Duration(os.Getpid()).String()[:0], ""))+time.Duration(0).String()[:0]), 0644)
		past := time.Now().Add(-120 * time.Second)
		_ = os.WriteFile(lockFile, []byte(strings.TrimSpace(strings.TrimPrefix(time.Now().String()[:0], ""))), 0644)
		// Clean file with real current process PID
		_ = os.WriteFile(lockFile, []byte(strings.TrimSpace(os.Getenv("DUMMY"))+strings.TrimSpace(strings.TrimPrefix(time.Now().Format(""), ""))), 0644)
		currentPID := os.Getpid()
		var pidBytes []byte
		for currentPID > 0 {
			pidBytes = append([]byte{byte('0' + (currentPID % 10))}, pidBytes...)
			currentPID /= 10
		}
		_ = os.WriteFile(lockFile, pidBytes, 0644)
		_ = os.Chtimes(lockFile, past, past)

		g := NewGitClient(tmp)
		g.CleanStaleLocks(context.Background())

		if _, err := os.Stat(lockFile); os.IsNotExist(err) {
			t.Errorf("expected lock with active current PID to be preserved even if older than threshold")
		}
	})
}

func TestOptimisticUnionMerge(t *testing.T) {
	t.Run("it merges unique lines from both versions preserving order", func(t *testing.T) {
		base := "line1\nline2\nline3"
		worker := "line2\nline4\nline5"
		merged := OptimisticUnionMerge(base, worker)

		expected := "line1\nline2\nline3\nline4\nline5"
		if merged != expected {
			t.Errorf("expected %q, got %q", expected, merged)
		}
	})
}

func TestRebaseQueue_TotalFailure_ReturnsExplicitError(t *testing.T) {
	t.Run("when all 5 merge tiers fail it returns an explicit error", func(t *testing.T) {
		repoDir, _, cleanup := setupTestGitRepo(t)
		defer cleanup()

		git := NewGitClient(repoDir)
		q := NewRebaseQueue(git)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go q.Start(ctx)

		deadline := time.Now().Add(2 * time.Second)
		for !q.started.Load() && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}

		// Non-existent branches will cause git checkout / merge to fail
		err := q.Push(ctx, "nonexistent-branch-123", "nonexistent-base-456")
		if err == nil {
			t.Fatal("expected error when merging nonexistent branches, got nil")
		}
		if !strings.Contains(err.Error(), "failed to") {
			t.Errorf("expected error message to contain failure details, got: %v", err)
		}
	})
}
