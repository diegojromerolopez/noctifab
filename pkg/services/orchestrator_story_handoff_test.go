package services

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestOrchestratorForHandoff(t *testing.T, pollInterval time.Duration) (*Orchestrator, *mockRepo) {
	t.Helper()
	repo := &mockRepo{state: &domain.State{
		ID:          "test-session",
		ProjectPath: t.TempDir(),
		BuildStatus: domain.BuildUnknown,
		StoryStatus: domain.StoryRunning,
	}}
	reg := NewToolRegistry()
	llmClient := &mockLLM{}
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(repo.state.ProjectPath)
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(nil, false, nil, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{PollInterval: pollInterval}

	orch := NewOrchestrator(repo, reg, llmClient, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil, nil)
	return orch, repo
}

func TestOrchestrator_StoryHandoff_WakeupOnStoryCompletion(t *testing.T) {
	t.Run("when a story completes it signals storyCompletedChan and immediately interrupts SleepWithInterrupt", func(t *testing.T) {
		orch, _ := createTestOrchestratorForHandoff(t, 5*time.Minute)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		start := time.Now()
		go func() {
			time.Sleep(30 * time.Millisecond)
			orch.NotifyStoryCompleted()
		}()

		err := SleepWithInterrupt(ctx, orch.cfg.PollInterval, orch.StoryCompletedChan())
		elapsed := time.Since(start)

		assert.True(t, err == nil || strings.Contains(err.Error(), "interrupted") || strings.Contains(err.Error(), "context"))
		assert.Less(t, elapsed, 500*time.Millisecond, "Story completion must interrupt sleep within milliseconds")
	})
}

func TestOrchestrator_StoryHandoff_ChannelAccessorsAndSafety(t *testing.T) {
	t.Run("when accessing channels on valid orchestrator it returns non-nil channels", func(t *testing.T) {
		orch, _ := createTestOrchestratorForHandoff(t, 10*time.Millisecond)

		require.NotNil(t, orch.StoryCompletedChan())
		require.NotNil(t, orch.TaskCompletedChan())
	})

	t.Run("when accessing channels or notifying on nil orchestrator it does not panic", func(t *testing.T) {
		var nilOrch *Orchestrator
		assert.Nil(t, nilOrch.StoryCompletedChan())
		assert.Nil(t, nilOrch.TaskCompletedChan())
		assert.NotPanics(t, func() {
			nilOrch.NotifyStoryCompleted()
		})
	})

	t.Run("when NotifyStoryCompleted is invoked with full channel it does not deadlock or block", func(t *testing.T) {
		orch, _ := createTestOrchestratorForHandoff(t, 10*time.Millisecond)

		done := make(chan struct{})
		go func() {
			// Buffered capacity is 50; push 120 times to guarantee saturation
			for i := 0; i < 120; i++ {
				orch.NotifyStoryCompleted()
			}
			close(done)
		}()

		select {
		case <-done:
			// Success: did not block on full channel
		case <-time.After(1 * time.Second):
			t.Fatal("NotifyStoryCompleted deadlocked when channel was saturated")
		}

		// Verify channel is readable
		select {
		case <-orch.StoryCompletedChan():
		default:
			t.Fatal("expected at least one event in storyCompletedChan")
		}
	})
}

func TestOrchestrator_StoryHandoff_RunOnceSignalsCompletion(t *testing.T) {
	t.Run("when RunOnce finalizes a story it invokes NotifyStoryCompleted", func(t *testing.T) {
		orch, repo := createTestOrchestratorForHandoff(t, 10*time.Millisecond)

		now := time.Now().UTC()
		repo.state.StoryStatus = domain.StoryRunning
		repo.state.Metadata.FeatureName = "US-TEST"
		repo.state.Tasks = []domain.Task{
			{
				ID:        "T-001",
				Title:     "Task 1",
				Status:    domain.TaskSuccess,
				Progress:  100,
				UpdatedAt: now,
			},
		}

		// Drain any existing tokens
		select {
		case <-orch.StoryCompletedChan():
		default:
		}

		hasWork, err := orch.RunOnce(context.Background())
		require.NoError(t, err)
		assert.False(t, hasWork, "RunOnce should return false when all tasks are finished")

		// Verify storyCompletedChan received the signal
		select {
		case <-orch.StoryCompletedChan():
			// Pass! Signal was delivered immediately
		case <-time.After(500 * time.Millisecond):
			t.Fatal("expected StoryCompletedChan to receive notification upon story completion in RunOnce")
		}

		assert.Equal(t, domain.StorySuccess, repo.state.StoryStatus)
		assert.Equal(t, domain.BuildPassing, repo.state.BuildStatus)
	})
}

func TestOrchestrator_StoryHandoff_ConcurrentNotifications(t *testing.T) {
	t.Run("when multiple goroutines emit task and story completions concurrently no deadlock occurs", func(t *testing.T) {
		orch, _ := createTestOrchestratorForHandoff(t, 10*time.Millisecond)

		const workers = 8
		const iterations = 50
		var wg sync.WaitGroup
		wg.Add(workers)

		for w := 0; w < workers; w++ {
			go func(workerID int) {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					if workerID%2 == 0 {
						orch.NotifyStoryCompleted()
					} else {
						select {
						case orch.taskCompletedChan <- struct{}{}:
						default:
						}
					}
					time.Sleep(100 * time.Microsecond)
				}
			}(w)
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Passed cleanly
		case <-time.After(3 * time.Second):
			t.Fatal("concurrent task and story completion notifications deadlocked")
		}
	})
}
