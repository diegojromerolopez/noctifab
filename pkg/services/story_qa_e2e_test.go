package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPartialStoryQASandbox struct {
	cmdExecuted string
	output      string
	err         error
}

func (m *mockPartialStoryQASandbox) RunCommand(_ context.Context, _ string, cmd string, _ string) (string, error) {
	m.cmdExecuted = cmd
	return m.output, m.err
}

func TestStoryQAAuditor_PartialDeliverableE2E(t *testing.T) {
	t.Run("fails when partial deliverable E2E produces 0 tests for implemented commands", func(t *testing.T) {
		tmpDir := t.TempDir()
		// create pyproject.toml so E2E command is detected as pytest
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte("[tool.pytest]\n"), 0644))
		e2eDir := filepath.Join(tmpDir, "tests", "e2e")
		require.NoError(t, os.MkdirAll(e2eDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(e2eDir, "test_kv.py"), []byte("def test_get(): pass\ndef test_set(): pass\n"), 0644))

		mockRunner := &mockPartialStoryQASandbox{
			output: "0 tests collected\nno tests ran in 0.01s",
			err:    nil,
		}

		auditor := NewStoryQAAuditor(nil, mockRunner)
		auditor.SetE2ECommand("pytest tests/e2e")

		state := &domain.State{
			ProjectPath: tmpDir,
			Metadata: domain.StateMetadata{
				FeatureName: "US-001: Implement GET and SET commands",
			},
			Tasks: []domain.Task{
				{
					ID:          "task-1",
					Title:       "Implement GET and SET",
					TargetFiles: []string{"src/get.py", "src/set.py"},
				},
			},
		}

		passed, log := auditor.runPartialDeliverableE2E(context.Background(), state)
		assert.False(t, passed, "should fail when 0 tests ran for implemented commands")
		assert.Contains(t, log, "produced 0 tests for command(s) [get, set]")
		assert.Contains(t, mockRunner.cmdExecuted, `-k "get or set"`)
	})

	t.Run("fails when partial deliverable in-scope E2E tests fail", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte("[tool.pytest]\n"), 0644))
		e2eDir := filepath.Join(tmpDir, "tests", "e2e")
		require.NoError(t, os.MkdirAll(e2eDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(e2eDir, "test_kv.py"), []byte("def test_get(): pass\n"), 0644))

		mockRunner := &mockPartialStoryQASandbox{
			output: "FAILED tests/e2e/test_kv.py::test_get - AssertionError\n1 failed in 0.1s",
			err:    errors.New("exit code 1"),
		}

		auditor := NewStoryQAAuditor(nil, mockRunner)
		auditor.SetE2ECommand("pytest tests/e2e")

		state := &domain.State{
			ProjectPath: tmpDir,
			Metadata: domain.StateMetadata{
				FeatureName: "US-001: Implement GET",
			},
			Tasks: []domain.Task{
				{
					ID:          "task-1",
					Title:       "Implement GET",
					TargetFiles: []string{"src/get.py"},
				},
			},
		}

		passed, log := auditor.runPartialDeliverableE2E(context.Background(), state)
		assert.False(t, passed)
		assert.Contains(t, log, "Partial deliverable E2E test suite failed")
	})

	t.Run("passes when partial deliverable in-scope E2E tests succeed", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte("[tool.pytest]\n"), 0644))
		e2eDir := filepath.Join(tmpDir, "tests", "e2e")
		require.NoError(t, os.MkdirAll(e2eDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(e2eDir, "test_kv.py"), []byte("def test_get(): pass\ndef test_set(): pass\n"), 0644))

		mockRunner := &mockPartialStoryQASandbox{
			output: "tests/e2e/test_kv.py::test_get PASSED\ntests/e2e/test_kv.py::test_set PASSED\n2 passed in 0.2s",
			err:    nil,
		}

		auditor := NewStoryQAAuditor(nil, mockRunner)
		auditor.SetE2ECommand("pytest tests/e2e")

		state := &domain.State{
			ProjectPath: tmpDir,
			Metadata: domain.StateMetadata{
				FeatureName: "US-001: Implement GET and SET",
			},
			Tasks: []domain.Task{
				{
					ID:          "task-1",
					Title:       "Implement GET and SET",
					TargetFiles: []string{"src/get.py", "src/set.py"},
				},
			},
		}

		passed, log := auditor.runPartialDeliverableE2E(context.Background(), state)
		assert.True(t, passed)
		assert.Empty(t, log)
		assert.Contains(t, mockRunner.cmdExecuted, `-k "get or set"`)
	})

	t.Run("US-001 walking skeleton enforces partial deliverable E2E verification", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte("[tool.pytest]\n"), 0644))
		e2eDir := filepath.Join(tmpDir, "tests", "e2e")
		require.NoError(t, os.MkdirAll(e2eDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(e2eDir, "test_smoke.py"), []byte("def test_ping(): pass\n"), 0644))

		mockRunner := &mockPartialStoryQASandbox{
			output: "tests/e2e/test_smoke.py::test_entrypoint_ping PASSED\n1 passed in 0.05s",
			err:    nil,
		}

		auditor := NewStoryQAAuditor(nil, mockRunner)
		auditor.SetE2ECommand("pytest tests/e2e")

		state := &domain.State{
			ProjectPath: tmpDir,
			Metadata: domain.StateMetadata{
				FeatureName: "US-001-walking-skeleton-thin-tcp-ping-entrypoint",
			},
			Tasks: []domain.Task{
				{
					ID:          "US-001-TASK-001",
					StoryID:     "US-001",
					Title:       "Foundational Walking Skeleton & Ping Entrypoint",
					TargetFiles: []string{"src/main.py", "Makefile"},
				},
			},
		}

		passed, log := auditor.runPartialDeliverableE2E(context.Background(), state)
		assert.True(t, passed, "US-001 partial deliverable must pass E2E smoke/ping verification")
		assert.Empty(t, log)
		assert.Contains(t, mockRunner.cmdExecuted, "ping")
	})

	t.Run("fails when implemented commands have no E2E command detected", func(t *testing.T) {
		tmpDir := t.TempDir()
		mockRunner := &mockPartialStoryQASandbox{}
		auditor := NewStoryQAAuditor(nil, mockRunner)

		state := &domain.State{
			ProjectPath: tmpDir,
			Metadata: domain.StateMetadata{
				FeatureName: "US-002: Implement DEL command",
			},
			Tasks: []domain.Task{
				{
					ID:          "task-del",
					Title:       "Implement DEL command",
					TargetFiles: []string{"src/del.py"},
				},
			},
		}

		passed, log := auditor.runPartialDeliverableE2E(context.Background(), state)
		assert.False(t, passed)
		assert.Contains(t, log, "Mandatory Partial Deliverability E2E failure: No E2E test suite detected")
	})

	t.Run("fails when implemented commands are not added to E2E test suite", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte("[tool.pytest]\n"), 0644))
		e2eDir := filepath.Join(tmpDir, "tests", "e2e")
		require.NoError(t, os.MkdirAll(e2eDir, 0755))
		// Only has test for ping, but story implements DEL
		require.NoError(t, os.WriteFile(filepath.Join(e2eDir, "test_smoke.py"), []byte("def test_ping(): pass\n"), 0644))

		mockRunner := &mockPartialStoryQASandbox{}
		auditor := NewStoryQAAuditor(nil, mockRunner)
		auditor.SetE2ECommand("pytest tests/e2e")

		state := &domain.State{
			ProjectPath: tmpDir,
			Metadata: domain.StateMetadata{
				FeatureName: "US-002: Implement DEL command",
			},
			Tasks: []domain.Task{
				{
					ID:          "task-del",
					Title:       "Implement DEL command",
					TargetFiles: []string{"src/del.py"},
				},
			},
		}

		passed, log := auditor.runPartialDeliverableE2E(context.Background(), state)
		assert.False(t, passed)
		assert.Contains(t, log, "User story implemented command(s) [del]")
	})
}
