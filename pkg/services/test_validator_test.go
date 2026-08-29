package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// scriptedSandbox returns a scripted sequence of outcomes for each
// successive RunCommand call.
type scriptedSandbox struct {
	mu      sync.Mutex
	results []error
	outputs []string
	calls   int
}

func (s *scriptedSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	tmpDir := t.TempDir()
	state := &domain.State{ProjectPath: tmpDir}

	t.Run("when anti-stub validator detects stubs it fails before running commands", func(t *testing.T) {
		taskDir := t.TempDir()
		taskState := &domain.State{ProjectPath: taskDir}
		_ = os.WriteFile(filepath.Join(taskDir, "stub.py"), []byte("def hello():\n    raise NotImplementedError()\n"), 0644)
		task := domain.Task{ID: "T1", Title: "stub task", TargetFiles: []string{"stub.py"}}

		sb := &scriptedSandbox{results: []error{nil}, outputs: []string{"ok 1 test"}}
		v := NewTestValidator(sb, false, nil, nil)
		ok, msg, err := v.ValidateTask(context.Background(), taskState, task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatal("expected validation failure due to anti-stub violation")
		}
		if !strings.Contains(msg, "Anti-stub / anti-gaming validation failed") {
			t.Errorf("expected anti-stub failure message, got %q", msg)
		}
		if sb.calls != 0 {
			t.Errorf("expected 0 command runner calls when anti-stub fails, got %d", sb.calls)
		}
	})

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
		if !strings.Contains(msg, "FAIL A") && !strings.Contains(msg, "FAIL B") {
			t.Errorf("expected failure output in message, got %q", msg)
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

	t.Run("when linter fails within MaxLinterIssues threshold it passes validation", func(t *testing.T) {
		linterOut := "app.py:1:1: E501 line too long\napp.py:2:1: W293 blank line whitespace\n"
		sb := &scriptedSandbox{
			results: []error{errors.New("linter exit status 1"), nil},
			outputs: []string{linterOut, "PASS: 1 test passed"},
		}
		v := NewTestValidator(sb, false, nil, nil)
		v.LinterCommand = "make lint"
		v.MaxLinterIssues = 10
		ok, msg, err := v.ValidateTask(context.Background(), state, validatorTask())
		if err != nil || !ok {
			t.Fatalf("expected pass when linter issue count (2) <= threshold (10), got ok=%v msg=%q err=%v", ok, msg, err)
		}
	})

	t.Run("when linter fails twice consecutively it defers enforcement on the third call", func(t *testing.T) {
		linterErr := errors.New("linter exit status 1")
		linterOut := "app.py:1:1: E501 line too long\n"
		// 1st run: linter fail
		sb1 := &scriptedSandbox{results: []error{linterErr}, outputs: []string{linterOut}}
		v := NewTestValidator(sb1, false, nil, nil)
		v.LinterCommand = "make lint"
		v.MaxLinterIssues = 0

		ok1, _, _ := v.ValidateTask(context.Background(), state, validatorTask())
		if ok1 {
			t.Fatal("expected failure on 1st linter run")
		}

		// 2nd run: linter fail (counter = 2)
		sb2 := &scriptedSandbox{results: []error{linterErr}, outputs: []string{linterOut}}
		v.Runner = sb2
		ok2, _, _ := v.ValidateTask(context.Background(), state, validatorTask())
		if ok2 {
			t.Fatal("expected failure on 2nd linter run")
		}

		// 3rd run: counter >= 2 -> linter skipped, tests run and pass!
		sb3 := &scriptedSandbox{results: []error{nil}, outputs: []string{"PASS: 1 test passed"}}
		v.Runner = sb3
		ok3, msg3, err3 := v.ValidateTask(context.Background(), state, validatorTask())
		if err3 != nil || !ok3 {
			t.Fatalf("expected pass on 3rd run due to linter deferral, got ok=%v msg=%q err=%v", ok3, msg3, err3)
		}
	})

	t.Run("when MaxLinterConsecutiveFailures is customized to 1 it defers enforcement on the second call", func(t *testing.T) {
		linterErr := errors.New("linter exit status 1")
		linterOut := "app.py:1:1: E501 line too long\n"

		// 1st run: linter fail (counter becomes 1)
		sb1 := &scriptedSandbox{results: []error{linterErr}, outputs: []string{linterOut}}
		v := NewTestValidator(sb1, false, nil, nil)
		v.LinterCommand = "make lint"
		v.MaxLinterConsecutiveFailures = 1

		ok1, _, _ := v.ValidateTask(context.Background(), state, validatorTask())
		if ok1 {
			t.Fatal("expected failure on 1st linter run")
		}

		// 2nd run: counter >= 1 -> linter skipped, tests run and pass!
		sb2 := &scriptedSandbox{results: []error{nil}, outputs: []string{"PASS: 1 test passed"}}
		v.Runner = sb2
		ok2, msg2, err2 := v.ValidateTask(context.Background(), state, validatorTask())
		if err2 != nil || !ok2 {
			t.Fatalf("expected pass on 2nd run due to threshold=1 deferral, got ok=%v msg=%q err=%v", ok2, msg2, err2)
		}
	})
}
