package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWarmupCommandForDir(t *testing.T) {
	t.Run("returns cargo check for Rust", func(t *testing.T) {
		tempDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(tempDir, "Cargo.toml"), []byte("[package]\nname=\"test\"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		bin, args := WarmupCommandForDir(tempDir)
		if bin != "cargo" || len(args) != 2 || args[0] != "check" {
			t.Errorf("expected cargo check, got %s %v", bin, args)
		}
	})

	t.Run("returns go test for Go", func(t *testing.T) {
		tempDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module test\n"), 0644); err != nil {
			t.Fatal(err)
		}
		bin, args := WarmupCommandForDir(tempDir)
		if bin != "go" || len(args) != 3 || args[0] != "test" {
			t.Errorf("expected go test, got %s %v", bin, args)
		}
	})

	t.Run("returns pytest for Python", func(t *testing.T) {
		tempDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(tempDir, "pyproject.toml"), []byte("[project]\n"), 0644); err != nil {
			t.Fatal(err)
		}
		bin, args := WarmupCommandForDir(tempDir)
		if bin != "python3" || len(args) < 3 || args[1] != "pytest" {
			t.Errorf("expected pytest collect, got %s %v", bin, args)
		}
	})

	t.Run("returns dotnet build for C#", func(t *testing.T) {
		tempDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(tempDir, "app.csproj"), []byte("<Project />"), 0644); err != nil {
			t.Fatal(err)
		}
		bin, args := WarmupCommandForDir(tempDir)
		if bin != "dotnet" || len(args) != 2 || args[0] != "build" {
			t.Errorf("expected dotnet build, got %s %v", bin, args)
		}
	})

	t.Run("returns empty when no recognized manifest", func(t *testing.T) {
		tempDir := t.TempDir()
		bin, args := WarmupCommandForDir(tempDir)
		if bin != "" || len(args) != 0 {
			t.Errorf("expected empty, got %s %v", bin, args)
		}
	})
}

func TestWarmupWorktreeToolchain_NonBlocking(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	done := WarmupWorktreeToolchain(context.Background(), tempDir)
	// It should return immediately without blocking
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("WarmupWorktreeToolchain blocked for %v, expected non-blocking immediate return", elapsed)
	}

	select {
	case <-done:
		// Succeeded
	case <-time.After(5 * time.Second):
		t.Errorf("timed out waiting for background warmup channel")
	}
}
