package usecase

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

func buildDiagnosticPrompt(title, description string, watchdogErr error, output string) string {
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
}

func buildRetryPrompt(prevPrompt, testOutput string, testErr error) string {
	return fmt.Sprintf(`%s

The fix attempt was made but tests still failed or hung:

Test output:
%s

Test error: %v

Please try a different approach to fix the hang/deadlock.
`, prevPrompt, testOutput, testErr)
}

type WatchdogRepair struct {
	llmClient  domain.LLMClient
	maxRetries int
	sandbox    Sandbox
	tools      map[string]Tool
}

type RepairResult struct {
	Success    bool
	Output     string
	FixedCode  bool
	Attempts   int
	FailureLog string
}

func NewWatchdogRepair(llmClient domain.LLMClient, sandbox Sandbox, tools map[string]Tool) *WatchdogRepair {
	maxRetries := 3
	return &WatchdogRepair{
		llmClient:  llmClient,
		maxRetries: maxRetries,
		sandbox:    sandbox,
		tools:      tools,
	}
}

func (wr *WatchdogRepair) AttemptRepair(
	ctx context.Context,
	state *domain.State,
	task domain.Task,
	watchdogOutput string,
	watchdogErr error,
) (*RepairResult, error) {
	diagPrompt := buildDiagnosticPrompt(task.Title, task.Description, watchdogErr, watchdogOutput)

	for attempt := 0; attempt < wr.maxRetries; attempt++ {
		resp, err := wr.llmClient.Complete(ctx, diagPrompt)
		if err != nil {
			return nil, fmt.Errorf("repair LLM call failed: %w", err)
		}

		for _, action := range resp.Actions {
			if tool, ok := wr.tools[action.Tool]; ok {
				if _, err := tool.Execute(ctx, state, action.Args); err != nil {
					fmt.Fprintf(os.Stderr, "Repair tool %s failed: %v\n", action.Tool, err)
				}
			}
		}

		testOutput, testErr := wr.sandbox.RunCommand(ctx, state.ProjectPath, "", "")
		if testErr == nil {
			return &RepairResult{
				Success:   true,
				Output:    testOutput,
				FixedCode: true,
				Attempts:  attempt + 1,
			}, nil
		}

		diagPrompt = buildRetryPrompt(diagPrompt, testOutput, testErr)
	}

	return &RepairResult{
		Success:    false,
		Attempts:   wr.maxRetries,
		FailureLog: "all repair attempts failed to resolve the hang/deadlock",
	}, nil
}
