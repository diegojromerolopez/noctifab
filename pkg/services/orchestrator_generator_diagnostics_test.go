package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestBuildFailureDiagnosticsContext(t *testing.T) {
	tmpDir := t.TempDir()

	testFilePath := filepath.Join(tmpDir, "test_server.py")
	testCode := "def test_ping():\n    assert False, 'connection timeout'\n"
	if err := os.WriteFile(testFilePath, []byte(testCode), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	srcFilePath := filepath.Join(tmpDir, "server.py")
	srcCode := "def run():\n    pass\n"
	if err := os.WriteFile(srcFilePath, []byte(srcCode), 0o644); err != nil {
		t.Fatalf("failed to write src file: %v", err)
	}

	rawFailureLog := `FAILED test_server.py::test_ping - AssertionError: connection timeout
File "test_server.py", line 2, in test_ping
    assert False, 'connection timeout'
`

	task := domain.Task{
		ID:          "task-1",
		Title:       "Implement TCP Server",
		FailureLog:  rawFailureLog,
		TargetFiles: []string{"server.py"},
	}

	diagCtx := buildFailureDiagnosticsContext(task, tmpDir)

	if !strings.Contains(diagCtx, "FAILING TEST & ERROR TRACE CONTEXT") {
		t.Errorf("expected header 'FAILING TEST & ERROR TRACE CONTEXT', got %s", diagCtx)
	}
	if !strings.Contains(diagCtx, "connection timeout") {
		t.Errorf("expected failure trace to contain 'connection timeout', got %s", diagCtx)
	}
	if !strings.Contains(diagCtx, "test_server.py") {
		t.Errorf("expected failing test file to be referenced, got %s", diagCtx)
	}
	if !strings.Contains(diagCtx, "server.py") {
		t.Errorf("expected target file to be referenced, got %s", diagCtx)
	}
}

func TestFormatTurnDiagnosticFeedback(t *testing.T) {
	out := "ERROR: line 10: undefined variable 'conn'\nTraceback (most recent call last):\n  File 'server.py', line 10"
	execErr := errors.New("exit status 1")

	feedback := formatTurnDiagnosticFeedback("run_tests", execErr, out)

	if !strings.Contains(feedback, "Tool run_tests failed: exit status 1") {
		t.Errorf("expected failure tool name and error, got %s", feedback)
	}
	if !strings.Contains(feedback, "Full Diagnostic Trace") {
		t.Errorf("expected full diagnostic trace header, got %s", feedback)
	}
	if !strings.Contains(feedback, "undefined variable 'conn'") {
		t.Errorf("expected trace content, got %s", feedback)
	}
}

func TestSinglePassRetryWithDiagnostics(t *testing.T) {
	tmpDir := t.TempDir()
	taskGit := NewGitClient(tmpDir)
	_, _ = taskGit.Run(context.Background(), true, "init")

	task := &domain.Task{
		ID:         "task-retry",
		Title:      "Implement Protocol",
		Retries:    1,
		FailureLog: "FAILED test_proto.py::test_parse - TimeoutError: no response in 5s",
	}

	state := &domain.State{
		ProjectPath: tmpDir,
	}

	// Verify buildFailureDiagnosticsContext doesn't panic on empty files
	ctx := buildFailureDiagnosticsContext(*task, state.ProjectPath)
	if !strings.Contains(ctx, "TimeoutError: no response in 5s") {
		t.Errorf("expected timeout error in diagnostics, got %s", ctx)
	}
}
