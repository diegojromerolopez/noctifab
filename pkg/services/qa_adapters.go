package services

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// OSQAFileSystem is the production filesystem adapter.
type OSQAFileSystem struct{}

func (OSQAFileSystem) Abs(path string) (string, error)              { return filepath.Abs(path) }
func (OSQAFileSystem) MkdirAll(path string, perm fs.FileMode) error { return os.MkdirAll(path, perm) }
func (OSQAFileSystem) RemoveAll(path string) error                  { return os.RemoveAll(path) }
func (OSQAFileSystem) Open(path string) (io.ReadCloser, error)      { return os.Open(path) }
func (OSQAFileSystem) OpenFile(path string, flag int, perm fs.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(path, flag, perm)
}
func (OSQAFileSystem) ReadFile(path string) ([]byte, error)   { return os.ReadFile(path) }
func (OSQAFileSystem) Lstat(path string) (fs.FileInfo, error) { return os.Lstat(path) }
func (OSQAFileSystem) Stat(path string) (fs.FileInfo, error)  { return os.Stat(path) }

// SystemQAClock is the production wall clock.
type SystemQAClock struct{}

func (SystemQAClock) Now() time.Time { return time.Now() }

// ExecReviewGitRunner executes non-interactive Git commands in a selected repository.
type ExecReviewGitRunner struct {
	process QAProcessRunner
	timeout time.Duration
}

func NewExecReviewGitRunner(process QAProcessRunner, timeout time.Duration) *ExecReviewGitRunner {
	return &ExecReviewGitRunner{process: process, timeout: timeout}
}

func (r *ExecReviewGitRunner) Run(ctx context.Context, repositoryPath string, args ...string) (string, error) {
	result, err := r.process.Run(ctx, QAProcess{
		Name: "git", Args: args, Dir: repositoryPath,
		Env: []string{"GIT_TERMINAL_PROMPT=0"}, Timeout: r.timeout,
	})
	if err != nil {
		return result.Stdout + result.Stderr, err
	}
	if result.ExitCode != 0 {
		return result.Stdout + result.Stderr, &ProcessExitError{ExitCode: result.ExitCode}
	}
	return result.Stdout, nil
}

// ExecQAProcessRunner is the production bounded process adapter.
type ExecQAProcessRunner struct{}

// ProcessExitError reports a completed process with a non-zero status.
type ProcessExitError struct{ ExitCode int }

func (e *ProcessExitError) Error() string {
	return "process exited with status " + strconv.Itoa(e.ExitCode)
}

func (ExecQAProcessRunner) Run(ctx context.Context, process QAProcess) (QAProcessResult, error) {
	if process.Name == "" {
		return QAProcessResult{}, errors.New("process: empty executable")
	}
	if process.Timeout <= 0 {
		return QAProcessResult{}, errors.New("process: timeout must be positive")
	}
	if process.OutputLimit <= 0 {
		process.OutputLimit = defaultBoundedBufferMax
	}
	runCtx, cancel := context.WithTimeout(ctx, process.Timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, process.Name, process.Args...)
	cmd.Dir = process.Dir
	cmd.Env = append(os.Environ(), process.Env...)
	cmd.Stdin = strings.NewReader(process.Stdin)
	stdout := NewBoundedBuffer(process.OutputLimit)
	stderr := NewBoundedBuffer(process.OutputLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	result := QAProcessResult{
		Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: 0,
		TimedOut:  errors.Is(runCtx.Err(), context.DeadlineExceeded),
		Truncated: stdout.Truncated() || stderr.Truncated(),
	}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		if result.TimedOut {
			return result, nil
		}
		return result, nil
	}
	return result, err
}
