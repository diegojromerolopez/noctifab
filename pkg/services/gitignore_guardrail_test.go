package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitignoreGuardrail(t *testing.T) {
	t.Run("when .gitignore is missing, it creates default gitignore with critical rules", func(t *testing.T) {
		tempDir := t.TempDir()

		err := EnsureProjectGitignore(tempDir)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		gitignorePath := filepath.Join(tempDir, ".gitignore")
		data, err := os.ReadFile(gitignorePath)
		if err != nil {
			t.Fatalf("failed to read created .gitignore: %v", err)
		}

		content := string(data)
		for _, rule := range CriticalGitignoreRules {
			if !strings.Contains(content, rule) {
				t.Errorf("expected %q in .gitignore, but was missing", rule)
			}
		}
	})

	t.Run("when .gitignore exists with partial rules, it appends missing critical rules non-destructively", func(t *testing.T) {
		tempDir := t.TempDir()
		gitignorePath := filepath.Join(tempDir, ".gitignore")

		initial := "# Custom existing rules\nmy_custom_dir/\n*.secret\ntarget/\n"
		if err := os.WriteFile(gitignorePath, []byte(initial), 0o644); err != nil {
			t.Fatal(err)
		}

		err := EnsureProjectGitignore(tempDir)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		data, err := os.ReadFile(gitignorePath)
		if err != nil {
			t.Fatalf("failed to read updated .gitignore: %v", err)
		}

		content := string(data)
		if !strings.HasPrefix(content, initial) {
			t.Errorf("expected initial content to be preserved at the beginning")
		}
		if !strings.Contains(content, "node_modules/") {
			t.Errorf("expected node_modules/ to be appended")
		}
		if !strings.Contains(content, ".noctifab/") {
			t.Errorf("expected .noctifab/ to be appended")
		}
		// Count target/ occurrences - should only be once
		if strings.Count(content, "target/") != 1 {
			t.Errorf("expected target/ to not be duplicated, count: %d", strings.Count(content, "target/"))
		}
	})

	t.Run("when .gitignore already has all critical rules, it leaves the file unchanged", func(t *testing.T) {
		tempDir := t.TempDir()
		gitignorePath := filepath.Join(tempDir, ".gitignore")

		fullContent := DefaultGitignoreContent()
		if err := os.WriteFile(gitignorePath, []byte(fullContent), 0o644); err != nil {
			t.Fatal(err)
		}

		err := EnsureProjectGitignore(tempDir)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		data, err := os.ReadFile(gitignorePath)
		if err != nil {
			t.Fatalf("failed to read .gitignore: %v", err)
		}

		if string(data) != fullContent {
			t.Errorf("expected .gitignore to remain completely unchanged")
		}
	})
}
