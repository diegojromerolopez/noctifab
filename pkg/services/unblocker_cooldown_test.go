package services

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
)

func stallFor(task domain.Task) StalledTask {
	return StalledTask{
		Task:      task,
		Reason:    StallReasonFrozenProgress,
		ReasonStr: StallReasonFrozenProgress.String(),
	}
}

func TestUnblockerLLMAssessmentCooldown(t *testing.T) {
	t.Parallel()

	t.Run("when a stall is seen for the first time, it is passed to the LLM", func(t *testing.T) {
		t.Parallel()
		u := NewUnblockerAgent(nil, nil, nil, 0, 0, 0, 0, true)
		task := taskWithStatus("t1", domain.TaskInProgress, 10*time.Minute)
		state := &domain.State{Tasks: []domain.Task{task}}

		fresh := u.filterStallsForLLMAssessment(state, []StalledTask{stallFor(task)})
		assert.Len(t, fresh, 1)
	})

	t.Run("when the same stall repeats within the cooldown, it is filtered out", func(t *testing.T) {
		t.Parallel()
		u := NewUnblockerAgent(nil, nil, nil, 0, 0, 0, 0, true)
		task := taskWithStatus("t1", domain.TaskInProgress, 10*time.Minute)
		state := &domain.State{Tasks: []domain.Task{task}}

		first := u.filterStallsForLLMAssessment(state, []StalledTask{stallFor(task)})
		assert.Len(t, first, 1)
		second := u.filterStallsForLLMAssessment(state, []StalledTask{stallFor(task)})
		assert.Empty(t, second, "repeat assessment within cooldown must be suppressed")
	})

	t.Run("when the cooldown has elapsed, the stall is re-assessed", func(t *testing.T) {
		t.Parallel()
		u := NewUnblockerAgent(nil, nil, nil, 0, 0, 0, 0, true)
		u.llmCooldown = 10 * time.Millisecond
		task := taskWithStatus("t1", domain.TaskInProgress, 10*time.Minute)
		state := &domain.State{Tasks: []domain.Task{task}}

		assert.Len(t, u.filterStallsForLLMAssessment(state, []StalledTask{stallFor(task)}), 1)
		time.Sleep(20 * time.Millisecond)
		assert.Len(t, u.filterStallsForLLMAssessment(state, []StalledTask{stallFor(task)}), 1)
	})

	t.Run("when the task status changes, the old cooldown entry is cleared and the stall is re-assessed", func(t *testing.T) {
		t.Parallel()
		u := NewUnblockerAgent(nil, nil, nil, 0, 0, 0, 0, true)
		inProgress := taskWithStatus("t1", domain.TaskInProgress, 10*time.Minute)
		state := &domain.State{Tasks: []domain.Task{inProgress}}
		assert.Len(t, u.filterStallsForLLMAssessment(state, []StalledTask{stallFor(inProgress)}), 1)

		// Status change: IN_PROGRESS -> CONFLICT_BLOCKED gives a new key.
		blocked := inProgress
		blocked.Status = domain.TaskConflictBlocked
		state = &domain.State{Tasks: []domain.Task{blocked}}
		fresh := u.filterStallsForLLMAssessment(state, []StalledTask{stallFor(blocked)})
		assert.Len(t, fresh, 1, "stall with a new status must be re-assessed immediately")

		u.cooldownMu.Lock()
		_, staleKept := u.llmAssessedAt[stallCooldownKey("t1", domain.TaskInProgress)]
		u.cooldownMu.Unlock()
		assert.False(t, staleKept, "stale cooldown entry for the old status must be pruned")
	})

	t.Run("when different tasks stall, each gets its own cooldown key", func(t *testing.T) {
		t.Parallel()
		u := NewUnblockerAgent(nil, nil, nil, 0, 0, 0, 0, true)
		t1 := taskWithStatus("t1", domain.TaskInProgress, 10*time.Minute)
		t2 := taskWithStatus("t2", domain.TaskInProgress, 10*time.Minute)
		state := &domain.State{Tasks: []domain.Task{t1, t2}}

		assert.Len(t, u.filterStallsForLLMAssessment(state, []StalledTask{stallFor(t1)}), 1)
		assert.Len(t, u.filterStallsForLLMAssessment(state, []StalledTask{stallFor(t2)}), 1)
	})

	t.Run("when the agent was built via struct literal without a map, it does not panic", func(t *testing.T) {
		t.Parallel()
		u := makeUnblocker(5*time.Minute, 15*time.Minute)
		task := taskWithStatus("t1", domain.TaskInProgress, 10*time.Minute)
		state := &domain.State{Tasks: []domain.Task{task}}
		assert.Len(t, u.filterStallsForLLMAssessment(state, []StalledTask{stallFor(task)}), 1)
	})
}

func TestUnblockerStartIdempotent(t *testing.T) {
	t.Run("when Start is called twice, only the first call launches the loop", func(t *testing.T) {
		u := NewUnblockerAgent(nil, nil, nil, time.Hour, 0, 0, 0, false)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		u.Start(ctx)
		assert.True(t, u.started.Load())
		// Second call must be a no-op (no panic, still started).
		u.Start(ctx)
		assert.True(t, u.started.Load())
	})
}
