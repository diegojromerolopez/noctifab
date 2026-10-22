package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type scriptedPassRateSandbox struct {
	out string
	err error
}

func (s *scriptedPassRateSandbox) RunCommand(ctx context.Context, projectPath, command, pkg string) (string, error) {
	return s.out, s.err
}

func TestTestValidator_MinPassRate_SoftPass(t *testing.T) {
	t.Run("when 4 of 5 tests pass (80%) and exit code is 1, it soft passes with default 80% threshold", func(t *testing.T) {
		// Output simulating TAP or pytest output where 4 passed and 1 failed
		output := "1..5\nok 1 - test_add\nok 2 - test_sub\nok 3 - test_mul\nok 4 - test_div\nnot ok 5 - test_edge_case\nFAILED (failures=1)"
		sb := &scriptedPassRateSandbox{
			out: output,
			err: errors.New("exit status 1"),
		}
		v := NewTestValidator(sb, false, nil, nil)
		v.MinPassRate = 80.0

		state := &domain.State{ProjectPath: "/mock/project"}
		task := domain.Task{ID: "task-soft-pass", Title: "Math operations"}

		ok, msg, err := v.ValidateTask(context.Background(), state, task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected 80%% pass rate to soft pass, but it failed: %s", msg)
		}
		if !strings.Contains(msg, "passed") {
			t.Errorf("expected pass message, got: %s", msg)
		}
	})

	t.Run("when 3 of 5 tests pass (60%) and threshold is 80%, it fails", func(t *testing.T) {
		output := "1..5\nok 1 - test_add\nok 2 - test_sub\nok 3 - test_mul\nnot ok 4 - test_div\nnot ok 5 - test_mod\nFAILED (failures=2)"
		sb := &scriptedPassRateSandbox{
			out: output,
			err: errors.New("exit status 1"),
		}
		v := NewTestValidator(sb, false, nil, nil)
		v.MinPassRate = 80.0

		state := &domain.State{ProjectPath: "/mock/project"}
		task := domain.Task{ID: "task-soft-fail", Title: "Math operations"}

		ok, msg, err := v.ValidateTask(context.Background(), state, task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatalf("expected 60%% pass rate to fail under 80%% threshold, but it passed: %s", msg)
		}
	})

	t.Run("when threshold is custom configured to 50%, 3 of 5 tests pass soft passes", func(t *testing.T) {
		output := "1..5\nok 1 - test_add\nok 2 - test_sub\nok 3 - test_mul\nnot ok 4 - test_div\nnot ok 5 - test_mod\nFAILED (failures=2)"
		sb := &scriptedPassRateSandbox{
			out: output,
			err: errors.New("exit status 1"),
		}
		v := NewTestValidator(sb, false, nil, nil)
		v.MinPassRate = 50.0

		state := &domain.State{ProjectPath: "/mock/project"}
		task := domain.Task{ID: "task-custom-threshold", Title: "Math operations"}

		ok, msg, err := v.ValidateTask(context.Background(), state, task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected 60%% pass rate to pass under 50%% threshold, but it failed: %s", msg)
		}
	})
}
