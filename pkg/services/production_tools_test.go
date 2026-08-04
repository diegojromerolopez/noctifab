package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func requirePython(t *testing.T) {
	t.Helper()
	if _, err3 := exec.LookPath("python3"); err3 != nil {
		if _, err := exec.LookPath("python"); err != nil {
			t.Skip("python interpreter not available")
		}
	}
}

func TestCheckPythonSyntax(t *testing.T) {
	t.Run("when the file is not a python file it is skipped without error", func(t *testing.T) {
		if err := checkPythonSyntax(context.Background(), "/tmp/whatever.go"); err != nil {
			t.Errorf("expected nil for non-python file, got %v", err)
		}
	})

	t.Run("when the python file is valid it passes", func(t *testing.T) {
		requirePython(t)
		path := filepath.Join(t.TempDir(), "ok.py")
		if err := os.WriteFile(path, []byte("x = 1\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := checkPythonSyntax(context.Background(), path); err != nil {
			t.Errorf("expected valid syntax, got %v", err)
		}
	})

	t.Run("when the python file has a syntax error it fails", func(t *testing.T) {
		requirePython(t)
		path := filepath.Join(t.TempDir(), "bad.py")
		if err := os.WriteFile(path, []byte("def broken(:\n"), 0644); err != nil {
			t.Fatal(err)
		}
		err := checkPythonSyntax(context.Background(), path)
		if err == nil {
			t.Fatal("expected a syntax error")
		}
		if !strings.Contains(err.Error(), "python syntax compilation failed") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("when the caller ctx is already cancelled it fails fast", func(t *testing.T) {
		requirePython(t)
		path := filepath.Join(t.TempDir(), "ok.py")
		if err := os.WriteFile(path, []byte("x = 1\n"), 0644); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		start := time.Now()
		if err := checkPythonSyntax(ctx, path); err == nil {
			t.Error("expected an error with a cancelled context")
		}
		if time.Since(start) > 5*time.Second {
			t.Error("cancelled syntax check did not fail fast")
		}
	})
}
