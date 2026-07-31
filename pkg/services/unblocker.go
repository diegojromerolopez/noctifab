package services

import (
	"context"
	"fmt"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// UnblockerAgent is an autonomous daemon goroutine that periodically scans the
// shared pipeline state for stalled or blocked tasks and agents, diagnoses the
// root cause, and injects corrective interventions via the CommandMailbox.
//
// It runs on its own independent timer (controlled by PollInterval), orthogonal
// to the orchestrator's task-dispatch polling loop, following the same pattern
// as ClarificationPoller.
type UnblockerAgent struct {
	repo              domain.StateRepository
	llmClient         domain.LLMClient
	mailbox           *CommandMailbox
	pollInterval      time.Duration
	maxRetries        int
	stallThreshold    time.Duration
	conflictThreshold time.Duration
	llmAssessment     bool
}

// NewUnblockerAgent creates a new UnblockerAgent via dependency injection.
func NewUnblockerAgent(
	repo domain.StateRepository,
	llmClient domain.LLMClient,
	mailbox *CommandMailbox,
	pollInterval time.Duration,
	maxRetries int,
	stallThreshold time.Duration,
	conflictThreshold time.Duration,
	llmAssessment bool,
) *UnblockerAgent {
	if pollInterval <= 0 {
		pollInterval = 30 * time.Second
	}
	if maxRetries <= 0 {
		maxRetries = 3
	}
	if stallThreshold <= 0 {
		stallThreshold = 5 * time.Minute
	}
	if conflictThreshold <= 0 {
		conflictThreshold = 15 * time.Minute
	}
	return &UnblockerAgent{
		repo:              repo,
		llmClient:         llmClient,
		mailbox:           mailbox,
		pollInterval:      pollInterval,
		maxRetries:        maxRetries,
		stallThreshold:    stallThreshold,
		conflictThreshold: conflictThreshold,
		llmAssessment:     llmAssessment,
	}
}

// Start launches the unblocker polling loop as a background goroutine.
// It returns immediately; cancel ctx to stop.
func (u *UnblockerAgent) Start(ctx context.Context) {
	go u.loop(ctx)
}

func (u *UnblockerAgent) loop(ctx context.Context) {
	ticker := time.NewTicker(u.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			u.checkAndUnblock(ctx)
		}
	}
}

func (u *UnblockerAgent) sendCmd(cmd Command) {
	if u.mailbox != nil {
		u.mailbox.Send(cmd)
	}
}

// checkAndUnblock loads the current state, detects stalls, and dispatches
// corrective actions via the CommandMailbox.
func (u *UnblockerAgent) checkAndUnblock(ctx context.Context) {
	if u.repo == nil {
		return
	}

	state, err := u.repo.Load(ctx)
	if err != nil {
		fmt.Printf("UnblockerAgent: failed to load state: %v\n", err)
		return
	}

	// Only act when the story is actively running or idle with tasks.
	if state.StoryStatus != domain.StoryRunning && state.StoryStatus != domain.StoryIdle {
		return
	}
	if len(state.Tasks) == 0 {
		return
	}

	stalls := u.detectStalledTasks(state)
	if len(stalls) == 0 {
		return
	}

	fmt.Printf("🔍 [UnblockerAgent] Detected %d stall(s). Assessing...\n", len(stalls))

	if u.llmAssessment && u.llmClient != nil {
		u.assessWithLLM(ctx, state, stalls)
	} else {
		u.assessHeuristic(ctx, stalls)
	}
}

