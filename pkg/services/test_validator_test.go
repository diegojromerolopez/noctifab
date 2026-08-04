package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// scriptedSandbox returns a scripted sequence of outcomes for each
// successive RunCommand call.
type scriptedSandbox struct {
	results []error
	outputs []string
	calls   int
}

func (s *scriptedSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	i := s.calls
	s.calls++
	var out string
	var err error
	if i < len(s.outputs) {
		out = s.outputs[i]
	}
	if i < len(s.results) {
		err = s.results[i]
	}
	return out, err
}

func validatorTask() domain.Task {
	return domain.Task{ID: "T1", Title: "task"}
}

func TestTestValidatorValidateTask(t *testing.T) {
	state := &domain.State{ProjectPath: "/tmp"}

	t.Run("when constructed via NewTestValidator it defaults to a single run", func(t *testing.T) {
		v := NewTestValidator(&scriptedSandbox{}, false, nil, nil)
		if v.Runs != 1 {
			t.Errorf("expected default Runs=1, got %d", v.Runs)
		}
	})

	t.Run("when the single default run passes it validates successfully", func(t *testing.T) {
		sb := &scriptedSandbox{results: []error{nil}, outputs: []string{"ok 1 test"}}
		v := NewTestValidator(sb, false, nil, nil)
		ok, msg, err := v.ValidateTask(context.Background(), state, validatorTask())
		if err != nil || !ok {
			t.Fatalf("expected pass, got ok=%v msg=%q err=%v", ok, msg, err)
		}
		if sb.calls != 1 {
			t.Errorf("expected exactly 1 run, got %d", sb.calls)
		}
	})

	t.Run("when the single default run fails it reports 0/1 runs passed", func(t *testing.T) {
		sb := &scriptedSandbox{results: []error{errors.New("boom")}, outputs: []string{"FAIL"}}
		v := NewTestValidator(sb, false, nil, nil)
		ok, msg, err := v.ValidateTask(context.Background(), state, validatorTask())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatal("expected validation failure")
		}
		if !strings.Contains(msg, "(0/1 runs passed)") {
			t.Errorf("expected accurate 0/1 message, got %q", msg)
		}
	})

	t.Run("when configured with 3 runs and 2 pass it validates by majority vote", func(t *testing.T) {
		sb := &scriptedSandbox{
			results: []error{nil, errors.New("flaky"), nil},
			outputs: []string{"ok", "FAIL", "ok"},
		}
		v := NewTestValidator(sb, false, nil, nil)
		v.Runs = 3
		ok, msg, err := v.ValidateTask(context.Background(), state, validatorTask())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected majority pass, got msg=%q", msg)
		}
		if !strings.Contains(msg, "(2/3 runs passed)") {
			t.Errorf("expected majority vote message, got %q", msg)
		}
		if sb.calls != 3 {
			t.Errorf("expected 3 runs, got %d", sb.calls)
		}
	})

	t.Run("when configured with 3 runs and only 1 passes it fails with an accurate count", func(t *testing.T) {
		sb := &scriptedSandbox{
			results: []error{errors.New("a"), nil, errors.New("b")},
			outputs: []string{"FAIL A", "ok", "FAIL B"},
		}
		v := NewTestValidator(sb, false, nil, nil)
		v.Runs = 3
		ok, msg, err := v.ValidateTask(context.Background(), state, validatorTask())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatal("expected validation failure")
		}
		if !strings.Contains(msg, "(1/3 runs passed)") {
			t.Errorf("expected 1/3 message, got %q", msg)
		}
		if !strings.Contains(msg, "FAIL B") {
			t.Errorf("expected last failure output in message, got %q", msg)
		}
	})

	t.Run("when all configured runs pass it reports full success", func(t *testing.T) {
		sb := &scriptedSandbox{results: []error{nil, nil, nil}, outputs: []string{"ok", "ok", "ok"}}
		v := NewTestValidator(sb, false, nil, nil)
		v.Runs = 3
		ok, msg, err := v.ValidateTask(context.Background(), state, validatorTask())
		if err != nil || !ok {
			t.Fatalf("expected pass, got ok=%v err=%v", ok, err)
		}
		if msg != "All validation runs passed successfully" {
			t.Errorf("unexpected message: %q", msg)
		}
	})

	t.Run("when a run reports no tests ran it counts as a failed run", func(t *testing.T) {
		sb := &scriptedSandbox{results: []error{nil}, outputs: []string{"Ran 0 tests in 0.000s"}}
		v := NewTestValidator(sb, false, nil, nil)
		ok, msg, err := v.ValidateTask(context.Background(), state, validatorTask())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Errorf("expected failure for no tests ran, got msg=%q", msg)
		}
	})
}
