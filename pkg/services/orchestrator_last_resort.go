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

// RunLastResortAgent executes the sovereign Last-Resort Agent to resolve persistent
// blockers, contradictory specifications, or missing sandbox toolchains.
// Returns true if the repair succeeded and tests pass cleanly.
//
// Triggering the Last-Resort Agent is a critical, worrying event: it is explicitly
// logged to standard output, standard error, the execution event stream, and persisted
// to the state database.
func (o *Orchestrator) RunLastResortAgent(
	ctx context.Context,
	task *domain.Task,
	taskState *domain.State,
	taskGit *GitClient,
	failureLog string,
	triggerReason string,
) (bool, string) {
	if !o.cfg.LastResort.Enabled || o.llmClient == nil {
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

	ctx, span := telemetry.Tracer().Start(ctx, "RunLastResortAgent",
		trace.WithAttributes(
			attribute.String("task.id", effectiveTask.ID),
			attribute.String("trigger.reason", triggerReason),
		))
	defer span.End()

	maxTurns := o.cfg.LastResort.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 2
	}

	turnTimeout := time.Duration(o.cfg.LastResort.Timeout)
	if turnTimeout <= 0 {
		turnTimeout = 180 * time.Second
	}

	// 1. Prominent Console & Stderr Alert
	alertMsg := fmt.Sprintf("🚨 [CRITICAL ALERT] Last-Resort Agent triggered for task %s (Reason: %s)!", effectiveTask.ID, triggerReason)
	fmt.Fprintf(os.Stderr, "%s\n", alertMsg)
	fmt.Printf("%s Starting sovereign repair (Max turns: %d)...\n", alertMsg, maxTurns)

	// 2. Telemetry & Execution Observer Recording
	o.registerAgentStart(ctx, string(domain.AgentRoleLastResort), effectiveTask.ID)
	if o.observer != nil {
		o.observer.Observe(ctx, domain.ExecutionEvent{
			Kind:          domain.EventFindingRecorded,
			TaskID:        effectiveTask.ID,
			AgentRole:     string(domain.AgentRoleLastResort),
			ErrorCategory: "CRITICAL_LAST_RESORT_TRIGGERED",
			Evidence:      fmt.Sprintf("Trigger: %s; Summary: %s", triggerReason, summarizeFailureLog(failureLog)),
			At:            time.Now().UTC(),
		})
	}

	// 3. Database Persistence: Record Trigger in State.LastActions & Task Metadata
	if task != nil {
		task.LastResortUsed = true
	}
	if taskState != nil {
		for i := range taskState.Tasks {
			if taskState.Tasks[i].ID == effectiveTask.ID {
				taskState.Tasks[i].LastResortUsed = true
				break
			}
		}
		taskState.LastActions = append(taskState.LastActions, domain.Action{
			Timestamp: time.Now().UTC(),
			Tool:      "last_resort_agent_trigger",
			Reasoning: fmt.Sprintf("CRITICAL: Summoned Last-Resort Agent for task %s due to %s", effectiveTask.ID, triggerReason),
			Result:    summarizeFailureLog(failureLog),
			Success:   false,
		})
		if o.repo != nil {
			if saveErr := o.repo.Save(ctx, taskState); saveErr != nil {
				fmt.Fprintf(os.Stderr, "⚠ [Last-Resort Agent] State save on trigger failed: %v\n", saveErr)
			}
		}
	}

	currentLog := failureLog

	for turn := 1; turn <= maxTurns; turn++ {
		fmt.Printf("🔧 [Last-Resort Agent] Starting sovereign turn %d/%d for task %s...\n", turn, maxTurns, effectiveTask.ID)

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
		contextBlock := buildLastResortContext(currentLog, diffContext, triggerReason, turn, maxTurns)

		data := prompts.TaskPromptData{
			Title:       effectiveTask.Title,
			Description: effectiveTask.Description,
			Context:     contextBlock,
			TargetFiles: effectiveTask.TargetFiles,
		}

		var promptBody string
		if o.promptRenderer != nil {
			rendered, err := o.promptRenderer.Render(prompts.AgentLastResort, "repair", data)
			if err == nil {
				promptBody = rendered.Full()
			}
		}

		if promptBody == "" {
			promptBody = fmt.Sprintf("Last-Resort Agent Repair:\nTask: %s - %s\n%s\n%s",
				effectiveTask.Title, effectiveTask.Description, contextBlock, prompts.Contract(prompts.AgentLastResort))
		}

		llmCtx, cancel := context.WithTimeout(ctx, turnTimeout)
		llmCtx = context.WithValue(llmCtx, AgentRoleKey, "last_resort")
		llmCtx = domain.WithUncompactableTail(llmCtx, len(prompts.Contract(prompts.AgentLastResort)))

		resp, err := o.llmClient.Complete(llmCtx, promptBody)
		cancel()
		o.recordTokenUsage(ctx, promptBody, resp)

		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠ [Last-Resort Agent] Turn %d LLM call failed: %v\n", turn, err)
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
							fmt.Fprintf(os.Stderr, "⚠ [Last-Resort Tool Failed] %s: %v\n", action.Tool, execErr)
						}
					}
				}
			}

			// Stage and commit changes with standardized tag
			if taskGit != nil {
				if commitErr := o.stageAndCommit(ctx, taskGit, effectiveTask.ID,
					"fix(lra): sovereign unblock for task %s - %s [turn %d/%d]", effectiveTask.Title, turn, maxTurns); commitErr != nil {
					fmt.Fprintf(os.Stderr, "⚠ [Last-Resort Agent] Git commit failed on turn %d: %v\n", turn, commitErr)
				}
			}
		}

		// Re-evaluate tests
		if o.evaluator != nil {
			passed, newLogMsg, _ := o.evaluator.ValidateTask(ctx, taskState, effectiveTask)
			if passed {
				successMsg := fmt.Sprintf("✨ [Last-Resort Agent] Sovereign unblock successful on turn %d/%d for task %s!", turn, maxTurns, effectiveTask.ID)
				fmt.Println(successMsg)
				fmt.Fprintf(os.Stderr, "%s\n", successMsg)

				o.registerAgentComplete(ctx, string(domain.AgentRoleLastResort), effectiveTask.ID, nil)
				if taskState != nil {
					taskState.LastActions = append(taskState.LastActions, domain.Action{
						Timestamp: time.Now().UTC(),
						Tool:      "last_resort_agent_success",
						Reasoning: fmt.Sprintf("Sovereign repair succeeded on turn %d/%d for task %s", turn, maxTurns, effectiveTask.ID),
						Result:    "All tests passed after sovereign repair",
						Success:   true,
					})
					if o.repo != nil {
						if saveErr := o.repo.Save(ctx, taskState); saveErr != nil {
							fmt.Fprintf(os.Stderr, "⚠ [Last-Resort Agent] State save on success failed: %v\n", saveErr)
						}
					}
				}
				return true, newLogMsg
			}
			currentLog = newLogMsg
		}
	}

	failAlert := fmt.Sprintf("🚨 [CRITICAL ALERT] Last-Resort Agent completed %d turns without resolving task %s.", maxTurns, effectiveTask.ID)
	fmt.Fprintf(os.Stderr, "%s Remaining failure trace:\n%s\n", failAlert, currentLog)

	o.registerAgentComplete(ctx, string(domain.AgentRoleLastResort), effectiveTask.ID, fmt.Errorf("last resort repair unexhausted after %d turns", maxTurns))
	if taskState != nil {
		taskState.LastActions = append(taskState.LastActions, domain.Action{
			Timestamp: time.Now().UTC(),
			Tool:      "last_resort_agent_failed",
			Reasoning: fmt.Sprintf("Last-Resort Agent failed to unblock task %s after %d turns", effectiveTask.ID, maxTurns),
			Result:    summarizeFailureLog(currentLog),
			Success:   false,
		})
		if o.repo != nil {
			if saveErr := o.repo.Save(ctx, taskState); saveErr != nil {
				fmt.Fprintf(os.Stderr, "⚠ [Last-Resort Agent] State save on failure failed: %v\n", saveErr)
			}
		}
	}

	return false, currentLog
}

func buildLastResortContext(failureLog, diffContext, triggerReason string, turn, maxTurns int) string {
	var sb strings.Builder
	sb.WriteString("\n### 🎯 LAST-RESORT SOVEREIGN DIAGNOSTIC CONTEXT:\n")
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
