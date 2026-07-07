package services

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSleepWithInterrupt_TimerExpiry(t *testing.T) {
	start := time.Now()
	err := SleepWithInterrupt(context.Background(), 20*time.Millisecond, nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed < 20*time.Millisecond {
		t.Errorf("expected at least 20ms sleep, got %v", elapsed)
	}
}

func TestSleepWithInterrupt_ZeroDuration(t *testing.T) {
	err := SleepWithInterrupt(context.Background(), 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSleepWithInterrupt_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := SleepWithInterrupt(ctx, 10*time.Second, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestSleepWithInterrupt_WakeupFires(t *testing.T) {
	wakeup := make(chan struct{}, 1)
	wakeup <- struct{}{}

	err := SleepWithInterrupt(context.Background(), 10*time.Second, wakeup)
	if err == nil {
		t.Fatal("expected ErrInterrupted, got nil")
	}
	if !errors.Is(err, ErrInterrupted) {
		t.Errorf("expected ErrInterrupted, got: %v", err)
	}
}

func TestSleepWithInterrupt_WakeupChannelBlocksUntilFire(t *testing.T) {
	wakeup := make(chan struct{}, 1)

	go func() {
		time.Sleep(20 * time.Millisecond)
		wakeup <- struct{}{}
	}()

	start := time.Now()
	err := SleepWithInterrupt(context.Background(), 10*time.Second, wakeup)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected ErrInterrupted, got nil")
	}
	if !errors.Is(err, ErrInterrupted) {
		t.Errorf("expected ErrInterrupted, got: %v", err)
	}
	if elapsed < 20*time.Millisecond {
		t.Errorf("expected at least 20ms before wakeup, got %v", elapsed)
	}
}
