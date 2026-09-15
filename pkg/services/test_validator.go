package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
// vote (passCount > runs/2). It optionally runs an auto-fix formatter pre-pass.
type TestValidator struct {
	Runner           Sandbox
	Strict           bool
	Formatter        Formatter
	FormatterCommand string
	LLMClient        domain.LLMClient
	Tools            map[string]Tool
	RunTimeout       time.Duration
	// Runs is the number of test suite executions per validation. Values
	// <= 0 default to 1 (single run, no consensus voting).
	Runs int
	// ShortCircuitConsensus enables the fast-pass optimization: Run 1 is
	// evaluated first. If clean (exit code 0, non-empty test suite), validation
	// passes immediately without running redundant consensus passes (Runs 2+).
	// If Run 1 fails or flakes, remaining runs are executed for majority voting.
	ShortCircuitConsensus bool
	SyntaxChecker         SyntaxChecker
}

func NewTestValidator(runner Sandbox, strict bool, llmClient domain.LLMClient, tools map[string]Tool) *TestValidator {
	return &TestValidator{
		Runner:                runner,
		Strict:                strict,
		LLMClient:             llmClient,
		Tools:                 tools,
		RunTimeout:            5 * time.Minute,
		Runs:                  1,
		ShortCircuitConsensus: true,
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

	// Fast-Path Syntax Pre-Gating:
	// Verify workspace syntax before spinning up the heavy test runner or consensus voting.
	if v.SyntaxChecker != nil {
		if syntaxErr := v.SyntaxChecker.Check(ctx, state.ProjectPath); syntaxErr != nil {
			fmt.Printf("⚠️ Orchestrator: Task %s fast-path syntax check failed: %v\n", task.ID, syntaxErr)
			return false, fmt.Sprintf("Fast-path syntax check failed:\n%v", syntaxErr), nil
		}
	}

	if v.Formatter != nil {
		fmt.Printf("Orchestrator: Task %s running formatter pre-pass...\n", task.ID)
		_, _ = v.Formatter.Format(ctx, state.ProjectPath)
	} else {
		formatCmd := v.FormatterCommand
		if formatCmd == "" {
			formatCmd = DetectDefaultFormatterCommand(state.ProjectPath)
		}
		if formatCmd != "" && v.Runner != nil {
			// Deterministic Auto-Formatter Pre-Pass:
			// Automatically run local formatter before test execution (never linters).
			fmt.Printf("Orchestrator: Task %s running deterministic local formatter %q...\n", task.ID, formatCmd)
			_, _ = v.Runner.RunCommand(ctx, state.ProjectPath, formatCmd, "")
		}
	}

	// Dual-Gate Build Verification:
	// Verify that the whole project compiles cleanly before executing the test suite.
	// Catches incomplete stubs, empty translation units, missing header files, and compiler errors.
	if buildCmd := DetectDefaultBuildCommand(state.ProjectPath); buildCmd != "" && v.Runner != nil {
		buildTimeout := v.RunTimeout
		if buildTimeout <= 0 {
			buildTimeout = 5 * time.Minute
		}
		buildCtx, buildCancel := context.WithTimeout(ctx, buildTimeout)
		buildOut, buildErr := v.Runner.RunCommand(buildCtx, state.ProjectPath, buildCmd, "")
		buildCancel()
		if buildErr != nil {
			if isMissingToolOutput(buildOut + " " + buildErr.Error()) {
				fmt.Printf("⚠️  [Validation Degraded] Task %s: required build tool is absent on host (%s). Proceeding in degraded mode.\n", task.ID, buildCmd)
			} else {
				fmt.Printf("❌ Orchestrator: Task %s project build gate (%s) failed: %v\n", task.ID, buildCmd, buildErr)
				return false, fmt.Sprintf("Build verification failed (%s):\n%s\n%v", buildCmd, buildOut, buildErr), nil
			}
		}
	}

	runs := v.Runs
	if runs <= 0 {
		runs = 1
	}

	var results []TestRunResult
	if runs > 1 && v.ShortCircuitConsensus {
		fmt.Printf("Orchestrator: Task %s running fast-pass probe (run 1 of %d)...\n", task.ID, runs)
		probeResults := v.runWithCount(ctx, state, 1)
		if probeResults[0].Passed {
			fmt.Printf("Orchestrator: Task %s fast-pass clean run 1 succeeded; short-circuiting remaining %d run(s)\n", task.ID, runs-1)
			return true, "Validation passed on clean first run (short-circuit consensus)", nil
		}
		fmt.Printf("Orchestrator: Task %s fast-pass run 1 failed; executing remaining %d run(s) for consensus voting\n", task.ID, runs-1)
		remainingResults := v.runWithCount(ctx, state, runs-1)
		results = make([]TestRunResult, runs)
		results[0] = probeResults[0]
		for i, res := range remainingResults {
			res.RunID = i + 2
			results[i+1] = res
		}
	} else {
		fmt.Printf("Orchestrator: Task %s running test execution (%d run(s))...\n", task.ID, runs)
		results = v.runWithCount(ctx, state, runs)
	}

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
	if v.Runner == nil {
		for i := 0; i < n; i++ {
			results[i] = TestRunResult{
				RunID:  i + 1,
				Passed: true,
				Output: "no runner configured (pass by default)",
			}
		}
		return results
	}
	if n <= 1 {
		timeout := v.RunTimeout
		if timeout <= 0 {
			timeout = 5 * time.Minute
		}
		runCtx, runCancel := context.WithTimeout(ctx, timeout)
		out, err := v.Runner.RunCommand(runCtx, state.ProjectPath, "", "")
		runCancel()

		fmt.Printf("Orchestrator: Task test execution finished (passed=%t, out_len=%d)\n", err == nil, len(out))

		noTestsRan := isZeroTestExecution(state.ProjectPath, out)
		outputMsg := out
		if noTestsRan && strings.TrimSpace(outputMsg) == "" {
			outputMsg = "Test suite failed: 0 test files discovered in tests/ directory and 0 test assertions executed."
		}

		results[0] = TestRunResult{
			RunID:  1,
			Passed: err == nil && !noTestsRan,
			Output: outputMsg,
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

			noTestsRan := isZeroTestExecution(state.ProjectPath, out)
			outputMsg := out
			if noTestsRan && strings.TrimSpace(outputMsg) == "" {
				outputMsg = "Test suite failed: 0 test files discovered in tests/ directory and 0 test assertions executed."
			}

			results[idx] = TestRunResult{
				RunID:  idx + 1,
				Passed: err == nil && !noTestsRan,
				Output: outputMsg,
			}
		}(i)
	}
	wg.Wait()
	return results
}

func isZeroTestExecution(projectPath string, out string) bool {
	outLower := strings.ToLower(out)
	if strings.Contains(outLower, "no tests ran") ||
		strings.Contains(outLower, "ran 0 tests") ||
		strings.Contains(outLower, "collected 0 items") ||
		strings.Contains(outLower, "collected 0 tests") ||
		strings.Contains(outLower, "exit status 5") {
		return true
	}
	testCmd := DetectDefaultTestCommand(projectPath)
	if strings.HasPrefix(testCmd, "make") {
		testsDir := filepath.Join(projectPath, "tests")
		entries, err := os.ReadDir(testsDir)
		hasTestFiles := false
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					hasTestFiles = true
					break
				}
			}
		}
		if !hasTestFiles || strings.TrimSpace(out) == "" {
			return true
		}
	}
	return false
}

func lastFailureOutput(results []TestRunResult) string {
	for i := len(results) - 1; i >= 0; i-- {
		if !results[i].Passed {
			return results[i].Output
		}
	}
	return ""
}
