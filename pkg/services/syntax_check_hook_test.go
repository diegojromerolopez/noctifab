package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNoopSyntaxChecker(t *testing.T) {
	t.Run("when called it always returns nil", func(t *testing.T) {
		checker := &NoopSyntaxChecker{}
		if err := checker.Check(context.Background(), "/any/path/file.py"); err != nil {
			t.Errorf("expected nil, got %v", err)
		}
	})

	t.Run("when context is cancelled it still returns nil", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		checker := &NoopSyntaxChecker{}
		if err := checker.Check(ctx, "/any/path/file.go"); err != nil {
			t.Errorf("expected nil even with cancelled ctx, got %v", err)
		}
	})
}

func TestNewCommandSyntaxChecker(t *testing.T) {
	t.Run("when command is empty it returns a NoopSyntaxChecker", func(t *testing.T) {
		checker := NewCommandSyntaxChecker("")
		if _, ok := checker.(*NoopSyntaxChecker); !ok {
			t.Errorf("expected *NoopSyntaxChecker, got %T", checker)
		}
	})

	t.Run("when command is whitespace only it returns a NoopSyntaxChecker", func(t *testing.T) {
		checker := NewCommandSyntaxChecker("   ")
		if _, ok := checker.(*NoopSyntaxChecker); !ok {
			t.Errorf("expected *NoopSyntaxChecker for whitespace command, got %T", checker)
		}
	})

	t.Run("when command is non-empty it returns a CommandSyntaxChecker", func(t *testing.T) {
		checker := NewCommandSyntaxChecker("echo {file}")
		if _, ok := checker.(*CommandSyntaxChecker); !ok {
			t.Errorf("expected *CommandSyntaxChecker, got %T", checker)
		}
	})
}

func TestCommandSyntaxChecker_Check(t *testing.T) {
	t.Run("when command is empty it is a no-op and returns nil", func(t *testing.T) {
		checker := &CommandSyntaxChecker{Command: ""}
		if err := checker.Check(context.Background(), "/some/file.py"); err != nil {
			t.Errorf("expected nil for empty command, got %v", err)
		}
	})

	t.Run("when command succeeds it returns nil", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "ok.txt")
		if err := os.WriteFile(path, []byte("hello\n"), 0644); err != nil {
			t.Fatal(err)
		}
		checker := &CommandSyntaxChecker{Command: "echo {file}"}
		if err := checker.Check(context.Background(), path); err != nil {
			t.Errorf("expected nil for succeeding command, got %v", err)
		}
	})

	t.Run("when command fails it returns an error with output", func(t *testing.T) {
		checker := &CommandSyntaxChecker{Command: "false"}
		err := checker.Check(context.Background(), "/tmp/somefile.py")
		if err == nil {
			t.Fatal("expected error from failing command, got nil")
		}
	})

	t.Run("when file placeholder is substituted correctly", func(t *testing.T) {
		dir := t.TempDir()
		outFile := filepath.Join(dir, "captured.txt")
		// Use sh -c to capture the file path argument
		checker := &CommandSyntaxChecker{Command: "sh -c 'echo {file} > " + outFile + "'"}
		target := filepath.Join(dir, "source.py")
		if wErr := os.WriteFile(target, []byte("x=1\n"), 0644); wErr != nil {
			t.Fatal(wErr)
		}
		if err := checker.Check(context.Background(), target); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, readErr := os.ReadFile(outFile)
		if readErr != nil {
			t.Fatal(readErr)
		}
		content := string(got)
		if content == "" {
			t.Error("expected captured path output to be non-empty")
		}
	})

	t.Run("when context is already cancelled it fails fast", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		checker := &CommandSyntaxChecker{Command: "echo {file}"}
		// Should return promptly (error from cancelled context)
		err := checker.Check(ctx, "/tmp/file.py")
		if err == nil {
			t.Error("expected error with cancelled context")
		}
	})
}
