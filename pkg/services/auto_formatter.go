package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// Formatter executes a configurable code formatting or format-checking command.
// When an LLMClient is injected, it provides self-healing capabilities:
//  1. If formatting fails, it consults the LLM to determine if the command
//     itself is wrong or inapplicable for this project. If so, it requests
//     a new command, updates the in-memory command (ignoring configuration),
//     and re-runs.
//  2. If the command is valid but code/configuration has formatting or syntax
//     errors, it requests the LLM to fix the offending files, writes them back
//     to disk, and re-checks.
type Formatter interface {
	Format(ctx context.Context, projectPath string) (string, error)
	GetCommand() string
	SetCommand(cmd string)
}

// NoopFormatter is a Formatter that does nothing and always succeeds.
type NoopFormatter struct{}

// Format implements Formatter. Always returns empty string and nil.
func (n *NoopFormatter) Format(_ context.Context, _ string) (string, error) {
	return "", nil
}

// GetCommand returns empty string.
func (n *NoopFormatter) GetCommand() string {
	return ""
}

// SetCommand is a no-op on NoopFormatter.
func (n *NoopFormatter) SetCommand(_ string) {}

// CommandFormatter executes a configurable shell command to perform code formatting
// or format checks in the workspace.
type CommandFormatter struct {
	Command   string
	Runner    Sandbox
	LLMClient domain.LLMClient
	mu        sync.RWMutex
}

// NewCommandFormatter returns a CommandFormatter without an LLM client.
func NewCommandFormatter(command string, runner Sandbox) Formatter {
	return NewCommandFormatterWithLLM(command, runner, nil)
}

// NewCommandFormatterWithLLM returns a CommandFormatter with an injected LLM client.
func NewCommandFormatterWithLLM(command string, runner Sandbox, llmClient domain.LLMClient) Formatter {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return &NoopFormatter{}
	}
	return &CommandFormatter{
		Command:   trimmed,
		Runner:    runner,
		LLMClient: llmClient,
	}
}

// GetCommand returns the current in-memory command thread-safely.
func (f *CommandFormatter) GetCommand() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.Command
}

// SetCommand updates the in-memory command thread-safely,
// overriding any static configuration.
func (f *CommandFormatter) SetCommand(cmd string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Command = strings.TrimSpace(cmd)
}

// Format runs the formatter command in projectPath. If execution fails and an
// LLMClient is present, it self-heals by diagnosing the command and repairing
// any broken files.
func (f *CommandFormatter) Format(ctx context.Context, projectPath string) (string, error) {
	cmdTemplate := f.GetCommand()
	if strings.TrimSpace(cmdTemplate) == "" || f.Runner == nil {
		return "", nil
	}

	out, err := f.Runner.RunCommand(ctx, projectPath, cmdTemplate, "")
	if err == nil {
		return out, nil
	}

	// If no LLMClient is provided, return standard error.
	if f.LLMClient == nil {
		return out, fmt.Errorf("formatter command %q failed:\n%s", cmdTemplate, out)
	}

	// Step 1: Analyze with the LLM if the format command itself is wrong or inapplicable.
	diag, diagErr := f.diagnoseCommand(ctx, projectPath, cmdTemplate, out)
	if diagErr == nil && (diag.CommandIsWrong || !diag.AppliesToProject) {
		fmt.Fprintf(os.Stderr, "⚠ [Formatter] Format command %q is inapplicable/wrong for %s: %s. Updating command to %q.\n",
			cmdTemplate, projectPath, diag.Explanation, diag.SuggestedCommand)
		f.SetCommand(diag.SuggestedCommand)

		if !diag.AppliesToProject || f.GetCommand() == "" {
			return out, nil
		}

		// Re-run with the corrected command.
		newOut, newErr := f.Runner.RunCommand(ctx, projectPath, f.GetCommand(), "")
		if newErr == nil {
			return newOut, nil
		}
		out = newOut
	}

	// Step 2: The command is applicable, but failed due to syntax/config errors.
	// Try to fix it via calls to the LLM.
	if repaired, repairErr := f.repairFormatting(ctx, projectPath, f.GetCommand(), out); repairErr == nil && repaired {
		secondOut, secondErr := f.Runner.RunCommand(ctx, projectPath, f.GetCommand(), "")
		if secondErr == nil {
			fmt.Fprintf(os.Stderr, "✨ [Formatter] Successfully auto-repaired formatting/configuration error in %s via LLM.\n", projectPath)
			return secondOut, nil
		}
		out = secondOut
	}

	return out, fmt.Errorf("formatter command %q failed:\n%s", f.GetCommand(), out)
}

