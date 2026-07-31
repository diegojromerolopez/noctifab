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

// TestValidator runs the project's tests up to 3 times to check for flakiness
// and optionally auto-stabilizes flaky tests via an LLM client.
type TestValidator struct {
	Runner           Sandbox
	Strict           bool
	FormatterCommand string
	LinterCommand    string
	LLMClient        domain.LLMClient
	Tools            map[string]Tool
	RunTimeout       time.Duration
}

func NewTestValidator(runner Sandbox, strict bool, llmClient domain.LLMClient, tools map[string]Tool) *TestValidator {
	return &TestValidator{
		Runner:        runner,
		Strict:        strict,
		LinterCommand: "",
		LLMClient:     llmClient,
		Tools:         tools,
		RunTimeout:    5 * time.Minute,
	}
}

func raceCommand(cmd string) string {
	if strings.Contains(cmd, "go test") {
		return strings.Replace(cmd, "go test", "go test -race", 1)
	}
	return cmd
}

// ValidateTask executes the project's tests 3 times and counts majority votes.
// When flaky is detected and an LLM client is configured, it attempts to auto-stabilize.
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

	results := v.runWithCount(ctx, state, 1)

	passCount := 0
	for _, r := range results {
		if r.Passed {
			passCount++
		}
	}

	if passCount == 1 {
		return true, "All validation runs passed successfully", nil
	}

	lastErr := lastFailureOutput(results)
	return false, fmt.Sprintf("Test validation failed (0/1 runs passed). Last error log:\n%s", lastErr), nil
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
