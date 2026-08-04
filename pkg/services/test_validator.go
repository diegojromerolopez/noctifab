package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TestValidator validates a task by running the project's tests. By default
// it performs a single validation run; when Runs is configured to a value
// greater than 1 it runs the suite N times and passes on a strict majority
// vote (passCount > runs/2). It optionally runs formatter/linter pre-passes.
type TestValidator struct {
	Runner           Sandbox
	Strict           bool
	FormatterCommand string
	LinterCommand    string
	LLMClient        domain.LLMClient
	Tools            map[string]Tool
	RunTimeout       time.Duration
	// Runs is the number of test suite executions per validation. Values
	// <= 0 default to 1 (single run, no consensus voting).
	Runs int
}

func NewTestValidator(runner Sandbox, strict bool, llmClient domain.LLMClient, tools map[string]Tool) *TestValidator {
	return &TestValidator{
		Runner:        runner,
		Strict:        strict,
		LinterCommand: "",
		LLMClient:     llmClient,
		Tools:         tools,
		RunTimeout:    5 * time.Minute,
		Runs:          1,
	}
}

func raceCommand(cmd string) string {
	if strings.Contains(cmd, "go test") {
		return strings.Replace(cmd, "go test", "go test -race", 1)
	}
	return cmd
}

// ValidateTask executes the project's tests Runs times (default 1) and
// passes when a strict majority of runs pass (passCount > runs/2). With the
// default single run this is simply pass/fail of that one run.
func (v *TestValidator) ValidateTask(ctx context.Context, state *domain.State, task domain.Task) (bool, string, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "ValidateTask",
		trace.WithAttributes(
			attribute.String("task.id", task.ID),
			attribute.String("task.title", task.Title),
			attribute.Bool("strict", v.Strict),
		))
	defer span.End()

	if v.FormatterCommand != "" {
		// Deterministic Auto-Formatter Pre-Pass:
		// Automatically run auto-fix formatter before linter evaluation.
		_, _ = v.Runner.RunCommand(ctx, state.ProjectPath, v.FormatterCommand, "")
	}

	if v.LinterCommand != "" {
		out, err := v.Runner.RunCommand(ctx, state.ProjectPath, v.LinterCommand, "")
		if err != nil && v.FormatterCommand != "" {
			// Try auto-formatting once more to resolve formatting/style linter offenses automatically
			_, _ = v.Runner.RunCommand(ctx, state.ProjectPath, v.FormatterCommand, "")
			outRetry, errRetry := v.Runner.RunCommand(ctx, state.ProjectPath, v.LinterCommand, "")
			if errRetry == nil {
				out = outRetry
				err = nil
			}
		}
		if err != nil {
			return false, fmt.Sprintf("Linter validation failed. Command: %s. Output:\n%s", v.LinterCommand, out), nil
		}
	}

	runs := v.Runs
	if runs <= 0 {
		runs = 1
	}
	results := v.runWithCount(ctx, state, runs)

	passCount := 0
	for _, r := range results {
		if r.Passed {
			passCount++
		}
	}

	// Strict majority vote; with the default single run this reduces to
	// requiring that one run to pass.
	if passCount > runs/2 {
		if passCount == runs {
			return true, "All validation runs passed successfully", nil
		}
		return true, fmt.Sprintf("Validation passed by majority vote (%d/%d runs passed)", passCount, runs), nil
	}

	lastErr := lastFailureOutput(results)
	return false, fmt.Sprintf("Test validation failed (%d/%d runs passed). Last error log:\n%s", passCount, runs, lastErr), nil
}

func (v *TestValidator) runWithCount(ctx context.Context, state *domain.State, n int) []TestRunResult {
	results := make([]TestRunResult, n)
	for i := 0; i < n; i++ {
		timeout := v.RunTimeout
		if timeout <= 0 {
			timeout = 5 * time.Minute
		}
		runCtx, runCancel := context.WithTimeout(ctx, timeout)
		out, err := v.Runner.RunCommand(runCtx, state.ProjectPath, "", "")
		runCancel()

		outLower := strings.ToLower(out)
		noTestsRan := strings.Contains(outLower, "no tests ran") ||
			strings.Contains(outLower, "ran 0 tests") ||
			strings.Contains(outLower, "collected 0 items") ||
			strings.Contains(outLower, "collected 0 tests") ||
			strings.Contains(outLower, "exit status 5")

		results[i] = TestRunResult{
			RunID:  i + 1,
			Passed: err == nil && !noTestsRan,
			Output: out,
		}
	}
	return results
}

func lastFailureOutput(results []TestRunResult) string {
	for i := len(results) - 1; i >= 0; i-- {
		if !results[i].Passed {
			return results[i].Output
		}
	}
	return ""
}
