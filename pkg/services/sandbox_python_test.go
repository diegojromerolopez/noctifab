package services

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunWithProcessGroupKill(t *testing.T) {
	t.Run("when the command completes it returns and does not leak the watcher goroutine", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		before := runtime.NumGoroutine()
		for i := 0; i < 10; i++ {
			cmd := exec.CommandContext(ctx, "true")
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := runWithProcessGroupKill(ctx, cmd); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		// Give watcher goroutines a moment to observe the done channel.
		time.Sleep(100 * time.Millisecond)
		after := runtime.NumGoroutine()
		if after > before+2 {
			t.Errorf("goroutines leaked: before=%d after=%d", before, after)
		}
	})

	t.Run("when the ctx is cancelled it kills the process group", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "sleep", "30")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		errCh := make(chan error, 1)
		go func() {
			errCh <- runWithProcessGroupKill(ctx, cmd)
		}()

		time.Sleep(100 * time.Millisecond)
		cancel()

		select {
		case err := <-errCh:
			if err == nil {
				t.Error("expected an error after cancellation kill")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("command was not killed after ctx cancellation")
		}
	})

	t.Run("when a failing command runs it propagates the error", func(t *testing.T) {
		ctx := context.Background()
		cmd := exec.CommandContext(ctx, "false")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := runWithProcessGroupKill(ctx, cmd); err == nil {
			t.Error("expected a non-nil error for a failing command")
		}
	})
}

func TestRunPythonTestsIsolatedOutputBounded(t *testing.T) {
	t.Run("when no test files exist it reports nothing to isolate", func(t *testing.T) {
		s := NewHostSandbox([]string{"*"}, "", 0, nil)
		out, err := s.runPythonTestsIsolated(context.Background(), t.TempDir(), "python -m unittest discover tests")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "No test files found") {
			t.Errorf("expected no-test-files message, got %q", out)
		}
	})
}
