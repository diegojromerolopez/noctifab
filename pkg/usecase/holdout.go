package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// HoldoutEvaluator runs acceptance tests up to 3 times to detect flakiness
type HoldoutEvaluator struct {
	Runner Sandbox
	Strict bool
}

func NewHoldoutEvaluator(runner Sandbox, strict bool) *HoldoutEvaluator {
	return &HoldoutEvaluator{
		Runner: runner,
		Strict: strict,
	}
}

// EvaluateTask executes the tests 3 times and counts majority votes
func (h *HoldoutEvaluator) EvaluateTask(ctx context.Context, state *domain.State, task domain.Task, testPkg string) (bool, string, error) {
	successes := 0
	var lastErrorLog string
	var logs []string

	for run := 1; run <= 3; run++ {
		// Run tests for the package
		out, err := h.Runner.RunCommand(ctx, state.ProjectPath, "", testPkg)
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
		if h.Strict {
			return false, fmt.Sprintf("Strict Mode Rejection: Task is flaky (2/3 runs passed). Logs: %s", strings.Join(logs, ", ")), nil
		}
		return true, "Warning: Potentially Flaky Build", nil
	}

	// Sanitized output format
	return false, fmt.Sprintf("Holdout Evaluation Failed (%d/3 runs passed). Last error log:\n%s", successes, lastErrorLog), nil
}
