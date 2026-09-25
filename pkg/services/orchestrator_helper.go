package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RunTesterAgent runs the test writer agent for the task. action selects the
// tester prompt template (see the prompts package catalog: write, fix,
// refactor, write_breadth_first); feedback carries the generator's test-fix
// feedback for the fix action (empty otherwise).
func (o *Orchestrator) RunTesterAgent(ctx context.Context, task domain.Task, state *domain.State, fileContexts []string, action string, feedback string) {
	ctx, span := telemetry.Tracer().Start(ctx, "RunTesterAgent",
		trace.WithAttributes(
			attribute.String("task.id", task.ID),
			attribute.String("action", action),
		))
	defer span.End()

	// Fail fast on unknown actions before doing any reader-phase work.
	if err := prompts.ValidateKey(prompts.AgentTester, action); err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Task %s [Tester] invalid prompt action: %v\n", task.ID, err)
		o.registerAgentStart(ctx, "tester", task.ID)
		o.registerAgentComplete(ctx, "tester", task.ID, err)
		return
	}

	// Reader Phase: collect inspection context first! (Requirement 5)
	readerContexts := o.RunReaderPhase(ctx, "tester", task, state)

	var promptContext []string
	if len(fileContexts) > 0 {
		promptContext = append(promptContext, fmt.Sprintf("Existing files context:\n%s", strings.Join(fileContexts, "\n\n")))
	}
	if len(readerContexts) > 0 {
		promptContext = append(promptContext, fmt.Sprintf("Inspection context gathered:\n%s", strings.Join(readerContexts, "\n\n")))
	}
	if len(task.UserDirectives) > 0 {
		promptContext = append(promptContext, fmt.Sprintf("### 🎯 [USER HUMAN-IN-THE-LOOP STEERING DIRECTIVES]\n%s", strings.Join(task.UserDirectives, "\n")))
	}
	if task.Retries > 0 && task.FailureLog != "" {
		warning := "WARNING: The previous implementation/refactoring changes from the failed attempt have been preserved in the workspace files. You must inspect the existing code/tests, identify the bugs, and modify the files to fix the failures."
		summary := summarizeFailureLog(task.FailureLog)
		promptContext = append(promptContext, fmt.Sprintf("%s\n\nPrevious implementation attempt FAILED. Key failure details from the test run:\n%s\n\nFix the tests to address these specific errors.", warning, summary))
	}
	contextBlock := ""
	if len(promptContext) > 0 {
		contextBlock = "\n\n" + strings.Join(promptContext, "\n\n")
	}

	rendered, err := o.promptRenderer.Render(prompts.AgentTester, action, prompts.TaskPromptData{
		Title:             task.Title,
		Description:       task.Description,
		Context:           contextBlock,
		Feedback:          feedback,
		RecoveryDirective: task.RecoveryDirective,
		TargetFiles:       task.TargetFiles,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Task %s [Tester] prompt rendering failed for action %q: %v\n", task.ID, action, err)
		o.registerAgentStart(ctx, "tester", task.ID)
		o.registerAgentComplete(ctx, "tester", task.ID, err)
		return
	}
	testPrompt := rendered.Full()

	testerCtx := context.WithValue(ctx, AgentRoleKey, "tester")
	// Compaction must never rewrite the output contract at the end of the prompt.
	testerCtx = domain.WithUncompactableTail(testerCtx, len(rendered.Contract))
	testerCtx = domain.WithCacheablePrefix(testerCtx, len(rendered.Body))
	o.registerAgentStart(ctx, "tester", task.ID)

	currentPrompt := testPrompt
	maxTurns := iterationsOrDefault(o.cfg.TestersIterations)
	var lastErr error
	runTestsCalled := false
	diagCache := NewTaskDiagnosticCache(o.cfg.GetWorkspaceCache().IsEnabled())
	diagCache.SeedContexts(fileContexts, readerContexts)
	// consecutiveLinterFailures tracks back-to-back run_linter failures without
	// any file mutation in between. When it reaches 2, run_linter is skipped
	// for the remainder of this task to prevent the stale-cache lock-in spiral.
	consecutiveLinterFailures := 0
	linterDeferred := false
	seenFileDependentCalls := make(map[string]bool)
	circuitBreaker := NewTaskCircuitBreaker()

	for turn := 0; turn < maxTurns; turn++ {
		circuitBreaker.ResetTurn()
		testResp, err := o.llmClient.Complete(testerCtx, currentPrompt)
		o.recordTokenUsage(ctx, currentPrompt, testResp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator: Task %s [Tester] turn %d LLM call failed: %v\n", task.ID, turn, err)
			lastErr = err
			break
		}

		executed := 0
		blocked := 0
		var turnToolOutputs []string
		hasNoop := false
		fileMutated := false

		for _, action := range testResp.Actions {
			if action.Tool == "noop" {
				hasNoop = true
				continue
			}
			if action.Tool == "run_tests" {
				runTestsCalled = true
			}
			fmt.Printf("Orchestrator: Task %s [Tester] action: tool=%s args=%+v\n", task.ID, action.Tool, action.Args)
			domainAction := domain.Action{
				Tool: action.Tool,
				Args: action.Args,
			}
			valRes, valErr := o.validator.Validate(testerCtx, domainAction, state)
			if valErr != nil || (valRes != nil && !valRes.Allowed) {
				blocked++
				reason := ""
				if valRes != nil {
					reason = valRes.Reason
				}
				fmt.Fprintf(os.Stderr, "Orchestrator: Task %s [Tester] action %s blocked: %s\n", task.ID, action.Tool, reason)
				turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s blocked by policy: %s", action.Tool, reason))
				continue
			}

			// Precondition: check cached inspection before rejecting duplicates
			if cachedOut, cachedErr, hasCache := diagCache.TryGetCachedInspection(action.Tool, action.Args); hasCache {
				fmt.Printf("Orchestrator: Task %s [Tester] inspection action %s served from cache\n", task.ID, action.Tool)
				circuitBreaker.RecordDuplicateInspection()
				warn, forceTurn, reason := circuitBreaker.ShouldBreakReadLoop()
				if forceTurn {
					fmt.Printf("⚡ [Circuit Breaker] Task %s [Tester]: %s\n", task.ID, reason)
					hasNoop = true
					turnToolOutputs = append(turnToolOutputs, reason)
					break
				}
				if cachedErr != nil {
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s failed: %v\nOutput: %s", action.Tool, cachedErr, cachedOut))
				} else {
					executed++
					msg := fmt.Sprintf("Tool %s served from cache (unmodified). Output:\n%s", action.Tool, cachedOut)
					if warn {
						msg += "\n" + reason
					}
					turnToolOutputs = append(turnToolOutputs, msg)
				}
				continue
			}

			// Precondition: A tool that depends on files cannot be called twice with identical arguments if no file mutations have occurred in between
			if IsFileDependentTool(action.Tool) {
				key := buildArgsKey(action.Tool, action.Args)
				if seenFileDependentCalls[key] {
					circuitBreaker.RecordDuplicateInspection()
					warn, forceTurn, reason := circuitBreaker.ShouldBreakReadLoop()
					if forceTurn {
						fmt.Printf("⚡ [Circuit Breaker] Task %s [Tester]: %s\n", task.ID, reason)
						hasNoop = true
						turnToolOutputs = append(turnToolOutputs, reason)
						break
					}
					fmt.Printf("Orchestrator: Task %s [Tester] action %s rejected: duplicate call without file mutations\n", task.ID, action.Tool)
					if warn {
						turnToolOutputs = append(turnToolOutputs, reason)
					} else {
						turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("[TOOL CALL REJECTED: NO WORKSPACE CHANGES] You have already executed '%s' with identical arguments and no files have been modified since. Re-running inspection or diagnostic tools without modifying code produces identical results. You MUST now call write_file, edit_file, or apply_patch to implement your changes, or call 'noop' if verification is complete and tests are passing.", action.Tool))
					}
					continue
				}
				seenFileDependentCalls[key] = true
			}

			if cachedOut, cachedErr, hasCache := diagCache.TryGetCachedResult(action.Tool); hasCache {
				fmt.Printf("Orchestrator: Task %s [Tester] diagnostic action %s served from cache\n", task.ID, action.Tool)
				if cachedErr != nil {
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s failed: %v\nOutput: %s", action.Tool, cachedErr, cachedOut))
				} else {
					executed++
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s executed successfully. Output:\n%s", action.Tool, cachedOut))
				}
				continue
			}

			if action.Tool == "run_linter" && linterDeferred {
				msg := "[LINTER DEFERRED] run_linter skipped: linter failed twice consecutively without file changes. Focus on run_tests — tests are the primary quality gate. Linter cleanup will happen in a later pass."
				turnToolOutputs = append(turnToolOutputs, msg)
				continue
			}

			tool, ok := o.registry.Get(action.Tool)
			if ok {
				out, execErr := tool.Execute(testerCtx, state, action.Args)
				diagCache.OnToolExecuted(action.Tool, action.Args, out, execErr)
				fmt.Printf("🛠️  [Tool Executed] task=%s role=TESTER tool=%s success=%t\n", task.ID, action.Tool, execErr == nil)
				if action.Tool == "run_tests" || action.Tool == "run_e2e_tests" {
					circuitBreaker.RecordTestResult(execErr == nil)
					if execErr == nil {
						fmt.Printf("🚀 [Fast Exit on Verified Green] Task %s [Tester]: explicit %s passed cleanly! Fast-exiting turn loop.\n", task.ID, action.Tool)
						hasNoop = true
					}
				}
				if execErr != nil {
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s failed: %v\nOutput: %s", action.Tool, execErr, out))
					// Track linter consecutive failures.
					if action.Tool == "run_linter" {
						consecutiveLinterFailures++
						if consecutiveLinterFailures >= 2 {
							linterDeferred = true
							fmt.Fprintf(os.Stderr, "⚠ [Tester] Linter failed %d consecutive times without file changes for task %s — deferring linter enforcement. Tests are the primary quality gate.\n", consecutiveLinterFailures, task.ID)
						}
					}
					if action.Tool != "run_linter" || !linterDeferred {
						hasNoop = false
					}
				} else {
					executed++
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s executed successfully. Output:\n%s", action.Tool, out))
					// Reset linter failure counter and duplicate tool tracker on any successful file mutation.
					if IsMutatingTool(action.Tool) {
						fileMutated = true
						consecutiveLinterFailures = 0
						seenFileDependentCalls = make(map[string]bool)
						circuitBreaker.RecordAction(action.Tool, action.Args)
					}
				}
			}
		}

		// Speculative Fast Tool Execution (Local Pre-Validation) for Tester
		if fileMutated && !runTestsCalled {
			runTestsTool, ok := o.registry.Get("run_tests")
			if ok {
				out, execErr := runTestsTool.Execute(testerCtx, state, map[string]any{})
				diagCache.OnToolExecuted("run_tests", map[string]any{}, out, execErr)
				if execErr == nil {
					fmt.Printf("🚀 [Speculative Fast Validation] Task %s [Tester]: tests pass cleanly! Fast-exiting turn loop.\n", task.ID)
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("[Speculative Fast Validation] All tests PASSED cleanly:\n%s", capText(out, 2000)))
					circuitBreaker.RecordTestResult(true)
					runTestsCalled = true
					hasNoop = true
				} else {
					fmt.Printf("ℹ [Speculative Fast Validation] Task %s [Tester]: tests failing: %v\n", task.ID, execErr)
					circuitBreaker.RecordTestResult(false)
				}
			}
		}

		currentProgress := task.Progress
		if circuitBreaker.ConsecutiveTestPasses > 0 && currentProgress < 70 {
			currentProgress = 100
		}
		if tripped, reason := circuitBreaker.ShouldTrip(currentProgress); tripped {
			fmt.Printf("⚡ [Circuit Breaker] Task %s [Tester]: %s\n", task.ID, reason)
			hasNoop = true
		}

		if hasNoop || len(testResp.Actions) == 0 {
			if !runTestsCalled {
				fmt.Printf("Orchestrator: Agent returned noop without executing run_tests; auto-triggering run_tests fallback for task %s\n", task.ID)
				runTestsTool, ok := o.registry.Get("run_tests")
				if ok {
					out, execErr := runTestsTool.Execute(testerCtx, state, map[string]any{})
					if execErr != nil {
						fmt.Printf("Orchestrator: Auto-triggered run_tests failed for task %s: %v. Rejecting noop.\n", task.ID, execErr)
						turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Action 'noop' rejected: auto-triggered run_tests failed: %v\nOutput:\n%s", execErr, out))
						hasNoop = false
					}
				}
			}
			if hasNoop {
				break
			}
		}

		// Append errors and tool outputs to the body for the next turn. The
		// non-overridable output contract stays at the END of the prompt so
		// the JSON schema is the last thing the model reads.
		currentPrompt = fmt.Sprintf("%s\n\nTOOL OUTPUTS FROM PREVIOUS TURN (turn %d/%d):\n%s\n\nBased on these outputs, take your next actions. If everything is done and verified, call noop. You have %d turns remaining.\n%s",
			rendered.Body, turn+1, maxTurns,
			joinCappedToolOutputs(turnToolOutputs),
			maxTurns-turn-1,
			rendered.Contract)
	}

	o.registerAgentComplete(ctx, "tester", task.ID, lastErr)
}
