package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// syntaxCheckTimeout bounds each syntax check invocation so a hung
// interpreter cannot block file write/edit tools indefinitely.
const syntaxCheckTimeout = 10 * time.Second

// SyntaxChecker defines the contract for post-write syntax validation hooks.
// Implementations must be safe for concurrent use from multiple goroutines.
type SyntaxChecker interface {
	// Check runs a syntax validation against the file at the given absolute
	// path. Returning a non-nil error causes the file write operation to be
	// reported as failed to the calling agent tool.
	Check(ctx context.Context, path string) error
}

// NoopSyntaxChecker is a SyntaxChecker that always succeeds without executing
// any external process. It is the default when no syntax_check_command is
// configured, keeping file tools as pure I/O operations.
type NoopSyntaxChecker struct{}

// Check implements SyntaxChecker. Always returns nil.
func (n *NoopSyntaxChecker) Check(_ context.Context, _ string) error {
	return nil
}

// CommandSyntaxChecker executes a configurable shell command template to
// perform language-agnostic syntax validation after every file write or edit.
//
// When an LLMClient is injected, it provides self-healing capabilities:
//  1. If syntax checking fails, it consults the LLM to determine if the command
//     itself is wrong or inapplicable for this file type/project. If so, it requests
//     a new command, updates the in-memory command (ignoring the configuration),
//     and re-verifies.
//  2. If the command is valid but code syntax is broken, it requests the LLM to
//     fix the syntax error, writes the repaired content back to disk, and re-checks.
type CommandSyntaxChecker struct {
	Command   string
	LLMClient domain.LLMClient
	mu        sync.RWMutex
}

// NewCommandSyntaxChecker returns a CommandSyntaxChecker without an LLM client.
// If the command is empty, a NoopSyntaxChecker is returned.
func NewCommandSyntaxChecker(command string) SyntaxChecker {
	return NewCommandSyntaxCheckerWithLLM(command, nil)
}

// NewCommandSyntaxCheckerWithLLM returns a CommandSyntaxChecker with an LLM client.
// If command is empty and llmClient is nil, a NoopSyntaxChecker is returned.
func NewCommandSyntaxCheckerWithLLM(command string, llmClient domain.LLMClient) SyntaxChecker {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" && llmClient == nil {
		return &NoopSyntaxChecker{}
	}
	return &CommandSyntaxChecker{
		Command:   trimmed,
		LLMClient: llmClient,
	}
}

// GetCommand returns the current in-memory command template thread-safely.
func (c *CommandSyntaxChecker) GetCommand() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Command
}

// SetCommand updates the in-memory command template thread-safely,
// overriding any static configuration.
func (c *CommandSyntaxChecker) SetCommand(cmd string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Command = strings.TrimSpace(cmd)
}

// Check implements SyntaxChecker. It executes the configured syntax check command,
// and if execution fails and an LLMClient is present, it self-heals by analyzing
// the command and repairing syntax errors.
func (c *CommandSyntaxChecker) Check(ctx context.Context, path string) error {
	cmdTemplate := c.GetCommand()
	if strings.TrimSpace(cmdTemplate) == "" {
		return nil
	}

	// First execution attempt with current command template.
	out, err := c.runCommand(ctx, cmdTemplate, path)
	if err == nil {
		return nil
	}

	// If no LLMClient is provided, return standard syntax check error.
	if c.LLMClient == nil {
		return fmt.Errorf("syntax check failed for %s:\n%s", path, out)
	}

	// Read current file content for LLM diagnostics and repair.
	contentBytes, readErr := os.ReadFile(path)
	if readErr != nil {
		return fmt.Errorf("syntax check failed for %s:\n%s", path, out)
	}
	content := string(contentBytes)

	// Step 1: Analyze with the LLM if the syntax check command itself is wrong or inapplicable.
	diag, diagErr := c.diagnoseCommand(ctx, path, cmdTemplate, out, content)
	if diagErr == nil && (diag.CommandIsWrong || !diag.AppliesToFile) {
		fmt.Fprintf(os.Stderr, "⚠ [SyntaxChecker] Syntax check command %q is inapplicable/wrong for %s: %s. Updating command to %q.\n",
			cmdTemplate, path, diag.Explanation, diag.SuggestedCommand)
		c.SetCommand(diag.SuggestedCommand)

		if !diag.AppliesToFile || c.GetCommand() == "" {
			// Inapplicable file type (e.g. non-source config or unsupported toolchain). Pass write.
			return nil
		}

		// Re-run with the corrected command.
		newOut, newErr := c.runCommand(ctx, c.GetCommand(), path)
		if newErr == nil {
			return nil
		}
		out = newOut
	}

	// Step 2: The command is applicable, but the file content has a syntax error.
	// Try to fix it via calls to the LLM.
	repairedContent, repairErr := c.repairSyntax(ctx, path, c.GetCommand(), out, content)
	if repairErr == nil && strings.TrimSpace(repairedContent) != "" && repairedContent != content {
		perm := os.FileMode(0644)
		if info, statErr := os.Stat(path); statErr == nil {
			perm = info.Mode().Perm()
		}
		if writeErr := os.WriteFile(path, []byte(repairedContent), perm); writeErr == nil {
			secondOut, secondErr := c.runCommand(ctx, c.GetCommand(), path)
			if secondErr == nil {
				fmt.Fprintf(os.Stderr, "✨ [SyntaxChecker] Successfully auto-repaired syntax error in %s via LLM.\n", path)
				return nil
			}
			out = secondOut
		}
	}

	return fmt.Errorf("syntax check failed for %s:\n%s", path, out)
}

