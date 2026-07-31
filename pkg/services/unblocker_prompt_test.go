package services

import (
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildUnblockerPrompt(t *testing.T) {
	t.Parallel()

	t.Run("when one frozen task exists, it includes task details in the prompt", func(t *testing.T) {
		t.Parallel()
		state := &domain.State{
			StoryStatus: domain.StoryRunning,
			BuildStatus: domain.BuildFailing,
			Tasks: []domain.Task{
				{ID: "t1", Title: "Write core logic", Status: domain.TaskInProgress, Progress: 30},
			},
			ActiveAgents: []domain.Agent{},
		}
		stalls := []StalledTask{
			{
				Task:          state.Tasks[0],
				Reason:        StallReasonFrozenProgress,
				ReasonStr:     StallReasonFrozenProgress.String(),
				StalledFor:    7 * time.Minute,
				StalledForStr: "7m0s",
			},
		}

		prompt := buildUnblockerPrompt(state, stalls)

		require.NotEmpty(t, prompt)
		assert.Contains(t, prompt, "t1")
		assert.Contains(t, prompt, "Write core logic")
		assert.Contains(t, prompt, "frozen_progress")
		assert.Contains(t, prompt, "7m0s")
		assert.Contains(t, prompt, "reset_task")
		assert.Contains(t, prompt, "fail_task")
	})

	t.Run("when multiple stalls exist, it lists all of them in the prompt", func(t *testing.T) {
		t.Parallel()
		state := &domain.State{
			StoryStatus: domain.StoryRunning,
			BuildStatus: domain.BuildUnknown,
			Tasks: []domain.Task{
				{ID: "t1", Status: domain.TaskInProgress},
				{ID: "t2", Status: domain.TaskConflictBlocked},
			},
			ActiveAgents: []domain.Agent{},
		}
		stalls := []StalledTask{
			{Task: state.Tasks[0], Reason: StallReasonFrozenProgress, ReasonStr: StallReasonFrozenProgress.String(), StalledForStr: "6m0s"},
			{Task: state.Tasks[1], Reason: StallReasonConflictBlocked, ReasonStr: StallReasonConflictBlocked.String(), StalledForStr: "20m0s"},
		}

		prompt := buildUnblockerPrompt(state, stalls)

		assert.Contains(t, prompt, "t1")
		assert.Contains(t, prompt, "t2")
		// Each task ID appears exactly once in the injected stall data JSON.
		assert.Equal(t, 1, strings.Count(prompt, `"t1"`), "task t1 should appear once in stall data")
		assert.Equal(t, 1, strings.Count(prompt, `"t2"`), "task t2 should appear once in stall data")
	})

	t.Run("when no stalls exist, it generates a valid prompt with 0 stalls", func(t *testing.T) {
		t.Parallel()
		state := &domain.State{
			StoryStatus:  domain.StoryRunning,
			BuildStatus:  domain.BuildPassing,
			Tasks:        []domain.Task{},
			ActiveAgents: []domain.Agent{},
		}

		prompt := buildUnblockerPrompt(state, nil)

		require.NotEmpty(t, prompt)
		assert.Contains(t, prompt, "0 stall(s)")
	})
}

func TestToStalledSummaries(t *testing.T) {
	t.Parallel()

	t.Run("when agent is nil, it omits agent fields from summary", func(t *testing.T) {
		t.Parallel()
		task := domain.Task{ID: "t1", Title: "Task A", Status: domain.TaskInProgress, Retries: 1, MaxRetries: 3}
		stalls := []StalledTask{
			{Task: task, Agent: nil, Reason: StallReasonOrphanedTask, ReasonStr: "orphaned_task", StalledForStr: "3m0s"},
		}

		summaries := toStalledSummaries(stalls)

		require.Len(t, summaries, 1)
		assert.Equal(t, "t1", summaries[0].TaskID)
		assert.Equal(t, "Task A", summaries[0].TaskTitle)
		assert.Equal(t, "orphaned_task", summaries[0].Reason)
		assert.Empty(t, summaries[0].AgentID)
	})

	t.Run("when agent is present, it includes agent fields in summary", func(t *testing.T) {
		t.Parallel()
		task := domain.Task{ID: "t2", Status: domain.TaskInProgress}
		agent := &domain.Agent{ID: "agent-42", Role: domain.AgentRoleGenerator}
		stalls := []StalledTask{
			{Task: task, Agent: agent, Reason: StallReasonFrozenProgress, ReasonStr: "frozen_progress", StalledForStr: "8m0s"},
		}

		summaries := toStalledSummaries(stalls)

		require.Len(t, summaries, 1)
		assert.Equal(t, "agent-42", summaries[0].AgentID)
		assert.Equal(t, "GENERATOR", summaries[0].AgentRole)
	})
}
