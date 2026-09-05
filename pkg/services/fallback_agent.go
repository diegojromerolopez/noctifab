package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
)

// defaultLLMAssessmentCooldown is the minimum interval between LLM
// re-assessments of the same stall (keyed by task ID + task status).
const defaultLLMAssessmentCooldown = 5 * time.Minute

// FallbackAgent is the unified autonomous recovery and sovereign repair agent for Noctifab.
// It operates in two complementary modes:
//  1. Passive Watchdog: Periodically scans the shared pipeline state for stalled/looping tasks,
//     approaching budget/timeout cliffs, and deadlocks, dispatching scope triage and unblocking directives.
//  2. Active Sovereign Omni-Builder: When stalls or retries exhaust, directly takes over the task
//     and synthesizes working production code and accompanying tests with full compromise authority.
type FallbackAgent struct {
	repo                     domain.StateRepository
	llmClient                domain.LLMClient
	mailbox                  *CommandMailbox
	pollInterval             time.Duration
	maxRetries               int
	stallThreshold           time.Duration
	conflictThreshold        time.Duration
	inconsistencyGracePeriod time.Duration
	llmAssessment            bool
	budgetCliffRatio         float64
	stallCountThreshold      int
	maxDuration              time.Duration

	// started makes Start idempotent so double-starting (e.g. serve.go plus
	// Orchestrator.Start) never spawns two competing polling loops.
	started atomic.Bool

	// llmCooldown throttles LLM re-assessment of the same stall; entries in
	// llmAssessedAt are keyed by task ID + status and pruned when the task's
	// status changes.
	llmCooldown   time.Duration
	cooldownMu    sync.Mutex
	llmAssessedAt map[string]time.Time
}

// UnblockerAgent is a backwards-compatible type alias for FallbackAgent.
type UnblockerAgent = FallbackAgent

// NewFallbackAgent creates a new FallbackAgent via dependency injection.
func NewFallbackAgent(
	repo domain.StateRepository,
	llmClient domain.LLMClient,
	mailbox *CommandMailbox,
	pollInterval time.Duration,
	maxRetries int,
	stallThreshold time.Duration,
	conflictThreshold time.Duration,
	llmAssessment bool,
) *FallbackAgent {
	if pollInterval <= 0 {
		pollInterval = 30 * time.Second
	}
	if maxRetries <= 0 {
		maxRetries = 5
	}
	if stallThreshold <= 0 {
		stallThreshold = 2 * time.Minute
	}
	if conflictThreshold <= 0 {
		conflictThreshold = 5 * time.Minute
	}
	return &FallbackAgent{
		repo:                     repo,
		llmClient:                llmClient,
		mailbox:                  mailbox,
		pollInterval:             pollInterval,
		maxRetries:               maxRetries,
		stallThreshold:           stallThreshold,
		conflictThreshold:        conflictThreshold,
		inconsistencyGracePeriod: 30 * time.Second,
		llmAssessment:            llmAssessment,
		budgetCliffRatio:         0.50,
		stallCountThreshold:      2,
		llmCooldown:              defaultLLMAssessmentCooldown,
		llmAssessedAt:            make(map[string]time.Time),
	}
}

// NewUnblockerAgent is a backwards-compatible constructor for NewFallbackAgent.
func NewUnblockerAgent(
	repo domain.StateRepository,
	llmClient domain.LLMClient,
	mailbox *CommandMailbox,
	pollInterval time.Duration,
	maxRetries int,
	stallThreshold time.Duration,
	conflictThreshold time.Duration,
	llmAssessment bool,
) *FallbackAgent {
	return NewFallbackAgent(repo, llmClient, mailbox, pollInterval, maxRetries, stallThreshold, conflictThreshold, llmAssessment)
}

// SetBudgetCliff sets the budget cliff ratio and maximum execution duration.
func (u *FallbackAgent) SetBudgetCliff(ratio float64, maxDuration time.Duration) {
	if ratio > 0 && ratio <= 1.0 {
		u.budgetCliffRatio = ratio
	}
	if maxDuration > 0 {
		u.maxDuration = maxDuration
	}
}

// SetStallCountThreshold sets the number of stalls before escalating to sovereign execution.
func (u *FallbackAgent) SetStallCountThreshold(threshold int) {
	if threshold > 0 {
		u.stallCountThreshold = threshold
	}
}

// Start launches the fallback watchdog polling loop as a background goroutine.
func (u *FallbackAgent) Start(ctx context.Context) {
	if !u.started.CompareAndSwap(false, true) {
		return
	}
	go u.loop(ctx)
}