// detectStalledTasks inspects the state and returns all detected stalls.
func (u *UnblockerAgent) detectStalledTasks(state *domain.State) []StalledTask {
	var stalls []StalledTask

	// Build a lookup map from taskID -> agent for quick access.
	agentByTaskID := make(map[string]*domain.Agent)
	for i := range state.ActiveAgents {
		ag := &state.ActiveAgents[i]
		if ag.TaskID != "" {
			agentByTaskID[ag.TaskID] = ag
		}
	}

	// Build a lookup map from taskID -> task status.
	statusByID := make(map[string]domain.TaskStatus)
	for _, t := range state.Tasks {
		statusByID[t.ID] = t.Status
	}

	for i := range state.Tasks {
		task := state.Tasks[i]
		// Edge case guard: ignore uninitialized UpdatedAt timestamps to prevent false stalls.
		if task.UpdatedAt.IsZero() {
			continue
		}

		now := time.Now()

		switch task.Status {
		case domain.TaskInProgress:
			stalledFor := now.Sub(task.UpdatedAt)
			// 1. Frozen progress: IN_PROGRESS with no update beyond threshold.
			if stalledFor > u.stallThreshold {
				agent := agentByTaskID[task.ID]
				stalls = append(stalls, StalledTask{
					Task:          task,
					Agent:         agent,
					Reason:        StallReasonFrozenProgress,
					ReasonStr:     StallReasonFrozenProgress.String(),
					StalledFor:    stalledFor,
					StalledForStr: stalledFor.Round(time.Second).String(),
				})
				continue
			}
			// 2. Orphaned: IN_PROGRESS but no WORKING agent is assigned.
			if stalledFor > u.stallThreshold/2 {
				agent := agentByTaskID[task.ID]
				if agent == nil || agent.Status != domain.AgentWorking {
					stalls = append(stalls, StalledTask{
						Task:          task,
						Agent:         agent,
						Reason:        StallReasonOrphanedTask,
						ReasonStr:     StallReasonOrphanedTask.String(),
						StalledFor:    stalledFor,
						StalledForStr: stalledFor.Round(time.Second).String(),
					})
				}
			}

		case domain.TaskConflictBlocked:
			stalledFor := now.Sub(task.UpdatedAt)
			// 3. CONFLICT_BLOCKED for too long.
			if stalledFor > u.conflictThreshold {
				stalls = append(stalls, StalledTask{
					Task:          task,
					Agent:         agentByTaskID[task.ID],
					Reason:        StallReasonConflictBlocked,
					ReasonStr:     StallReasonConflictBlocked.String(),
					StalledFor:    stalledFor,
					StalledForStr: stalledFor.Round(time.Second).String(),
				})
			}
		}
	}

	// 4. Agent inconsistency: WORKING agent whose task is not IN_PROGRESS.
	for i := range state.ActiveAgents {
		ag := &state.ActiveAgents[i]
		if ag.Status != domain.AgentWorking || ag.TaskID == "" {
			continue
		}
		taskStatus, exists := statusByID[ag.TaskID]
		if !exists || taskStatus != domain.TaskInProgress {
			var affectedTask domain.Task
			for _, t := range state.Tasks {
				if t.ID == ag.TaskID {
					affectedTask = t
					break
				}
			}
			stalledFor := time.Since(ag.StartedAt)
			if ag.StartedAt.IsZero() {
				stalledFor = time.Minute
			}
			stalls = append(stalls, StalledTask{
				Task:          affectedTask,
				Agent:         ag,
				Reason:        StallReasonAgentInconsistency,
				ReasonStr:     StallReasonAgentInconsistency.String(),
				StalledFor:    stalledFor,
				StalledForStr: stalledFor.Round(time.Second).String(),
			})
		}
	}

	return stalls
}

