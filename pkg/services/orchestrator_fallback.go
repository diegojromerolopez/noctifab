package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RunFallbackAgent executes the sovereign Fallback Agent (Omni-Agent) to resolve persistent
// blockers, contradictory specifications, or missing sandbox toolchains.
// Returns true if the repair succeeded and tests pass cleanly.
func (o *Orchestrator) RunFallbackAgent(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	failureLog string,
	triggerReason string,
) (bool, string) {
	fbCfg := o.cfg.GetFallback()
	if !fbCfg.Enabled || o.llmClient == nil {
		return false, failureLog
	}

	effectiveTask := domain.Task{
		ID:          "sovereign-repair",
		Title:       "Sovereign Repair",
		Description: "Sovereign repair task",
	}
	if task != nil {
		effectiveTask = *task
	}

	ctx, span := telemetry.Tracer().Start(ctx, "RunFallbackAgent",
		trace.WithAttributes(
			attribute.String("task.id", effectiveTask.ID),
			attribute.String("trigger.reason", triggerReason),
		))
	defer span.End()

	maxTurns := fbCfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 2
	}

	turnTimeout := time.Duration(fbCfg.Timeout)
	if turnTimeout <= 0 {
		turnTimeout = 180 * time.Second
	}

	// 1. Prominent Console & Stderr Alert
	alertMsg := fmt.Sprintf("🚨 [CRITICAL ALERT] Fallback Agent triggered for task %s (Reason: %s)!", effectiveTask.ID, triggerReason)
	fmt.Fprintf(os.Stderr, "%s\n", alertMsg)
	fmt.Printf("%s Starting sovereign repair (Max turns: %d)...\n", alertMsg, maxTurns)

	// 2. Telemetry & Execution Observer Recording
	o.registerAgentStart(ctx, string(domain.AgentRoleFallback), effectiveTask.ID)
	if o.observer != nil {
		o.observer.Observe(ctx, domain.ExecutionEvent{
			Kind:          domain.EventFindingRecorded,
			TaskID:        effectiveTask.ID,
			AgentRole:     string(domain.AgentRoleFallback),
			ErrorCategory: "CRITICAL_FALLBACK_TRIGGERED",
			Evidence:      fmt.Sprintf("Trigger: %s; Summary: %s", triggerReason, summarizeFailureLog(failureLog)),
			At:            time.Now().UTC(),
		})
	}

	// 3. Database Persistence: Record Trigger in State.LastActions & Task Metadata
	if task != nil {
		task.LastResortUsed = true
		task.FallbackUsed = true
	}
	if taskState != nil {
		// Update local copy so unit tests and subsequent steps see it
		for i := range taskState.Tasks {
			if taskState.Tasks[i].ID == effectiveTask.ID {
				taskState.Tasks[i].LastResortUsed = true
				taskState.Tasks[i].FallbackUsed = true
				break
			}
		}
		triggerAction := domain.Action{
			Timestamp: time.Now().UTC(),
			Tool:      "fallback_agent_trigger",
			Reasoning: fmt.Sprintf("CRITICAL: Summoned Fallback Agent for task %s due to %s", effectiveTask.ID, triggerReason),
			Result:    summarizeFailureLog(failureLog),
			Success:   false,
		}
		taskState.LastActions = append(taskState.LastActions, triggerAction)

		saveErr := o.updateStateWithRetry(ctx, func(st *domain.State) error {
			for i := range st.Tasks {
				if st.Tasks[i].ID == effectiveTask.ID {
					st.Tasks[i].LastResortUsed = true
					st.Tasks[i].FallbackUsed = true
					break
				}
			}
			st.LastActions = append(st.LastActions, triggerAction)
			return nil
		})
		if saveErr != nil {
			fmt.Fprintf(os.Stderr, "⚠ [Fallback Agent] State save on trigger failed: %v\n", saveErr)
		}
	}

	currentLog := failureLog

	for turn := 1; turn <= maxTurns; turn++ {
		fmt.Printf("🔧 [Fallback Agent] Starting sovereign turn %d/%d for task %s...\n", turn, maxTurns, effectiveTask.ID)

		// Collect recent diff context from git
		diffContext := ""
		if taskGit != nil {
			diffOut, _ := taskGit.Run(ctx, false, "diff", "HEAD~1")
			if strings.TrimSpace(diffOut) == "" {
				diffOut, _ = taskGit.Run(ctx, false, "diff")
			}
			diffContext = diffOut
		}

		// Assemble multi-file context block with secret sanitization
		contextBlock := buildFallbackContext(currentLog, diffContext, triggerReason, turn, maxTurns)

		data := prompts.TaskPromptData{
			Title:       effectiveTask.Title,
			Description: effectiveTask.Description,
			Context:     contextBlock,
			TargetFiles: effectiveTask.TargetFiles,
		}

		var promptBody string
		if o.promptRenderer != nil {
			rendered, err := o.promptRenderer.Render(prompts.AgentFallback, "repair", data)
			if err == nil {
				promptBody = rendered.Full()
			}
		}

		if promptBody == "" {
			promptBody = fmt.Sprintf("Fallback Agent Repair:\nTask: %s - %s\n%s\n%s",
				effectiveTask.Title, effectiveTask.Description, contextBlock, prompts.Contract(prompts.AgentFallback))
		}

		llmCtx, cancel := context.WithTimeout(ctx, turnTimeout)
		llmCtx = context.WithValue(llmCtx, AgentRoleKey, "fallback")
		llmCtx = domain.WithUncompactableTail(llmCtx, len(prompts.Contract(prompts.AgentFallback)))

		resp, err := o.llmClient.Complete(llmCtx, promptBody)
		cancel()
		o.recordTokenUsage(ctx, promptBody, resp)

		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠ [Fallback Agent] Turn %d LLM call failed: %v\n", turn, err)
			continue
		}

		if resp != nil && len(resp.Actions) > 0 {
			for _, action := range resp.Actions {
				if action.Tool == "noop" {
					continue
				}
				if o.registry != nil {
					tool, ok := o.registry.Get(action.Tool)
					if ok {
						_, execErr := tool.Execute(ctx, taskState, action.Args)
						if execErr != nil {
							fmt.Fprintf(os.Stderr, "⚠ [Fallback Tool Failed] %s: %v\n", action.Tool, execErr)
						}
					}
				}
			}

			// Stage and commit changes with standardized tag
			if taskGit != nil {
				if commitErr := o.stageAndCommit(ctx, taskGit, effectiveTask.ID,
					"fix(fallback): sovereign unblock for task %s - %s [turn %d/%d]", effectiveTask.Title, turn, maxTurns); commitErr != nil {
					fmt.Fprintf(os.Stderr, "⚠ [Fallback Agent] Git commit failed on turn %d: %v\n", turn, commitErr)
				}
			}
		}

		// Re-evaluate tests
		if o.evaluator != nil {
			passed, newLogMsg, _ := o.evaluator.ValidateTask(ctx, taskState, effectiveTask)
			if passed {
				successMsg := fmt.Sprintf("✨ [Fallback Agent] Sovereign unblock successful on turn %d/%d for task %s!", turn, maxTurns, effectiveTask.ID)
				fmt.Println(successMsg)
				fmt.Fprintf(os.Stderr, "%s\n", successMsg)

				o.registerAgentComplete(ctx, string(domain.AgentRoleFallback), effectiveTask.ID, nil)
				if taskState != nil {
					successAction := domain.Action{
						Timestamp: time.Now().UTC(),
						Tool:      "fallback_agent_success",
						Reasoning: fmt.Sprintf("Sovereign repair succeeded on turn %d/%d for task %s", turn, maxTurns, effectiveTask.ID),
						Result:    "All tests passed after sovereign repair",
						Success:   true,
					}
					taskState.LastActions = append(taskState.LastActions, successAction)

					saveErr := o.updateStateWithRetry(ctx, func(st *domain.State) error {
						st.LastActions = append(st.LastActions, successAction)
						return nil
					})
					if saveErr != nil {
						fmt.Fprintf(os.Stderr, "⚠ [Fallback Agent] State save on success failed: %v\n", saveErr)
					}
				}
				return true, newLogMsg
			}
			currentLog = newLogMsg
		}
	}

	failAlert := fmt.Sprintf("🚨 [CRITICAL ALERT] Fallback Agent completed %d turns without resolving task %s.", maxTurns, effectiveTask.ID)
	fmt.Fprintf(os.Stderr, "%s Remaining failure trace:\n%s\n", failAlert, currentLog)

	o.registerAgentComplete(ctx, string(domain.AgentRoleFallback), effectiveTask.ID, fmt.Errorf("fallback repair unexhausted after %d turns", maxTurns))
	if taskState != nil {
		failAction := domain.Action{
			Timestamp: time.Now().UTC(),
			Tool:      "fallback_agent_failed",
			Reasoning: fmt.Sprintf("Fallback Agent failed to unblock task %s after %d turns", effectiveTask.ID, maxTurns),
			Result:    summarizeFailureLog(currentLog),
			Success:   false,
		}
		taskState.LastActions = append(taskState.LastActions, failAction)

		saveErr := o.updateStateWithRetry(ctx, func(st *domain.State) error {
			st.LastActions = append(st.LastActions, failAction)
			return nil
		})
		if saveErr != nil {
			fmt.Fprintf(os.Stderr, "⚠ [Fallback Agent] State save on failure failed: %v\n", saveErr)
		}
	}

	return false, currentLog
}

