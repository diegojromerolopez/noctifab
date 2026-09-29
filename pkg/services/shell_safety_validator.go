package services

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var (
	forbiddenInteractiveRE = regexp.MustCompile(`(?i)\b(vim|vi|nano|emacs|less|more|man|top|htop)\b`)
	errorMaskingRE         = regexp.MustCompile(`(?i)(\|\|\s*true\b|\|\|\s*exit\s+0\b|^\s*set\s+\+[a-z]*e\b)`)
	destructivePathRE      = regexp.MustCompile(`(?i)\brm\s+-[a-z]*r[a-z]*f?[a-z]*\s+(?:/|\*|\.git|\.noctifab)(?:[\s/]|$|\b)`)
	cdCommandRE            = regexp.MustCompile(`(?i)(?:^|[\;&\|\n])\s*(?:cd|pushd|popd)\b`)
)

// BinaryLookupFunc abstracts os/exec.LookPath for dependency injection and mocking.
type BinaryLookupFunc func(file string) (string, error)

// ShellSafetyValidator inspects shell command lines deterministically to prevent directory escapes,
// interactive hangs, crash signal suppression, and destructive commands.
type ShellSafetyValidator struct {
	lookPath BinaryLookupFunc
}

// NewShellSafetyValidator initializes a new validator.
func NewShellSafetyValidator(lookup ...BinaryLookupFunc) *ShellSafetyValidator {
	fn := exec.LookPath
	if len(lookup) > 0 && lookup[0] != nil {
		fn = lookup[0]
	}
	return &ShellSafetyValidator{lookPath: fn}
}

// ValidateShellCommand inspects the command string and returns an error if any safety invariant is violated.
func (v *ShellSafetyValidator) ValidateShellCommand(cmdLine string) error {
	trimmed := strings.TrimSpace(cmdLine)
	if trimmed == "" {
		return fmt.Errorf("empty shell command")
	}

	// 1. Forbid directory mutations
	if cdCommandRE.MatchString(trimmed) {
		return fmt.Errorf("command execution rejected: 'cd', 'pushd', or 'popd' are forbidden (orchestrator manages working directory)")
	}

	// 2. Forbid interactive commands that cause infinite timeouts
	if matches := forbiddenInteractiveRE.FindString(trimmed); matches != "" {
		return fmt.Errorf("command execution rejected: interactive utility %q would block headless execution", matches)
	}

	// 3. Forbid error masking
	if errorMaskingRE.MatchString(trimmed) {
		return fmt.Errorf("command execution rejected: error masking ('|| true', '|| exit 0', 'set +e') is forbidden by quality gates")
	}

	// 4. Forbid destructive workspace deletions
	if destructivePathRE.MatchString(trimmed) {
		return fmt.Errorf("command execution rejected: destructive deletion targeting root, wildcards, or VCS directories is forbidden")
	}

	// 5. Verify the primary executable binary exists
	primaryBinary := extractPrimaryBinary(trimmed)
	if primaryBinary != "" && !isShellBuiltin(primaryBinary) {
		if _, err := v.lookPath(primaryBinary); err != nil {
			return fmt.Errorf("binary %q not found on host PATH: %w", primaryBinary, err)
		}
	}

	return nil
}

func extractPrimaryBinary(cmdLine string) string {
	// Strip environment variable assignments (e.g., CI=1 GOOS=linux cmd ...)
	tokens := strings.Fields(cmdLine)
	for _, tok := range tokens {
		if strings.Contains(tok, "=") && !strings.HasPrefix(tok, "-") {
			continue
		}
		// Return first actual command token
		clean := strings.Trim(tok, `"'()[]{};&|`)
		return clean
	}
	return ""
}

func isShellBuiltin(cmd string) bool {
	builtins := map[string]bool{
		"echo": true, "export": true, "test": true, "[": true, "[[": true,
		"true": true, "false": true, "source": true, ".": true, "return": true,
		"exit": true, "set": true, "unset": true, "shift": true, "trap": true,
		"printf": true, "type": true, "read": true,
	}
	return builtins[strings.ToLower(cmd)]
}
