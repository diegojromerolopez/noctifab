package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
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

func TestCommandMailbox_SendSync_And_IsRunning(t *testing.T) {
	t.Run("when mailbox starts and stops, IsRunning reflects lifecycle", func(t *testing.T) {
		repo := &mockRepo{state: &domain.State{ID: "story-1"}}
		mailbox := NewCommandMailbox(repo)
		if mailbox.IsRunning() {
			t.Error("expected IsRunning to be false before Start")
		}

		ctx, cancel := context.WithCancel(context.Background())
		go mailbox.Start(ctx)

		// Allow loop to start
		time.Sleep(5 * time.Millisecond)
		if !mailbox.IsRunning() {
			t.Error("expected IsRunning to be true after Start")
		}

		cancel()
		time.Sleep(10 * time.Millisecond)
		if mailbox.IsRunning() {
			t.Error("expected IsRunning to be false after cancel")
		}
	})

	t.Run("when SendSync executes StateMutationCmd, it mutates repository serially", func(t *testing.T) {
		repo := &mockRepo{state: &domain.State{ID: "story-1", Tasks: []domain.Task{}}}
		mailbox := NewCommandMailbox(repo)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go mailbox.Start(ctx)

		time.Sleep(5 * time.Millisecond)

		err := mailbox.SendSync(ctx, &StateMutationCmd{
			UpdateFn: func(st *domain.State) error {
				st.Tasks = append(st.Tasks, domain.Task{ID: "task-1", Title: "T1"})
				return nil
			},
		})
		if err != nil {
			t.Fatalf("unexpected SendSync error: %v", err)
		}

		loaded, _ := repo.Load(ctx)
		if len(loaded.Tasks) != 1 || loaded.Tasks[0].ID != "task-1" {
			t.Errorf("expected task-1 persisted, got %+v", loaded.Tasks)
		}
	})

	t.Run("when mutation returns error, SendSync propagates it to caller", func(t *testing.T) {
		repo := &mockRepo{state: &domain.State{ID: "story-1"}}
		mailbox := NewCommandMailbox(repo)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go mailbox.Start(ctx)

		time.Sleep(5 * time.Millisecond)

		expectedErr := errors.New("business logic validation failure")
		err := mailbox.SendSync(ctx, &StateMutationCmd{
			UpdateFn: func(st *domain.State) error {
				return expectedErr
			},
		})
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("when context is cancelled during SendSync, it returns context error", func(t *testing.T) {
		repo := &mockRepo{state: &domain.State{ID: "story-1"}}
		mailbox := NewCommandMailbox(repo) // Not started!

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := mailbox.SendSync(ctx, &StateMutationCmd{
			UpdateFn: func(st *domain.State) error { return nil },
		})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})

	t.Run("when multiple concurrent goroutines SendSync, writes execute sequentially without conflict", func(t *testing.T) {
		repo := &mockRepo{state: &domain.State{ID: "story-1", Tasks: []domain.Task{}}}
		mailbox := NewCommandMailbox(repo)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go mailbox.Start(ctx)

		time.Sleep(5 * time.Millisecond)

		const count = 20
		errs := make(chan error, count)
		for i := 0; i < count; i++ {
			taskID := i
			go func() {
				err := mailbox.SendSync(ctx, &StateMutationCmd{
					UpdateFn: func(st *domain.State) error {
						st.Tasks = append(st.Tasks, domain.Task{
							ID:    string(rune('A' + taskID)),
							Title: "Task",
						})
						return nil
					},
				})
				errs <- err
			}()
		}

		for i := 0; i < count; i++ {
			if err := <-errs; err != nil {
				t.Errorf("goroutine SendSync failed: %v", err)
			}
		}

		loaded, _ := repo.Load(ctx)
		if len(loaded.Tasks) != count {
			t.Errorf("expected %d tasks appended serially, got %d", count, len(loaded.Tasks))
		}
	})
}
