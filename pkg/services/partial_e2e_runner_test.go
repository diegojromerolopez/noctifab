package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPartialSandbox struct {
	cmdExecuted string
	output      string
	err         error
}

func (m *mockPartialSandbox) RunCommand(_ context.Context, _ string, cmd string, _ string) (string, error) {
	m.cmdExecuted = cmd
	return m.output, m.err
}

func TestPartialStateE2ERunner_ExtractCommands(t *testing.T) {
	runner := NewPartialStateE2ERunner(nil)
	t.Run("extracts specific commands from task title and targets", func(t *testing.T) {
		task := domain.Task{
			ID:          "task-1",
			Title:       "Implement GET and SET commands for Pyedis",
			TargetFiles: []string{"src/get.py", "src/set.py"},
		}

		cmds := runner.ExtractImplementedCommands(nil, task)
		require.Len(t, cmds, 2)
		assert.Contains(t, cmds, "get")
		assert.Contains(t, cmds, "set")
	})

	t.Run("US-001 walking skeleton defaults to entrypoint smoke and health targets", func(t *testing.T) {
		task := domain.Task{
			ID:          "US-001-TASK-001",
			StoryID:     "US-001",
			Title:       "Project scaffolding and walking skeleton",
			TargetFiles: []string{"main.go", "Makefile"},
		}

		cmds := runner.ExtractImplementedCommands(nil, task)
		require.NotEmpty(t, cmds)
		assert.Contains(t, cmds, "ping")
		assert.Contains(t, cmds, "smoke")
		assert.Contains(t, cmds, "health")
	})
}

func TestPartialStateE2ERunner_BuildTargetedCommand(t *testing.T) {
	runner := NewPartialStateE2ERunner(nil)

	t.Run("synthesizes pytest keyword filter", func(t *testing.T) {
		cmd := runner.BuildTargetedE2ECommand("pytest tests/e2e", []string{"get", "set"})
		assert.Equal(t, `pytest tests/e2e -k "get or set"`, cmd)
	})

	t.Run("synthesizes go test run filter", func(t *testing.T) {
		cmd := runner.BuildTargetedE2ECommand("go test -v ./tests/e2e", []string{"get", "set"})
		assert.Equal(t, `go test -v ./tests/e2e -run "Test.*(get|set)"`, cmd)
	})
}

func TestPartialStateE2ERunner_ValidatePartialE2E(t *testing.T) {
	t.Run("passes when in-scope tests run and pass", func(t *testing.T) {
		sb := &mockPartialSandbox{
			output: "tests/e2e/test_kv.py::test_get PASSED\ntests/e2e/test_kv.py::test_set PASSED\n2 passed in 0.12s",
			err:    nil,
		}
		runner := NewPartialStateE2ERunner(sb)
		task := domain.Task{
			ID:          "task-1",
			Title:       "Support GET and SET",
			TargetFiles: []string{"src/get.py", "src/set.py"},
		}

		passed, msg, err := runner.ValidatePartialE2E(context.Background(), "/proj", "pytest tests/e2e", nil, task)
		require.NoError(t, err)
		assert.True(t, passed)
		assert.Empty(t, msg)
		assert.Contains(t, sb.cmdExecuted, `-k "get or set"`)
	})

	t.Run("fails when in-scope tests fail", func(t *testing.T) {
		sb := &mockPartialSandbox{
			output: "FAILED tests/e2e/test_kv.py::test_get\n1 failed in 0.12s",
			err:    errors.New("exit code 1"),
		}
		runner := NewPartialStateE2ERunner(sb)
		task := domain.Task{
			ID:          "task-1",
			Title:       "Support GET",
			TargetFiles: []string{"src/get.py"},
		}

		passed, msg, err := runner.ValidatePartialE2E(context.Background(), "/proj", "pytest tests/e2e", nil, task)
		require.NoError(t, err)
		assert.False(t, passed)
		assert.Contains(t, msg, "Partial-state E2E test failed")
	})

	t.Run("fails when 0 tests were executed for implemented commands", func(t *testing.T) {
		sb := &mockPartialSandbox{
			output: "0 tests collected\nno tests ran in 0.01s",
			err:    nil,
		}
		runner := NewPartialStateE2ERunner(sb)
		task := domain.Task{
			ID:          "task-1",
			Title:       "Support GET",
			TargetFiles: []string{"src/get.py"},
		}

		passed, msg, err := runner.ValidatePartialE2E(context.Background(), "/proj", "pytest tests/e2e", nil, task)
		require.NoError(t, err)
		assert.False(t, passed)
		assert.Contains(t, msg, "0 passing E2E tests executed")
	})
}

