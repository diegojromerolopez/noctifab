package services

import (
	"errors"
	"strings"
	"testing"
)

func TestCategorizeFailureLog_Timeout(t *testing.T) {
	tests := []struct {
		log  string
		cat  FailureCategory
		name string
	}{
		{"command killed: no output produced within idle timeout", FailureTimeout, "idle timeout"},
		{"max wall-clock duration exceeded", FailureTimeout, "max duration"},
		{"command killed by watchdog", FailureTimeout, "watchdog kill"},
		{"sandbox violation: path outside workspace", FailureSandbox, "sandbox"},
		{"compile error: syntax error", FailureCompile, "compile"},
		{"ERROR: test failure", FailureTestLogic, "test error"},
		{"FAIL: TestFoo", FailureTestLogic, "test fail"},
		{"Traceback (most recent call last)", FailureTestLogic, "traceback"},
		{"random output line", FailureUnknown, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CategorizeFailureLog(tt.log); got != tt.cat {
				t.Errorf("CategorizeFailureLog(%q) = %v, want %v", tt.log, got, tt.cat)
			}
		})
	}
}

func TestBuildDiagnosticPrompt_IncludesFields(t *testing.T) {
	err := errors.New("command killed: idle timeout")
	output := "test output here"
	prompt := buildDiagnosticPrompt("My Task", "Do the thing", err, output, FailureTimeout)

	for _, s := range []string{"My Task", "Do the thing", "idle timeout", "test output here"} {
		if !strings.Contains(prompt, s) {
			t.Errorf("expected prompt to contain %q", s)
		}
	}
}

func TestBuildRetryPrompt_AppendsContext(t *testing.T) {
	prev := "original diagnostic"
	testOut := "test run 1 output"
	testErr := errors.New("exit status 1")

	prompt := buildRetryPrompt(prev, testOut, testErr, FailureTimeout)
	for _, s := range []string{prev, testOut, "exit status 1", "The fix attempt was made"} {
		if !strings.Contains(prompt, s) {
			t.Errorf("expected retry prompt to contain %q", s)
		}
	}
}

func TestFailureCategory_String(t *testing.T) {
	tests := []struct {
		cat  FailureCategory
		want string
	}{
		{FailureUnknown, "unknown"},
		{FailureTestLogic, "test_logic"},
		{FailureTimeout, "timeout"},
		{FailureCompile, "compile"},
		{FailureSandbox, "sandbox"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.cat.String(); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildDiagnosticPrompt_EmptyFields(t *testing.T) {
	prompt := buildDiagnosticPrompt("", "", nil, "", FailureTimeout)
	if prompt == "" {
		t.Error("expected non-empty prompt even with empty fields")
	}
}
