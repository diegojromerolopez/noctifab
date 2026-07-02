package services

import (
	"strings"
	"testing"
)

func TestDetectFlaky_ThreePass(t *testing.T) {
	results := []TestRunResult{
		{RunID: 1, Passed: true, Output: "ok"},
		{RunID: 2, Passed: true, Output: "ok"},
		{RunID: 3, Passed: true, Output: "ok"},
	}
	r := DetectFlaky(results)
	if r.Flaky {
		t.Error("expected not flaky for 3/3 passes")
	}
	if r.PassedCount != 3 {
		t.Errorf("expected 3 passed, got %d", r.PassedCount)
	}
	if r.FailedCount != 0 {
		t.Errorf("expected 0 failed, got %d", r.FailedCount)
	}
}

func TestDetectFlaky_TwoPassOneFail(t *testing.T) {
	results := []TestRunResult{
		{RunID: 1, Passed: true, Output: "ok"},
		{RunID: 2, Passed: false, Output: "FAIL"},
		{RunID: 3, Passed: true, Output: "ok"},
	}
	r := DetectFlaky(results)
	if !r.Flaky {
		t.Error("expected flaky for 2/3 passes")
	}
	if r.PassedCount != 2 {
		t.Errorf("expected 2 passed, got %d", r.PassedCount)
	}
	if r.FailedCount != 1 {
		t.Errorf("expected 1 failed, got %d", r.FailedCount)
	}
}

func TestDetectFlaky_OnePassTwoFail(t *testing.T) {
	results := []TestRunResult{
		{RunID: 1, Passed: true, Output: "ok"},
		{RunID: 2, Passed: false, Output: "FAIL"},
		{RunID: 3, Passed: false, Output: "FAIL"},
	}
	r := DetectFlaky(results)
	if r.Flaky {
		t.Error("expected not flaky for 1/3 passes (consistently failing)")
	}
}

func TestDetectFlaky_ZeroPass(t *testing.T) {
	results := []TestRunResult{
		{RunID: 1, Passed: false, Output: "FAIL"},
		{RunID: 2, Passed: false, Output: "FAIL"},
		{RunID: 3, Passed: false, Output: "FAIL"},
	}
	r := DetectFlaky(results)
	if r.Flaky {
		t.Error("expected not flaky for 0/3 passes")
	}
}

func TestDetectFlaky_EmptyResults(t *testing.T) {
	r := DetectFlaky(nil)
	if r.Flaky {
		t.Error("expected not flaky for empty results")
	}
	if r.PassedCount != 0 {
		t.Errorf("expected 0 passed, got %d", r.PassedCount)
	}
}

func TestDetectFlaky_LessThanThree(t *testing.T) {
	results := []TestRunResult{
		{RunID: 1, Passed: true, Output: "ok"},
		{RunID: 2, Passed: false, Output: "FAIL"},
	}
	r := DetectFlaky(results)
	if r.Flaky {
		t.Error("expected not flaky with fewer than 3 results")
	}
}

func TestBuildFlakyPrompt_IncludesOutputs(t *testing.T) {
	results := []TestRunResult{
		{RunID: 1, Passed: true, Output: "output1"},
		{RunID: 2, Passed: false, Output: "output2"},
		{RunID: 3, Passed: true, Output: "output3"},
	}
	prompt := BuildFlakyStabilizationPrompt(results, "race-output")
	for _, s := range []string{"output1", "output2", "output3", "Run 1", "Run 2", "Run 3"} {
		if !strings.Contains(prompt, s) {
			t.Errorf("expected prompt to contain %q", s)
		}
	}
}

func TestBuildFlakyPrompt_IncludesRaceOutput(t *testing.T) {
	results := []TestRunResult{{RunID: 1, Passed: true, Output: "ok"}}
	prompt := BuildFlakyStabilizationPrompt(results, "WARNING: DATA RACE")
	if !strings.Contains(prompt, "WARNING: DATA RACE") {
		t.Error("expected prompt to include race detection output")
	}
	if !strings.Contains(prompt, "deterministic") {
		t.Error("expected prompt to include stabilization instructions")
	}
}

func TestFormatResults(t *testing.T) {
	results := []TestRunResult{
		{RunID: 1, Passed: true, Output: "ok"},
		{RunID: 2, Passed: false, Output: "fail"},
	}
	out := formatResults(results)
	if !strings.Contains(out, "Run 1: PASS") {
		t.Error("expected Run 1: PASS")
	}
	if !strings.Contains(out, "Run 2: FAIL") {
		t.Error("expected Run 2: FAIL")
	}
}
