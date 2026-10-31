package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// runPartialDeliverableE2E executes targeted E2E verification on the partial deliverable
// produced by the current user story, verifying that in-scope commands pass.
func (a *StoryQAAuditor) runPartialDeliverableE2E(ctx context.Context, state *domain.State) (passed bool, log string) {
	if a.runner == nil {
		return true, ""
	}

	partialRunner := NewPartialStateE2ERunner(a.runner)
	var dummyTask domain.Task
	if state.Metadata.FeatureName != "" {
		dummyTask.Title = state.Metadata.FeatureName
	}
	for _, t := range state.Tasks {
		dummyTask.TargetFiles = append(dummyTask.TargetFiles, t.TargetFiles...)
		dummyTask.Title += " " + t.Title
	}

	commands := partialRunner.ExtractImplementedCommands(state, dummyTask)
	e2eCmd := a.detectE2ECommand(state.ProjectPath)
	if e2eCmd == "" {
		// Fallback to workspace test command if no dedicated E2E runner is present
		e2eCmd = a.detectWorkspaceTestCommand(state.ProjectPath)
	}

	// Mandatory Partial Deliverability E2E Gate:
	// If the story implemented commands/features, a test command MUST exist.
	if e2eCmd == "" {
		if len(commands) > 0 {
			fmt.Printf("❌ [Story QA] Mandatory Partial Deliverability E2E failed: no test suite detected for [%s]\n", strings.Join(commands, ", "))
			return false, fmt.Sprintf("Mandatory Partial Deliverability E2E failure: No E2E test suite detected (no 'make e2e', docker-compose, or 'tests/e2e/' harness found in workspace). Each user story must add and maintain runnable black-box E2E tests for its implemented features [%s].", strings.Join(commands, ", "))
		}
		return true, ""
	}

	if !a.isCommandAllowed(e2eCmd) {
		fmt.Printf("⚠️  [Story QA Pre-Flight] Skipping E2E command %q: binary not in sandbox allowed_commands\n", e2eCmd)
		return true, ""
	}

	// Verify that the user story actually added black-box E2E tests for its implemented features
	if ok, missingMsg := partialRunner.VerifyStoryAddedE2ETests(state.ProjectPath, state, commands); !ok {
		fmt.Printf("❌ [Story QA] %s\n", missingMsg)
		return false, missingMsg
	}

	guard := NewContainerTeardownGuard(nil, nil)
	_ = guard.PreFlightClean(ctx, state.ProjectPath)
	defer func() {
		_ = guard.PostRunClean(ctx, state.ProjectPath)
	}()

	execCmd := e2eCmd
	if len(commands) > 0 {
		execCmd = partialRunner.BuildTargetedE2ECommand(e2eCmd, commands)
		fmt.Printf("🔍 [Story QA] Running Mandatory Partial Deliverable E2E verification for command(s) [%s]: %q...\n",
			strings.Join(commands, ", "), execCmd)
	} else {
		fmt.Printf("🔍 [Story QA] Running Mandatory E2E test verification command: %q...\n", execCmd)
	}

	e2eOut, e2eErr := a.runner.RunCommand(ctx, state.ProjectPath, execCmd, "")
	if e2eErr != nil {
		if isSandboxViolation(e2eErr, e2eOut) {
			fmt.Printf("⚠️  [Story QA Pre-Flight] Sandbox policy restriction on E2E command %q (%v); skipping E2E gate.\n", execCmd, e2eErr)
			return true, ""
		}
		fmt.Printf("❌ [Story QA] Mandatory Partial deliverable E2E verification failed: %v\n", e2eErr)
		return false, fmt.Sprintf("Mandatory Partial deliverable E2E test suite failed (%s): %v\n%s", execCmd, e2eErr, capText(e2eOut, 1500))
	}

	noTestsRan, notice := EvaluateE2ETestExecution(e2eOut)
	if noTestsRan {
		if len(commands) > 0 {
			fmt.Printf("❌ [Story QA] Mandatory Partial deliverable E2E suite produced 0 tests for command(s) [%s]: %s\n", strings.Join(commands, ", "), notice)
			return false, fmt.Sprintf("Mandatory Partial deliverable E2E suite produced 0 tests for command(s) [%s]: %s", strings.Join(commands, ", "), notice)
		}
		fmt.Printf("❌ [Story QA] Mandatory Partial deliverable E2E suite produced 0 tests: %s\n", notice)
		return false, fmt.Sprintf("Mandatory Partial deliverable E2E suite produced 0 tests (%s): %s", execCmd, notice)
	}

	fmt.Printf("✅ [Story QA] Mandatory Partial deliverable E2E verification passed successfully.\n")
	return true, ""
}

func (a *StoryQAAuditor) detectE2ECommand(projectPath string) string {
	return DetectE2ECommand(projectPath, a.e2eMode, a.e2eCmd)
}

func (a *StoryQAAuditor) isCommandAllowed(cmdStr string) bool {
	allowed := a.allowedCommands
	if len(allowed) == 0 {
		if hs, ok := a.runner.(*HostSandbox); ok && len(hs.AllowedCommands) > 0 {
			allowed = hs.AllowedCommands
		}
	}
	if len(allowed) == 0 {
		return true
	}
	trimmed := strings.TrimSpace(cmdStr)
	if trimmed == "" {
		return true
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return true
	}
	binary := parts[0]
	for _, auth := range allowed {
		if auth == "*" || auth == binary || auth == "sh" || auth == "bash" {
			return true
		}
	}
	return false
}

func isSandboxViolation(err error, out string) bool {
	if err == nil {
		return false
	}
	combined := err.Error() + " " + out
	return strings.Contains(combined, "Sandbox violation") ||
		strings.Contains(combined, "whitelist of allowed commands") ||
		strings.Contains(combined, "Role authorization violation")
}

func (a *StoryQAAuditor) detectWorkspaceTestCommand(projectPath string) string {
	if a.defaultTestCmd != "" && a.isCommandAllowed(a.defaultTestCmd) {
		return a.defaultTestCmd
	}
	if a.isCommandAllowed("make") {
		if _, err := os.Stat(filepath.Join(projectPath, "Makefile")); err == nil {
			if content, rErr := os.ReadFile(filepath.Join(projectPath, "Makefile")); rErr == nil {
				c := string(content)
				if strings.Contains(c, "test:") || strings.Contains(c, "test-all:") || strings.Contains(c, "check:") {
					return "make test"
				}
			}
		}
	}
	if a.isCommandAllowed("go") {
		if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
			return "go test ./..."
		}
	}
	if a.isCommandAllowed("pytest") || a.isCommandAllowed("python") || a.isCommandAllowed("python3") {
		if _, err := os.Stat(filepath.Join(projectPath, "pyproject.toml")); err == nil {
			return "pytest -v"
		}
	}
	if a.isCommandAllowed("cargo") {
		if _, err := os.Stat(filepath.Join(projectPath, "Cargo.toml")); err == nil {
			return "cargo test"
		}
	}
	if a.isCommandAllowed("npm") {
		if _, err := os.Stat(filepath.Join(projectPath, "package.json")); err == nil {
			return "npm test"
		}
	}
	if a.isCommandAllowed("dune") {
		if _, err := os.Stat(filepath.Join(projectPath, "dune-project")); err == nil {
			return "dune runtest"
		}
	}
	return ""
}
