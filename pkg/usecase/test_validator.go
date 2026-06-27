package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// TestValidator runs the project's tests up to 3 times to check for flakiness
type TestValidator struct {
	Runner        Sandbox
	Strict        bool
	LinterCommand string
}

func NewTestValidator(runner Sandbox, strict bool) *TestValidator {
	return &TestValidator{
		Runner:        runner,
		Strict:        strict,
		LinterCommand: "",
	}
}

// ValidateTask executes the project's tests 3 times and counts majority votes
func (v *TestValidator) ValidateTask(ctx context.Context, state *domain.State, task domain.Task) (bool, string, error) {
	// 1. Run linter first if configured
	if v.LinterCommand != "" {
		out, err := v.Runner.RunCommand(ctx, state.ProjectPath, v.LinterCommand, "")
		if err != nil {
			return false, fmt.Sprintf("Linter validation failed. Command: %s. Output:\n%s", v.LinterCommand, out), nil
		}
	}

	successes := 0
	var lastErrorLog string
	var logs []string

	for run := 1; run <= 3; run++ {
		// Run tests for the workspace by passing empty package and command arguments.
		// This defaults to executing the project's configured test suite.
		out, err := v.Runner.RunCommand(ctx, state.ProjectPath, "", "")

		hasFailed := err != nil
		outLower := strings.ToLower(out)
		noTestsRan := strings.Contains(outLower, "no tests ran") ||
			strings.Contains(outLower, "ran 0 tests") ||
			strings.Contains(outLower, "collected 0 items") ||
			strings.Contains(outLower, "collected 0 tests") ||
			strings.Contains(outLower, "exit status 5") // pytest exits with 5 on no tests collected

		if hasFailed || noTestsRan {
			logs = append(logs, fmt.Sprintf("Run %d: FAIL", run))
			if noTestsRan {
				lastErrorLog = "Error: Test runner executed 0 tests. Output:\n" + out
			} else {
				lastErrorLog = out
			}
		} else {
			successes++
			logs = append(logs, fmt.Sprintf("Run %d: PASS", run))
		}
	}

	if successes == 3 {
		return true, "All validation runs passed successfully", nil
	}

	if successes == 2 {
		if v.Strict {
			return false, fmt.Sprintf("Strict Mode Rejection: Task is flaky (2/3 runs passed). Logs: %s. Last error:\n%s", strings.Join(logs, ", "), lastErrorLog), nil
		}
		return true, "Warning: Potentially Flaky Build", nil
	}

	// Sanitized output format
	return false, fmt.Sprintf("Test validation failed (%d/3 runs passed). Last error log:\n%s", successes, lastErrorLog), nil
}
