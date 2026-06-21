package usecase

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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
}

var _ Sandbox = (*HostSandbox)(nil)

func NewHostSandbox(allowed []string, defaultCmd string) *HostSandbox {
	return &HostSandbox{
		AllowedCommands: allowed,
		DefaultCommand:  defaultCmd,
	}
}

func (s *HostSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
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

	var cmd *exec.Cmd
	if len(parts) > 1 {
		cmd = exec.CommandContext(ctx, binary, parts[1:]...)
	} else {
		cmd = exec.CommandContext(ctx, binary)
	}
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
	output := stdoutStderr.String()
	if err != nil {
		return output, fmt.Errorf("command execution failed: %w (output: %s)", err, output)
	}
	return output, nil
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
	cmdStr := command
	if cmdStr == "" {
		cmdStr = "go test -v ./..."
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
