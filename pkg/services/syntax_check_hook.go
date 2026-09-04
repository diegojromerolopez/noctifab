package services

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
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
// The Command field must contain the shell command with the literal string
// "{file}" as a placeholder that is replaced with the absolute path of the
// written file before execution. Example:
//
//	"python3 -m py_compile {file}"
//	"ruby -c {file}"
//
// If Command is empty the checker behaves as a no-op, preserving language
// agnosticism when no syntax check is configured for the target project.
type CommandSyntaxChecker struct {
	// Command is the shell command template (may contain shell operators like
	// &&, ||, ;). Use "{file}" as the placeholder for the file path.
	Command string
}

// NewCommandSyntaxChecker returns a CommandSyntaxChecker for the given command
// template, or a NoopSyntaxChecker when the command is empty.
func NewCommandSyntaxChecker(command string) SyntaxChecker {
	if strings.TrimSpace(command) == "" {
		return &NoopSyntaxChecker{}
	}
	return &CommandSyntaxChecker{Command: command}
}

// Check implements SyntaxChecker. It substitutes "{file}" in the configured
// command template with the provided path and executes the resulting command
// within a bounded timeout derived from the parent context.
func (c *CommandSyntaxChecker) Check(ctx context.Context, path string) error {
	if strings.TrimSpace(c.Command) == "" {
		return nil
	}

	// Substitute {file} placeholder with the actual file path.
	cmdStr := strings.ReplaceAll(c.Command, "{file}", path)

	// Bound execution time: min(parent deadline, syntaxCheckTimeout).
	checkCtx, cancel := context.WithTimeout(ctx, syntaxCheckTimeout)
	defer cancel()

	var cmd *exec.Cmd
	if needsShell(cmdStr) {
		cmd = exec.CommandContext(checkCtx, "sh", "-c", cmdStr)
	} else {
		parts := strings.Fields(cmdStr)
		if len(parts) == 0 {
			return nil
		}
		if len(parts) == 1 {
			cmd = exec.CommandContext(checkCtx, parts[0])
		} else {
			cmd = exec.CommandContext(checkCtx, parts[0], parts[1:]...)
		}
	}

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("syntax check failed for %s:\n%s", path, string(out))
	}
	return nil
}
