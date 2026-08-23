package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// HostQABuildSandbox executes project build code on the host workspace in host sandbox mode.
type HostQABuildSandbox struct {
	process QAProcessRunner
	fs      QAFileSystem
	timeout time.Duration
}

var _ QABuildSandbox = (*HostQABuildSandbox)(nil)

func NewHostQABuildSandbox(process QAProcessRunner, fsys QAFileSystem, timeout time.Duration) *HostQABuildSandbox {
	return &HostQABuildSandbox{process: process, fs: fsys, timeout: timeout}
}

func (s *HostQABuildSandbox) Run(
	ctx context.Context,
	workspace ReviewWorkspace,
	command []string,
	outputLimit int,
) (QACommandResult, error) {
	if s.process == nil || s.fs == nil {
		return QACommandResult{}, errors.New("host qa build sandbox: missing dependency")
	}
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" || s.timeout <= 0 || outputLimit <= 0 {
		return QACommandResult{}, errors.New("host qa build sandbox: invalid request")
	}
	path, err := s.fs.Abs(workspace.Path)
	if err != nil || strings.TrimSpace(workspace.Path) == "" {
		return QACommandResult{}, fmt.Errorf("host qa build sandbox: invalid workspace: %w", err)
	}
	if info, statErr := s.fs.Stat(path); statErr != nil || !info.IsDir() {
		return QACommandResult{}, errors.New("host qa build sandbox: workspace unavailable")
	}

	// Sanitize long-running server start commands for interpreted projects (e.g. "make run", "npm start")
	fullCmd := strings.ToLower(strings.Join(command, " "))
	if strings.Contains(fullCmd, "make run") || strings.Contains(fullCmd, "npm start") ||
		strings.Contains(fullCmd, "python -m src.main") || strings.Contains(fullCmd, "python3 -m src.main") {
		// Interpreted languages do not require compilation; skip blocking synchronous server execution
		return QACommandResult{ExitCode: 0}, nil
	}

	processResult, err := s.process.Run(ctx, QAProcess{
		Name:        command[0],
		Args:        command[1:],
		Dir:         path,
		Timeout:     s.timeout,
		OutputLimit: outputLimit,
	})
	result := commandResult(processResult)
	if err != nil {
		return result, fmt.Errorf("host qa build sandbox: execution failed: %w", err)
	}
	return result, nil
}

// HostQASandboxRunner executes QA commands directly on the host review workspace.
type HostQASandboxRunner struct {
	process QAProcessRunner
	fs      QAFileSystem
	timeout time.Duration

	mu       sync.RWMutex
	verified bool
	source   string
	artifact string
	runtime  string
}

var _ QASandboxRunner = (*HostQASandboxRunner)(nil)

func NewHostQASandboxRunner(process QAProcessRunner, fsys QAFileSystem, timeout time.Duration) *HostQASandboxRunner {
	return &HostQASandboxRunner{process: process, fs: fsys, timeout: timeout}
}

func (r *HostQASandboxRunner) Verify(ctx context.Context, sourcePath, artifactPath, runtimePath string) error {
	r.mu.Lock()
	r.verified = false
	r.source, r.artifact, r.runtime = "", "", ""
	r.mu.Unlock()

	if r.process == nil || r.fs == nil {
		return errors.New("host qa sandbox verify: missing dependency")
	}

	absSource, err := r.fs.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("host qa sandbox verify: invalid source path: %w", err)
	}
	absArtifact, err := r.fs.Abs(artifactPath)
	if err != nil {
		return fmt.Errorf("host qa sandbox verify: invalid artifact path: %w", err)
	}
	absRuntime, err := r.fs.Abs(runtimePath)
	if err != nil {
		return fmt.Errorf("host qa sandbox verify: invalid runtime path: %w", err)
	}

	for _, directory := range []string{"tmp", "home", "cache"} {
		if err := r.fs.MkdirAll(filepath.Join(absRuntime, directory), 0o700); err != nil {
			return fmt.Errorf("host qa sandbox verify: create runtime %s: %w", directory, err)
		}
	}

	r.mu.Lock()
	r.verified = true
	r.source = absSource
	r.artifact = absArtifact
	r.runtime = absRuntime
	r.mu.Unlock()

	return nil
}

func (r *HostQASandboxRunner) Run(ctx context.Context, command QACommand) (QACommandResult, error) {
	r.mu.RLock()
	verified := r.verified
	runtimeDir := r.runtime
	r.mu.RUnlock()

	if !verified {
		return QACommandResult{}, errors.New("host qa sandbox: runner not verified")
	}
	if len(command.Argv) == 0 || strings.TrimSpace(command.Argv[0]) == "" {
		return QACommandResult{}, errors.New("host qa sandbox: empty command")
	}

	timeout := command.Timeout
	if timeout <= 0 {
		timeout = r.timeout
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	processResult, err := r.process.Run(ctx, QAProcess{
		Name:        command.Argv[0],
		Args:        command.Argv[1:],
		Dir:         runtimeDir,
		Stdin:       command.Stdin,
		Timeout:     timeout,
		OutputLimit: command.OutputLimit,
	})
	result := commandResult(processResult)
	if err != nil {
		return result, fmt.Errorf("host qa sandbox: command failed: %w", err)
	}
	return result, nil
}
