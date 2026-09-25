package cli

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// ToolchainFallbackStrategy constants.
const (
	ToolchainStrategyAuto   = "auto"
	ToolchainStrategyDocker = "docker"
	ToolchainStrategyLocal  = "local"
	ToolchainStrategyOff    = "off"
)

// CheckDockerAvailable probes whether the host has a working docker daemon accessible.
func CheckDockerAvailable(ctx context.Context) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(checkCtx, "docker", "info")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// ResolveToolchainStrategy returns the effective toolchain strategy ("docker", "local", or "off").
// When strategy is "auto" (the default), it checks if Docker is functional on the host.
func ResolveToolchainStrategy(ctx context.Context, strategy string, dockerChecker func(context.Context) bool) string {
	strat := strings.ToLower(strings.TrimSpace(strategy))
	if strat == "" {
		strat = ToolchainStrategyAuto
	}

	switch strat {
	case ToolchainStrategyDocker:
		return ToolchainStrategyDocker
	case ToolchainStrategyLocal:
		return ToolchainStrategyLocal
	case ToolchainStrategyOff, "none", "disabled":
		return ToolchainStrategyOff
	case ToolchainStrategyAuto:
		if dockerChecker == nil {
			dockerChecker = CheckDockerAvailable
		}
		if dockerChecker(ctx) {
			return ToolchainStrategyDocker
		}
		return ToolchainStrategyLocal
	default:
		return ToolchainStrategyLocal
	}
}

// DetectMissingToolchainIndicator checks whether error/diagnostic logs indicate a missing host executable.
func DetectMissingToolchainIndicator(failureLog string) bool {
	lower := strings.ToLower(failureLog)
	indicators := []string{
		"executable file not found in $path",
		"not found in $path",
		"command not found",
		"no such file or directory",
		"exit status 127",
		"exit code 127",
		"cannot find",
		"is not recognized as an internal or external command",
	}
	for _, ind := range indicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	return false
}

// BuildToolchainFallbackDirective constructs actionable instructions for Sovereign Rescue
// based on the resolved toolchain strategy.
func BuildToolchainFallbackDirective(resolvedStrategy string, detectedMissing bool) string {
	switch resolvedStrategy {
	case ToolchainStrategyDocker:
		var sb strings.Builder
		sb.WriteString("=== MISSING HOST TOOLCHAIN & CONTAINERIZED DOCKER FALLBACK ===\n")
		if detectedMissing {
			sb.WriteString("ATTENTION: Diagnostics indicate a required compiler, runtime, or test runner binary is MISSING on the host system.\n")
		}
		sb.WriteString("You are authorized and required to create a containerized build/test environment if any host toolchain is missing:\n")
		sb.WriteString("1. Write a minimal Dockerfile at the project root installing the required language runtime, compiler, or dependencies.\n")
		sb.WriteString("2. Update the project Makefile targets (build, test, e2e) to build and execute inside the container:\n")
		sb.WriteString("   IMAGE_NAME = project-env\n")
		sb.WriteString("   .docker-build: Dockerfile\n")
		sb.WriteString("   \tdocker build -t $(IMAGE_NAME) .\n")
		sb.WriteString("   \ttouch .docker-build\n")
		sb.WriteString("   build: .docker-build\n")
		sb.WriteString("   \tdocker run --rm -v $(PWD):/app -w /app $(IMAGE_NAME) <build_command>\n")
		sb.WriteString("   test: .docker-build\n")
		sb.WriteString("   \tdocker run --rm -v $(PWD):/app -w /app $(IMAGE_NAME) <test_command>\n")
		sb.WriteString("3. Ensure output files persist cleanly on the host via the $(PWD):/app volume mount.\n\n")
		return sb.String()

	case ToolchainStrategyLocal:
		var sb strings.Builder
		sb.WriteString("=== MISSING HOST TOOLCHAIN & LOCAL PACKAGE INSTALLATION FALLBACK ===\n")
		if detectedMissing {
			sb.WriteString("ATTENTION: Diagnostics indicate a required compiler, runtime, or test runner binary is MISSING on the host system.\n")
		}
		sb.WriteString("You are authorized to use the install_package tool to install missing compilers, tools, or dependencies onto the local host environment.\n")
		sb.WriteString("Example: {\"tool\": \"install_package\", \"args\": {\"package\": \"<tool-or-lib>\", \"manager\": \"<pip|npm|cargo|etc>\"}}\n\n")
		return sb.String()

	default:
		return ""
	}
}
