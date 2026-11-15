package services

import (
	"os/exec"
	"path/filepath"
	"strings"
)

func isCommandToolMissing(runner Sandbox, cmd, output string) bool {
	if _, isHost := runner.(*HostSandbox); isHost {
		bin := extractCommandBinary(cmd)
		if bin != "" {
			if _, err := exec.LookPath(bin); err == nil {
				return false // Tool binary is verified installed on host PATH.
			}
			return true // Tool binary is not found on host PATH.
		}
	}
	return isMissingToolOutput(output)
}

func extractCommandBinary(cmd string) string {
	fields := strings.Fields(cmd)
	for _, f := range fields {
		if strings.Contains(f, "=") {
			continue
		}
		base := filepath.Base(f)
		if base != "sudo" && base != "env" && base != "sh" && base != "bash" {
			return base
		}
	}
	if len(fields) > 0 {
		return filepath.Base(fields[0])
	}
	return ""
}

func isMissingToolOutput(output string) bool {
	lower := strings.ToLower(output)

	// If the failure occurred during test assertions or code panic, it is not a missing host tool.
	if strings.Contains(lower, "assertionerror") ||
		strings.Contains(lower, "failed tests") ||
		strings.Contains(lower, "--- fail") ||
		strings.Contains(lower, "panic:") {
		return false
	}

	// If the failure occurred during a container build or containerized runtime execution
	// (e.g., Dockerfile build step, BuildKit solve error, compose service exit, or missing file
	// inside the container build context), Docker itself was present and executed.
	// Container-internal build/script errors must NOT be classified as a missing host tool.
	if strings.Contains(lower, "failed to solve") ||
		strings.Contains(lower, "load build definition") ||
		strings.Contains(lower, "transferring dockerfile") ||
		strings.Contains(lower, "transferring context") ||
		strings.Contains(lower, "executor failed running") ||
		strings.Contains(lower, "dockerfile:") ||
		strings.Contains(lower, "dockerfile.e2e:") {
		return false
	}

	return strings.Contains(lower, "command not found") ||
		strings.Contains(lower, "executable file not found") ||
		strings.Contains(lower, "not found in $path") ||
		strings.Contains(lower, "not found in %path%") ||
		strings.Contains(lower, "is not recognized as an internal or external command") ||
		strings.Contains(lower, "is evicted") ||
		strings.Contains(lower, "exit status 127") ||
		(strings.Contains(lower, "fork/exec") && strings.Contains(lower, "no such file or directory"))
}
