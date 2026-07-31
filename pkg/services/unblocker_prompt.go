package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// StallReason classifies why a task or agent is considered stalled.
type StallReason int

const (
	// StallReasonFrozenProgress indicates the task has been IN_PROGRESS with
	// no progress update for longer than the configured stall threshold.
	StallReasonFrozenProgress StallReason = iota
	// StallReasonOrphanedTask indicates a task is IN_PROGRESS but has no
	// WORKING agent registered in ActiveAgents.
	StallReasonOrphanedTask
	// StallReasonAgentInconsistency indicates an agent is WORKING but its
	// assigned task is not IN_PROGRESS (e.g., already succeeded or failed).
	StallReasonAgentInconsistency
	// StallReasonConflictBlocked indicates the task has been CONFLICT_BLOCKED
	// for longer than the configured conflict threshold.
	StallReasonConflictBlocked
	// StallReasonPipelineDeadlock indicates the story is RUNNING but no tasks
	// are IN_PROGRESS and no eligible PENDING tasks remain — a pipeline deadlock.
	StallReasonPipelineDeadlock
)

// String returns a human-readable name for the StallReason.
func (r StallReason) String() string {
	names := [...]string{
		"frozen_progress",
		"orphaned_task",
		"agent_inconsistency",
		"conflict_blocked",
		"pipeline_deadlock",
	}
	if int(r) < len(names) {
		return names[r]
	}
	return "unknown"
}

// StalledTask captures a detected stall, including the affected task, the
// responsible agent (if any), the classified reason, and how long it has been stalled.
type StalledTask struct {
	Task          domain.Task   `json:"task"`
	Agent         *domain.Agent `json:"agent,omitempty"`
	Reason        StallReason   `json:"-"`
	ReasonStr     string        `json:"reason"`
	StalledFor    time.Duration `json:"-"`
	StalledForStr string        `json:"stalled_for"`
}

// stalledTaskSummary is the JSON-serialisable view sent to the LLM.
type stalledTaskSummary struct {
	TaskID     string `json:"task_id"`
	TaskTitle  string `json:"task_title"`
	TaskStatus string `json:"task_status"`
	Progress   int    `json:"progress"`
	Retries    int    `json:"retries"`
	MaxRetries int    `json:"max_retries"`
	FailureLog string `json:"failure_log,omitempty"`
	AgentID    string `json:"agent_id,omitempty"`
	AgentRole  string `json:"agent_role,omitempty"`
	Reason     string `json:"stall_reason"`
	StalledFor string `json:"stalled_for"`
}

func toStalledSummaries(stalls []StalledTask) []stalledTaskSummary {
	summaries := make([]stalledTaskSummary, 0, len(stalls))
	for _, s := range stalls {
		sum := stalledTaskSummary{
			TaskID:     s.Task.ID,
			TaskTitle:  s.Task.Title,
			TaskStatus: string(s.Task.Status),
			Progress:   s.Task.Progress,
			Retries:    s.Task.Retries,
			MaxRetries: s.Task.MaxRetries,
			FailureLog: s.Task.FailureLog,
			Reason:     s.Reason.String(),
			StalledFor: s.StalledFor.Round(time.Second).String(),
		}
		if s.Agent != nil {
			sum.AgentID = s.Agent.ID
			sum.AgentRole = string(s.Agent.Role)
		}
		summaries = append(summaries, sum)
	}
	return summaries
}

// buildUnblockerPrompt constructs the LLM diagnostic prompt for the unblocker,
// injecting current state summary and structured stall details.
func buildUnblockerPrompt(state *domain.State, stalls []StalledTask) string {
	summaries := toStalledSummaries(stalls)
	stallsJSON, _ := json.MarshalIndent(summaries, "", "  ")

	totalTasks := len(state.Tasks)
	inProgressCount := 0
	pendingCount := 0
	successCount := 0
	failedCount := 0
	for _, t := range state.Tasks {
		switch t.Status {
		case domain.TaskInProgress:
			inProgressCount++
		case domain.TaskPending:
			pendingCount++
		case domain.TaskSuccess:
			successCount++
		case domain.TaskFailed:
			failedCount++
		}
	}

	return fmt.Sprintf(`You are the Unblocker Agent of a dark factory autonomous orchestration system.
You must respond ONLY with a single JSON block. Do not include conversational text or markdown fences.

Your role is to diagnose stalled or blocked tasks/agents detected in the execution pipeline and
suggest the minimal corrective action to restore forward progress.

Available corrective actions:
- "reset_task":   reset an IN_PROGRESS or stalled task back to PENDING so it can be re-dispatched.
  Use when: task is frozen but recoverable (not at max retries, not permanently broken).
- "fail_task":    permanently mark a task as FAILED with a clear diagnostic reason.
  Use when: task is at max retries, appears unrecoverable, or is blocking the entire pipeline.
- "log_message":  record an observation without changing task status.
  Use when: you need to note an anomaly but no corrective action is needed yet.
- "noop":         take no action.
  Use when: stalls look transient or will self-resolve shortly.

Current Pipeline State:
- Story Status: %s
- Build Status: %s
- Total Tasks: %d (pending=%d, in_progress=%d, success=%d, failed=%d)
- Active Agents: %d

Stalled Tasks/Agents Detected (%d stall(s)):
%s

Return format:
{
  "reasoning": "Explain your diagnosis of each stall and why you chose each action",
  "actions": [
    { "tool": "reset_task",   "args": { "task_id": "...", "reason": "..." } },
    { "tool": "fail_task",    "args": { "task_id": "...", "reason": "..." } },
    { "tool": "log_message",  "args": { "message": "..." } },
    { "tool": "noop",         "args": {} }
  ]
}
`,
		string(state.StoryStatus),
		string(state.BuildStatus),
		totalTasks, pendingCount, inProgressCount, successCount, failedCount,
		len(state.ActiveAgents),
		len(stalls),
		string(stallsJSON),
	)
}
