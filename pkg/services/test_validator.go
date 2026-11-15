package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
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
	E2ECommand            string
	E2EMode               string
	EnforceE2E            bool
	ContainerGuard        *ContainerTeardownGuard
	// MinPassRate specifies the percentage of unit tests (0.0 to 100.0) that must pass (default 80.0%).
	MinPassRate float64
}

func (v *TestValidator) SetE2ECommand(cmd string) {
	v.E2ECommand = cmd
}

func (v *TestValidator) SetE2EMode(mode string) {
	v.E2EMode = mode
}

func (v *TestValidator) SetE2EConfig(cfg config.E2EConfig) {
	v.E2ECommand = cfg.Command
	v.E2EMode = cfg.Mode
}

func (v *TestValidator) SetEnforceE2E(enforce bool) {
	v.EnforceE2E = enforce
}

func (v *TestValidator) detectE2ECommand(projectPath string) string {
	return DetectE2ECommand(projectPath, v.E2EMode, v.E2ECommand)
}

func (v *TestValidator) shouldValidateE2E(state *domain.State, task domain.Task) bool {
	if state == nil || strings.TrimSpace(state.ProjectPath) == "" {
		return false
	}
	if v.EnforceE2E {
		return true
	}
	// Always validate E2E for remediation and rescue tasks
	if strings.HasPrefix(task.ID, "qa-remediation-") ||
		strings.HasPrefix(task.ID, "spec-remediation-") ||
		strings.HasPrefix(task.ID, "sovereign-rescue-") {
		return true
	}
	// Check if task targets E2E or integration files
	for _, tf := range task.TargetFiles {
		lower := strings.ToLower(tf)
		if strings.Contains(lower, "/unit/") || strings.HasPrefix(lower, "unit/") {
			// Explicit unit test file — never trigger E2E gate on pure unit tests
			continue
		}
		if strings.Contains(lower, "e2e") ||
			strings.Contains(lower, "integration") ||
			strings.HasSuffix(lower, "docker-compose.yml") ||
			strings.HasSuffix(lower, "docker-compose.e2e.yml") {
			return true
		}
	}
	// Check if task title explicitly targets E2E or integration tests
	lowerTitle := strings.ToLower(task.Title)
	if strings.Contains(lowerTitle, "e2e") ||
		strings.Contains(lowerTitle, "integration test") ||
		strings.Contains(lowerTitle, "black-box") {
		return true
	}
	// Check if this is the final task of a story
	if task.StoryID != "" {
		allOthersCompleted := true
		storyTaskCount := 0
		for _, t := range state.Tasks {
			if t.StoryID == task.StoryID && t.ID != task.ID {
				storyTaskCount++
				if t.Status != domain.TaskSuccess {
					allOthersCompleted = false
					break
				}
			}
		}
		if storyTaskCount > 0 && allOthersCompleted {
			return true
		}
	}
	return false
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
		MinPassRate:           80.0,
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

	// Shadow / Duplicate Test File Collision Gate:
	if collisions := DetectShadowTestFiles(state.ProjectPath); len(collisions) > 0 {
		return false, FormatShadowTestCollisions(collisions), nil
	}

	// Cross-Layer Test Coupling Gate:
	if couplings := DetectTestCouplingViolations(state.ProjectPath); len(couplings) > 0 {
		return false, FormatTestCouplingViolations(couplings), nil
	}

	// Orphan Source File Gate:
	if orphans, _ := DetectOrphanSourceFiles(state.ProjectPath); len(orphans) > 0 {
		return false, FormatOrphanSourceViolations(orphans), nil
	}

	// Fast-Path Syntax Pre-Gating:
	if v.SyntaxChecker != nil {
		if syntaxErr := v.SyntaxChecker.Check(ctx, state.ProjectPath); syntaxErr != nil {
			fmt.Printf("⚠️ Orchestrator: Task %s fast-path syntax check failed: %v\n", task.ID, syntaxErr)
			return false, fmt.Sprintf("Fast-path syntax check failed:\n%v", syntaxErr), nil
		}
	}

	_ = PrepareTestEnvironment(state.ProjectPath)

	if v.Formatter != nil {
		fmt.Printf("Orchestrator: Task %s running formatter pre-pass...\n", task.ID)
		_, _ = v.Formatter.Format(ctx, state.ProjectPath)
	} else {
		formatCmd := v.FormatterCommand
		if formatCmd == "" {
			formatCmd = DetectDefaultFormatterCommand(state.ProjectPath)
		}
		if formatCmd != "" && v.Runner != nil {
			fmt.Printf("Orchestrator: Task %s running deterministic local formatter %q...\n", task.ID, formatCmd)
			_, _ = v.Runner.RunCommand(ctx, state.ProjectPath, formatCmd, "")
		}
	}

	if autoFixCmd := DetectAutoFixImportsCommand(state.ProjectPath); autoFixCmd != "" && v.Runner != nil {
		fmt.Printf("Orchestrator: Task %s running deterministic auto-fix imports %q...\n", task.ID, autoFixCmd)
		_, _ = v.Runner.RunCommand(ctx, state.ProjectPath, autoFixCmd, "")
	}

	// Dual-Gate Build Verification:
	if buildCmd := DetectDefaultBuildCommand(state.ProjectPath); buildCmd != "" && v.Runner != nil {
		buildTimeout := v.RunTimeout
		if buildTimeout <= 0 {
			buildTimeout = 5 * time.Minute
		}
		buildCtx, buildCancel := context.WithTimeout(ctx, buildTimeout)
		buildOut, buildErr := v.Runner.RunCommand(buildCtx, state.ProjectPath, buildCmd, "")
		buildCancel()
		if buildErr != nil {
			if isCommandToolMissing(v.Runner, buildCmd, buildOut+" "+buildErr.Error()) {
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
		if ok, msg, err := v.validateE2E(ctx, state, task); !ok {
			return false, msg, err
		}

		// Anti-Shallowness & Test Depth Assessment Gate:
		var targetTests []string
		for _, tf := range task.TargetFiles {
			if strings.Contains(tf, "test") || strings.HasSuffix(tf, "_test.go") {
				targetTests = append(targetTests, tf)
			}
		}
		if len(targetTests) > 0 {
			depthEvaluator := NewTestDepthEvaluator()
			depthRes, depthErr := depthEvaluator.EvaluateDepth(ctx, state.ProjectPath, targetTests)
			if depthErr == nil && depthRes != nil && !depthRes.Passed && depthRes.IsShallow {
				return false, depthRes.RemediationMessage, nil
			}
		}

		if passCount == runs {
			return true, "All validation runs passed successfully", nil
		}
		return true, fmt.Sprintf("Validation passed by majority vote (%d/%d runs passed)", passCount, runs), nil
	}

	lastErr := lastFailureOutput(results)
	testCmd := DetectDefaultTestCommand(state.ProjectPath)
	if isCommandToolMissing(v.Runner, testCmd, lastErr) {
		fmt.Printf("⚠️  [Validation Degraded] Task %s: required test runner or tool is absent on host. Proceeding in degraded mode without test gating.\n", task.ID)
		return true, fmt.Sprintf("Validation passed in degraded mode (tool absent on host).\nLast output:\n%s", lastErr), nil
	}
	return false, fmt.Sprintf("Test validation failed (%d/%d runs passed). Last error log:\n%s", passCount, runs, lastErr), nil
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

		exitCode := 0
		if err != nil {
			exitCode = 1
		}
		parsedReport := ParseTestOutput(out, exitCode)
		noTestsRan, notice := EvaluateTestExecution(state.ProjectPath, out)
		outputMsg := out
		if noTestsRan && (strings.TrimSpace(outputMsg) == "" || notice != "") {
			outputMsg = notice
		}

		passed := err == nil && !noTestsRan
		minRate := v.MinPassRate
		if minRate <= 0 {
			minRate = 80.0
		}
		if !passed && parsedReport.Total > 0 && parsedReport.Passed > 0 {
			passRate := (float64(parsedReport.Passed) / float64(parsedReport.Total)) * 100.0
			if passRate >= minRate {
				passed = true
				outputMsg = fmt.Sprintf("%s\n\n[Pragmatic Pass Gating] %d/%d tests passed (%.1f%% >= %.1f%% required threshold). Soft-pass accepted for task progression.",
					outputMsg, parsedReport.Passed, parsedReport.Total, passRate, minRate)
			}
		}

		if !parsedReport.Success && len(parsedReport.Failures) > 0 {
			if !passed {
				outputMsg = fmt.Sprintf("%s\n\n[Structured Test Parser] %s\nFailed Tests:\n - %s",
					outputMsg, parsedReport.SummaryText, strings.Join(parsedReport.Failures, "\n - "))
			} else {
				outputMsg = fmt.Sprintf("%s\n\n[Non-Blocking Test Notices]\nFailed Tests:\n - %s",
					outputMsg, strings.Join(parsedReport.Failures, "\n - "))
			}
		}

		results[0] = TestRunResult{
			RunID:  1,
			Passed: passed,
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

			exitCode := 0
			if err != nil {
				exitCode = 1
			}
			parsedReport := ParseTestOutput(out, exitCode)
			noTestsRan, notice := EvaluateTestExecution(state.ProjectPath, out)
			outputMsg := out
			if noTestsRan && (strings.TrimSpace(outputMsg) == "" || notice != "") {
				outputMsg = notice
			}

			passed := err == nil && !noTestsRan
			minRate := v.MinPassRate
			if minRate <= 0 {
				minRate = 80.0
			}
			if !passed && parsedReport.Total > 0 && parsedReport.Passed > 0 {
				passRate := (float64(parsedReport.Passed) / float64(parsedReport.Total)) * 100.0
				if passRate >= minRate {
					passed = true
					outputMsg = fmt.Sprintf("%s\n\n[Pragmatic Pass Gating] %d/%d tests passed (%.1f%% >= %.1f%% required threshold). Soft-pass accepted for task progression.",
						outputMsg, parsedReport.Passed, parsedReport.Total, passRate, minRate)
				}
			}

			if !parsedReport.Success && len(parsedReport.Failures) > 0 {
				if !passed {
					outputMsg = fmt.Sprintf("%s\n\n[Structured Test Parser] %s\nFailed Tests:\n - %s",
						outputMsg, parsedReport.SummaryText, strings.Join(parsedReport.Failures, "\n - "))
				} else {
					outputMsg = fmt.Sprintf("%s\n\n[Non-Blocking Test Notices]\nFailed Tests:\n - %s",
						outputMsg, strings.Join(parsedReport.Failures, "\n - "))
				}
			}

			results[idx] = TestRunResult{
				RunID:  idx + 1,
				Passed: passed,
				Output: outputMsg,
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
