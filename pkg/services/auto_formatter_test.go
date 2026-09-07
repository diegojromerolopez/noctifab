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

func TestNoopFormatter(t *testing.T) {
	formatter := &NoopFormatter{}
	out, err := formatter.Format(context.Background(), "/tmp")
	if err != nil || out != "" {
		t.Fatalf("expected empty string and nil error, got out=%q, err=%v", out, err)
	}
	if formatter.GetCommand() != "" {
		t.Errorf("expected empty command, got %q", formatter.GetCommand())
	}
	formatter.SetCommand("some-cmd")
	if formatter.GetCommand() != "" {
		t.Errorf("expected command to remain empty on NoopFormatter, got %q", formatter.GetCommand())
	}
}

func TestCommandFormatter_GetSetCommand(t *testing.T) {
	formatter := &CommandFormatter{Command: "cargo fmt"}
	if formatter.GetCommand() != "cargo fmt" {
		t.Errorf("expected 'cargo fmt', got %q", formatter.GetCommand())
	}
	formatter.SetCommand("ruff format .")
	if formatter.GetCommand() != "ruff format ." {
		t.Errorf("expected 'ruff format .', got %q", formatter.GetCommand())
	}
}

func TestCommandFormatter_Format(t *testing.T) {
	t.Run("when command is empty, returns empty string and nil", func(t *testing.T) {
		formatter := NewCommandFormatter("", nil)
		out, err := formatter.Format(context.Background(), "/tmp")
		if err != nil || out != "" {
			t.Fatalf("expected nil error and empty output, got out=%q, err=%v", out, err)
		}
	})

	t.Run("when command succeeds, returns output and nil", func(t *testing.T) {
		mockRunner := &mockFormatterSandboxRunner{
			runFunc: func(ctx context.Context, projectPath, command, pkg string) (string, error) {
				return "Formatted 3 files", nil
			},
		}
		formatter := NewCommandFormatter("echo format", mockRunner)
		out, err := formatter.Format(context.Background(), "/tmp")
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
		if out != "Formatted 3 files" {
			t.Errorf("expected 'Formatted 3 files', got %q", out)
		}
	})

	t.Run("when command fails and no LLM client, returns output and error", func(t *testing.T) {
		mockRunner := &mockFormatterSandboxRunner{
			runFunc: func(ctx context.Context, projectPath, command, pkg string) (string, error) {
				return "make: *** No rule to make target 'format'. Stop.", errors.New("exit status 2")
			},
		}
		formatter := NewCommandFormatter("make format", mockRunner)
		out, err := formatter.Format(context.Background(), "/tmp")
		if err == nil {
			t.Fatal("expected error when command fails without LLM, got nil")
		}
		if !strings.Contains(err.Error(), "formatter command") {
			t.Errorf("expected error to mention formatter command, got: %v", err)
		}
		if !strings.Contains(out, "No rule to make target 'format'") {
			t.Errorf("expected output to contain error message, got: %q", out)
		}
	})

	t.Run("when command is wrong, LLM diagnoses error, replaces command in-memory, and ignores config", func(t *testing.T) {
		executedCommands := []string{}
		mockRunner := &mockFormatterSandboxRunner{
			runFunc: func(ctx context.Context, projectPath, command, pkg string) (string, error) {
				executedCommands = append(executedCommands, command)
				if command == "make format" {
					return "make: *** No rule to make target 'format'. Stop.", errors.New("exit status 2")
				}
				if command == "npm run format" {
					return "All matched files are formatted!", nil
				}
				return "", errors.New("unknown command")
			},
		}

		mockLLM := &mockFormatLLMClient{
			completeFunc: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				return &domain.LLMResponse{
					Actions: []domain.LLMAction{
						{
							Tool: "diagnose_format_command",
							Args: map[string]any{
								"command_is_wrong":   true,
								"explanation":        "Makefile does not contain a format target; use npm run format instead",
								"suggested_command":  "npm run format",
								"applies_to_project": true,
							},
						},
					},
				}, nil
			},
		}

		formatter := NewCommandFormatterWithLLM("make format", mockRunner, mockLLM)
		out, err := formatter.Format(context.Background(), "/tmp/project")
		if err != nil {
			t.Fatalf("expected format to succeed after command replacement, got: %v", err)
		}
		if out != "All matched files are formatted!" {
			t.Errorf("expected output from replacement command, got: %q", out)
		}
		if formatter.GetCommand() != "npm run format" {
			t.Errorf("expected in-memory command to be updated to 'npm run format', got: %q", formatter.GetCommand())
		}
		if len(executedCommands) != 2 || executedCommands[0] != "make format" || executedCommands[1] != "npm run format" {
			t.Errorf("expected commands [make format, npm run format], got %v", executedCommands)
		}
	})

	t.Run("when project cannot run formatter, LLM sets applies_to_project=false and check passes", func(t *testing.T) {
		mockRunner := &mockFormatterSandboxRunner{
			runFunc: func(ctx context.Context, projectPath, command, pkg string) (string, error) {
				return "rubocop: command not found", errors.New("exit status 127")
			},
		}

		mockLLM := &mockFormatLLMClient{
			completeFunc: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				return &domain.LLMResponse{
					Actions: []domain.LLMAction{
						{
							Tool: "diagnose_format_command",
							Args: map[string]any{
								"command_is_wrong":   true,
								"explanation":        "RuboCop is not installed in this environment; formatting should be disabled",
								"suggested_command":  "",
								"applies_to_project": false,
							},
						},
					},
				}, nil
			},
		}

		formatter := NewCommandFormatterWithLLM("rubocop -A", mockRunner, mockLLM)
		_, err := formatter.Format(context.Background(), "/tmp/project")
		if err != nil {
			t.Fatalf("expected format to pass for disabled formatter, got error: %v", err)
		}
		if formatter.GetCommand() != "" {
			t.Errorf("expected in-memory command to be disabled (empty string), got: %q", formatter.GetCommand())
		}
	})

	t.Run("when command is valid and files have formatting/config error, LLM repairs files and re-run passes", func(t *testing.T) {
		dir := t.TempDir()
		configFile := filepath.Join(dir, ".rubocop.yml")
		if err := os.WriteFile(configFile, []byte("# empty\n"), 0644); err != nil {
			t.Fatal(err)
		}

		firstAttempt := true
		mockRunner := &mockFormatterSandboxRunner{
			runFunc: func(ctx context.Context, projectPath, command, pkg string) (string, error) {
				if firstAttempt {
					firstAttempt = false
					return "The following cops were added to RuboCop, but are not configured", errors.New("exit status 1")
				}
				// Verify file was repaired on disk before second run
				content, err := os.ReadFile(configFile)
				if err != nil || !strings.Contains(string(content), "NewCops: enable") {
					return "", errors.New("file not repaired on disk")
				}
				return "0 files inspected, no offenses detected", nil
			},
		}

		mockLLM := &mockFormatLLMClient{
			completeFunc: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				if strings.Contains(prompt, "diagnose_format_command") {
					return &domain.LLMResponse{
						Actions: []domain.LLMAction{
							{
								Tool: "diagnose_format_command",
								Args: map[string]any{
									"command_is_wrong":   false,
									"explanation":        "RuboCop command is valid, but .rubocop.yml is missing NewCops configuration",
									"suggested_command":  "",
									"applies_to_project": true,
								},
							},
						},
					}, nil
				}
				// Repair step: write valid .rubocop.yml
				return &domain.LLMResponse{
					Actions: []domain.LLMAction{
						{
							Tool: "write_file",
							Args: map[string]any{
								"path":    ".rubocop.yml",
								"content": "AllCops:\n  NewCops: enable\n",
							},
						},
					},
				}, nil
			},
		}

		formatter := NewCommandFormatterWithLLM("rubocop -A", mockRunner, mockLLM)
		out, err := formatter.Format(context.Background(), dir)
		if err != nil {
			t.Fatalf("expected format to succeed after auto-repair, got error: %v", err)
		}
		if !strings.Contains(out, "0 files inspected") {
			t.Errorf("expected success output after repair, got: %q", out)
		}

		repaired, err := os.ReadFile(configFile)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(repaired), "NewCops: enable") {
			t.Errorf("expected .rubocop.yml on disk to contain 'NewCops: enable', got: %s", string(repaired))
		}
	})

	t.Run("when LLM call fails, falls back gracefully to standard formatter error", func(t *testing.T) {
		mockRunner := &mockFormatterSandboxRunner{
			runFunc: func(ctx context.Context, projectPath, command, pkg string) (string, error) {
				return "formatter broken", errors.New("exit status 1")
			},
		}

		mockLLM := &mockFormatLLMClient{
			completeFunc: func(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
				return nil, errors.New("network failure")
			},
		}

		formatter := NewCommandFormatterWithLLM("format-cmd", mockRunner, mockLLM)
		_, err := formatter.Format(context.Background(), "/tmp")
		if err == nil {
			t.Fatal("expected error when formatter fails and LLM fails, got nil")
		}
		if !strings.Contains(err.Error(), "formatter command") {
			t.Errorf("expected standard formatter error, got: %v", err)
		}
	})
}

type mockFormatLLMClient struct {
	completeFunc func(ctx context.Context, prompt string) (*domain.LLMResponse, error)
}

func (m *mockFormatLLMClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, prompt)
	}
	return &domain.LLMResponse{}, nil
}

type mockFormatterSandboxRunner struct {
	runFunc func(ctx context.Context, projectPath, command, pkg string) (string, error)
}

func (m *mockFormatterSandboxRunner) RunCommand(ctx context.Context, projectPath, command, pkg string) (string, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, projectPath, command, pkg)
	}
	return "", nil
}
