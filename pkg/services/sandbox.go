package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type SandboxMode string

const (
	SandboxModeHost   SandboxMode = "host"
	SandboxModeDocker SandboxMode = "docker"
)

type Sandbox interface {
	RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error)
}

// HostSandbox executes processes on the host with whitelisting and PGID jailing.
type HostSandbox struct {
	AllowedCommands []string
	DefaultCommand  string
	IdleTimeout     time.Duration
	DepMgr          *DependencyManager
}

var _ Sandbox = (*HostSandbox)(nil)

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

	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return "", errors.New("empty command")
	}
	binary := parts[0]

	// Check whitelist
	allowed := false
	for _, a := range s.AllowedCommands {
		if a == "*" || a == binary {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", fmt.Errorf("Sandbox violation: command '%s' is not in the whitelist of allowed commands", binary)
	}

	targetDir := projectPath
	if pkg != "" {
		targetDir = filepath.Clean(filepath.Join(projectPath, pkg))
	}

	// Verify targetDir path jail prefix
	cleanProj := filepath.Clean(projectPath)
	if !strings.HasPrefix(targetDir, cleanProj) {
		return "", fmt.Errorf("Sandbox violation: package target '%s' is outside the workspace prefix", pkg)
	}

	// Intercept Python unittest discover for Test Suite Isolation
	if strings.Contains(cmdStr, "unittest discover") {
		return s.runPythonTestsIsolated(ctx, targetDir, cmdStr)
	}

	var cmd *exec.Cmd
	if len(parts) > 1 {
		cmd = exec.CommandContext(ctx, binary, parts[1:]...)
	} else {
		cmd = exec.CommandContext(ctx, binary)
	}
	cmd.Dir = targetDir

	watchdog := Watchdog{IdleTimeout: s.IdleTimeout}
	output, err := watchdog.Run(ctx, cmd)
	if err != nil && s.DepMgr != nil {
		if tool, found := s.DepMgr.DetectMissingTool(string(output)); found {
			if installErr := s.DepMgr.InstallTool(ctx, tool); installErr == nil {
				watchdog2 := Watchdog{IdleTimeout: s.IdleTimeout}
				output2, err2 := watchdog2.Run(ctx, cmd)
				if err2 == nil {
					return string(output2), nil
				}
			}
		}
	}
	if err != nil {
		return string(output), fmt.Errorf("command execution failed: %w (output: %s)", err, string(output))
	}
	return string(output), nil
}

// DockerSandbox routes execution inside a warm Docker container.
type DockerSandbox struct {
	ContainerName string
}

var _ Sandbox = (*DockerSandbox)(nil)

func NewDockerSandbox(containerName string) *DockerSandbox {
	return &DockerSandbox{
		ContainerName: containerName,
	}
}

func (s *DockerSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "RunCommand",
		trace.WithAttributes(
			telemetry.Attr("command", command),
			telemetry.Attr("project_path", projectPath),
			telemetry.Attr("package", pkg),
			attribute.String("container_name", s.ContainerName),
		))
	defer span.End()
	cmdStr := command
	if cmdStr == "" {
		if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
			cmdStr = "go test -v ./..."
		} else {
			cmdStr = "python -m unittest discover tests"
		}
	}

	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return "", errors.New("empty command")
	}

	args := []string{"exec", "-w", "/app/" + pkg, s.ContainerName}
	args = append(args, parts...)

	cmd := exec.CommandContext(ctx, "docker", args...)

	var stdoutStderr strings.Builder
	cmd.Stdout = &stdoutStderr
	cmd.Stderr = &stdoutStderr

	err := cmd.Run()
	output := stdoutStderr.String()
	if err != nil {
		return output, fmt.Errorf("docker exec command failed: %w (output: %s)", err, output)
	}
	return output, nil
}

func (s *HostSandbox) runPythonTestsIsolated(ctx context.Context, targetDir string, command string) (string, error) {
	// Find all test_*.py files recursively under targetDir
	var testFiles []string
	err := filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasPrefix(info.Name(), "test_") && strings.HasSuffix(info.Name(), ".py") {
			// Read file to verify it actually contains a TestCase class definition
			content, readErr := os.ReadFile(path)
			if readErr == nil && !strings.Contains(string(content), "TestCase") {
				return nil
			}
			// Get relative path from targetDir
			rel, err := filepath.Rel(targetDir, path)
			if err == nil {
				// Exclude virtualenv, holdout or cache directories
				if !strings.Contains(rel, "node_modules") && !strings.Contains(rel, ".noctifab") && !strings.Contains(rel, "venv") && !strings.Contains(rel, "holdout") {
					testFiles = append(testFiles, rel)
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to walk tests directory: %w", err)
	}

	if len(testFiles) == 0 {
		return "No test files found to isolate.", nil
	}

	// Sort test files to ensure deterministic execution order
	sort.Strings(testFiles)

	var overallOutput strings.Builder
	var lastErr error

	for _, file := range testFiles {
		// Prepare command: python -m unittest <file>
		parts := strings.Fields(command)
		pythonBin := "python"
		if len(parts) > 0 && (parts[0] == "python" || parts[0] == "python3") {
			pythonBin = parts[0]
		}

		cmdArgs := []string{"-m", "unittest", file}
		cmd := exec.CommandContext(ctx, pythonBin, cmdArgs...)
		cmd.Dir = targetDir
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

		var stdoutStderr strings.Builder
		cmd.Stdout = &stdoutStderr
		cmd.Stderr = &stdoutStderr

		// PGID process group termination on cancellation
		go func() {
			<-ctx.Done()
			if cmd.Process != nil {
				pgid, err := syscall.Getpgid(cmd.Process.Pid)
				if err == nil {
					_ = syscall.Kill(-pgid, syscall.SIGKILL)
				}
			}
		}()

		err := cmd.Run()
		runOutput := stdoutStderr.String()

		fmt.Fprintf(&overallOutput, "=== RUNNING TEST FILE: %s ===\n", file)
		overallOutput.WriteString(runOutput)
		overallOutput.WriteString("\n")

		if err != nil {
			lastErr = fmt.Errorf("test file %s failed: %w", file, err)
		}
	}

	if lastErr != nil {
		return overallOutput.String(), fmt.Errorf("some tests failed: %w", lastErr)
	}
	return overallOutput.String(), nil
}
