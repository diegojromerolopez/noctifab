package services

import (
	"context"
	"errors"
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
}
