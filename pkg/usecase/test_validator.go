package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// TestValidator runs the project's tests up to 3 times to check for flakiness
type TestValidator struct {
	Runner Sandbox
	Strict bool
}

func NewTestValidator(runner Sandbox, strict bool) *TestValidator {
	return &TestValidator{
		Runner: runner,
		Strict: strict,
	}
}

// ValidateTask executes the project's tests 3 times and counts majority votes
func (v *TestValidator) ValidateTask(ctx context.Context, state *domain.State, task domain.Task) (bool, string, error) {
	successes := 0
	var lastErrorLog string
	var logs []string

	for run := 1; run <= 3; run++ {
		// Run tests for the workspace by passing empty package and command arguments.
		// This defaults to executing the project's configured test suite.
		out, err := v.Runner.RunCommand(ctx, state.ProjectPath, "", "")
		if err == nil || strings.Contains(out, "NO TESTS RAN") {
			successes++
			logs = append(logs, fmt.Sprintf("Run %d: PASS", run))
		} else {
			logs = append(logs, fmt.Sprintf("Run %d: FAIL", run))
			lastErrorLog = out
		}
	}

	if successes == 3 {
		return true, "All validation runs passed successfully", nil
	}

	if successes == 2 {
		if v.Strict {
			return false, fmt.Sprintf("Strict Mode Rejection: Task is flaky (2/3 runs passed). Logs: %s", strings.Join(logs, ", ")), nil
		}
		return true, "Warning: Potentially Flaky Build", nil
	}

	// Sanitized output format
	return false, fmt.Sprintf("Test validation failed (%d/3 runs passed). Last error log:\n%s", successes, lastErrorLog), nil
}