func (u *FallbackAgent) loop(ctx context.Context) {
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

func (u *FallbackAgent) sendCmd(cmd Command) {
	if u.mailbox != nil {
		u.mailbox.Send(cmd)
	}
}

// checkAndUnblock loads the current state, detects stalls, and dispatches
// corrective actions via the CommandMailbox.
func (u *FallbackAgent) checkAndUnblock(ctx context.Context) {
	if u.repo == nil {
		return
	}

	state, err := u.repo.Load(ctx)
	if err != nil {
		fmt.Printf("FallbackAgent: failed to load state: %v\n", err)
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

	// Scope Triage & Budget Cliff Check:
	// If multiple user stories exist and we observe stalls or high retry counts, triage backlog
	// to ensure US-001/US-002 deliver a clean working program within budget.
	if len(state.Stories) > 2 && len(stalls) > 0 {
		hasDeferred := false
		for i := 2; i < len(state.Stories); i++ {
			if state.Stories[i].Status != domain.StoryDeferred && state.Stories[i].Status != domain.StorySuccess {
				hasDeferred = true
				break
			}
		}
		if hasDeferred {
			fmt.Printf("✂ [FallbackAgent] High stall pressure with %d backlog stories detected. Triggering scope triage...\n", len(state.Stories))
			u.sendCmd(&ScopeTriageCmd{
				Reason:      fmt.Sprintf("fallback agent detected %d stalls; triaging non-essential backlog to prioritize core Walking Skeleton", len(stalls)),
				KeepStories: 2,
			})
		}
	}

	if len(stalls) == 0 {
		return
	}

	// Process Fast-Path Regex, Fallback Bypass, & Escalation Hard-Stops
	var remainingStalls []StalledTask
	for _, s := range stalls {
		// Hard-stop check: If task has stalled 5 or more times, fail it
		if s.Task.StallCount >= 5 {
			reason := fmt.Sprintf("fallback: task %s reached max stall escalations (%d); task unrecoverable", s.Task.ID, s.Task.StallCount)
			fmt.Printf("❌ [FallbackAgent] Hard-stop max stall limit reached for task %s\n", s.Task.ID)
			u.sendCmd(&FailTaskCmd{TaskID: s.Task.ID, Reason: reason})
			continue
		}

		// Pre-hard-stop check: If task has stalled at or beyond stallCountThreshold (default: 2),
		// immediately bypass to sovereign Fallback Agent repair!
		threshold := u.stallCountThreshold
		if threshold <= 0 {
			threshold = 2
		}
		if s.Task.StallCount >= threshold {
			reason := fmt.Sprintf("fallback: task %s reached stall count %d; bypassing to Last-Resort Agent / Fallback Agent", s.Task.ID, s.Task.StallCount)
			fmt.Printf("⚡ [FallbackAgent] Bypassing task %s to sovereign Fallback Agent\n", s.Task.ID)
			u.sendCmd(&BypassToFallbackCmd{
				TaskID:    s.Task.ID,
				Reason:    reason,
				Directive: fmt.Sprintf("SOVEREIGN REPAIR DIRECTIVE: Task %s has stalled %d times. Fallback Agent must execute with full cross-domain authority.", s.Task.ID, s.Task.StallCount),
			})
			continue
		}

		// Fast-path regex pre-filter for zero-token unblocking
		if len(s.RecentLogs) > 0 {
			combinedLogs := strings.Join(s.RecentLogs, "\n")
			fp := FastPathClassify(combinedLogs)
			if fp.Matched {
				fmt.Printf("⚡ [FallbackAgent] Fast-path regex hit (%s) for task %s\n", fp.Reason, s.Task.ID)
				u.sendCmd(&ResetTaskCmd{TaskID: s.Task.ID, Reason: fp.Reason, Directive: fp.Directive})
				continue
			}
		}
		remainingStalls = append(remainingStalls, s)
	}

	if len(remainingStalls) == 0 {
		return
	}

	fmt.Printf("🔍 [FallbackAgent] Detected %d stall(s). Assessing...\n", len(remainingStalls))

	if u.llmAssessment && u.llmClient != nil {
		fresh := u.filterStallsForLLMAssessment(state, remainingStalls)
		if len(fresh) == 0 {
			fmt.Printf("FallbackAgent: all %d stall(s) are within the LLM assessment cooldown; skipping re-assessment.\n", len(remainingStalls))
			return
		}
		u.assessWithLLM(ctx, state, fresh)
	} else {
		u.assessHeuristic(ctx, remainingStalls)
	}
}

// assessWithLLM calls the LLM to diagnose stalls and dispatches corrective commands.
func (u *FallbackAgent) assessWithLLM(ctx context.Context, state *domain.State, stalls []StalledTask) {
	prompt := buildUnblockerPrompt(state, stalls)
	unblockerCtx := context.WithValue(ctx, AgentRoleKey, "fallback")
	// Compaction must never rewrite the JSON-schema suffix of the prompt.
	unblockerCtx = domain.WithUncompactableTail(unblockerCtx, len(unblockerPromptTail))
	resp, err := u.llmClient.Complete(unblockerCtx, prompt)
	if err != nil {
		fmt.Printf("FallbackAgent: LLM assessment failed: %v. Falling back to heuristic.\n", err)
		u.assessHeuristic(ctx, stalls)
		return
	}

	tokens := llm.EstimateUsageTokens(prompt, resp)
	u.sendCmd(&LogFallbackActionCmd{
		Message:    fmt.Sprintf("FallbackAgent LLM assessment: reasoning=%s", resp.Reasoning),
		TokensUsed: tokens,
	})

	for _, action := range resp.Actions {
		switch action.Tool {
		case "bypass_to_fallback", "bypass_to_last_resort":
			taskID, _ := action.Args["task_id"].(string)
			reason, _ := action.Args["reason"].(string)
			if taskID != "" {
				fmt.Printf("⚡ [FallbackAgent] LLM triggered bypass to FallbackAgent for task %s: %s\n", taskID, reason)
				u.sendCmd(&BypassToFallbackCmd{TaskID: taskID, Reason: reason})
			}
		case "scope_triage":
			reason, _ := action.Args["reason"].(string)
			fmt.Printf("✂ [FallbackAgent] LLM triggered scope triage: %s\n", reason)
			u.sendCmd(&ScopeTriageCmd{Reason: reason, KeepStories: 2})
		case "reset_task":
			taskID, _ := action.Args["task_id"].(string)
			reason, _ := action.Args["reason"].(string)
			if taskID != "" {
				directive := fmt.Sprintf("Diagnostic Guidance from Fallback Agent: %s", reason)
				fmt.Printf("🔧 [FallbackAgent] Resetting task %s: %s\n", taskID, reason)
				u.sendCmd(&ResetTaskCmd{TaskID: taskID, Reason: reason, Directive: directive})
			}
		case "fail_task":
			taskID, _ := action.Args["task_id"].(string)
			reason, _ := action.Args["reason"].(string)
			if taskID != "" {
				fmt.Printf("❌ [FallbackAgent] Failing task %s: %s\n", taskID, reason)
				u.sendCmd(&FailTaskCmd{TaskID: taskID, Reason: reason})
			}
		case "log_message":
			msg, _ := action.Args["message"].(string)
			if msg != "" {
				u.sendCmd(&LogFallbackActionCmd{Message: msg})
			}
		case "noop":
			fmt.Printf("FallbackAgent: LLM chose noop — stalls appear transient.\n")
		}
	}
}

// assessHeuristic applies deterministic corrective actions without calling the LLM.
// Used when llm_assessment is false or the LLM call fails.
func (u *FallbackAgent) assessHeuristic(ctx context.Context, stalls []StalledTask) {
	for _, s := range stalls {
		switch s.Reason {
		case StallReasonFrozenProgress, StallReasonOrphanedTask:
			maxAllowed := s.Task.MaxRetries
			if u.maxRetries > 0 && (maxAllowed == 0 || u.maxRetries < maxAllowed) {
				maxAllowed = u.maxRetries
			}
			if maxAllowed > 0 && s.Task.Retries >= maxAllowed {
				reason := fmt.Sprintf("heuristic: task frozen for %s and max retries (%d) reached", s.StalledForStr, maxAllowed)
				fmt.Printf("❌ [FallbackAgent] Heuristic fail task %s: %s\n", s.Task.ID, reason)
				u.sendCmd(&FailTaskCmd{TaskID: s.Task.ID, Reason: reason})
			} else {
				reason := fmt.Sprintf("heuristic: task frozen for %s (reason: %s)", s.StalledForStr, s.ReasonStr)
				fmt.Printf("🔧 [FallbackAgent] Heuristic reset task %s: %s\n", s.Task.ID, reason)
				u.sendCmd(&ResetTaskCmd{TaskID: s.Task.ID, Reason: reason})
			}
		case StallReasonConflictBlocked:
			reason := fmt.Sprintf("heuristic: task conflict-blocked for %s", s.StalledForStr)
			fmt.Printf("🔧 [FallbackAgent] Heuristic reset conflict-blocked task %s: %s\n", s.Task.ID, reason)
			u.sendCmd(&ResetTaskCmd{TaskID: s.Task.ID, Reason: reason})
		case StallReasonAgentInconsistency:
			if s.Agent != nil {
				reason := fmt.Sprintf("agent %s WORKING on task %s (status=%s)", s.Agent.ID, s.Task.ID, string(s.Task.Status))
				fmt.Printf("🧹 [FallbackAgent] Clearing inconsistent agent %s: %s\n", s.Agent.ID, reason)
				u.sendCmd(&ClearInconsistentAgentCmd{AgentID: s.Agent.ID, Reason: reason})
			}
		case StallReasonPipelineDeadlock:
			msg := "pipeline deadlock detected: story is running but no tasks are IN_PROGRESS or PENDING"
			fmt.Printf("⚠️  [FallbackAgent] %s\n", msg)
			u.sendCmd(&LogFallbackActionCmd{Message: msg})
		}
	}
}
