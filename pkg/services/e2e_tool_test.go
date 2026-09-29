package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockE2ERunner struct {
	lastCmd string
	lastPkg string
	retOut  string
	retErr  error
	delay   time.Duration
}

func (m *mockE2ERunner) RunCommand(ctx context.Context, dir, cmd, pkg string) (string, error) {
	m.lastCmd = cmd
	m.lastPkg = pkg
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return m.retOut, m.retErr
}

func (m *mockE2ERunner) RunLinter(ctx context.Context, dir, cmd string) (string, error) {
	return "", nil
}

func TestRunE2ETestsTool(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "Makefile"), []byte("e2e:\n\tpytest tests/e2e\n"), 0600))

	state := &domain.State{ProjectPath: tmpDir}

	t.Run("executes auto-detected E2E command successfully", func(t *testing.T) {
		runner := &mockE2ERunner{retOut: "E2E PASSED", retErr: nil}
		tool := &RunE2ETestsTool{Runner: runner, E2EMode: "native"}

		out, err := tool.Execute(context.Background(), state, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, "E2E PASSED", out)
		assert.Equal(t, "make e2e", runner.lastCmd)
		assert.Equal(t, "run_e2e_tests", tool.Name())
		assert.NotEmpty(t, tool.Description())
	})

	t.Run("executes explicit command argument when provided", func(t *testing.T) {
		runner := &mockE2ERunner{retOut: "DOCKER E2E OK", retErr: nil}
		tool := &RunE2ETestsTool{Runner: runner}

		out, err := tool.Execute(context.Background(), state, map[string]any{
			"command": "docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner-e2e",
		})
		require.NoError(t, err)
		assert.Equal(t, "DOCKER E2E OK", out)
		assert.Equal(t, "docker compose -f docker-compose.e2e.yml up --build --exit-code-from test-runner-e2e", runner.lastCmd)
	})

	t.Run("returns informative message when no E2E command is detected", func(t *testing.T) {
		emptyDir := t.TempDir()
		emptyState := &domain.State{ProjectPath: emptyDir}
		runner := &mockE2ERunner{}
		tool := &RunE2ETestsTool{Runner: runner, E2EMode: "docker"}

		out, err := tool.Execute(context.Background(), emptyState, map[string]any{})
		require.NoError(t, err)
		assert.Contains(t, out, "No E2E test suite configured or detected")
		assert.Empty(t, runner.lastCmd)
	})

	t.Run("returns timeout error when runner exceeds deadline", func(t *testing.T) {
		runner := &mockE2ERunner{delay: 200 * time.Millisecond}
		tool := &RunE2ETestsTool{Runner: runner, Timeout: 50 * time.Millisecond}

		out, err := tool.Execute(context.Background(), state, map[string]any{"command": "make e2e"})
		require.Error(t, err)
		assert.Contains(t, out, "TIMEOUT")
		assert.Contains(t, err.Error(), "timed out")
	})

	t.Run("errors on missing runner or invalid state", func(t *testing.T) {
		tool := &RunE2ETestsTool{}
		_, err := tool.Execute(context.Background(), state, map[string]any{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no sandbox execution engine registered")

		tool.Runner = &mockE2ERunner{}
		_, err = tool.Execute(context.Background(), nil, map[string]any{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing project path")
	})
}
