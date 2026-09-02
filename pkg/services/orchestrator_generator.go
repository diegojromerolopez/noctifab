package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
)

// RunGeneratorAgent runs the generator agent to implement task functionality.
// action selects the generator prompt template (see the prompts package
// catalog: implement, refactor, fix, single_pass, single_pass_fix,
// implement_breadth_first, implement_breadth_first_fix, surgical_repair).
func (o *Orchestrator) RunGeneratorAgent(ctx context.Context, task domain.Task, state *domain.State, fileContexts []string, recentTestsContext string, action string) {
	// Fail fast on unknown actions before doing any reader-phase work.
	if err := prompts.ValidateKey(prompts.AgentGenerator, action); err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Task %s [Generator] invalid prompt action: %v\n", task.ID, err)
		o.registerAgentStart(ctx, "generator", task.ID)
		o.registerAgentComplete(ctx, "generator", task.ID, err)
		return
	}

	// Reader Phase: collect inspection context first (skip for single-turn surgical repair)
	var readerContexts []string
	if action != "surgical_repair" {
		readerContexts = o.RunReaderPhase(ctx, "generator", task, state)
	}

	var promptContext []string
	if len(fileContexts) > 0 {
		promptContext = append(promptContext, fmt.Sprintf("Existing files context:\n%s", strings.Join(fileContexts, "\n\n")))
	}
	if len(readerContexts) > 0 {
		promptContext = append(promptContext, fmt.Sprintf("Inspection context gathered:\n%s", strings.Join(readerContexts, "\n\n")))
	}
	if recentTestsContext != "" {
		promptContext = append(promptContext, recentTestsContext)
	}
	// Add previous stall recovery directive if task was unblocked
	if task.RecoveryDirective != "" {
		promptContext = append(promptContext, fmt.Sprintf("### ⚠️ PREVIOUS ATTEMPT STALL RECOVERY DIRECTIVE\n%s", task.RecoveryDirective))
	}
	// Add user human-in-the-loop steering directives
	if len(task.UserDirectives) > 0 {
		promptContext = append(promptContext, fmt.Sprintf("### 🎯 [USER HUMAN-IN-THE-LOOP STEERING DIRECTIVES]\n%s", strings.Join(task.UserDirectives, "\n")))
	}
	// Add previous failure context and git diff if retrying
	if task.Retries > 0 {
		if task.FailureEnvelope != nil {
			var sb strings.Builder
			fmt.Fprintf(&sb, "### ⚠️ PREVIOUS ATTEMPT FAILURE DIAGNOSTICS (Stage: %s)\n", task.FailureEnvelope.Stage)
			if task.FailureEnvelope.Command != "" {
				fmt.Fprintf(&sb, "- **Command:** `%s`\n", task.FailureEnvelope.Command)
			}
			if task.FailureEnvelope.ExitCode != 0 {
				fmt.Fprintf(&sb, "- **Exit Code:** `%d`\n", task.FailureEnvelope.ExitCode)
			}
			if len(task.FailureEnvelope.FailingFiles) > 0 {
				fmt.Fprintf(&sb, "- **Failing Files:** `%s`\n", strings.Join(task.FailureEnvelope.FailingFiles, "`, `"))
			}
			if task.FailureEnvelope.Stderr != "" || task.FailureEnvelope.Stdout != "" {
				sb.WriteString("\n#### Output Logs:\n```\n")
				if task.FailureEnvelope.Stdout != "" {
					sb.WriteString(task.FailureEnvelope.Stdout)
					if !strings.HasSuffix(task.FailureEnvelope.Stdout, "\n") {
						sb.WriteString("\n")
					}
				}
				if task.FailureEnvelope.Stderr != "" {
					sb.WriteString(task.FailureEnvelope.Stderr)
					if !strings.HasSuffix(task.FailureEnvelope.Stderr, "\n") {
						sb.WriteString("\n")
					}
				}
				sb.WriteString("```\n")
			}
			promptContext = append(promptContext, sb.String())
		} else if task.FailureLog != "" {
			warning := "WARNING: The previous implementation/refactoring changes from the failed attempt have been preserved in the workspace files. You must inspect the existing code/tests, identify the bugs, and modify the files to fix the failures."
			summary := summarizeFailureLog(task.FailureLog)
			promptContext = append(promptContext, fmt.Sprintf("%s\n\nPrevious implementation attempt FAILED. Key failure details from the test run:\n%s\n\nFix the code to address these specific errors.", warning, summary))
		}
		if diffOut, dErr := o.git.Run(ctx, false, "diff"); dErr == nil && strings.TrimSpace(diffOut) != "" {
			promptContext = append(promptContext, fmt.Sprintf("### 🔍 FAILED ATTEMPT GIT DIFF\nThe following diff shows the exact changes made in the failed attempt:\n```diff\n%s\n```", strings.TrimSpace(diffOut)))
		}
	}

	contextBlock := ""
	if len(promptContext) > 0 {
		contextBlock = "\n\n" + strings.Join(promptContext, "\n\n")
	}

	rendered, err := o.promptRenderer.Render(prompts.AgentGenerator, action, prompts.TaskPromptData{
		Title:              task.Title,
		Description:        task.Description,
		Context:            contextBlock,
		RecentTestsContext: recentTestsContext,
		RecoveryDirective:  task.RecoveryDirective,
		TargetFiles:        task.TargetFiles,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Task %s [Generator] prompt rendering failed for action %q: %v\n", task.ID, action, err)
		o.registerAgentStart(ctx, "generator", task.ID)
		o.registerAgentComplete(ctx, "generator", task.ID, err)
		return
	}
	genPrompt := rendered.Full()

	genCtx := context.WithValue(ctx, AgentRoleKey, "generator")
	// Compaction must never rewrite the output contract at the end of the prompt.
	genCtx = domain.WithUncompactableTail(genCtx, len(rendered.Contract))
	o.registerAgentStart(ctx, "generator", task.ID)

	currentPrompt := genPrompt
	maxTurns := iterationsOrDefault(o.cfg.GeneratorsIterations)
	if action == "surgical_repair" {
		maxTurns = 2
	}
	var lastErr error
	runTestsCalled := false
	testFixRequestCount := 0
	diagCache := NewTaskDiagnosticCache(o.cfg.GetWorkspaceCache().IsEnabled())
	diagCache.SeedContexts(fileContexts, readerContexts)
	// consecutiveLinterFailures tracks back-to-back run_linter failures without
	// any file mutation in between. When it reaches 2, run_linter is skipped
	// for the remainder of this task to prevent the stale-cache lock-in spiral.
	consecutiveLinterFailures := 0
	linterDeferred := false
	seenFileDependentCalls := make(map[string]bool)

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := o.llmClient.Complete(genCtx, currentPrompt)
		o.recordTokenUsage(ctx, currentPrompt, resp)
		if err != nil {
			lastErr = err
			break
		}

		executed := 0
		blocked := 0
		var turnToolOutputs []string
		hasNoop := false

		for _, action := range resp.Actions {
			if action.Tool == "noop" {
				hasNoop = true
				continue
			}
			if action.Tool == "run_tests" {
				runTestsCalled = true
			}
			fmt.Printf("Orchestrator: Task %s [Generator] action: tool=%s args=%+v\n", task.ID, action.Tool, action.Args)
			domainAction := domain.Action{
				Tool: action.Tool,
				Args: action.Args,
			}
			valRes, valErr := o.validator.Validate(genCtx, domainAction, state)
			if valErr != nil || (valRes != nil && !valRes.Allowed) {
				blocked++
				reason := ""
				if valRes != nil {
					reason = valRes.Reason
				}
				fmt.Fprintf(os.Stderr, "Orchestrator: Task %s [Generator] action %s blocked: %s\n", task.ID, action.Tool, reason)
				turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s blocked by policy: %s", action.Tool, reason))
				continue
			}

			// Precondition: A tool that depends on files cannot be called twice with identical arguments if no file mutations have occurred in between
			if IsFileDependentTool(action.Tool) {
				key := buildArgsKey(action.Tool, action.Args)
				if seenFileDependentCalls[key] {
					fmt.Printf("Orchestrator: Task %s [Generator] action %s rejected: duplicate call without file mutations\n", task.ID, action.Tool)
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("[TOOL CALL REJECTED: NO WORKSPACE CHANGES] You have already executed '%s' with identical arguments and no files have been modified since. Re-running inspection or diagnostic tools without modifying code produces identical results. You MUST now call write_file, edit_file, or apply_patch to implement your changes.", action.Tool))
					continue
				}
				seenFileDependentCalls[key] = true
			}

			if cachedOut, cachedErr, hasCache := diagCache.TryGetCachedInspection(action.Tool, action.Args); hasCache {
				fmt.Printf("Orchestrator: Task %s [Generator] inspection action %s served from cache\n", task.ID, action.Tool)
				if cachedErr != nil {
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s failed: %v\nOutput: %s", action.Tool, cachedErr, cachedOut))
				} else {
					executed++
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s executed successfully. Output:\n%s", action.Tool, cachedOut))
				}
				continue
			}

			if cachedOut, cachedErr, hasCache := diagCache.TryGetCachedResult(action.Tool); hasCache {
				fmt.Printf("Orchestrator: Task %s [Generator] diagnostic action %s served from cache\n", task.ID, action.Tool)
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
				out, execErr := tool.Execute(genCtx, state, action.Args)
				diagCache.OnToolExecuted(action.Tool, action.Args, out, execErr)
				fmt.Printf("🛠️  [Tool Executed] task=%s role=GENERATOR tool=%s success=%t\n", task.ID, action.Tool, execErr == nil)
				if execErr != nil {
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s failed: %v\nOutput: %s", action.Tool, execErr, out))
					// Track linter consecutive failures.
					if action.Tool == "run_linter" {
						consecutiveLinterFailures++
						if consecutiveLinterFailures >= 2 {
							linterDeferred = true
							fmt.Fprintf(os.Stderr, "⚠ [Generator] Linter failed %d consecutive times without file changes for task %s — deferring linter enforcement. Tests are the primary quality gate.\n", consecutiveLinterFailures, task.ID)
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
						consecutiveLinterFailures = 0
						seenFileDependentCalls = make(map[string]bool)
					}
				}
			}

			if action.Tool == "request_test_fix" {
				if testFixRequestCount >= 1 {
					errMsg := "Action blocked: 'request_test_fix' limit reached (max 1 request per task execution). Edit the test files directly using write_file/edit_file."
					turnToolOutputs = append(turnToolOutputs, errMsg)
					continue
				}
				testFixRequestCount++

				feedback, _ := action.Args["feedback"].(string)
				fmt.Printf("Orchestrator: Task %s [Generator] requested test fix (count %d): %s\n", task.ID, testFixRequestCount, feedback)

				o.RunTesterAgent(ctx, task, state, fileContexts, "fix", feedback)

				// Stage and commit test fixes
				statusOut, _ := o.git.Run(ctx, false, "status", "--porcelain")
				if strings.TrimSpace(statusOut) != "" {
					_, _ = o.git.Run(ctx, true, "add", "--all", "--", ":!.noctifab")
					stagedOut, _ := o.git.Run(ctx, false, "diff", "--cached", "--name-only")
					if strings.TrimSpace(stagedOut) != "" {
						commitMsg := fmt.Sprintf("test(core): fix tests for task %s - %s (requested by generator)", task.ID, task.Title)
						_, commitErr := o.git.Run(ctx, true, "commit", "-m", commitMsg)
						if commitErr != nil {
							fmt.Fprintf(os.Stderr, "Orchestrator: Git commit failed for task %s test fixes: %v\n", task.ID, commitErr)
						}
					}
				}
			}
		}

		if hasNoop || len(resp.Actions) == 0 {
			if !runTestsCalled {
				fmt.Printf("Orchestrator: Agent returned noop without executing run_tests; auto-triggering run_tests fallback for task %s\n", task.ID)
				runTestsTool, ok := o.registry.Get("run_tests")
				if ok {
					out, execErr := runTestsTool.Execute(genCtx, state, map[string]any{})
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

	o.registerAgentComplete(ctx, "generator", task.ID, lastErr)
}

func summarizeFailureLog(log string) string {
	lines := strings.Split(log, "\n")
	var importantLines []string
	capture := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "ERROR:") || strings.HasPrefix(trimmed, "FAIL:") {
			capture = true
		}
		if capture {
			importantLines = append(importantLines, line)
		} else if strings.Contains(line, "Error:") || strings.Contains(line, "Exception") || strings.Contains(line, "FAILED") || strings.Contains(line, "error:") || strings.Contains(line, "Anti-stub") {
			importantLines = append(importantLines, line)
		}
	}

	if len(importantLines) == 0 {
		// Fallback to last 15 lines if no specific failures are captured
		start := len(lines) - 15
		if start < 0 {
			start = 0
		}
		return strings.Join(lines[start:], "\n")
	}

	return strings.Join(importantLines, "\n")
}
