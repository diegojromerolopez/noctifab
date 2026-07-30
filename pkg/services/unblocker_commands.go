package services

import (
	"context"
	"fmt"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ResetTaskCmd resets a stalled IN_PROGRESS or CONFLICT_BLOCKED task back to PENDING
// so the orchestrator can re-dispatch it in the next scheduling cycle.
type ResetTaskCmd struct {
	// TaskID is the unique identifier of the task to reset.
	TaskID string
	// Reason is a human-readable diagnostic explanation logged to LastActions.
	Reason string
}

// Execute implements Command for ResetTaskCmd.
func (c *ResetTaskCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	state, err := repo.Load(ctx)
	if err != nil {
		return fmt.Errorf("ResetTaskCmd: failed to load state: %w", err)
	}

	found := false
	for i := range state.Tasks {
		if state.Tasks[i].ID == c.TaskID {
			found = true
			// Race condition guard: if task self-resolved or finished in the interim, skip.
			if state.Tasks[i].Status == domain.TaskSuccess || state.Tasks[i].Status == domain.TaskFailed {
				fmt.Printf("Unblocker: ResetTaskCmd skipped for task %s (already in terminal state %s)\n", c.TaskID, state.Tasks[i].Status)
				return nil
			}

			// Increment retries to avoid infinite reset loops for permanently broken tasks.
			state.Tasks[i].Retries++
			if state.Tasks[i].MaxRetries > 0 && state.Tasks[i].Retries >= state.Tasks[i].MaxRetries {
				state.Tasks[i].Status = domain.TaskFailed
				state.Tasks[i].FailureLog = fmt.Sprintf("[Unblocker] task reset limit reached (%d/%d): %s", state.Tasks[i].Retries, state.Tasks[i].MaxRetries, c.Reason)
				state.BuildStatus = domain.BuildFailing
				fmt.Printf("❌ [Unblocker] Task %s reached max retries (%d/%d); marking FAILED\n", c.TaskID, state.Tasks[i].Retries, state.Tasks[i].MaxRetries)
			} else {
				state.Tasks[i].Status = domain.TaskPending
				state.Tasks[i].Progress = 0
			}
			state.Tasks[i].UpdatedAt = time.Now()
			break
		}
	}
	if !found {
		return fmt.Errorf("ResetTaskCmd: task %s not found in state", c.TaskID)
	}

	// Clean up any ActiveAgents assigned to this task so state remains consistent.
	for i := range state.ActiveAgents {
		if state.ActiveAgents[i].TaskID == c.TaskID && state.ActiveAgents[i].Status == domain.AgentWorking {
			state.ActiveAgents[i].Status = domain.AgentCompleted
			state.ActiveAgents[i].CompletedAt = time.Now()
			state.ActiveAgents[i].LastError = fmt.Sprintf("reset by unblocker: %s", c.Reason)
		}
	}

	state.LastActions = append(state.LastActions, domain.Action{
		Timestamp: time.Now(),
		Tool:      "unblocker_reset",
		Success:   true,
		Result:    fmt.Sprintf("task %s reset to PENDING by unblocker: %s", c.TaskID, c.Reason),
	})

	return repo.Save(ctx, state)
}

// FailTaskCmd permanently marks a stuck task as FAILED with a diagnostic reason
// written into the task's FailureLog and into State.LastActions.
type FailTaskCmd struct {
	// TaskID is the unique identifier of the task to fail.
	TaskID string
	// Reason is a human-readable diagnostic explanation describing why the
	// unblocker decided to permanently fail this task.
	Reason string
}

// Execute implements Command for FailTaskCmd.
func (c *FailTaskCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	state, err := repo.Load(ctx)
	if err != nil {
		return fmt.Errorf("FailTaskCmd: failed to load state: %w", err)
	}

	found := false
	for i := range state.Tasks {
		if state.Tasks[i].ID == c.TaskID {
			found = true
			// Race condition guard: if task already succeeded in the interim, skip.
			if state.Tasks[i].Status == domain.TaskSuccess {
				fmt.Printf("Unblocker: FailTaskCmd skipped for task %s (task already succeeded)\n", c.TaskID)
				return nil
			}

			state.Tasks[i].Status = domain.TaskFailed
			state.Tasks[i].FailureLog = fmt.Sprintf("[Unblocker] %s", c.Reason)
			state.Tasks[i].Progress = 0
			state.Tasks[i].UpdatedAt = time.Now()
			break
		}
	}
	if !found {
		return fmt.Errorf("FailTaskCmd: task %s not found in state", c.TaskID)
	}

	// Clean up any ActiveAgents assigned to this task.
	for i := range state.ActiveAgents {
		if state.ActiveAgents[i].TaskID == c.TaskID && state.ActiveAgents[i].Status == domain.AgentWorking {
			state.ActiveAgents[i].Status = domain.AgentCompleted
			state.ActiveAgents[i].CompletedAt = time.Now()
			state.ActiveAgents[i].LastError = fmt.Sprintf("failed by unblocker: %s", c.Reason)
		}
	}

	state.BuildStatus = domain.BuildFailing
	state.LastActions = append(state.LastActions, domain.Action{
		Timestamp: time.Now(),
		Tool:      "unblocker_fail",
		Success:   false,
		Result:    fmt.Sprintf("task %s permanently failed by unblocker: %s", c.TaskID, c.Reason),
	})

	return repo.Save(ctx, state)
}

// LogUnblockerActionCmd appends a diagnostic audit entry to State.LastActions
// without modifying any task status. Used by the unblocker to record observations
// and assessments for full traceability.
type LogUnblockerActionCmd struct {
	// Message is the diagnostic message to append.
	Message string
}

// Execute implements Command for LogUnblockerActionCmd.
func (c *LogUnblockerActionCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	state, err := repo.Load(ctx)
	if err != nil {
		return fmt.Errorf("LogUnblockerActionCmd: failed to load state: %w", err)
	}

	state.LastActions = append(state.LastActions, domain.Action{
		Timestamp: time.Now(),
		Tool:      "unblocker_log",
		Success:   true,
		Result:    c.Message,
	})

	return repo.Save(ctx, state)
}

// ClearInconsistentAgentCmd marks an inconsistent worker agent as COMPLETED
// to clean up stale agent state.
type ClearInconsistentAgentCmd struct {
	AgentID string
	Reason  string
}

// Execute implements Command for ClearInconsistentAgentCmd.
func (c *ClearInconsistentAgentCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	state, err := repo.Load(ctx)
	if err != nil {
		return fmt.Errorf("ClearInconsistentAgentCmd: failed to load state: %w", err)
	}

	for i := range state.ActiveAgents {
		if state.ActiveAgents[i].ID == c.AgentID && state.ActiveAgents[i].Status == domain.AgentWorking {
			state.ActiveAgents[i].Status = domain.AgentCompleted
			state.ActiveAgents[i].CompletedAt = time.Now()
			state.ActiveAgents[i].LastError = fmt.Sprintf("cleared by unblocker: %s", c.Reason)
		}
	}

	state.LastActions = append(state.LastActions, domain.Action{
		Timestamp: time.Now(),
		Tool:      "unblocker_clear_agent",
		Success:   true,
		Result:    fmt.Sprintf("agent %s cleared by unblocker: %s", c.AgentID, c.Reason),
	})

	return repo.Save(ctx, state)
}
