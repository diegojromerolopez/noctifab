package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type SandboxMode string

const (
	SandboxModeHost   SandboxMode = "host"
	SandboxModeDocker SandboxMode = "docker"
)

// shellOperators are tokens that, when present in a command string, mean the
// command must be run through `sh -c` rather than split with strings.Fields
// and passed as exec.Command args. Without this, `cargo fmt --check &&
// cargo clippy -- -D warnings` passes `&&` as a literal argument to
// `cargo fmt`, which fails.
var shellOperators = []string{"&&", "||", ";", "|", ">", "<", "$("}

// needsShell reports whether a command string contains shell operators and
// therefore must be dispatched through `sh -c`.
func needsShell(cmd string) bool {
	for _, op := range shellOperators {
		if strings.Contains(cmd, op) {
			return true
		}
	}
	return false
}

type Sandbox interface {
	RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error)
}

// HostSandbox executes processes on the host with whitelisting and PGID jailing.
type HostSandbox struct {
	AllowedCommands []string
	DefaultCommand  string
	IdleTimeout     time.Duration
	DepMgr          *DependencyManager
	evictedMu       sync.RWMutex
	evictedTools    map[string]bool
}

var _ Sandbox = (*HostSandbox)(nil)

func (s *HostSandbox) EvictTool(tool string) {
	if tool == "" {
		return
	}
	s.evictedMu.Lock()
	defer s.evictedMu.Unlock()
	if s.evictedTools == nil {
		s.evictedTools = make(map[string]bool)
	}
	s.evictedTools[tool] = true
	fmt.Printf("⚠️  [Tool Evicted] Tool %q evicted from sandbox. Subsequent invocations will degrade gracefully.\n", tool)
}

func (s *HostSandbox) IsToolEvicted(tool string) bool {
	s.evictedMu.RLock()
	defer s.evictedMu.RUnlock()
	if s.evictedTools == nil {
		return false
	}
	return s.evictedTools[tool]
}

func (s *HostSandbox) GetEvictedTools() []string {
	s.evictedMu.RLock()
	defer s.evictedMu.RUnlock()
	var tools []string
	for t := range s.evictedTools {
		tools = append(tools, t)
	}
	return tools
}

// DetectProjectLanguage inspects the project directory for manifest files
// and returns the detected programming language identifier.
func DetectProjectLanguage(projectPath string) string {
	if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
		return "go"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "Cargo.toml")); err == nil {
		return "rust"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "package.json")); err == nil {
		return "javascript"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "requirements.txt")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "setup.py")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "pom.xml")); err == nil {
		return "java"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "build.gradle")); err == nil {
		return "java"
	}
	return ""
}

func NewHostSandbox(allowed []string, defaultCmd string, idleTimeout time.Duration, depMgr *DependencyManager) *HostSandbox {
	return &HostSandbox{
		AllowedCommands: allowed,
		DefaultCommand:  defaultCmd,
		IdleTimeout:     idleTimeout,
		DepMgr:          depMgr,
		evictedTools:    make(map[string]bool),
	}
}

