package services

import (
	"context"
	"io"
	"io/fs"
	"time"
)

// ReviewWorkspace is one branch-backed copy of an immutable source commit.
type ReviewWorkspace struct {
	Path   string
	Branch string
}

// ReviewWorkspaceFactory creates and removes the isolated review workspaces.
type ReviewWorkspaceFactory interface {
	Create(ctx context.Context, repositoryPath, sourceCommit string) (
		build ReviewWorkspace, tester ReviewWorkspace, qa ReviewWorkspace, err error,
	)
	Cleanup(ctx context.Context, workspaces ...ReviewWorkspace) error
}

// QAReviewSession retains cleanup ownership until its terminal result is persisted.
type QAReviewSession struct {
	Result     QAReviewResult
	workspaces []ReviewWorkspace
	factory    ReviewWorkspaceFactory
}

// Cleanup releases review isolation. It is safe to retry after an error.
func (s *QAReviewSession) Cleanup(ctx context.Context) error {
	if s == nil || s.factory == nil || len(s.workspaces) == 0 {
		return nil
	}
	if err := s.factory.Cleanup(ctx, s.workspaces...); err != nil {
		return err
	}
	s.workspaces = nil
	return nil
}

// QAArtifactBuilder creates and verifies an immutable executable artifact.
type QAArtifactBuilder interface {
	Build(ctx context.Context, workspace ReviewWorkspace, sourceCommit string, buildCommand,
		validationExecutables []string, artifactPath string, outputLimit int) (QAArtifact, QACommandResult, error)
	Verify(artifact QAArtifact) error
}

// QABuildSandbox executes project build code without exposing the host filesystem.
type QABuildSandbox interface {
	Run(ctx context.Context, workspace ReviewWorkspace, command []string, outputLimit int) (QACommandResult, error)
}

// QACommand is an argument-vector process request. It is never interpreted by a shell.
type QACommand struct {
	Argv        []string
	Stdin       string
	Timeout     time.Duration
	OutputLimit int
}

// QACommandResult contains bounded observable process output.
type QACommandResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	TimedOut  bool
	Truncated bool
}

// QASandboxRunner executes QA commands inside a previously verified boundary.
type QASandboxRunner interface {
	Verify(ctx context.Context, sourcePath, artifactPath, runtimePath string) error
	Run(ctx context.Context, command QACommand) (QACommandResult, error)
}

// ReviewGitRunner isolates workspace management from a concrete Git client.
type ReviewGitRunner interface {
	Run(ctx context.Context, repositoryPath string, args ...string) (string, error)
}

// QAProcess describes an injected host process invocation, including Docker itself.
type QAProcess struct {
	Name        string
	Args        []string
	Dir         string
	Env         []string
	Stdin       string
	Timeout     time.Duration
	OutputLimit int
}

// QAProcessResult is the low-level result returned by a process adapter.
type QAProcessResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	TimedOut  bool
	Truncated bool
}

// QAProcessRunner starts bounded host processes without a shell.
type QAProcessRunner interface {
	Run(ctx context.Context, process QAProcess) (QAProcessResult, error)
}

// QAFileSystem contains filesystem operations used by the QA foundation.
type QAFileSystem interface {
	Abs(path string) (string, error)
	MkdirAll(path string, perm fs.FileMode) error
	RemoveAll(path string) error
	Open(path string) (io.ReadCloser, error)
	OpenFile(path string, flag int, perm fs.FileMode) (io.WriteCloser, error)
	ReadFile(path string) ([]byte, error)
	Lstat(path string) (fs.FileInfo, error)
	Stat(path string) (fs.FileInfo, error)
}

// QAClock supplies collision-resistant workspace names in a testable manner.
type QAClock interface {
	Now() time.Time
}
