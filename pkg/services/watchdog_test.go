package services

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

func TestWatchdog_NormalExit(t *testing.T) {
	w := Watchdog{MaxDuration: 5 * time.Second, IdleTimeout: 2 * time.Second}
	cmd := exec.CommandContext(context.Background(), "echo", "hello world")
	out, err := w.Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got %q", string(out))
	}
}

func TestWatchdog_NormalExitNoLimits(t *testing.T) {
	w := Watchdog{}
	cmd := exec.CommandContext(context.Background(), "echo", "no limits")
	out, err := w.Run(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "no limits\n" {
		t.Errorf("expected 'no limits\\n', got %q", string(out))
	}
}

func TestWatchdog_MaxDurationExceeded(t *testing.T) {
	w := Watchdog{MaxDuration: 50 * time.Millisecond, IdleTimeout: 0}
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sleep", "10")
	_, err := w.Run(ctx, cmd)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrWatchdogMaxDuration) {
		t.Errorf("expected ErrWatchdogMaxDuration, got: %v", err)
	}
}

func TestWatchdog_IdleTimeoutExceeded(t *testing.T) {
	w := Watchdog{MaxDuration: 5 * time.Second, IdleTimeout: 50 * time.Millisecond}
	ctx := context.Background()
	// A command that produces no output and runs longer than the idle timeout
	cmd := exec.CommandContext(ctx, "sleep", "10")
	_, err := w.Run(ctx, cmd)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrWatchdogIdleTimeout) {
		t.Errorf("expected ErrWatchdogIdleTimeout, got: %v", err)
	}
}

func TestWatchdog_OutputResetsIdleTimer(t *testing.T) {
	// A command that produces output periodically should not hit idle timeout
	script := `for i in 1 2 3 4 5; do echo "tick $i"; sleep 0.05; done`
	w := Watchdog{MaxDuration: 5 * time.Second, IdleTimeout: 500 * time.Millisecond}
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	out, err := w.Run(ctx, cmd)
	if err != nil {
		t.Fatalf("expected success with periodic output, got: %v", err)
	}
	if len(out) == 0 {
		t.Error("expected output, got empty")
	}
}

func TestWatchdog_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := Watchdog{MaxDuration: 5 * time.Second, IdleTimeout: 2 * time.Second}
	cmd := exec.CommandContext(ctx, "sleep", "10")

	errCh := make(chan error, 1)
	go func() {
		_, err := w.Run(ctx, cmd)
		errCh <- err
	}()

	// Cancel context after a short delay
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error from context cancellation, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watchdog to respond to cancellation")
	}
}