type formatCommandDiagnosis struct {
	CommandIsWrong   bool   `json:"command_is_wrong"`
	Explanation      string `json:"explanation"`
	SuggestedCommand string `json:"suggested_command"`
	AppliesToProject bool   `json:"applies_to_project"`
}

func (f *CommandFormatter) diagnoseCommand(ctx context.Context, projectPath, command, errOut string) (*formatCommandDiagnosis, error) {
	prompt := fmt.Sprintf(`You are an expert build and toolchain engineer.
A project code formatter / format checker command failed in Noctifab.
Project directory: %s
Executed command: %s
Command output:
%s

Analyze whether the formatter command itself is wrong, failing because of missing Makefile targets (e.g. 'make: *** No rule to make target "format"'), wrong tool for the project language, wrong flags, missing binaries, or unconfigured options (e.g. rubocop new cops).
Rules:
1. If the command failed because there is no such binary, no rule in Makefile, wrong tool for this project language, or invalid flags, command_is_wrong is true.
2. If this project cannot or should not run this formatter command, command_is_wrong is true, and applies_to_project is false.
3. If the command IS appropriate for this project and only failed due to syntax or formatting errors in files, command_is_wrong is false, and applies_to_project is true.
4. If command_is_wrong is true, provide suggested_command: a working format command for this project, OR empty string "" if formatting should be disabled.

Return a JSON envelope with action tool "diagnose_format_command" and args:
{
  "command_is_wrong": true,
  "explanation": "...",
  "suggested_command": "...",
  "applies_to_project": false
}`, projectPath, command, truncateString(errOut, 3000))

	llmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := f.LLMClient.Complete(llmCtx, prompt)
	if err != nil {
		return nil, err
	}

	for _, act := range resp.Actions {
		if act.Tool == "diagnose_format_command" || act.Tool == "diagnose" {
			data, mErr := json.Marshal(act.Args)
			if mErr == nil {
				var diag formatCommandDiagnosis
				if err := json.Unmarshal(data, &diag); err == nil {
					return &diag, nil
				}
			}
		}
	}

	var diag formatCommandDiagnosis
	cleaned := extractJSONBlock(resp.Reasoning)
	if err := json.Unmarshal([]byte(cleaned), &diag); err == nil {
		return &diag, nil
	}

	return nil, errors.New("failed to parse format diagnosis from LLM response")
}

func (f *CommandFormatter) repairFormatting(ctx context.Context, projectPath, command, errOut string) (bool, error) {
	prompt := fmt.Sprintf(`You are an expert software developer and build systems engineer.
A project code formatter / format checker command failed in Noctifab.
Project directory: %s
Formatter command: %s
Error output:
%s

Analyze the error output. If configuration files (e.g. .rubocop.yml, Makefile, pyproject.toml, etc.) or source files are missing, malformed, or have syntax errors preventing formatting, fix them so the formatter succeeds.
Do not remove real project functionality.
Return a tool action "write_file" with args {"path": "<relative path in project>", "content": "<repaired file content>"} or "write_files" with args {"files": {"<relative path>": "<repaired content>"}}.`,
		projectPath, command, truncateString(errOut, 3000))

	llmCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	resp, err := f.LLMClient.Complete(llmCtx, prompt)
	if err != nil {
		return false, err
	}

	filesWritten := false
	for _, act := range resp.Actions {
		switch act.Tool {
		case "write_file", "repair_file":
			p, _ := act.Args["path"].(string)
			cnt, _ := act.Args["content"].(string)
			if p != "" && cnt != "" {
				if err := writeProjectFile(projectPath, p, cnt); err == nil {
					filesWritten = true
				}
			}
		case "write_files":
			files, _ := act.Args["files"].(map[string]any)
			for p, rawCnt := range files {
				if cnt, ok := rawCnt.(string); ok && p != "" && cnt != "" {
					if err := writeProjectFile(projectPath, p, cnt); err == nil {
						filesWritten = true
					}
				}
			}
		}
	}

	return filesWritten, nil
}

func writeProjectFile(projectPath, relPath, content string) error {
	fullPath := relPath
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(projectPath, relPath)
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	perm := os.FileMode(0644)
	if info, err := os.Stat(fullPath); err == nil {
		perm = info.Mode().Perm()
	}
	return os.WriteFile(fullPath, []byte(content), perm)
}
