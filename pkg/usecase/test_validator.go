package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// TestValidator runs the project's tests up to 3 times to check for flakiness
// and optionally auto-stabilizes flaky tests via an LLM client.
type TestValidator struct {
	Runner        Sandbox
	Strict        bool
	LinterCommand string
	LLMClient     domain.LLMClient
	Tools         map[string]Tool
}

func NewTestValidator(runner Sandbox, strict bool, llmClient domain.LLMClient, tools map[string]Tool) *TestValidator {
	return &TestValidator{
		Runner:        runner,
		Strict:        strict,
		LinterCommand: "",
		LLMClient:     llmClient,
		Tools:         tools,
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
	if v.LinterCommand != "" {
		out, err := v.Runner.RunCommand(ctx, state.ProjectPath, v.LinterCommand, "")
		if err != nil {
			return false, fmt.Sprintf("Linter validation failed. Command: %s. Output:\n%s", v.LinterCommand, out), nil
		}
	}

	results := v.runWithCount(ctx, state, 3)
	flaky := DetectFlaky(results)

	passCount, failCount := 0, 0
	for _, r := range results {
		if r.Passed {
			passCount++
		} else {
			failCount++
		}
	}

	if passCount == 3 {
		return true, "All validation runs passed successfully", nil
	}

	if flaky.Flaky && v.LLMClient != nil {
		stabilized := v.attemptStabilization(ctx, state, results)
		if stabilized {
			return true, "Flaky test stabilized after LLM-assisted fix", nil
		}
		// Stabilization failed; fall through to original behavior
	}

	if passCount >= 2 {
		if v.Strict {
			lastErr := lastFailureOutput(results)
			return false, fmt.Sprintf("Strict Mode Rejection: Task is flaky (2/3 runs passed). Last error:\n%s", lastErr), nil
		}
		return true, "Warning: Potentially Flaky Build", nil
	}

	lastErr := lastFailureOutput(results)
	return false, fmt.Sprintf("Test validation failed (%d/3 runs passed). Last error log:\n%s", passCount, lastErr), nil
}

func (v *TestValidator) runWithCount(ctx context.Context, state *domain.State, n int) []TestRunResult {
	results := make([]TestRunResult, n)
	for i := 0; i < n; i++ {
		runCtx, runCancel := context.WithTimeout(ctx, 60*time.Second)
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

func (v *TestValidator) attemptStabilization(ctx context.Context, state *domain.State, results []TestRunResult) bool {
	defaultCmd := ""
	raceOutput, raceErr := v.Runner.RunCommand(ctx, state.ProjectPath, raceCommand(defaultCmd), "")
	if raceErr != nil {
		raceOutput = fmt.Sprintf("Race detection command failed: %v\nOutput: %s", raceErr, raceOutput)
	}

	prompt := BuildFlakyStabilizationPrompt(results, raceOutput)
	stabilizeCtx, stabilizeCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer stabilizeCancel()

	resp, llmErr := v.LLMClient.Complete(stabilizeCtx, prompt)
	if llmErr != nil || resp == nil {
		return false
	}

	for _, action := range resp.Actions {
		if action.Tool == "write_file" || action.Tool == "edit_file" {
			if tool, ok := v.Tools[action.Tool]; ok {
				_, _ = tool.Execute(stabilizeCtx, state, action.Args)
			}
		}
	}

	restabilized := v.runWithCount(stabilizeCtx, state, 3)
	return !DetectFlaky(restabilized).Flaky
}

func lastFailureOutput(results []TestRunResult) string {
	for i := len(results) - 1; i >= 0; i-- {
		if !results[i].Passed {
			return results[i].Output
		}
	}
	return ""
}
