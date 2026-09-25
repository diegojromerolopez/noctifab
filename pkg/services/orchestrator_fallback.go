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
		projectPath := ""
		if taskState != nil {
			projectPath = taskState.ProjectPath
		}
		contextBlock := buildFallbackContext(currentLog, diffContext, triggerReason, turn, maxTurns, projectPath, o.cfg.SandboxTelemetry.Inject)

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
		llmCtx = domain.WithRoleContext(llmCtx, string(domain.AgentRoleFallback))
		llmCtx = context.WithValue(llmCtx, AgentRoleKey, "fallback")
		contractLen := len(prompts.Contract(prompts.AgentFallback))
		llmCtx = domain.WithUncompactableTail(llmCtx, contractLen)
		if len(promptBody) > contractLen {
			llmCtx = domain.WithCacheablePrefix(llmCtx, len(promptBody)-contractLen)
		}

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

			// E2E Verification Gate:
			// If unit tests passed and an E2E suite is detected, run the E2E verification gate only if within task E2E scope.
			if passed && o.evaluator != nil && o.evaluator.Runner != nil && taskState != nil && taskState.ProjectPath != "" && o.evaluator.shouldValidateE2E(taskState, effectiveTask) {
				e2eMode := o.cfg.E2E.Mode
				e2eCmd := o.cfg.E2E.Command
				detectedE2E := DetectE2ECommand(taskState.ProjectPath, e2eMode, e2eCmd)
				if detectedE2E != "" {
					e2eTimeout := 5 * time.Minute
					if turnTimeout > 0 {
						e2eTimeout = turnTimeout
					}
					e2eCtx, e2eCancel := context.WithTimeout(ctx, e2eTimeout)
					fmt.Printf("🔍 [Fallback Agent] Running E2E verification gate: %q...\n", detectedE2E)
					e2eOut, e2eErr := o.evaluator.Runner.RunCommand(e2eCtx, taskState.ProjectPath, detectedE2E, "")
					e2eCancel()
					if e2eErr != nil {
						if isCommandToolMissing(o.evaluator.Runner, detectedE2E, e2eOut+" "+e2eErr.Error()) {
							fmt.Printf("⚠️  [Fallback Agent Degraded] Task %s: required E2E tool is absent on host (%s). Proceeding in degraded mode.\n", effectiveTask.ID, detectedE2E)
						} else if !isE2EFailureInScope(taskState, effectiveTask, e2eOut+"\n"+e2eErr.Error()) {
							fmt.Printf("⚠️  [Fallback Agent] Task %s E2E failure(s) are outside the scope of active feature; ignoring out-of-scope failure in fallback loop.\n", effectiveTask.ID)
						} else {
							passed = false
							newLogMsg = fmt.Sprintf("Unit tests passed, but E2E verification failed (%s):\n%s\n%v", detectedE2E, e2eOut, e2eErr)
							fmt.Printf("⚠️ [Fallback Agent] E2E verification failed: %v\n", e2eErr)
						}
					} else {
						fmt.Printf("✅ [Fallback Agent] E2E verification gate PASSED (%s)!\n", detectedE2E)
					}
				}
			}

			// Anti-Stub & Anti-Gaming Quality Gate:
			if passed && taskState != nil && taskState.ProjectPath != "" {
				antiStub := NewAntiStubValidator()
				violations, _ := antiStub.ValidateWorkspace(taskState.ProjectPath, effectiveTask.TargetFiles)
				if len(violations) > 0 {
					passed = false
					var sb strings.Builder
					fmt.Fprintf(&sb, "Anti-stub / anti-gaming validation failed with %d violation(s):\n", len(violations))
					for _, v := range violations {
						fmt.Fprintf(&sb, "- %s:%d: [%s] %s\n", v.Path, v.Line, v.Rule, v.Snippet)
					}
					newLogMsg = sb.String()
					fmt.Printf("⚠️ [Fallback Agent] Anti-Stub validation failed with %d violation(s)\n", len(violations))
				}
			}

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

func buildFallbackContext(failureLog, diffContext, triggerReason string, turn, maxTurns int, projectPath string, telemetryInject bool) string {
	var sb strings.Builder
	sb.WriteString("\n### 🎯 FALLBACK SOVEREIGN DIAGNOSTIC CONTEXT:\n")
	fmt.Fprintf(&sb, "* **Trigger Reason:** %s\n", triggerReason)
	fmt.Fprintf(&sb, "* **Active Turn:** %d of %d\n\n", turn, maxTurns)

	sb.WriteString("#### Failing Error / Test Trace:\n```\n")
	sanitizedLog := SanitizeLog(failureLog)
	sb.WriteString(summarizeFailureLog(sanitizedLog))
	sb.WriteString("\n```\n\n")

	if telemetryInject && projectPath != "" {
		if spans, err := telemetry.ReadRecentSpans(projectPath, 15); err == nil && len(spans) > 0 {
			sb.WriteString(telemetry.FormatSpansForPrompt(spans))
			sb.WriteString("\n\n")
		}
	}

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
