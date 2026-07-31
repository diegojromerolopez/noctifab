package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// RunGeneratorAgent runs the generator agent to implement task functionality
func (o *Orchestrator) RunGeneratorAgent(ctx context.Context, task domain.Task, state *domain.State, fileContexts []string, recentTestsContext string, customPrompt string) {
	// Reader Phase: collect inspection context first!
	readerContexts := o.RunReaderPhase(ctx, "generator", task, state)

	genPrompt := customPrompt
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
	// Add previous failure context if retrying
	if task.Retries > 0 && task.FailureLog != "" {
		warning := "WARNING: The previous implementation/refactoring changes from the failed attempt have been preserved in the workspace files. You must inspect the existing code/tests, identify the bugs, and modify the files to fix the failures."
		summary := summarizeFailureLog(task.FailureLog)
		promptContext = append(promptContext, fmt.Sprintf("%s\n\nPrevious implementation attempt FAILED. Key failure details from the test run:\n%s\n\nFix the code to address these specific errors.", warning, summary))
	}

	if len(promptContext) > 0 {
		genPrompt = fmt.Sprintf("%s\n\n%s", customPrompt, strings.Join(promptContext, "\n\n"))
	}

	genCtx := context.WithValue(ctx, AgentRoleKey, "generator")
	o.registerAgentStart(ctx, "generator", task.ID)

	currentPrompt := genPrompt
	maxTurns := 5
	var lastErr error
	runTestsCalled := false
	testFixRequestCount := 0
	diagCache := NewTaskDiagnosticCache(o.cfg.GetWorkspaceCache().IsEnabled())

	for turn := 0; turn < maxTurns; turn++ {
		resp, err := o.llmClient.Complete(genCtx, currentPrompt)
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

			tool, ok := o.registry.Get(action.Tool)
			if ok {
				out, execErr := tool.Execute(genCtx, state, action.Args)
				diagCache.OnToolExecuted(action.Tool, action.Args, out, execErr)
				fmt.Printf("🛠️  [Tool Executed] task=%s role=GENERATOR tool=%s success=%t\n", task.ID, action.Tool, execErr == nil)
				if execErr != nil {
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s failed: %v\nOutput: %s", action.Tool, execErr, out))
					hasNoop = false
				} else {
					executed++
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s executed successfully. Output:\n%s", action.Tool, out))
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

				testerPrompt := fmt.Sprintf("Fix the tests for task: %s - %s\n\nFeedback from generator agent:\n%s\n\nCorrect the test files to resolve this issue.", task.Title, task.Description, feedback)
				o.RunTesterAgent(ctx, task, state, fileContexts, testerPrompt)

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

		// Append errors and tool outputs to currentPrompt for the next turn
		currentPrompt = fmt.Sprintf("%s\n\nTOOL OUTPUTS FROM PREVIOUS TURN (turn %d/%d):\n%s\n\nBased on these outputs, take your next actions. If everything is done and verified, call noop. You have %d turns remaining.",
			genPrompt, turn+1, maxTurns,
			strings.Join(turnToolOutputs, "\n---\n"),
			maxTurns-turn-1)
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
		} else if strings.Contains(line, "Error:") || strings.Contains(line, "Exception") || strings.Contains(line, "FAILED") {
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
