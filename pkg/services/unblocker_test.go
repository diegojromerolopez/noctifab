package services

import (
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeUnblocker(stallThreshold, conflictThreshold time.Duration) *UnblockerAgent {
	return &UnblockerAgent{
		stallThreshold:    stallThreshold,
		conflictThreshold: conflictThreshold,
		llmAssessment:     false,
	}
}

func taskWithStatus(id string, status domain.TaskStatus, updatedBefore time.Duration) domain.Task {
	return domain.Task{
		ID:         id,
		Title:      "Task " + id,
		Status:     status,
		Progress:   10,
		Retries:    0,
		MaxRetries: 3,
		UpdatedAt:  time.Now().Add(-updatedBefore),
	}
}

func workingAgent(agentID, taskID string) domain.Agent {
	return domain.Agent{
		ID:        agentID,
		Role:      domain.AgentRoleGenerator,
		Status:    domain.AgentWorking,
		TaskID:    taskID,
		StartedAt: time.Now().Add(-10 * time.Minute),
	}
}

func TestDetectStalledTasks(t *testing.T) {
	t.Parallel()

	stall := 5 * time.Minute
	conflict := 15 * time.Minute
	u := makeUnblocker(stall, conflict)

	tests := []struct {
		name        string
		tasks       []domain.Task
		agents      []domain.Agent
		wantReasons []StallReason
	}{
		{
			name:        "when all tasks are SUCCESS, it detects no stalls",
			tasks:       []domain.Task{taskWithStatus("t1", domain.TaskSuccess, 10*time.Minute)},
			wantReasons: nil,
		},
		{
			name:        "when all tasks are PENDING, it detects no stalls",
			tasks:       []domain.Task{taskWithStatus("t1", domain.TaskPending, 10*time.Minute)},
			wantReasons: nil,
		},
		{
			name:        "when task is IN_PROGRESS for longer than stall_threshold, it detects StallReasonFrozenProgress",
			tasks:       []domain.Task{taskWithStatus("t1", domain.TaskInProgress, stall+time.Second)},
			agents:      []domain.Agent{workingAgent("agent-1", "t1")},
			wantReasons: []StallReason{StallReasonFrozenProgress},
		},
		{
			name:        "when task is IN_PROGRESS for less than stall_threshold, it detects no stalls",
			tasks:       []domain.Task{taskWithStatus("t1", domain.TaskInProgress, stall/2-time.Second)},
			agents:      []domain.Agent{workingAgent("agent-1", "t1")},
			wantReasons: nil,
		},
		{
			name:        "when task is IN_PROGRESS but no WORKING agent is assigned, it detects StallReasonOrphanedTask",
			tasks:       []domain.Task{taskWithStatus("t1", domain.TaskInProgress, stall/2+time.Second)},
			agents:      nil,
			wantReasons: []StallReason{StallReasonOrphanedTask},
		},
		{
			name:        "when task is CONFLICT_BLOCKED for longer than conflict_threshold, it detects StallReasonConflictBlocked",
			tasks:       []domain.Task{taskWithStatus("t1", domain.TaskConflictBlocked, conflict+time.Second)},
			wantReasons: []StallReason{StallReasonConflictBlocked},
		},
		{
			name:        "when task is CONFLICT_BLOCKED for less than conflict_threshold, it detects no stalls",
			tasks:       []domain.Task{taskWithStatus("t1", domain.TaskConflictBlocked, conflict-time.Minute)},
			wantReasons: nil,
		},
		{
			name:        "when agent is WORKING but its task is not IN_PROGRESS, it detects StallReasonAgentInconsistency",
			tasks:       []domain.Task{taskWithStatus("t1", domain.TaskSuccess, time.Minute)},
			agents:      []domain.Agent{workingAgent("agent-1", "t1")},
			wantReasons: []StallReason{StallReasonAgentInconsistency},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			state := &domain.State{
				Tasks:        tt.tasks,
				ActiveAgents: tt.agents,
				StoryStatus:  domain.StoryRunning,
			}

			stalls := u.detectStalledTasks(state)

			if len(tt.wantReasons) == 0 {
				assert.Empty(t, stalls, "expected no stalls but got: %v", stalls)
				return
			}
			require.Len(t, stalls, len(tt.wantReasons))
			for i, s := range stalls {
				assert.Equal(t, tt.wantReasons[i], s.Reason)
			}
		})
	}
}

func TestStallReasonString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		reason StallReason
		want   string
	}{
		{StallReasonFrozenProgress, "frozen_progress"},
		{StallReasonOrphanedTask, "orphaned_task"},
		{StallReasonAgentInconsistency, "agent_inconsistency"},
		{StallReasonConflictBlocked, "conflict_blocked"},
		{StallReasonPipelineDeadlock, "pipeline_deadlock"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.reason.String())
		})
	}
}

func TestNewUnblockerAgent_Defaults(t *testing.T) {
	t.Parallel()

	t.Run("when zero durations are provided, it applies safe defaults", func(t *testing.T) {
		t.Parallel()
		u := NewUnblockerAgent(nil, nil, nil, 0, 0, 0, 0, false)
		assert.Equal(t, 30*time.Second, u.pollInterval)
		assert.Equal(t, 5, u.maxRetries)
		assert.Equal(t, 5*time.Minute, u.stallThreshold)
		assert.Equal(t, 15*time.Minute, u.conflictThreshold)
	})

	t.Run("when custom durations are provided, it uses them", func(t *testing.T) {
		t.Parallel()
		u := NewUnblockerAgent(nil, nil, nil, time.Minute, 5, 10*time.Minute, 30*time.Minute, true)
		assert.Equal(t, time.Minute, u.pollInterval)
		assert.Equal(t, 5, u.maxRetries)
		assert.Equal(t, 10*time.Minute, u.stallThreshold)
		assert.Equal(t, 30*time.Minute, u.conflictThreshold)
		assert.True(t, u.llmAssessment)
	})
}