func TestGenerateDynamicRoundTripNonce(t *testing.T) {
	k1, v1 := GenerateDynamicRoundTripNonce()
	k2, v2 := GenerateDynamicRoundTripNonce()

	assert.NotEmpty(t, k1)
	assert.NotEmpty(t, v1)
	assert.NotEqual(t, k1, k2)
	assert.NotEqual(t, v1, v2)
	assert.True(t, strings.HasPrefix(k1, "k_"))
	assert.True(t, strings.HasPrefix(v1, "v_"))
}

func TestVerifyStoryAddedE2ETests(t *testing.T) {
	runner := NewPartialStateE2ERunner(nil)

	t.Run("empty commands returns true", func(t *testing.T) {
		ok, msg := runner.VerifyStoryAddedE2ETests("/dummy", nil, nil)
		assert.True(t, ok)
		assert.Empty(t, msg)
	})

	t.Run("no e2e test files in workspace returns false", func(t *testing.T) {
		tmp := t.TempDir()
		ok, msg := runner.VerifyStoryAddedE2ETests(tmp, nil, []string{"get", "set"})
		assert.False(t, ok)
		assert.Contains(t, msg, "No E2E test files found in workspace")
	})

	t.Run("missing coverage for implemented command returns false", func(t *testing.T) {
		tmp := t.TempDir()
		e2eDir := filepath.Join(tmp, "tests", "e2e")
		require.NoError(t, os.MkdirAll(e2eDir, 0755))
		testFile := filepath.Join(e2eDir, "test_get.py")
		require.NoError(t, os.WriteFile(testFile, []byte("def test_get(): pass\n"), 0644))

		ok, msg := runner.VerifyStoryAddedE2ETests(tmp, nil, []string{"get", "set"})
		assert.False(t, ok)
		assert.Contains(t, msg, "implemented command(s) [set]")
	})

	t.Run("all implemented commands covered in e2e returns true", func(t *testing.T) {
		tmp := t.TempDir()
		e2eDir := filepath.Join(tmp, "tests", "e2e")
		require.NoError(t, os.MkdirAll(e2eDir, 0755))
		testFile := filepath.Join(e2eDir, "test_commands.py")
		require.NoError(t, os.WriteFile(testFile, []byte("def test_get(): pass\ndef test_set(): pass\n"), 0644))

		ok, msg := runner.VerifyStoryAddedE2ETests(tmp, nil, []string{"get", "set"})
		assert.True(t, ok)
		assert.Empty(t, msg)
	})

	t.Run("US-001 fallback set satisfied when at least one smoke/ping/health is present", func(t *testing.T) {
		tmp := t.TempDir()
		e2eDir := filepath.Join(tmp, "tests", "e2e")
		require.NoError(t, os.MkdirAll(e2eDir, 0755))
		testFile := filepath.Join(e2eDir, "smoke.sh")
		require.NoError(t, os.WriteFile(testFile, []byte("#!/bin/sh\ncurl http://localhost:8080/health\n"), 0755))

		fallbackCmds := []string{"ping", "health", "smoke", "help", "version"}
		ok, msg := runner.VerifyStoryAddedE2ETests(tmp, nil, fallbackCmds)
		assert.True(t, ok)
		assert.Empty(t, msg)
	})

	t.Run("US-001 fallback fails when none of smoke/ping/health present", func(t *testing.T) {
		tmp := t.TempDir()
		e2eDir := filepath.Join(tmp, "tests", "e2e")
		require.NoError(t, os.MkdirAll(e2eDir, 0755))
		testFile := filepath.Join(e2eDir, "test_other.py")
		require.NoError(t, os.WriteFile(testFile, []byte("def test_random(): pass\n"), 0644))

		fallbackCmds := []string{"ping", "health", "smoke", "help", "version"}
		ok, msg := runner.VerifyStoryAddedE2ETests(tmp, nil, fallbackCmds)
		assert.False(t, ok)
		assert.Contains(t, msg, "US-001 did not add any black-box E2E smoke tests")
	})
}
