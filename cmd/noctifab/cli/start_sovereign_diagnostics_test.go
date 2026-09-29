package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
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

func TestCollectSovereignDiagnostics_SlidingWindow(t *testing.T) {
	longOutput := strings.Repeat("A", 1000) + "TARGET_FAILURE_TAIL"

	t.Run("when slidingWindow is 0 or negative, raw output is retained completely", func(t *testing.T) {
		diag := CollectSovereignDiagnosticsWithWindow("", nil, nil, nil, longOutput, 0)
		if !strings.Contains(diag, strings.Repeat("A", 1000)) {
			t.Errorf("expected full log retained when slidingWindow=0")
		}
		if !strings.Contains(diag, "TARGET_FAILURE_TAIL") {
			t.Errorf("expected failure tail present")
		}
		if strings.Contains(diag, "log truncated by sovereign rescue sliding window") {
			t.Errorf("did not expect truncation header when slidingWindow=0")
		}
	})

	t.Run("when slidingWindow is set, truncates to sliding window and preserves tail", func(t *testing.T) {
		diag := CollectSovereignDiagnosticsWithWindow("", nil, nil, nil, longOutput, 50)
		if !strings.Contains(diag, "log truncated by sovereign rescue sliding window") {
			t.Errorf("expected truncation notice when log exceeds sliding window")
		}
		if !strings.Contains(diag, "TARGET_FAILURE_TAIL") {
			t.Errorf("expected failure tail preserved in truncated log")
		}
		if strings.Contains(diag, strings.Repeat("A", 100)) {
			t.Errorf("expected head of 1000 As to be pruned by sliding window")
		}
	})
}

func TestCollectSovereignDiagnostics_WindowingSchema(t *testing.T) {
	tmpDir := t.TempDir()
	largeFile := filepath.Join(tmpDir, "large.py")
	var lines []string
	for i := 1; i <= 200; i++ {
		lines = append(lines, fmt.Sprintf("line_%d = %d", i, i))
	}
	_ = os.WriteFile(largeFile, []byte(strings.Join(lines, "\n")), 0644)

	t.Run("when diff_window mode is configured it slices around target line with omissions", func(t *testing.T) {
		snippet := extractFileSnippetWithConfig(tmpDir, largeFile, 100, config.ContextConfig{
			Mode:       "diff_window",
			WindowSize: 20,
		})
		if !strings.Contains(snippet, "omitted before") {
			t.Errorf("expected omission marker before window, got:\n%s", snippet)
		}
		if !strings.Contains(snippet, "omitted after") {
			t.Errorf("expected omission marker after window, got:\n%s", snippet)
		}
		if !strings.Contains(snippet, ">> 100 | line_100 = 100") {
			t.Errorf("expected target line 100 marked with >>, got:\n%s", snippet)
		}
	})

	t.Run("when tree_sitter mode is configured it extracts symbol definitions", func(t *testing.T) {
		pyCode := "import sys\nimport os\n\ndef my_func():\n    pass\n\nclass MyClass:\n    pass\n"
		for i := 0; i < 30; i++ {
			pyCode += fmt.Sprintf("x_%d = %d\n", i, i)
		}
		pyFile := filepath.Join(tmpDir, "symbols.py")
		_ = os.WriteFile(pyFile, []byte(pyCode), 0644)

		snippet := extractFileSnippetWithConfig(tmpDir, pyFile, 1, config.ContextConfig{
			Mode:       "tree_sitter",
			TreeSitter: true,
		})
		if !strings.Contains(snippet, "def my_func") {
			t.Errorf("expected def symbol in tree-sitter output, got:\n%s", snippet)
		}
		if !strings.Contains(snippet, "class MyClass") {
			t.Errorf("expected class symbol in tree-sitter output, got:\n%s", snippet)
		}
	})
}

func TestBuildSovereignRescuePrompt_Compaction(t *testing.T) {
	spec := "# The Specification\nPlease kindly ensure that you implement the system with utmost care."
	stories := []string{"US-001: Implement Core"}
	gaps := []string{"Missing endpoint"}
	failureLog := "Traceback (most recent call last):\n  File 'test.py', line 1"

	t.Run("when caveman compaction is specified it compacts the head and preserves JSON schema tail", func(t *testing.T) {
		prompt := buildSovereignRescuePrompt(spec, stories, gaps, failureLog, 1, 5, "auto", false, "caveman")
		if !strings.Contains(prompt, "=== AVAILABLE TOOLS ===") {
			t.Errorf("expected tools section preserved verbatim")
		}
		if !strings.Contains(prompt, "=== REQUIRED RESPONSE FORMAT ===") {
			t.Errorf("expected response format preserved verbatim")
		}
		if !strings.Contains(prompt, "write_file") || !strings.Contains(prompt, "check_socket") {
			t.Errorf("expected tools to remain intact")
		}
	})
}
