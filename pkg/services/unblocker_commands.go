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
	// Directive is an optional recovery instruction injected into worker prompts on retry.
	Directive string
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

			// Increment stall count for progressive escalation
			state.Tasks[i].StallCount++
			if c.Directive != "" {
				state.Tasks[i].RecoveryDirective = c.Directive
			}

			// Increment retries to avoid infinite reset loops for permanently broken tasks.
			state.Tasks[i].Retries++
			maxLimit := state.Tasks[i].MaxRetries
			if maxLimit <= 0 || maxLimit > 5 {
				maxLimit = 5
			}
			if state.Tasks[i].Retries >= maxLimit || state.Tasks[i].StallCount >= 5 {
				state.Tasks[i].Status = domain.TaskFailed
				state.Tasks[i].FailureLog = fmt.Sprintf("[Unblocker] task reset limit reached (%d/%d, stalls: %d): %s", state.Tasks[i].Retries, maxLimit, state.Tasks[i].StallCount, c.Reason)
				state.BuildStatus = domain.BuildFailing
				fmt.Printf("❌ [Unblocker] Task %s reached max resets (%d/%d, stalls: %d); marking FAILED\n", c.TaskID, state.Tasks[i].Retries, maxLimit, state.Tasks[i].StallCount)
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

	domain.AppendAction(state, domain.Action{
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
	domain.AppendAction(state, domain.Action{
		Timestamp: time.Now(),
		Tool:      "unblocker_fail",
		Success:   false,
		Result:    fmt.Sprintf("task %s permanently failed by unblocker: %s", c.TaskID, c.Reason),
	})

	return repo.Save(ctx, state)
}

// LogFallbackActionCmd appends a diagnostic audit entry to State.LastActions
// without modifying any task status. Used by the fallback agent to record observations
// and assessments for full traceability.
type LogFallbackActionCmd struct {
	// Message is the diagnostic message to append.
	Message string
	// TokensUsed is the optional count of tokens consumed during LLM assessment.
	TokensUsed int64
}

// LogUnblockerActionCmd is a backwards-compatible alias for LogFallbackActionCmd.
type LogUnblockerActionCmd = LogFallbackActionCmd

// Execute implements Command for LogFallbackActionCmd.
func (c *LogFallbackActionCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	state, err := repo.Load(ctx)
	if err != nil {
		return fmt.Errorf("LogFallbackActionCmd: failed to load state: %w", err)
	}

	if c.TokensUsed > 0 {
		state.Metadata.TotalTokensUsed += c.TokensUsed
	}

	domain.AppendAction(state, domain.Action{
		Timestamp: time.Now(),
		Tool:      "fallback_log",
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

	domain.AppendAction(state, domain.Action{
		Timestamp: time.Now(),
		Tool:      "unblocker_clear_agent",
		Success:   true,
		Result:    fmt.Sprintf("agent %s cleared by unblocker: %s", c.AgentID, c.Reason),
	})

	return repo.Save(ctx, state)
}

// BypassToFallbackCmd forces a stalled or looping task directly to the FallbackAgent
// with full sovereign repair authority, resetting the task to PENDING with high stall count
// and sovereign recovery directive.
type BypassToFallbackCmd struct {
	TaskID    string
	Reason    string
	Directive string
}

// BypassToLastResortCmd is a backwards-compatible alias for BypassToFallbackCmd.
type BypassToLastResortCmd = BypassToFallbackCmd

// Execute implements Command for BypassToFallbackCmd.
func (c *BypassToFallbackCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	state, err := repo.Load(ctx)
	if err != nil {
		return fmt.Errorf("BypassToFallbackCmd: failed to load state: %w", err)
	}

	found := false
	for i := range state.Tasks {
		if state.Tasks[i].ID == c.TaskID {
			found = true
			if state.Tasks[i].Status == domain.TaskSuccess || state.Tasks[i].Status == domain.TaskFailed {
				fmt.Printf("FallbackAgent: BypassToFallbackCmd skipped for task %s (already in terminal state %s)\n", c.TaskID, state.Tasks[i].Status)
				return nil
			}

			state.Tasks[i].StallCount = 2
			state.Tasks[i].LastResortUsed = true
			state.Tasks[i].FallbackUsed = true
			state.Tasks[i].Status = domain.TaskPending
			state.Tasks[i].Progress = 0
			directive := c.Directive
			if directive == "" {
				directive = fmt.Sprintf("SOVEREIGN REPAIR DIRECTIVE: bypass to FallbackAgent: %s", c.Reason)
			}
			state.Tasks[i].RecoveryDirective = directive
			state.Tasks[i].UpdatedAt = time.Now()
			break
		}
	}
	if !found {
		return fmt.Errorf("BypassToFallbackCmd: task %s not found in state", c.TaskID)
	}

	for i := range state.ActiveAgents {
		if state.ActiveAgents[i].TaskID == c.TaskID && state.ActiveAgents[i].Status == domain.AgentWorking {
			state.ActiveAgents[i].Status = domain.AgentCompleted
			state.ActiveAgents[i].CompletedAt = time.Now()
			state.ActiveAgents[i].LastError = fmt.Sprintf("bypassed to fallback: %s", c.Reason)
		}
	}

	domain.AppendAction(state, domain.Action{
		Timestamp: time.Now(),
		Tool:      "fallback_bypass",
		Success:   true,
		Result:    fmt.Sprintf("task %s bypassed to FallbackAgent: %s", c.TaskID, c.Reason),
	})

	return repo.Save(ctx, state)
}

// ScopeTriageCmd evaluates remaining backlog scope when approaching budget/timeout cliffs
// or suffering repeated stalls, deferring non-essential downstream stories (US-003+)
// and unblocking the delivery of a tested, working Walking Skeleton (US-001 / US-002).
type ScopeTriageCmd struct {
	Reason      string
	KeepStories int // Default is 2 (keep US-001 and US-002)
}

// Execute implements Command for ScopeTriageCmd.
func (c *ScopeTriageCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	state, err := repo.Load(ctx)
	if err != nil {
		return fmt.Errorf("ScopeTriageCmd: failed to load state: %w", err)
	}

	keep := c.KeepStories
	if keep <= 0 {
		keep = 2
	}

	var triagedStories []string
	for i := range state.Stories {
		if i >= keep && state.Stories[i].Status != domain.StorySuccess && state.Stories[i].Status != domain.StoryDeferred {
			triagedStories = append(triagedStories, state.Stories[i].ID)
			state.Stories[i].Status = domain.StoryDeferred
			state.Stories[i].UpdatedAt = time.Now()
		}
	}

	triagedTasks := 0
	if len(triagedStories) > 0 {
		triagedMap := make(map[string]bool, len(triagedStories))
		for _, sID := range triagedStories {
			triagedMap[sID] = true
		}

		for i := range state.Tasks {
			if triagedMap[state.Tasks[i].StoryID] && state.Tasks[i].Status != domain.TaskSuccess && state.Tasks[i].Status != domain.TaskDeferred {
				state.Tasks[i].Status = domain.TaskDeferred
				state.Tasks[i].FailureLog = fmt.Sprintf("[ScopeTriage] deferred to prioritize core deliverable: %s", c.Reason)
				state.Tasks[i].UpdatedAt = time.Now()
				triagedTasks++
			}
		}
	}

	resultMsg := fmt.Sprintf("scope triage executed: kept first %d stories, deferred %d downstream stories (%v) and %d tasks: %s",
		keep, len(triagedStories), triagedStories, triagedTasks, c.Reason)
	fmt.Printf("✂ [Scope Triage] %s\n", resultMsg)

	domain.AppendAction(state, domain.Action{
		Timestamp: time.Now(),
		Tool:      "scope_triage",
		Success:   true,
		Result:    resultMsg,
	})

	return repo.Save(ctx, state)
}
