package services

import (
	"fmt"
	"strings"
)

type TestRunResult struct {
	RunID  int
	Passed bool
	Output string
}

type FlakyResult struct {
	Flaky       bool
	FailedCount int
	PassedCount int
	Outputs     []string
}

func DetectFlaky(results []TestRunResult) *FlakyResult {
	passed, failed := 0, 0
	outputs := make([]string, len(results))
	for i, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
		if i < len(outputs) {
			outputs[i] = r.Output
		}
	}
	return &FlakyResult{
		Flaky:       len(results) >= 3 && passed >= 2 && failed >= 1,
		FailedCount: failed,
		PassedCount: passed,
		Outputs:     outputs,
	}
}

func BuildFlakyStabilizationPrompt(results []TestRunResult, raceOutput string) string {
	return fmt.Sprintf(`The test suite has a flaky test — it passes inconsistently across 3 runs.

Outputs:
%s

Race detection output:
%s

Analyze the test and implementation for:
- time.Sleep instead of deterministic polling or signals
- Shared state between tests (global variables, file system, env vars)
- Missing mutexes or race conditions
- Network dependency without retry or timeout
- Order-dependent test execution

Rewrite to make the test deterministic.
`, formatResults(results), raceOutput)
}

func formatResults(results []TestRunResult) string {
	var sb strings.Builder
	for i, r := range results {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
		}
		_, _ = fmt.Fprintf(&sb, "Run %d: %s\n%s\n", i+1, status, r.Output)
	}
	return sb.String()
}
