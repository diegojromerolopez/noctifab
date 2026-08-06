package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// runWithProcessGroupKill runs cmd (which must have Setpgid set) and kills
// its whole process group if ctx is cancelled while it is running. The
// watcher goroutine exits as soon as the command finishes (via the done
// channel), so it does not leak until the outer ctx is done.
func runWithProcessGroupKill(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan struct{})
	defer close(done)

	// cmd.Process is guaranteed to be set once Start returns, so the
	// watcher goroutine can read it without racing the exec machinery.
	go func() {
		select {
		case <-ctx.Done():
			pgid, err := syscall.Getpgid(cmd.Process.Pid)
			if err == nil {
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			}
		case <-done:
		}
	}()

	return cmd.Wait()
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

	overallOutput := NewBoundedBuffer(defaultBoundedBufferMax)
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

		stdoutStderr := NewBoundedBuffer(defaultBoundedBufferMax)
		cmd.Stdout = stdoutStderr
		cmd.Stderr = stdoutStderr

		// PGID process group termination on cancellation; the watcher
		// goroutine is released as soon as the command finishes.
		err := runWithProcessGroupKill(ctx, cmd)
		runOutput := stdoutStderr.String()

		_, _ = fmt.Fprintf(overallOutput, "=== RUNNING TEST FILE: %s ===\n", file)
		_, _ = overallOutput.Write([]byte(runOutput))
		_, _ = overallOutput.Write([]byte("\n"))

		if err != nil {
			lastErr = fmt.Errorf("test file %s failed: %w", file, err)
		}
	}

	if lastErr != nil {
		return overallOutput.String(), fmt.Errorf("some tests failed: %w", lastErr)
	}
	return overallOutput.String(), nil
}
