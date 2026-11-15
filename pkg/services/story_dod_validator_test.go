package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDoDRunner struct {
	runFunc func(ctx context.Context, projectPath, command, pkg string) (string, error)
}

func (m *mockDoDRunner) RunCommand(ctx context.Context, projectPath, command, pkg string) (string, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, projectPath, command, pkg)
	}
	return "", nil
}

func (m *mockDoDRunner) Name() string {
	return "mock"
}

func TestStoryDoDValidator(t *testing.T) {
	t.Run("passes when public contract expectations are met", func(t *testing.T) {
		runner := &mockDoDRunner{
			runFunc: func(ctx context.Context, projectPath, command, pkg string) (string, error) {
				if command == "./pyedis-cli PING" {
					return "+PONG\n", nil
				}
				return "", errors.New("command not found")
			},
		}

		v := NewStoryDoDValidator(runner, 2*time.Second)
		contract := domain.StoryContract{
			StoryID: "US-001",
			PublicContracts: []domain.PublicContract{
				{
					ID:                 "ping-command",
					Interface:          "Redis::PING",
					AllowedExecutables: []string{"./pyedis-cli PING"},
					ExitCodes:          []int{0},
					StdoutContains:     []string{"PONG"},
				},
			},
		}

		report, err := v.ValidateStoryContracts(context.Background(), "/tmp", contract)
		require.NoError(t, err)
		assert.True(t, report.Passed)
		assert.Equal(t, 1, report.ExecutedContracts)
		assert.Empty(t, report.Failures)
	})

	t.Run("fails when stdout does not contain expected string", func(t *testing.T) {
		runner := &mockDoDRunner{
			runFunc: func(ctx context.Context, projectPath, command, pkg string) (string, error) {
				return "ERR unknown command\n", nil
			},
		}

		v := NewStoryDoDValidator(runner, 2*time.Second)
		contract := domain.StoryContract{
			StoryID: "US-001",
			PublicContracts: []domain.PublicContract{
				{
					ID:                 "ping-command",
					Interface:          "Redis::PING",
					AllowedExecutables: []string{"./pyedis-cli PING"},
					ExitCodes:          []int{0},
					StdoutContains:     []string{"PONG"},
				},
			},
		}

		report, err := v.ValidateStoryContracts(context.Background(), "/tmp", contract)
		require.NoError(t, err)
		assert.False(t, report.Passed)
		assert.Len(t, report.Failures, 1)
		assert.Contains(t, report.Failures[0], "stdout missing required pattern \"PONG\"")
	})

	t.Run("checkTaskStoryContracts skips when executable binary does not exist on disk yet", func(t *testing.T) {
		tmpDir := t.TempDir()
		runner := &mockDoDRunner{
			runFunc: func(ctx context.Context, projectPath, command, pkg string) (string, error) {
				return "", errors.New("should not be called")
			},
		}
		evaluator := &TestValidator{Runner: runner}
		state := &domain.State{
			ProjectPath: tmpDir,
			StoryContracts: []domain.StoryContract{
				{
					StoryID: "US-001",
					PublicContracts: []domain.PublicContract{
						{
							ID:                 "cli-help",
							Interface:          "cli",
							AllowedExecutables: []string{"./bin/t4 --help"},
							ExitCodes:          []int{0},
							StdoutContains:     []string{"Usage:"},
						},
					},
				},
			},
		}
		task := &domain.Task{ID: "TASK-001", StoryID: "US-001", TargetFiles: []string{"internal/domain/task.go"}}
		passed, logMsg := checkTaskStoryContracts(context.Background(), evaluator, state, task, true, "OK")
		assert.True(t, passed)
		assert.Equal(t, "OK", logMsg)
	})

	t.Run("checkTaskStoryContracts fails before merge when executable binary exists on disk and violates contract", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(binDir, "t4"), []byte("#!/bin/sh\n"), 0755))

		runner := &mockDoDRunner{
			runFunc: func(ctx context.Context, projectPath, command, pkg string) (string, error) {
				return "wrong output\n", nil
			},
		}
		evaluator := &TestValidator{Runner: runner}
		state := &domain.State{
			ProjectPath: tmpDir,
			StoryContracts: []domain.StoryContract{
				{
					StoryID: "US-001",
					PublicContracts: []domain.PublicContract{
						{
							ID:                     "cli-help",
							Interface:              "cli",
							ApplicablePathPrefixes: []string{"cmd/"},
							AllowedExecutables:     []string{"./bin/t4 --help"},
							ExitCodes:              []int{0},
							StdoutContains:         []string{"Usage:"},
						},
					},
				},
			},
		}
		task := &domain.Task{ID: "TASK-002", StoryID: "US-001", TargetFiles: []string{"cmd/t4/main.go"}}
		passed, logMsg := checkTaskStoryContracts(context.Background(), evaluator, state, task, true, "OK")
		assert.False(t, passed)
		assert.Contains(t, logMsg, "Task pre-merge black-box contract smoke check failed")
		assert.Contains(t, logMsg, "stdout missing required pattern \"Usage:\"")
	})
}
