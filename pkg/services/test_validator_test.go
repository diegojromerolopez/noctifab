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
			results: []error{errors.New("flaky"), nil, nil},
			outputs: []string{"FAIL", "ok", "ok"},
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

	t.Run("when short-circuit consensus is enabled and run 1 passes it short-circuits after single run", func(t *testing.T) {
		sb := &scriptedSandbox{results: []error{nil, nil, nil}, outputs: []string{"ok 1", "ok 2", "ok 3"}}
		v := NewTestValidator(sb, false, nil, nil)
		v.Runs = 3
		v.ShortCircuitConsensus = true
		ok, msg, err := v.ValidateTask(context.Background(), state, validatorTask())
		if err != nil || !ok {
			t.Fatalf("expected pass, got ok=%v err=%v", ok, err)
		}
		if !strings.Contains(msg, "short-circuit consensus") {
			t.Errorf("expected short-circuit message, got %q", msg)
		}
		if sb.calls != 1 {
			t.Errorf("expected exactly 1 call due to short-circuit, got %d", sb.calls)
		}
	})

	t.Run("when all configured runs pass with short-circuit disabled it reports full success", func(t *testing.T) {
		sb := &scriptedSandbox{results: []error{nil, nil, nil}, outputs: []string{"ok", "ok", "ok"}}
		v := NewTestValidator(sb, false, nil, nil)
		v.Runs = 3
		v.ShortCircuitConsensus = false
		ok, msg, err := v.ValidateTask(context.Background(), state, validatorTask())
		if err != nil || !ok {
			t.Fatalf("expected pass, got ok=%v err=%v", ok, err)
		}
		if msg != "All validation runs passed successfully" {
			t.Errorf("unexpected message: %q", msg)
		}
		if sb.calls != 3 {
			t.Errorf("expected 3 calls with short-circuit disabled, got %d", sb.calls)
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

	t.Run("when formatter command is configured it runs auto-formatter before test execution", func(t *testing.T) {
		sb := &scriptedSandbox{
			results: []error{nil, nil},
			outputs: []string{"formatted files", "PASS: 1 test passed"},
		}
		v := NewTestValidator(sb, false, nil, nil)
		v.FormatterCommand = "cargo fmt"
		ok, msg, err := v.ValidateTask(context.Background(), state, validatorTask())
		if err != nil || !ok {
			t.Fatalf("expected pass with formatter executed, got ok=%v msg=%q err=%v", ok, msg, err)
		}
	})

	t.Run("when syntax check fails it aborts immediately before running test suite", func(t *testing.T) {
		sb := &scriptedSandbox{
			results: []error{nil},
			outputs: []string{"PASS: 1 test passed"},
		}
		v := NewTestValidator(sb, false, nil, nil)
		v.SyntaxChecker = &mockValidatorSyntaxChecker{err: errors.New("syntax error: unexpected token on line 4")}
		ok, msg, err := v.ValidateTask(context.Background(), state, validatorTask())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Errorf("expected failure when syntax check fails, got pass")
		}
		if !strings.Contains(msg, "Fast-path syntax check failed") {
			t.Errorf("expected fast-path syntax error in message, got %q", msg)
		}
		if sb.calls != 0 {
			t.Errorf("expected sandbox not to be called on syntax failure, got %d calls", sb.calls)
		}
	})

	t.Run("when project has Makefile build target and build fails it fails validation with compiler error", func(t *testing.T) {
		buildDir := t.TempDir()
		buildState := &domain.State{ProjectPath: buildDir}
		_ = os.WriteFile(filepath.Join(buildDir, "Makefile"), []byte("build:\n\tgcc src/*.c\n"), 0644)

		sb := &scriptedSandbox{
			results: []error{errors.New("exit status 1")},
			outputs: []string{"src/route.c:1: error: ISO C forbids an empty translation unit"},
		}
		v := NewTestValidator(sb, false, nil, nil)
		ok, msg, err := v.ValidateTask(context.Background(), buildState, validatorTask())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatalf("expected validation failure when build gate fails")
		}
		if !strings.Contains(msg, "Build verification failed (make build)") {
			t.Errorf("expected build verification failure message, got %q", msg)
		}
		if !strings.Contains(msg, "empty translation unit") {
			t.Errorf("expected compiler error in failure message, got %q", msg)
		}
	})

	t.Run("when project has Makefile build target and build tool is absent on host it proceeds in degraded mode", func(t *testing.T) {
		buildDir := t.TempDir()
		buildState := &domain.State{ProjectPath: buildDir}
		_ = os.WriteFile(filepath.Join(buildDir, "Makefile"), []byte("build:\n\tgcc -o app main.c\ntest:\n\t./test\n"), 0644)
		_ = os.Mkdir(filepath.Join(buildDir, "tests"), 0755)
		_ = os.WriteFile(filepath.Join(buildDir, "tests", "test_app.c"), []byte("int test() { return 1; }"), 0644)

		sb := &scriptedSandbox{
			results: []error{errors.New("exit status 127"), nil},
			outputs: []string{"make: command not found", "All tests passed"},
		}
		v := NewTestValidator(sb, false, nil, nil)
		ok, msg, err := v.ValidateTask(context.Background(), buildState, validatorTask())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation to proceed in degraded mode when build tool is absent, got: %s", msg)
		}
		if !strings.Contains(msg, "All validation runs passed successfully") {
			t.Errorf("expected test suite to pass after degraded build, got %q", msg)
		}
	})

	t.Run("when make test runs with 0 tests in tests directory it fails validation with zero tests error", func(t *testing.T) {
		testDir := t.TempDir()
		testState := &domain.State{ProjectPath: testDir}
		_ = os.WriteFile(filepath.Join(testDir, "Makefile"), []byte("test:\n\t./bin/test_suite\n"), 0644)
		_ = os.Mkdir(filepath.Join(testDir, "tests"), 0755)

		sb := &scriptedSandbox{
			results: []error{nil},
			outputs: []string{""},
		}
		v := NewTestValidator(sb, false, nil, nil)
		ok, msg, err := v.ValidateTask(context.Background(), testState, validatorTask())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatalf("expected validation failure when 0 test files exist in tests directory")
		}
		if !strings.Contains(msg, "0 test files discovered in tests/ directory") {
			t.Errorf("expected 0 test files error in message, got %q", msg)
		}
	})

	t.Run("when build block is resolved and make build succeeds, it proceeds to test suite and passes", func(t *testing.T) {
		resolvedDir := t.TempDir()
		resolvedState := &domain.State{ProjectPath: resolvedDir}
		_ = os.WriteFile(filepath.Join(resolvedDir, "Makefile"), []byte("build:\n\tgcc src/*.c\ntest:\n\t./bin/test_suite\n"), 0644)
		_ = os.Mkdir(filepath.Join(resolvedDir, "tests"), 0755)
		_ = os.WriteFile(filepath.Join(resolvedDir, "tests", "test_main.c"), []byte("#include <assert.h>\nint main() {\n    assert(1 == 1);\n    return 0;\n}\n"), 0644)

		sb := &scriptedSandbox{
			results: []error{nil, nil},
			outputs: []string{"compilation successful", "PASS: 5 tests passed"},
		}
		v := NewTestValidator(sb, false, nil, nil)
		ok, msg, err := v.ValidateTask(context.Background(), resolvedState, validatorTask())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation success after build and test blocks resolved, got: %s", msg)
		}
		if !strings.Contains(msg, "All validation runs passed successfully") {
			t.Errorf("expected success message, got %q", msg)
		}
		if sb.calls != 2 {
			t.Errorf("expected 2 sandbox calls (build pre-gate + test runner), got %d", sb.calls)
		}
	})

	t.Run("when zero-tests block is resolved by authoring test files in tests directory, it validates successfully", func(t *testing.T) {
		resolvedTestDir := t.TempDir()
		resolvedTestState := &domain.State{ProjectPath: resolvedTestDir}
		_ = os.WriteFile(filepath.Join(resolvedTestDir, "Makefile"), []byte("test:\n\t./bin/test_suite\n"), 0644)
		testsPath := filepath.Join(resolvedTestDir, "tests")
		_ = os.Mkdir(testsPath, 0755)
		_ = os.WriteFile(filepath.Join(testsPath, "test_fortune.c"), []byte("void test_fortune() {}\n"), 0644)

		sb := &scriptedSandbox{
			results: []error{nil},
			outputs: []string{"OK: 12 tests executed successfully"},
		}
		v := NewTestValidator(sb, false, nil, nil)
		ok, msg, err := v.ValidateTask(context.Background(), resolvedTestState, validatorTask())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation success once test files exist, got: %s", msg)
		}
	})

	t.Run("when anti-stub block is resolved by replacing stubs with concrete implementation, validation passes", func(t *testing.T) {
		taskDir := t.TempDir()
		taskState := &domain.State{ProjectPath: taskDir}
		pyFile := filepath.Join(taskDir, "service.py")
		// Initially a stub
		_ = os.WriteFile(pyFile, []byte("def run():\n    pass\n"), 0644)
		task := domain.Task{ID: "T1", Title: "real task", TargetFiles: []string{"service.py"}}

		sb := &scriptedSandbox{results: []error{nil}, outputs: []string{"OK: 1 test passed"}}
		v := NewTestValidator(sb, false, nil, nil)
		ok, _, _ := v.ValidateTask(context.Background(), taskState, task)
		if ok {
			t.Fatal("expected stub validation to fail initially")
		}

		// Resolve stub block by replacing with real, functional implementation
		_ = os.WriteFile(pyFile, []byte("def run():\n    result = 42 * 2\n    return result\n"), 0644)
		ok, msg, err := v.ValidateTask(context.Background(), taskState, task)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation to pass once stub block is resolved, got: %s", msg)
		}
	})
}