func (s *HostSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "RunCommand",
		trace.WithAttributes(
			attribute.String("command", command),
			attribute.String("project_path", projectPath),
			attribute.String("package", pkg),
			attribute.Int("allowed_commands", len(s.AllowedCommands)),
		))
	defer span.End()

	cmdStr := command
	if cmdStr == "" {
		cmdStr = s.DefaultCommand
	}
	if cmdStr == "" {
		cmdStr = "go test -v ./..."
	}

	// When the command contains shell operators (&&, ||, ;, |, etc.) we must
	// dispatch through `sh -c` so the operators are interpreted by a shell
	// rather than passed as literal arguments to the first binary. For
	// example, `cargo fmt --check && cargo clippy -- -D warnings` must run
	// as `sh -c '<whole string>'`, not `cargo fmt --check && cargo clippy...`.
	useShell := needsShell(cmdStr)

	var parts []string
	var binary string
	if useShell {
		// Extract the first binary token for whitelist checking.
		trimmed := strings.TrimSpace(cmdStr)
		firstSpace := strings.IndexAny(trimmed, " \t")
		if firstSpace == -1 {
			binary = trimmed
		} else {
			binary = trimmed[:firstSpace]
		}
		parts = []string{binary}
	} else {
		parts = strings.Fields(cmdStr)
		if len(parts) == 0 {
			return "", errors.New("empty command")
		}
		binary = parts[0]
	}

	// Check whitelist
	allowed := false
	for _, a := range s.AllowedCommands {
		if a == "*" || a == binary || a == "sh" {
			allowed = true
			break
		}
	}
	if !allowed {
		fmt.Printf("⚠️  [Sandbox Violation] Command %q binary %q is not in allowed whitelist\n", cmdStr, binary)
		return "", fmt.Errorf("Sandbox violation: command '%s' is not in the whitelist of allowed commands", binary)
	}

	targetDir := projectPath
	if pkg != "" {
		targetDir = filepath.Clean(filepath.Join(projectPath, pkg))
	}

	// Verify targetDir path jail prefix
	cleanProj := filepath.Clean(projectPath)
	if !strings.HasPrefix(targetDir, cleanProj) {
		fmt.Printf("⚠️  [Sandbox Jail Violation] Package target %q is outside workspace prefix %q\n", targetDir, cleanProj)
		return "", fmt.Errorf("Sandbox violation: package target '%s' is outside the workspace prefix", pkg)
	}

	// Auto-detect Cargo.toml in subdirectories for cargo test/check commands
	if binary == "cargo" && pkg == "" {
		if _, err := os.Stat(filepath.Join(targetDir, "Cargo.toml")); os.IsNotExist(err) {
			entries, readErr := os.ReadDir(targetDir)
			if readErr == nil {
				for _, entry := range entries {
					if entry.IsDir() {
						if _, cargoErr := os.Stat(filepath.Join(targetDir, entry.Name(), "Cargo.toml")); cargoErr == nil {
							targetDir = filepath.Join(targetDir, entry.Name())
							break
						}
					}
				}
			}
		}
	}

	// Intercept Python unittest discover for Test Suite Isolation
	if strings.Contains(cmdStr, "unittest discover") {
		return s.runPythonTestsIsolated(ctx, targetDir, cmdStr)
	}

	var cmd *exec.Cmd
	if useShell {
		// `sh -c` runs the entire command string as a shell script, so
		// operators like &&, ||, ;, | work correctly.
		cmd = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	} else if len(parts) > 1 {
		cmd = exec.CommandContext(ctx, binary, parts[1:]...)
	} else {
		cmd = exec.CommandContext(ctx, binary)
	}
	cmd.Dir = targetDir

	if s.IsToolEvicted(binary) {
		fmt.Printf("⚠️  [Sandbox Degraded] Tool %q was evicted. Skipping execution in degraded mode.\n", binary)
		return fmt.Sprintf("Tool %s is evicted on host environment", binary), fmt.Errorf("tool %s is evicted", binary)
	}

	watchdog := Watchdog{IdleTimeout: s.IdleTimeout}
	start := time.Now()
	output, err := watchdog.Run(ctx, cmd)
	if err != nil && s.DepMgr != nil {
		tool, found := s.DepMgr.DetectMissingTool(string(output))
		if !found && strings.Contains(strings.ToLower(string(output)), "not found") {
			tool = binary
			found = true
		}
		if found {
			fmt.Printf("🔍 [Tool Auto-Install] Missing tool %q detected, attempting auto-installation...\n", tool)
			if installErr := s.DepMgr.InstallTool(ctx, tool); installErr == nil {
				fmt.Printf("✅ [Tool Auto-Install Success] Installed %q successfully. Re-running command...\n", tool)
				watchdog2 := Watchdog{IdleTimeout: s.IdleTimeout}
				output2, err2 := watchdog2.Run(ctx, cmd)
				if err2 == nil {
					durMS := time.Since(start).Milliseconds()
					if obs := domain.ObserverFromContext(ctx); obs != nil {
						obs.Observe(ctx, domain.ExecutionEvent{
							Kind:           domain.EventSandboxFinished,
							Name:           cmdStr,
							At:             time.Now().UTC(),
							DurationMillis: &durMS,
							Outcome:        domain.OutcomeSuccess,
						})
					}
					return string(output2), nil
				}
			} else {
				fmt.Printf("❌ [Tool Auto-Install Failure] Failed to install %q: %v\n", tool, installErr)
				s.EvictTool(tool)
			}
		}
	}
	durMS := time.Since(start).Milliseconds()
	if obs := domain.ObserverFromContext(ctx); obs != nil {
		outcome := domain.OutcomeSuccess
		if err != nil {
			outcome = domain.OutcomeFailed
		}
		obs.Observe(ctx, domain.ExecutionEvent{
			Kind:           domain.EventSandboxFinished,
			Name:           cmdStr,
			At:             time.Now().UTC(),
			DurationMillis: &durMS,
			Outcome:        outcome,
		})
	}
	if err != nil {
		return string(output), fmt.Errorf("command execution failed: %w (output: %s)", err, string(output))
	}
	return string(output), nil
}
