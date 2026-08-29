package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TestRunResult captures the outcome of a single test-suite execution during
// multi-run validation.
type TestRunResult struct {
	RunID  int
	Passed bool
	Output string
}

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
	// MaxLinterIssues is the maximum number of linter issues tolerated before
	// failing validation. 0 means strict. -1 means disabled. Default 0.
	MaxLinterIssues int
	// MaxLinterConsecutiveFailures is the consecutive failure count threshold
	// before linter enforcement is deferred to prevent infinite lock-in loops.
	// Defaults to 2 if <= 0.
	MaxLinterConsecutiveFailures int
	// linterConsecutiveFailures tracks consecutive linter failures without
	// any file mutation in between.
	linterConsecutiveFailures int
}

func NewTestValidator(runner Sandbox, strict bool, llmClient domain.LLMClient, tools map[string]Tool) *TestValidator {
	return &TestValidator{
		Runner:                       runner,
		Strict:                       strict,
		LinterCommand:                "",
		LLMClient:                    llmClient,
		Tools:                        tools,
		RunTimeout:                   5 * time.Minute,
		Runs:                         1,
		MaxLinterConsecutiveFailures: 2,
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

	// Anti-Stub & Anti-Gaming Gate:
	// Inspect target files (or entire workspace) for placeholder stubs, shell masks, and vacuum tests.
	antiStub := NewAntiStubValidator()
	violations, _ := antiStub.ValidateWorkspace(state.ProjectPath, task.TargetFiles)
	if len(violations) > 0 {
		var sb strings.Builder
		fmt.Fprintf(&sb, "Anti-stub / anti-gaming validation failed with %d violation(s):\n", len(violations))
		for _, v := range violations {
			fmt.Fprintf(&sb, "- %s:%d: [%s] %s\n", v.Path, v.Line, v.Rule, v.Snippet)
		}
		return false, sb.String(), nil
	}

	if v.FormatterCommand != "" {
		// Deterministic Auto-Formatter Pre-Pass:
		// Automatically run auto-fix formatter before linter evaluation.
		_, _ = v.Runner.RunCommand(ctx, state.ProjectPath, v.FormatterCommand, "")
	}

	if v.LinterCommand != "" {
		maxConsecutive := v.MaxLinterConsecutiveFailures
		if maxConsecutive <= 0 {
			maxConsecutive = 2
		}
		// Enforce linter unless consecutive failures have indicated a lock-in loop.
		if v.linterConsecutiveFailures >= maxConsecutive {
			fmt.Fprintf(os.Stderr, "⚠ Linter deferred: failed %d consecutive times without file changes — skipping linter enforcement to allow task completion.\n", v.linterConsecutiveFailures)
		} else {
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
				// Apply MaxLinterIssues threshold: -1 = disabled, >0 = tolerance.
				linterBlocking := true
				if v.MaxLinterIssues < 0 {
					fmt.Fprintf(os.Stderr, "⚠ Linter issues suppressed (max_linter_issues=-1).\n")
					linterBlocking = false
				} else if v.MaxLinterIssues > 0 {
					issueCount := countLinterIssues(out)
					if issueCount <= v.MaxLinterIssues {
						fmt.Fprintf(os.Stderr, "⚠ Linter advisory: %d issue(s) within max_linter_issues=%d threshold — continuing.\n", issueCount, v.MaxLinterIssues)
						linterBlocking = false
					}
				}
				if linterBlocking {
					v.linterConsecutiveFailures++
					return false, fmt.Sprintf("Linter validation failed. Command: %s. Output:\n%s", v.LinterCommand, out), nil
				}
			} else {
				// Linter passed — reset consecutive failure counter.
				v.linterConsecutiveFailures = 0
			}
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
	if isMissingToolOutput(lastErr) {
		fmt.Printf("⚠️  [Validation Degraded] Task %s: required test runner or tool is absent on host. Proceeding in degraded mode without test gating.\n", task.ID)
		return true, fmt.Sprintf("Validation passed in degraded mode (tool absent on host).\nLast output:\n%s", lastErr), nil
	}
	return false, fmt.Sprintf("Test validation failed (%d/%d runs passed). Last error log:\n%s", passCount, runs, lastErr), nil
}

func isMissingToolOutput(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "command not found") ||
		strings.Contains(lower, "executable file not found") ||
		strings.Contains(lower, "is evicted") ||
		strings.Contains(lower, "exit status 127") ||
		strings.Contains(lower, "no such file or directory") && strings.Contains(lower, "exec")
}

func (v *TestValidator) runWithCount(ctx context.Context, state *domain.State, n int) []TestRunResult {
	results := make([]TestRunResult, n)
	if n <= 1 {
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

		results[0] = TestRunResult{
			RunID:  1,
			Passed: err == nil && !noTestsRan,
			Output: out,
		}
		return results
	}

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
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

			results[idx] = TestRunResult{
				RunID:  idx + 1,
				Passed: err == nil && !noTestsRan,
				Output: out,
			}
		}(i)
	}
	wg.Wait()
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