type mockValidatorSyntaxChecker struct {
	err error
}

func (m *mockValidatorSyntaxChecker) Check(ctx context.Context, path string) error {
	return m.err
}

func TestIsMissingToolOutput(t *testing.T) {
	t.Run("when host command is not found it reports true", func(t *testing.T) {
		samples := []string{
			"bash: docker: command not found",
			"exec: \"cargo\": executable file not found in $PATH",
			"exec: \"cargo\": executable file not found in %PATH%",
			"'cargo' is not recognized as an internal or external command",
			"fork/exec /usr/local/bin/nonexistent: no such file or directory",
			"tool binary is evicted from cache",
			"process exited with exit status 127",
		}
		for _, s := range samples {
			if !isMissingToolOutput(s) {
				t.Errorf("expected isMissingToolOutput(%q) to be true", s)
			}
		}
	})

	t.Run("when error is container build failure or missing in-container file it reports false", func(t *testing.T) {
		samples := []string{
			"Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?",
			"target e2e: failed to solve: process \"/bin/sh -c python -m compileall -q src && chmod +x /app/tests/e2e/run_tests.sh\" did not complete successfully: exit code: 1\nchmod: /app/tests/e2e/run_tests.sh: No such file or directory",
			"executor failed running [/bin/sh -c chmod +x /app/tests/e2e/run_tests.sh]: exit code: 1",
			"Dockerfile.e2e:5\nfailed to solve: process returned error",
			"load build definition from Dockerfile.e2e\ntransferring dockerfile: 198B done",
			"FileNotFoundError: [Errno 2] No such file or directory: 'data/dump.aof'\nTraceback (most recent call last):\n  File \"exec.py\", line 12",
			"FAILED tests/unit/test_resp.py - AssertionError: 1 != 2",
		}
		for _, s := range samples {
			if isMissingToolOutput(s) {
				t.Errorf("expected isMissingToolOutput(%q) to be false", s)
			}
		}
	})
}
