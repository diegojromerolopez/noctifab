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

	t.Run("when command is wrong, LLM diagnoses error, replaces command in-memory, and ignores config", func(t *testing.T) {
		dir := t.TempDir()
		sourceFile := filepath.Join(dir, "app.c")
		if err := os.WriteFile(sourceFile, []byte("int main() { return 0; }\n"), 0644); err != nil {
			t.Fatal(err)
		}

		mockLLM := &mockSyntaxLLMClient{
			completeFunc: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				return &domain.LLMResponse{
					Actions: []domain.LLMAction{
						{
							Tool: "diagnose_syntax_command",
							Args: map[string]any{
								"command_is_wrong":  true,
								"explanation":       "gofmt cannot check C source code",
								"suggested_command": "echo {file}",
								"applies_to_file":   true,
							},
						},
					},
				}, nil
			},
		}

		checker := &CommandSyntaxChecker{
			Command:   "false", // Initially broken command
			LLMClient: mockLLM,
		}

		if err := checker.Check(context.Background(), sourceFile); err != nil {
			t.Fatalf("expected check to pass after command replacement, got error: %v", err)
		}

		if checker.GetCommand() != "echo {file}" {
			t.Errorf("expected command to be updated in-memory to 'echo {file}', got: %q", checker.GetCommand())
		}
	})

	t.Run("when file is not applicable (e.g. .gitignore), LLM marks applies_to_file=false and check passes", func(t *testing.T) {
		dir := t.TempDir()
		gitignore := filepath.Join(dir, ".gitignore")
		if err := os.WriteFile(gitignore, []byte("node_modules/\n"), 0644); err != nil {
			t.Fatal(err)
		}

		mockLLM := &mockSyntaxLLMClient{
			completeFunc: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				return &domain.LLMResponse{
					Actions: []domain.LLMAction{
						{
							Tool: "diagnose_syntax_command",
							Args: map[string]any{
								"command_is_wrong":  true,
								"explanation":       ".gitignore is not a Python source file",
								"suggested_command": "python3 -m py_compile {file}",
								"applies_to_file":   false,
							},
						},
					},
				}, nil
			},
		}

		checker := &CommandSyntaxChecker{
			Command:   "false",
			LLMClient: mockLLM,
		}

		if err := checker.Check(context.Background(), gitignore); err != nil {
			t.Fatalf("expected check to pass for inapplicable file, got error: %v", err)
		}
	})

	t.Run("when command is valid and file has syntax error, LLM repairs syntax and passes", func(t *testing.T) {
		dir := t.TempDir()
		script := filepath.Join(dir, "script.sh")
		// Initially invalid file that triggers non-zero exit from validator command
		if err := os.WriteFile(script, []byte("syntax_error\n"), 0644); err != nil {
			t.Fatal(err)
		}

		// A validator command that fails if file does not contain "VALID", but succeeds if "VALID" is present
		validatorCmd := "grep -q VALID {file}"

		mockLLM := &mockSyntaxLLMClient{
			completeFunc: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				// Step 1: Diagnose returns command is valid
				if strings.Contains(prompt, "diagnose_syntax_command") {
					return &domain.LLMResponse{
						Actions: []domain.LLMAction{
							{
								Tool: "diagnose_syntax_command",
								Args: map[string]any{
									"command_is_wrong":  false,
									"explanation":       "Command is valid, file has syntax error",
									"suggested_command": "",
									"applies_to_file":   true,
								},
							},
						},
					}, nil
				}
				// Step 2: Repair returns fixed content
				return &domain.LLMResponse{
					Actions: []domain.LLMAction{
						{
							Tool: "write_file",
							Args: map[string]any{
								"path":    script,
								"content": "VALID\n",
							},
						},
					},
				}, nil
			},
		}

		checker := &CommandSyntaxChecker{
			Command:   validatorCmd,
			LLMClient: mockLLM,
		}

		if err := checker.Check(context.Background(), script); err != nil {
			t.Fatalf("expected check to pass after syntax repair, got error: %v", err)
		}

		repairedOnDisk, err := os.ReadFile(script)
		if err != nil {
			t.Fatal(err)
		}
		if string(repairedOnDisk) != "VALID\n" {
			t.Errorf("expected file on disk to be updated to repaired content, got %q", string(repairedOnDisk))
		}
	})

	t.Run("when LLM call fails, falls back gracefully to standard syntax check error", func(t *testing.T) {
		dir := t.TempDir()
		badFile := filepath.Join(dir, "bad.txt")
		if err := os.WriteFile(badFile, []byte("bad\n"), 0644); err != nil {
			t.Fatal(err)
		}

		mockLLM := &mockSyntaxLLMClient{
			completeFunc: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				return nil, errors.New("network timeout")
			},
		}

		checker := &CommandSyntaxChecker{
			Command:   "false",
			LLMClient: mockLLM,
		}

		err := checker.Check(context.Background(), badFile)
		if err == nil {
			t.Fatal("expected error when command fails and LLM fails, got nil")
		}
		if !strings.Contains(err.Error(), "syntax check failed") {
			t.Errorf("expected standard syntax check failed error message, got: %v", err)
		}
	})
}

type mockSyntaxLLMClient struct {
	completeFunc func(ctx context.Context, prompt string) (*domain.LLMResponse, error)
}

func (m *mockSyntaxLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, prompt)
	}
	return &domain.LLMResponse{}, nil
}
