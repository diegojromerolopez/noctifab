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
	ToolchainStrategyHost   = "host"
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

// ResolveToolchainStrategy returns the effective toolchain strategy ("docker", "local", "host", or "off").
func ResolveToolchainStrategy(ctx context.Context, strategy string, dockerChecker func(context.Context) bool) string {
	return ResolveToolchainStrategyForSandbox(ctx, strategy, "auto", dockerChecker)
}

// ResolveToolchainStrategyForSandbox returns the effective toolchain strategy taking the configured sandbox mode into account.
func ResolveToolchainStrategyForSandbox(ctx context.Context, strategy, sandboxMode string, dockerChecker func(context.Context) bool) string {
	strat := strings.ToLower(strings.TrimSpace(strategy))
	if strat == "" {
		strat = ToolchainStrategyAuto
	}

	mode := strings.ToLower(strings.TrimSpace(sandboxMode))
	if mode == "host" && (strat == ToolchainStrategyAuto || strat == ToolchainStrategyHost) {
		return ToolchainStrategyHost
	}

	switch strat {
	case ToolchainStrategyDocker:
		return ToolchainStrategyDocker
	case ToolchainStrategyLocal:
		return ToolchainStrategyLocal
	case ToolchainStrategyHost:
		return ToolchainStrategyHost
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
	return BuildToolchainFallbackDirectiveWithDiagnostics(resolvedStrategy, detectedMissing, "")
}

// BuildToolchainFallbackDirectiveWithDiagnostics constructs actionable instructions for Sovereign Rescue
// taking into account prior failure diagnostics (e.g. detecting failed Docker attempts in host mode).
func BuildToolchainFallbackDirectiveWithDiagnostics(resolvedStrategy string, detectedMissing bool, failureLog string) string {
	switch resolvedStrategy {
	case ToolchainStrategyHost:
		var sb strings.Builder
		sb.WriteString("=== HOST SANDBOX MANDATE (NO DOCKER) ===\n")
		sb.WriteString("CRITICAL CONSTRAINT: You are operating in HOST mode without Docker virtualization.\n")
		sb.WriteString("Do NOT invoke 'docker build', 'docker run', or container wrappers in Makefile or scripts.\n")
		sb.WriteString("All build, test, and e2e targets MUST execute directly on the host using native runtimes (e.g. python3 -m compileall, python3 -m unittest, go build, cargo test).\n")
		lowerLog := strings.ToLower(failureLog)
		if strings.Contains(lowerLog, "docker") || strings.Contains(lowerLog, "idle timeout") {
			sb.WriteString("\n⚠️  CRITICAL RECOVERY NOTICE: Diagnostics indicate a prior attempt to run Docker or an idle timeout occurred.\n")
			sb.WriteString("Container operations in this environment are not permitted and will hang. Remove all docker commands from Makefile and use native host toolchains directly.\n")
		}
		sb.WriteString("\n")
		return sb.String()

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
