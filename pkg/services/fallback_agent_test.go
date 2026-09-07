package services

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackAgent_Setters(t *testing.T) {
	fb := NewFallbackAgent(nil, nil, nil, 10*time.Second, 3, 1*time.Minute, 2*time.Minute, true)
	require.NotNil(t, fb)

	fb.SetBudgetCliff(0.60, 10*time.Minute)
	assert.Equal(t, 0.60, fb.budgetCliffRatio)
	assert.Equal(t, 10*time.Minute, fb.maxDuration)

	fb.SetStallCountThreshold(3)
	assert.Equal(t, 3, fb.stallCountThreshold)
}

func TestFallbackAgent_ScopeTriageOnCliff(t *testing.T) {
	now := time.Now().UTC()
	startedAt := now.Add(-6 * time.Minute) // 6 mins ago out of 10 mins (60% > 50%)

	state := &domain.State{
		ID: "test-story-cliff",
		Stories: []domain.Story{
			{ID: "US-001", Title: "Core Skeleton", Status: domain.StoryRunning, StartedAt: &startedAt},
			{ID: "US-002", Title: "Basic CLI", Status: domain.StoryPending},
			{ID: "US-003", Title: "Advanced Plugins", Status: domain.StoryPending},
			{ID: "US-004", Title: "Deep Telemetry", Status: domain.StoryPending},
		},
		Tasks: []domain.Task{
			{ID: "task-1", StoryID: "US-001", Status: domain.TaskInProgress, UpdatedAt: now.Add(-3 * time.Minute)},
			{ID: "task-2", StoryID: "US-003", Status: domain.TaskPending},
		},
	}

	repo := &inMemoryRepo{state: state}
	mailbox := NewCommandMailbox(repo)
	fb := NewFallbackAgent(repo, nil, mailbox, 10*time.Millisecond, 5, 2*time.Minute, 5*time.Minute, false)
	fb.SetBudgetCliff(0.50, 10*time.Minute)

	ctx := context.Background()
	fb.checkAndUnblock(ctx)

	cmds := mailbox.PopAll()
	require.NotEmpty(t, cmds)

	var hasScopeTriage bool
	for _, cmd := range cmds {
		if triageCmd, ok := cmd.(*ScopeTriageCmd); ok {
			hasScopeTriage = true
			assert.Equal(t, 2, triageCmd.KeepStories)
			assert.NotEmpty(t, triageCmd.Reason)
		}
	}
	assert.True(t, hasScopeTriage, "expected ScopeTriageCmd to be dispatched upon reaching budget cliff")
}

func TestFallbackAgent_StartStopIdempotent(t *testing.T) {
	repo := &inMemoryRepo{state: &domain.State{}}
	fb := NewFallbackAgent(repo, nil, nil, 10*time.Millisecond, 3, 1*time.Minute, 2*time.Minute, false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fb.Start(ctx)
	// Second Start call should be a no-op
	fb.Start(ctx)
	assert.True(t, fb.started.Load())
}

func TestBypassToFallbackCmd_Execute(t *testing.T) {
	t.Run("when task exists it increments stall count and sets recovery directive", func(t *testing.T) {
		state := &domain.State{
			Tasks: []domain.Task{
				{
					ID:         "task-fb-1",
					Status:     domain.TaskInProgress,
					StallCount: 1,
					Retries:    0,
				},
			},
		}
		repo := &inMemoryRepo{state: state}
		ctx := context.Background()

		cmd := &BypassToFallbackCmd{
			TaskID:    "task-fb-1",
			Reason:    "stalled twice",
			Directive: "SOVEREIGN REPAIR DIRECTIVE: take full authority",
		}

		err := cmd.Execute(ctx, repo)
		require.NoError(t, err)

		updated, err := repo.Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, domain.TaskPending, updated.Tasks[0].Status)
		assert.Equal(t, 2, updated.Tasks[0].StallCount)
		assert.True(t, updated.Tasks[0].FallbackUsed)
		assert.True(t, updated.Tasks[0].LastResortUsed)
		assert.Equal(t, "SOVEREIGN REPAIR DIRECTIVE: take full authority", updated.Tasks[0].RecoveryDirective)
	})

	t.Run("when task is already in terminal state it is skipped", func(t *testing.T) {
		state := &domain.State{
			Tasks: []domain.Task{
				{
					ID:     "task-fb-done",
					Status: domain.TaskSuccess,
				},
			},
		}
		repo := &inMemoryRepo{state: state}
		ctx := context.Background()

		cmd := &BypassToFallbackCmd{
			TaskID: "task-fb-done",
			Reason: "transient stall",
		}

		err := cmd.Execute(ctx, repo)
		require.NoError(t, err)

		updated, err := repo.Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, domain.TaskSuccess, updated.Tasks[0].Status)
	})
}

func TestScopeTriageCmd_Execute(t *testing.T) {
	state := &domain.State{
		Stories: []domain.Story{
			{ID: "US-001", Title: "Story 1", Status: domain.StoryRunning},
			{ID: "US-002", Title: "Story 2", Status: domain.StoryPending},
			{ID: "US-003", Title: "Story 3", Status: domain.StoryPending},
			{ID: "US-004", Title: "Story 4", Status: domain.StoryPending},
		},
		Tasks: []domain.Task{
			{ID: "t1", StoryID: "US-001", Status: domain.TaskInProgress},
			{ID: "t2", StoryID: "US-002", Status: domain.TaskPending},
			{ID: "t3", StoryID: "US-003", Status: domain.TaskPending},
			{ID: "t4", StoryID: "US-004", Status: domain.TaskPending},
		},
	}
	repo := &inMemoryRepo{state: state}
	ctx := context.Background()

	cmd := &ScopeTriageCmd{
		Reason:      "approaching budget cliff",
		KeepStories: 2,
	}

	err := cmd.Execute(ctx, repo)
	require.NoError(t, err)

	updated, err := repo.Load(ctx)
	require.NoError(t, err)

	// US-001 and US-002 should remain untouched
	assert.Equal(t, domain.StoryRunning, updated.Stories[0].Status)
	assert.Equal(t, domain.StoryPending, updated.Stories[1].Status)

	// US-003 and US-004 should be deferred
	assert.Equal(t, domain.StoryDeferred, updated.Stories[2].Status)
	assert.Equal(t, domain.StoryDeferred, updated.Stories[3].Status)

	// Task t3 and t4 should be deferred
	assert.Equal(t, domain.TaskInProgress, updated.Tasks[0].Status)
	assert.Equal(t, domain.TaskPending, updated.Tasks[1].Status)
	assert.Equal(t, domain.TaskDeferred, updated.Tasks[2].Status)
	assert.Equal(t, domain.TaskDeferred, updated.Tasks[3].Status)
}