func (c *CommandSyntaxChecker) runCommand(ctx context.Context, cmdTemplate, path string) (string, error) {
	if strings.TrimSpace(cmdTemplate) == "" {
		return "", nil
	}

	cmdStr := strings.ReplaceAll(cmdTemplate, "{file}", path)

	checkCtx, cancel := context.WithTimeout(ctx, syntaxCheckTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if needsShell(cmdStr) {
		cmd = exec.CommandContext(checkCtx, "sh", "-c", cmdStr)
	} else {
		parts := strings.Fields(cmdStr)
		if len(parts) == 0 {
			return "", nil
		}
		if len(parts) == 1 {
			cmd = exec.CommandContext(checkCtx, parts[0])
		} else {
			cmd = exec.CommandContext(checkCtx, parts[0], parts[1:]...)
		}
	}

	out, err := cmd.CombinedOutput()
	return string(out), err
}

type syntaxCommandDiagnosis struct {
	CommandIsWrong   bool   `json:"command_is_wrong"`
	Explanation      string `json:"explanation"`
	SuggestedCommand string `json:"suggested_command"`
	AppliesToFile    bool   `json:"applies_to_file"`
}

func (c *CommandSyntaxChecker) diagnoseCommand(ctx context.Context, path, command, errOut, content string) (*syntaxCommandDiagnosis, error) {
	prompt := fmt.Sprintf(`You are an expert compiler and build toolchain engineer.
A post-write syntax check hook failed for a file in Noctifab.
File path: %s
Executed command: %s
Command output:
%s
File content (first 3000 chars):
%s

Analyze whether the syntax check command itself is wrong or inappropriate for this file or project.
Rules:
1. If the file is NOT a source code file for this command (e.g. .gitignore, markdown, configuration like yaml/toml/json, Makefile, or a different programming language than what the command checks), command_is_wrong is true, and applies_to_file is false.
2. If the command itself has a typo, wrong flags, or wrong binary for this project, command_is_wrong is true.
3. If the command IS valid and appropriate for this file type, command_is_wrong is false, and applies_to_file is true.
4. If command_is_wrong is true, provide suggested_command: a working single-file syntax check command containing "{file}" as placeholder, OR empty string "" if single-file syntax check should be disabled for this project.

Return a JSON envelope with action tool "diagnose_syntax_command" and args:
{
  "command_is_wrong": true,
  "explanation": "...",
  "suggested_command": "...",
  "applies_to_file": false
}`, path, command, errOut, truncateString(content, 3000))

	llmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := c.LLMClient.Complete(llmCtx, prompt)
	if err != nil {
		return nil, err
	}

	for _, act := range resp.Actions {
		if act.Tool == "diagnose_syntax_command" || act.Tool == "diagnose" {
			data, mErr := json.Marshal(act.Args)
			if mErr == nil {
				var diag syntaxCommandDiagnosis
				if err := json.Unmarshal(data, &diag); err == nil {
					return &diag, nil
				}
			}
		}
	}

	var diag syntaxCommandDiagnosis
	cleaned := extractJSONBlock(resp.Reasoning)
	if err := json.Unmarshal([]byte(cleaned), &diag); err == nil {
		return &diag, nil
	}

	return nil, errors.New("failed to parse diagnosis from LLM response")
}

func (c *CommandSyntaxChecker) repairSyntax(ctx context.Context, path, command, errOut, content string) (string, error) {
	prompt := fmt.Sprintf(`You are an expert software developer. A syntax check command verified this file and detected a syntax error.
File path: %s
Syntax checker command: %s
Error output:
%s
Original file content:
%s

Fix all syntax errors in the file so that the syntax checker passes.
Do not remove real functionality or stub out code.
Return a tool action "write_file" with args {"path": "%s", "content": "<complete repaired file content>"} or return only the corrected file content in reasoning.`,
		path, command, errOut, content, path)

	llmCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	resp, err := c.LLMClient.Complete(llmCtx, prompt)
	if err != nil {
		return "", err
	}

	for _, act := range resp.Actions {
		if act.Tool == "write_file" || act.Tool == "write_files" || act.Tool == "repair_file" {
			if cnt, ok := act.Args["content"].(string); ok && len(cnt) > 0 {
				return cnt, nil
			}
		}
	}

	if len(resp.Reasoning) > 0 {
		cleaned := stripCodeFences(resp.Reasoning)
		if len(cleaned) > 0 {
			return cleaned, nil
		}
	}

	return "", errors.New("no repaired content found in LLM response")
}

func extractJSONBlock(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start != -1 && end != -1 && end > start {
		return s[start : end+1]
	}
	return s
}

func stripCodeFences(s string) string {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "```") {
		firstNL := strings.Index(trimmed, "\n")
		lastFence := strings.LastIndex(trimmed, "```")
		if firstNL != -1 && lastFence > firstNL {
			return strings.TrimSpace(trimmed[firstNL+1 : lastFence])
		}
	}
	return trimmed
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... [truncated]"
}
