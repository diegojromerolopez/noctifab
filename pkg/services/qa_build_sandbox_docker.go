package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DockerQABuildSandbox runs untrusted project build code in a disposable container.
type DockerQABuildSandbox struct {
	process      QAProcessRunner
	fs           QAFileSystem
	image        string
	timeout      time.Duration
	maxProcesses int
	memoryLimit  string
}

var _ QABuildSandbox = (*DockerQABuildSandbox)(nil)

func NewDockerQABuildSandbox(process QAProcessRunner, fsys QAFileSystem, image string,
	timeout time.Duration, maxProcesses int, memoryLimit string,
) *DockerQABuildSandbox {
	if maxProcesses <= 0 {
		maxProcesses = defaultQADockerPIDs
	}
	if memoryLimit == "" {
		memoryLimit = defaultQADockerMemory
	}
	return &DockerQABuildSandbox{process: process, fs: fsys, image: image, timeout: timeout,
		maxProcesses: maxProcesses, memoryLimit: memoryLimit}
}

func (s *DockerQABuildSandbox) Run(ctx context.Context, workspace ReviewWorkspace,
	command []string, outputLimit int,
) (QACommandResult, error) {
	if s.process == nil || s.fs == nil || strings.TrimSpace(s.image) == "" {
		return QACommandResult{}, errors.New("qa build sandbox: missing dependency or image")
	}
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" || s.timeout <= 0 || outputLimit <= 0 {
		return QACommandResult{}, errors.New("qa build sandbox: invalid request")
	}
	path, err := s.fs.Abs(workspace.Path)
	if err != nil || strings.TrimSpace(workspace.Path) == "" {
		return QACommandResult{}, fmt.Errorf("qa build sandbox: invalid workspace: %w", err)
	}
	if info, statErr := s.fs.Stat(path); statErr != nil || !info.IsDir() {
		return QACommandResult{}, errors.New("qa build sandbox: workspace unavailable")
	}
	version, err := s.process.Run(ctx, QAProcess{Name: "docker",
		Args: []string{"version", "--format", "{{.Server.Version}}"}, Timeout: 15 * time.Second,
		OutputLimit: qaDockerControlOutputLimit})
	if err != nil || version.ExitCode != 0 || strings.TrimSpace(version.Stdout) == "" {
		return QACommandResult{}, fmt.Errorf("qa build sandbox: Docker unavailable: %w", err)
	}
	probeArgs := append(s.dockerArgs(filepath.Clean(path)), "--entrypoint", "sh", s.image, "-c",
		"test -w /workspace && touch /workspace/.noctifab-build-probe && rm /workspace/.noctifab-build-probe && "+
			"! touch /sibling-write-probe 2>/dev/null && ! touch /host-write-probe 2>/dev/null")
	probe, err := s.process.Run(ctx, QAProcess{Name: "docker", Args: probeArgs,
		Timeout: 30 * time.Second, OutputLimit: qaDockerControlOutputLimit})
	if err != nil || probe.ExitCode != 0 || probe.TimedOut || probe.Truncated {
		return QACommandResult{}, fmt.Errorf("qa build sandbox: isolation probe failed: %w", err)
	}
	args := append(s.dockerArgs(filepath.Clean(path)), "--attach", "stdout", "--attach", "stderr",
		"--entrypoint", command[0], s.image)
	args = append(args, command[1:]...)
	processResult, err := s.process.Run(ctx, QAProcess{Name: "docker", Args: args,
		Timeout: s.timeout, OutputLimit: outputLimit})
	result := commandResult(processResult)
	if err != nil {
		return result, fmt.Errorf("qa build sandbox: Docker execution failed: %w", err)
	}
	if processResult.ExitCode == 125 || processResult.ExitCode == 126 || processResult.ExitCode == 127 {
		return result, fmt.Errorf("qa build sandbox: container environment failed with status %d", processResult.ExitCode)
	}
	return result, nil
}

func (s *DockerQABuildSandbox) dockerArgs(workspace string) []string {
	return []string{"run", "--rm", "--init", "--read-only", "--network", "none",
		"--pids-limit", strconv.Itoa(s.maxProcesses), "--memory", s.memoryLimit,
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--workdir", "/workspace", "--env", "HOME=/home", "--env", "TMPDIR=/tmp",
		"--env", "XDG_CACHE_HOME=/cache", "--tmpfs", "/tmp:rw,nosuid,nodev",
		"--tmpfs", "/home:rw,nosuid,nodev", "--tmpfs", "/cache:rw,nosuid,nodev",
		"--mount", bindMount(workspace, "/workspace", false)}
}
