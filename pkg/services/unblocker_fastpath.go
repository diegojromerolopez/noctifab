package services

import (
	"regexp"
)

// FastPathResult captures a static regex classifier hit for immediate stall unblocking.
type FastPathResult struct {
	Matched   bool
	Reason    string
	Directive string
}

var (
	stdinPromptRegex = regexp.MustCompile(`(?i)(\?.*do you want to|overwrite\?\s*\[y/n\]|proceed\?\s*\(y/n\)|interactive\s*input)`)
	portBindRegex    = regexp.MustCompile(`(?i)(bind:\s*address already in use|EADDRINUSE|port\s+\d+\s+is already in use)`)
	watchModeRegex   = regexp.MustCompile(`(?i)(watch usage:\s*press|press\s+f\s+to\t|watch mode enabled)`)
	missingToolRegex = regexp.MustCompile(`(?i)(pytest:\s*(?:command\s*)?not found|command not found:\s*pytest|exec:\s*"pytest":\s*executable file not found|sh: 1:\s*pytest:\s*not found|exit status 127)`)
)

// FastPathClassify inspects log output for known CLI stall patterns to provide 0-token unblocking.
func FastPathClassify(logSnippet string) *FastPathResult {
	if logSnippet == "" {
		return &FastPathResult{Matched: false}
	}

	if stdinPromptRegex.MatchString(logSnippet) {
		return &FastPathResult{
			Matched:   true,
			Reason:    "interactive_stdin_prompt_wait",
			Directive: "Previous execution froze waiting for interactive stdin input. Re-run all commands non-interactively (pass -y, --non-interactive, or CI=true).",
		}
	}

	if portBindRegex.MatchString(logSnippet) {
		return &FastPathResult{
			Matched:   true,
			Reason:    "port_binding_collision",
			Directive: "Previous execution stalled due to port binding collision (address already in use). Use dynamic port allocation or terminate stale background server processes.",
		}
	}

	if watchModeRegex.MatchString(logSnippet) {
		return &FastPathResult{
			Matched:   true,
			Reason:    "interactive_watch_mode",
			Directive: "Previous execution entered interactive watch mode. Run tests non-interactively (pass --watchAll=false or --ci).",
		}
	}

	if missingToolRegex.MatchString(logSnippet) {
		return &FastPathResult{
			Matched:   true,
			Reason:    "missing_toolchain_binary",
			Directive: "Previous execution failed because the test runner or toolchain binary is missing on the host. Use standard library or alternative available test runners (e.g. 'python3 -m unittest discover').",
		}
	}

	return &FastPathResult{Matched: false}
}
