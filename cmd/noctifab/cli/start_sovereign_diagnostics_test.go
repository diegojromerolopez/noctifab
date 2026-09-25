package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestCollectSovereignDiagnostics(t *testing.T) {
	tmpDir := t.TempDir()

	// Create sample source files
	srcDir := filepath.Join(tmpDir, "src")
	_ = os.MkdirAll(srcDir, 0755)
	serverPy := filepath.Join(srcDir, "server.py")
	_ = os.WriteFile(serverPy, []byte("import os\nimport sys\nfrom .resp import encode\n\ndef run(): pass\n"), 0644)

	respPy := filepath.Join(srcDir, "resp.py")
	_ = os.WriteFile(respPy, []byte("def bulk(): pass\n"), 0644)

	t.Run("when task failure log contains python traceback it extracts offending file snippet", func(t *testing.T) {
		trace := `Traceback (most recent call last):
  File "` + serverPy + `", line 3, in <module>
    from .resp import encode
ImportError: cannot import name 'encode' from 'src.resp'`

		state := &domain.State{
			ProjectPath: tmpDir,
			Tasks: []domain.Task{
				{
					ID:          "T1",
					Title:       "Server Core",
					Status:      domain.TaskFailed,
					FailureLog:  trace,
					TargetFiles: []string{"src/server.py"},
				},
			},
			LastActions: []domain.Action{
				{
					Timestamp: time.Now(),
					Tool:      "run_tests",
					Reasoning: "command failed with exit code 1",
					Success:   false,
				},
			},
		}

		diagnostics := CollectSovereignDiagnostics(
			tmpDir,
			state,
			[]string{"US-001 (Server implementation failed)"},
			[]string{"Contract commands.core failed"},
			"Validation failed: exit status 1",
		)

		if !strings.Contains(diagnostics, "T1") {
			t.Errorf("expected diagnostics to contain task ID T1")
		}
		if !strings.Contains(diagnostics, "ImportError: cannot import name 'encode'") {
			t.Errorf("expected diagnostics to contain traceback")
		}
		if !strings.Contains(diagnostics, "src/server.py") {
			t.Errorf("expected diagnostics to reference src/server.py")
		}
		if !strings.Contains(diagnostics, ">>   3 | from .resp import encode") {
			t.Errorf("expected highlighted line snippet in diagnostics, got:\n%s", diagnostics)
		}
		if !strings.Contains(diagnostics, "Contract commands.core failed") {
			t.Errorf("expected diagnostics to contain acceptance gap")
		}
	})

	t.Run("when state is nil it formats available arguments safely", func(t *testing.T) {
		diagnostics := CollectSovereignDiagnostics(
			tmpDir,
			nil,
			[]string{"US-002"},
			[]string{"Gap 1"},
			"compile error",
		)

		if !strings.Contains(diagnostics, "compile error") {
			t.Errorf("expected validation output in diagnostics")
		}
		if !strings.Contains(diagnostics, "Gap 1") {
			t.Errorf("expected acceptance gap in diagnostics")
		}
	})
}

func TestExtractOffendingFilePaths(t *testing.T) {
	tmpDir := t.TempDir()
	mainPy := filepath.Join(tmpDir, "main.py")
	_ = os.WriteFile(mainPy, []byte("print('hello')\n"), 0644)

	logs := `File "` + mainPy + `", line 1, in <module>
File "/Users/user/.asdf/installs/python/3.13.5/lib/python3.13/unittest.py", line 42, in run
main.py:1: syntax error`

	files := extractOffendingFilePaths(tmpDir, logs, []string{"main.py"})
	if _, ok := files[mainPy]; !ok {
		t.Errorf("expected %s to be detected as offending file", mainPy)
	}

	// External library should be ignored
	for f := range files {
		if strings.Contains(f, ".asdf") {
			t.Errorf("expected external runtime file to be ignored, got: %s", f)
		}
	}
}

func TestCollectSovereignDiagnostics_IncludesTelemetrySpans(t *testing.T) {
	tmpDir := t.TempDir()
	noctiDir := filepath.Join(tmpDir, ".noctifab")
	_ = os.MkdirAll(noctiDir, 0755)
	tracePath := filepath.Join(noctiDir, "traces.jsonl")
	data := `{"name":"RunCommand","duration_ms":2100,"status":"Error","attributes":{"command":"cargo test"}}` + "\n"
	_ = os.WriteFile(tracePath, []byte(data), 0644)

	// When sandbox.telemetry.inject is true:
	diagnostics := CollectSovereignDiagnostics(tmpDir, nil, []string{"US-001"}, nil, "cargo test failed", true)
	if !strings.Contains(diagnostics, "RECENT OPENTELEMETRY WORKFLOW & SUBPROCESS SPANS") {
		t.Errorf("expected telemetry spans section in sovereign diagnostics when inject=true")
	}
	if !strings.Contains(diagnostics, "cargo test") {
		t.Errorf("expected command attribute from span in sovereign diagnostics when inject=true")
	}

	// When sandbox.telemetry.inject is false:
	diagnosticsDisabled := CollectSovereignDiagnostics(tmpDir, nil, []string{"US-001"}, nil, "cargo test failed", false)
	if strings.Contains(diagnosticsDisabled, "RECENT OPENTELEMETRY WORKFLOW & SUBPROCESS SPANS") {
		t.Errorf("expected NO telemetry spans section in sovereign diagnostics when inject=false")
	}

	// When default (no flag passed):
	diagnosticsDefault := CollectSovereignDiagnostics(tmpDir, nil, []string{"US-001"}, nil, "cargo test failed")
	if strings.Contains(diagnosticsDefault, "RECENT OPENTELEMETRY WORKFLOW & SUBPROCESS SPANS") {
		t.Errorf("expected NO telemetry spans section in sovereign diagnostics by default")
	}
}
