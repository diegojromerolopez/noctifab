package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

var (
	ErrRepairFailed = errors.New("all repair attempts failed to fix the issue")
)

type FailureCategory int

const (
	FailureUnknown   FailureCategory = iota
	FailureTestLogic FailureCategory = iota
	FailureTimeout   FailureCategory = iota
	FailureCompile   FailureCategory = iota
	FailureSandbox   FailureCategory = iota
)

func (fc FailureCategory) String() string {
	return [...]string{"unknown", "test_logic", "timeout", "compile", "sandbox"}[fc]
}

func CategorizeFailureLog(log string) FailureCategory {
	lower := strings.ToLower(log)
	switch {
	case strings.Contains(lower, "no output produced within idle timeout"),
		strings.Contains(lower, "max wall-clock duration exceeded"),
		strings.Contains(lower, "command killed"):
		return FailureTimeout
	case strings.Contains(lower, "sandbox violation"):
		return FailureSandbox
	case strings.Contains(lower, "compile error"),
		strings.Contains(lower, "syntax error"),
		strings.Contains(lower, "compilation error"):
		return FailureCompile
	case strings.Contains(lower, "error:"),
		strings.Contains(lower, "fail:"),
		strings.Contains(lower, "traceback"):
		return FailureTestLogic
	default:
		return FailureUnknown
	}
}

func buildDiagnosticPrompt(title, description string, watchdogErr error, output string, category FailureCategory) string {
	switch category {
	case FailureTimeout:
		return fmt.Sprintf(`The test suite hung and was forcefully terminated by the watchdog.

Task: %s - %s

Watchdog error: %v

Last stdout output before timeout:
%s

This usually indicates:
- An infinite loop or deadlock
- An unjoined non-daemon thread
- A blocking operation (wait/sleep) that is never unblocked
- A resource leak exhausting file descriptors

Analyze the output above and fix the issue. Rewrite any files that need changes.
Focus on making the code terminate correctly.
`, title, description, watchdogErr, output)

	case FailureCompile:
		return fmt.Sprintf(`The compilation failed with error(s).

Task: %s - %s

Compilation error: %v

Compilation output:
%s

Analyze the compilation error(s) above and fix the issues in the code or tests immediately. Rewrite any files that need changes.
`, title, description, watchdogErr, output)

	case FailureTestLogic:
		return fmt.Sprintf(`The test suite execution failed with assertion or logic errors.

Task: %s - %s

Test validation error: %v

Test runner output:
%s

Analyze the test failure(s) above and fix the implementation or tests immediately. Rewrite any files that need changes.
`, title, description, watchdogErr, output)

	default:
		return fmt.Sprintf(`The test suite validation failed.

Task: %s - %s

Validation error: %v

Output:
%s

Analyze the output and fix the implementation or tests immediately. Rewrite any files that need changes.
`, title, description, watchdogErr, output)
	}
}

func buildRetryPrompt(prevPrompt, testOutput string, testErr error, category FailureCategory, toolOutputs []string) string {
	msg := "fix the hang/deadlock"
	if category == FailureCompile {
		msg = "resolve the compilation error(s)"
	} else if category == FailureTestLogic {
		msg = "resolve the test failure(s)"
	} else if category != FailureTimeout {
		msg = "resolve the test validation failure(s)"
	}

	var toolOutputsBlock string
	if len(toolOutputs) > 0 {
		toolOutputsBlock = fmt.Sprintf("\n\nResults of tools executed in the previous attempt:\n%s", strings.Join(toolOutputs, "\n---\n"))
	}

	return fmt.Sprintf(`%s%s

The fix attempt was made but validation still failed:

Output:
%s

Error: %v

Please try a different approach to %s.
`, prevPrompt, toolOutputsBlock, testOutput, testErr, msg)
}

type WatchdogRepair struct {
	llmClient  domain.LLMClient
	maxRetries int
	sandbox    Sandbox
	tools      map[string]Tool
	evaluator  *TestValidator
}

type RepairResult struct {
	Success    bool
	Output     string
	FixedCode  bool
	Attempts   int
	FailureLog string
}

func NewWatchdogRepair(llmClient domain.LLMClient, sandbox Sandbox, tools map[string]Tool, evaluator *TestValidator) *WatchdogRepair {
	maxRetries := 10
	return &WatchdogRepair{
		llmClient:  llmClient,
		maxRetries: maxRetries,
		sandbox:    sandbox,
		tools:      tools,
		evaluator:  evaluator,
	}
}

func (wr *WatchdogRepair) AttemptRepair(
	ctx context.Context,
	state *domain.State,
	task domain.Task,
	watchdogOutput string,
	watchdogErr error,
) (*RepairResult, error) {
	category := CategorizeFailureLog(watchdogOutput)
	diagPrompt := "Repair task: " + buildDiagnosticPrompt(task.Title, task.Description, watchdogErr, watchdogOutput, category)

	var lastTestOutput string
	for attempt := 0; attempt < wr.maxRetries; attempt++ {
		resp, err := wr.llmClient.Complete(ctx, diagPrompt)
		if err != nil {
			return nil, fmt.Errorf("repair LLM call failed: %w", err)
		}

		var toolOutputs []string
		for _, action := range resp.Actions {
			if tool, ok := wr.tools[action.Tool]; ok {
				fmt.Printf("Orchestrator: Repair action: tool=%s args=%v\n", action.Tool, action.Args)
				out, err := tool.Execute(ctx, state, action.Args)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Repair tool %s failed: %v\n", action.Tool, err)
					toolOutputs = append(toolOutputs, fmt.Sprintf("Action: tool=%s args=%v failed: %v\nOutput: %s", action.Tool, action.Args, err, out))
				} else {
					toolOutputs = append(toolOutputs, fmt.Sprintf("Action: tool=%s args=%v succeeded.\nOutput: %s", action.Tool, action.Args, out))
				}
			} else {
				toolOutputs = append(toolOutputs, fmt.Sprintf("Action: unknown tool %s", action.Tool))
			}
		}

		var passed bool
		var testOutput string
		var testErr error
		if wr.evaluator != nil {
			var err error
			passed, testOutput, err = wr.evaluator.ValidateTask(ctx, state, task)
			lastTestOutput = testOutput
			if err != nil {
				testErr = err
			} else if !passed {
				testErr = fmt.Errorf("validation failed: %s", category)
			}
		} else {
			testOutput, testErr = wr.sandbox.RunCommand(ctx, state.ProjectPath, "", "")
			passed = testErr == nil
			lastTestOutput = testOutput
		}

		if passed {
			return &RepairResult{
				Success:   true,
				Output:    testOutput,
				FixedCode: true,
				Attempts:  attempt + 1,
			}, nil
		}

		diagPrompt = buildRetryPrompt(diagPrompt, testOutput, testErr, category, toolOutputs)
	}

	return &RepairResult{
		Success:    false,
		Output:     lastTestOutput,
		Attempts:   wr.maxRetries,
		FailureLog: fmt.Sprintf("all repair attempts failed to resolve the %s failure", category),
	}, nil
}
