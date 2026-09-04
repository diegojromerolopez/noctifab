package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// errorSyntaxChecker is a test double that always returns the configured error.
type errorSyntaxChecker struct{ err error }

func (e *errorSyntaxChecker) Check(_ context.Context, _ string) error { return e.err }

func TestWriteFileTool_SyntaxChecker(t *testing.T) {
	t.Run("when SyntaxChecker is nil it defaults to noop and succeeds", func(t *testing.T) {
		dir := t.TempDir()
		tool := &WriteFileTool{SyntaxChecker: nil}
		state := &domain.State{ProjectPath: dir}
		_, err := tool.Execute(context.Background(), state, map[string]any{
			"path":    "hello.py",
			"content": "x = 1\n",
		})
		if err != nil {
			t.Errorf("expected nil with nil checker, got %v", err)
		}
	})

	t.Run("when SyntaxChecker succeeds the file is written without error", func(t *testing.T) {
		dir := t.TempDir()
		tool := &WriteFileTool{SyntaxChecker: &NoopSyntaxChecker{}}
		state := &domain.State{ProjectPath: dir}
		_, err := tool.Execute(context.Background(), state, map[string]any{
			"path":    "hello.rb",
			"content": "puts 'hi'\n",
		})
		if err != nil {
			t.Errorf("expected nil, got %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "hello.rb")); statErr != nil {
			t.Error("expected file to exist after successful write")
		}
	})

	t.Run("when SyntaxChecker returns an error the tool propagates it", func(t *testing.T) {
		dir := t.TempDir()
		syntaxErr := errors.New("syntax check failed: bad syntax")
		tool := &WriteFileTool{SyntaxChecker: &errorSyntaxChecker{err: syntaxErr}}
		state := &domain.State{ProjectPath: dir}
		_, err := tool.Execute(context.Background(), state, map[string]any{
			"path":    "bad.py",
			"content": "def broken(:\n",
		})
		if err == nil {
			t.Fatal("expected error from syntax checker, got nil")
		}
		if !strings.Contains(err.Error(), "syntax check failed") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestEditFileTool_SyntaxChecker(t *testing.T) {
	t.Run("when SyntaxChecker is nil it defaults to noop and succeeds", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "script.py")
		if err := os.WriteFile(path, []byte("x = 1\ny = 2\n"), 0644); err != nil {
			t.Fatal(err)
		}
		tool := &EditFileTool{SyntaxChecker: nil}
		state := &domain.State{ProjectPath: dir}
		_, err := tool.Execute(context.Background(), state, map[string]any{
			"path": "script.py",
			"edits": []any{map[string]any{
				"start_line":          float64(1),
				"end_line":            float64(1),
				"target_content":      "x = 1",
				"replacement_content": "x = 42",
			}},
		})
		if err != nil {
			t.Errorf("expected nil with nil checker, got %v", err)
		}
	})

	t.Run("when SyntaxChecker returns an error the tool propagates it", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "script.py")
		if err := os.WriteFile(path, []byte("x = 1\n"), 0644); err != nil {
			t.Fatal(err)
		}
		syntaxErr := errors.New("syntax check: parse error")
		tool := &EditFileTool{SyntaxChecker: &errorSyntaxChecker{err: syntaxErr}}
		state := &domain.State{ProjectPath: dir}
		_, err := tool.Execute(context.Background(), state, map[string]any{
			"path": "script.py",
			"edits": []any{map[string]any{
				"start_line":          float64(1),
				"end_line":            float64(1),
				"target_content":      "x = 1",
				"replacement_content": "x = 99",
			}},
		})
		if err == nil {
			t.Fatal("expected error from syntax checker, got nil")
		}
	})
}

type timeoutMockSandbox struct {
	Sandbox
}

func (s *timeoutMockSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	<-ctx.Done()
	return "partial log output", ctx.Err()
}

func TestRunTestsTool_Timeout(t *testing.T) {
	tool := &RunTestsTool{
		Runner:  &timeoutMockSandbox{},
		Timeout: 50 * time.Millisecond,
	}

	out, err := tool.Execute(context.Background(), &domain.State{ProjectPath: "/tmp"}, map[string]any{})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(out, "TIMEOUT: Test command timed out after 50ms") {
		t.Errorf("expected diagnostic timeout message, got %q", out)
	}
}

func TestRunLinterTool_Timeout(t *testing.T) {
	tool := &RunLinterTool{
		Runner:        &timeoutMockSandbox{},
		LinterCommand: "ruff check .",
		Timeout:       50 * time.Millisecond,
	}

	out, err := tool.Execute(context.Background(), &domain.State{ProjectPath: "/tmp"}, map[string]any{})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(out, "TIMEOUT: Linter command timed out after 50ms") {
		t.Errorf("expected diagnostic timeout message, got %q", out)
	}
}