// assessWithLLM calls the LLM to diagnose stalls and dispatches corrective commands.
func (u *UnblockerAgent) assessWithLLM(ctx context.Context, state *domain.State, stalls []StalledTask) {
	prompt := buildUnblockerPrompt(state, stalls)
	resp, err := u.llmClient.Complete(ctx, prompt)
	if err != nil {
		fmt.Printf("UnblockerAgent: LLM assessment failed: %v. Falling back to heuristic.\n", err)
		u.assessHeuristic(ctx, stalls)
		return
	}

	u.sendCmd(&LogUnblockerActionCmd{
		Message: fmt.Sprintf("UnblockerAgent LLM assessment: reasoning=%s", resp.Reasoning),
	})

	for _, action := range resp.Actions {
		switch action.Tool {
		case "reset_task":
			taskID, _ := action.Args["task_id"].(string)
			reason, _ := action.Args["reason"].(string)
			if taskID != "" {
				fmt.Printf("🔧 [UnblockerAgent] Resetting task %s: %s\n", taskID, reason)
				u.sendCmd(&ResetTaskCmd{TaskID: taskID, Reason: reason})
			}
		case "fail_task":
			taskID, _ := action.Args["task_id"].(string)
			reason, _ := action.Args["reason"].(string)
			if taskID != "" {
				fmt.Printf("❌ [UnblockerAgent] Failing task %s: %s\n", taskID, reason)
				u.sendCmd(&FailTaskCmd{TaskID: taskID, Reason: reason})
			}
		case "log_message":
			msg, _ := action.Args["message"].(string)
			if msg != "" {
				u.sendCmd(&LogUnblockerActionCmd{Message: msg})
			}
		case "noop":
			fmt.Printf("UnblockerAgent: LLM chose noop — stalls appear transient.\n")
		}
	}
}

// assessHeuristic applies deterministic corrective actions without calling the LLM.
// Used when llm_assessment is false or the LLM call fails.
func (u *UnblockerAgent) assessHeuristic(ctx context.Context, stalls []StalledTask) {
	for _, s := range stalls {
		switch s.Reason {
		case StallReasonFrozenProgress, StallReasonOrphanedTask:
			maxAllowed := s.Task.MaxRetries
			if u.maxRetries > 0 && (maxAllowed == 0 || u.maxRetries < maxAllowed) {
				maxAllowed = u.maxRetries
			}
			if maxAllowed > 0 && s.Task.Retries >= maxAllowed {
				reason := fmt.Sprintf("heuristic: task frozen for %s and max retries (%d) reached", s.StalledForStr, maxAllowed)
				fmt.Printf("❌ [UnblockerAgent] Heuristic fail task %s: %s\n", s.Task.ID, reason)
				u.sendCmd(&FailTaskCmd{TaskID: s.Task.ID, Reason: reason})
			} else {
				reason := fmt.Sprintf("heuristic: task frozen for %s (reason: %s)", s.StalledForStr, s.ReasonStr)
				fmt.Printf("🔧 [UnblockerAgent] Heuristic reset task %s: %s\n", s.Task.ID, reason)
				u.sendCmd(&ResetTaskCmd{TaskID: s.Task.ID, Reason: reason})
			}
		case StallReasonConflictBlocked:
			reason := fmt.Sprintf("heuristic: task conflict-blocked for %s", s.StalledForStr)
			fmt.Printf("🔧 [UnblockerAgent] Heuristic reset conflict-blocked task %s: %s\n", s.Task.ID, reason)
			u.sendCmd(&ResetTaskCmd{TaskID: s.Task.ID, Reason: reason})
		case StallReasonAgentInconsistency:
			if s.Agent != nil {
				reason := fmt.Sprintf("agent %s WORKING on task %s (status=%s)", s.Agent.ID, s.Task.ID, string(s.Task.Status))
				fmt.Printf("🧹 [UnblockerAgent] Clearing inconsistent agent %s: %s\n", s.Agent.ID, reason)
				u.sendCmd(&ClearInconsistentAgentCmd{AgentID: s.Agent.ID, Reason: reason})
			}
		case StallReasonPipelineDeadlock:
			msg := "pipeline deadlock detected: story is running but no tasks are IN_PROGRESS or PENDING"
			fmt.Printf("⚠️  [UnblockerAgent] %s\n", msg)
			u.sendCmd(&LogUnblockerActionCmd{Message: msg})
		}
	}
}
