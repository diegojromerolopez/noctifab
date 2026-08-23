package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
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
		{"sh: 1: pytest: not found", FailureSandbox, "pytest missing"},
		{"bash: make: command not found", FailureSandbox, "command not found"},
		{"exec: \"gcc\": executable file not found in $PATH", FailureSandbox, "exec lookpath missing"},
		{"process exited with exit status 127", FailureSandbox, "exit status 127"},
		{"redis-cli failed with: './tests/e2e/run_tests.sh: line 71: redis-cli: command not found'", FailureSandbox, "redis-cli missing script"},
		{"./bin/app: cannot execute binary file", FailureSandbox, "cannot execute binary"},
		{"'tool' is not recognized as an internal or external command", FailureSandbox, "windows unrecognized command"},
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
	toolOuts := []string{"tool read_file output content"}

	prompt := buildRetryPrompt(prev, testOut, testErr, FailureTimeout, toolOuts)
	for _, s := range []string{prev, testOut, "exit status 1", "The fix attempt was made", "tool read_file output content"} {
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

type mockRepairLLMClient struct {
	resp *domain.LLMResponse
}

func (m *mockRepairLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return m.resp, nil
}

type mockRepairSandbox struct {
	output string
	err    error
}

func (m *mockRepairSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	return m.output, m.err
}

type noopTool struct{}

func (t *noopTool) Name() string        { return "noop" }
func (t *noopTool) Description() string { return "noop" }
func (t *noopTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	return "noop success", nil
}

func TestAttemptRepair_FailurePropagatesOutput(t *testing.T) {
	llm := &mockRepairLLMClient{
		resp: &domain.LLMResponse{
			Actions: []domain.LLMAction{
				{Tool: "noop", Args: map[string]any{}},
			},
		},
	}
	sandbox := &mockRepairSandbox{
		output: "Linter failed. Renamed Layout/Tab to Layout/IndentationStyle",
		err:    errors.New("exit status 1"),
	}

	evaluator := NewTestValidator(sandbox, false, nil, nil)
	evaluator.LinterCommand = "rubocop"

	wr := NewWatchdogRepair(llm, sandbox, map[string]Tool{"noop": &noopTool{}}, evaluator)

	task := domain.Task{
		ID:    "task-1",
		Title: "Test Task",
	}
	state := &domain.State{
		ProjectPath: "/tmp",
	}

	res, err := wr.AttemptRepair(context.Background(), state, task, "original error", errors.New("original error"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Success {
		t.Error("expected repair to fail")
	}

	if !strings.Contains(res.Output, sandbox.output) {
		t.Errorf("expected propagated output to contain %q, got %q", sandbox.output, res.Output)
	}
}
