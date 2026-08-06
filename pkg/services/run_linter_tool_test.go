package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// stubSandbox records invocations and returns a programmable result per command.
type stubSandbox struct {
	results map[string][]string // command -> {output, error-as-string}
	errs    map[string]error
	calls   []string
}

func (s *stubSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	s.calls = append(s.calls, command)
	if err := s.errs[command]; err != nil {
		return s.results[command][0], err
	}
	return s.results[command][0], nil
}

// TestRunLinterTool_FormatterFailureIsNonFatal verifies that a failing formatter
// auto-fix pre-step (missing target/binary) does not fail the linter step and is
// logged rather than silently swallowed (octopus-review finding).
func TestRunLinterTool_FormatterFailureIsNonFatal(t *testing.T) {
	sb := &stubSandbox{
		results: map[string][]string{
			"make format": {"formatter crashed", ""},
			"make lint":   {"No lint errors found", ""},
		},
		errs: map[string]error{"make format": errors.New("exit status 1")},
	}
	tool := &RunLinterTool{
		Runner:           sb,
		LinterCommand:    "make lint",
		FormatterCommand: "make format",
		Timeout:          time.Minute,
	}

	out, err := tool.Execute(context.Background(), &domain.State{ProjectPath: "/tmp/x"}, nil)
	if err != nil {
		t.Fatalf("Execute should not fail when the formatter pre-step fails: %v", err)
	}
	if out != "No lint errors found" {
		t.Errorf("expected linter output to be returned despite formatter failure, got %q", out)
	}
	if len(sb.calls) != 2 || sb.calls[0] != "make format" || sb.calls[1] != "make lint" {
		t.Errorf("expected [make format, make lint] invocations, got %v", sb.calls)
	}
}

// TestRunLinterTool_NoFormatterSkipsPreStep verifies that an empty FormatterCommand
// skips the pre-step entirely.
func TestRunLinterTool_NoFormatterSkipsPreStep(t *testing.T) {
	sb := &stubSandbox{
		results: map[string][]string{"make lint": {"clean", ""}},
	}
	tool := &RunLinterTool{Runner: sb, LinterCommand: "make lint", Timeout: time.Minute}

	if _, err := tool.Execute(context.Background(), &domain.State{ProjectPath: "/tmp/x"}, nil); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(sb.calls) != 1 || sb.calls[0] != "make lint" {
		t.Errorf("expected only the linter to run, got %v", sb.calls)
	}
}

func TestRunLinterTool_MaxLinterIssuesThreshold(t *testing.T) {
	linterOutput := "main.py:10:1: E501 line too long\nmain.py:15:1: W293 blank line contains whitespace\n"
	sb := &stubSandbox{
		results: map[string][]string{"make lint": {linterOutput, "exit status 1"}},
		errs:    map[string]error{"make lint": errors.New("exit status 1")},
	}

	t.Run("within threshold succeeds as advisory warning", func(t *testing.T) {
		tool := &RunLinterTool{Runner: sb, LinterCommand: "make lint", MaxLinterIssues: 10, Timeout: time.Minute}
		out, err := tool.Execute(context.Background(), &domain.State{ProjectPath: "/tmp/x"}, nil)
		if err != nil {
			t.Fatalf("expected nil error when issues (2) <= threshold (10), got: %v", err)
		}
		if !strings.Contains(out, "LINTER ADVISORY") {
			t.Errorf("expected output to contain advisory header, got %q", out)
		}
	})

	t.Run("exceeding threshold returns error", func(t *testing.T) {
		tool := &RunLinterTool{Runner: sb, LinterCommand: "make lint", MaxLinterIssues: 1, Timeout: time.Minute}
		_, err := tool.Execute(context.Background(), &domain.State{ProjectPath: "/tmp/x"}, nil)
		if err == nil {
			t.Fatalf("expected error when issues (2) > threshold (1), got nil")
		}
	})

	t.Run("disabled threshold (-1) returns nil error", func(t *testing.T) {
		tool := &RunLinterTool{Runner: sb, LinterCommand: "make lint", MaxLinterIssues: -1, Timeout: time.Minute}
		out, err := tool.Execute(context.Background(), &domain.State{ProjectPath: "/tmp/x"}, nil)
		if err != nil {
			t.Fatalf("expected nil error when MaxLinterIssues is -1, got: %v", err)
		}
		if !strings.Contains(out, "issues suppressed by max_linter_issues=-1") {
			t.Errorf("expected output to contain suppressed header, got %q", out)
		}
	})
}