// RunLastResortAgent is a backwards-compatible delegator to RunFallbackAgent.
func (o *Orchestrator) RunLastResortAgent(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	failureLog string,
	triggerReason string,
) (bool, string) {
	return o.RunFallbackAgent(ctx, task, taskState, taskGit, failureLog, triggerReason)
}

func buildFallbackContext(failureLog, diffContext, triggerReason string, turn, maxTurns int) string {
	var sb strings.Builder
	sb.WriteString("\n### 🎯 FALLBACK SOVEREIGN DIAGNOSTIC CONTEXT:\n")
	fmt.Fprintf(&sb, "* **Trigger Reason:** %s\n", triggerReason)
	fmt.Fprintf(&sb, "* **Active Turn:** %d of %d\n\n", turn, maxTurns)

	sb.WriteString("#### Failing Error / Test Trace:\n```\n")
	sanitizedLog := SanitizeLog(failureLog)
	sb.WriteString(summarizeFailureLog(sanitizedLog))
	sb.WriteString("\n```\n\n")

	if strings.TrimSpace(diffContext) != "" {
		sanitizedDiff := SanitizeLog(diffContext)
		sb.WriteString("#### Recent Git Diff Context:\n```diff\n")
		if len(sanitizedDiff) > 16000 {
			sanitizedDiff = sanitizedDiff[:16000] + "\n...[diff truncated]..."
		}
		sb.WriteString(sanitizedDiff)
		sb.WriteString("\n```\n\n")
	}

	sb.WriteString("Apply the 4-Tier Compromise Hierarchy to resolve this failure. Ensure all code and tests compile cleanly.")
	return sb.String()
}
