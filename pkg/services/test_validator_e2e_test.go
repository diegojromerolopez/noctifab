package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedCommandSandbox struct {
	responses map[string]struct {
		out string
		err error
	}
	defaultOut string
	defaultErr error
	calls      []string
}

func (s *scriptedCommandSandbox) RunCommand(ctx context.Context, dir, cmd, stdin string) (string, error) {
	s.calls = append(s.calls, cmd)
	if resp, ok := s.responses[cmd]; ok {
		return resp.out, resp.err
	}
	return s.defaultOut, s.defaultErr
}

func TestTestValidator_E2E_RemediationTask(t *testing.T) {
	t.Parallel()

	t.Run("when task is qa-remediation and E2E fails, ValidateTask fails with E2E error", func(t *testing.T) {
		tmpDir := t.TempDir()
		// Setup fake Makefile with e2e: target so DetectE2ECommand finds "make e2e"
		makefilePath := filepath.Join(tmpDir, "Makefile")
		err := os.WriteFile(makefilePath, []byte("test:\n\tpytest\n\ne2e:\n\tpytest tests/e2e\n"), 0644)
		require.NoError(t, err)

		sb := &scriptedCommandSandbox{
			responses: map[string]struct {
				out string
				err error
			}{
				"": {
					out: "PASSED: 5 unit tests",
					err: nil,
				},
				"make e2e": {
					out: "FAILED: connection refused on port 6379",
					err: errors.New("exit code 1"),
				},
			},
		}

		v := NewTestValidator(sb, false, nil, nil)
		v.SetE2EConfig(config.E2EConfig{Mode: "native"})

		state := &domain.State{ProjectPath: tmpDir}
		task := domain.Task{
			ID:          "qa-remediation-1",
			Title:       "Remediate E2E integration failures",
			TargetFiles: []string{"server.py"},
		}

		passed, logMsg, valErr := v.ValidateTask(context.Background(), state, task)
		require.NoError(t, valErr)
		assert.False(t, passed, "expected ValidateTask to fail when E2E fails on remediation task")
		assert.Contains(t, logMsg, "E2E test validation failed")
		assert.Contains(t, logMsg, "connection refused on port 6379")
	})

	t.Run("when task is qa-remediation and both unit and E2E pass, ValidateTask succeeds", func(t *testing.T) {
		tmpDir := t.TempDir()
		makefilePath := filepath.Join(tmpDir, "Makefile")
		err := os.WriteFile(makefilePath, []byte("test:\n\tpytest\n\ne2e:\n\tpytest tests/e2e\n"), 0644)
		require.NoError(t, err)

		sb := &scriptedCommandSandbox{
			responses: map[string]struct {
				out string
				err error
			}{
				"": {
					out: "PASSED: 5 unit tests",
					err: nil,
				},
				"make e2e": {
					out: "PASSED: 2 e2e integration tests",
					err: nil,
				},
			},
		}

		v := NewTestValidator(sb, false, nil, nil)
		v.SetE2EConfig(config.E2EConfig{Mode: "native"})

		state := &domain.State{ProjectPath: tmpDir}
		task := domain.Task{
			ID:          "qa-remediation-1",
			Title:       "Remediate E2E integration failures",
			TargetFiles: []string{"server.py"},
		}

		passed, logMsg, valErr := v.ValidateTask(context.Background(), state, task)
		require.NoError(t, valErr)
		assert.True(t, passed, "expected ValidateTask to pass when both unit and E2E pass")
		assert.Contains(t, logMsg, "passed successfully")
	})
}

func TestTestValidator_E2E_TargetFiles(t *testing.T) {
	t.Parallel()

	t.Run("when task targets e2e file and E2E fails, ValidateTask fails", func(t *testing.T) {
		tmpDir := t.TempDir()
		makefilePath := filepath.Join(tmpDir, "Makefile")
		err := os.WriteFile(makefilePath, []byte("test:\n\tpytest\n\ne2e:\n\tpytest tests/e2e\n"), 0644)
		require.NoError(t, err)

		sb := &scriptedCommandSandbox{
			responses: map[string]struct {
				out string
				err error
			}{
				"": {
					out: "PASSED: 5 unit tests",
					err: nil,
				},
				"make e2e": {
					out: "FAIL: assertion error in test_server",
					err: errors.New("exit code 1"),
				},
			},
		}

		v := NewTestValidator(sb, false, nil, nil)
		v.SetE2EConfig(config.E2EConfig{Mode: "native"})

		state := &domain.State{ProjectPath: tmpDir}
		task := domain.Task{
			ID:          "US-001-T2",
			Title:       "Implement server integration suite",
			TargetFiles: []string{"tests/e2e/test_server.py"},
		}

		passed, logMsg, valErr := v.ValidateTask(context.Background(), state, task)
		require.NoError(t, valErr)
		assert.False(t, passed)
		assert.Contains(t, logMsg, "E2E test validation failed")
	})

	t.Run("when normal task does not target e2e and not final task, E2E is skipped", func(t *testing.T) {
		tmpDir := t.TempDir()
		makefilePath := filepath.Join(tmpDir, "Makefile")
		err := os.WriteFile(makefilePath, []byte("test:\n\tpytest\n\ne2e:\n\tpytest tests/e2e\n"), 0644)
		require.NoError(t, err)

		sb := &scriptedCommandSandbox{
			responses: map[string]struct {
				out string
				err error
			}{
				"": {
					out: "PASSED: 5 unit tests",
					err: nil,
				},
			},
		}

		v := NewTestValidator(sb, false, nil, nil)
		v.SetE2EConfig(config.E2EConfig{Mode: "native"})

		state := &domain.State{
			ProjectPath: tmpDir,
			Tasks: []domain.Task{
				{ID: "US-001-T1", StoryID: "US-001", Status: domain.TaskInProgress},
				{ID: "US-001-T2", StoryID: "US-001", Status: domain.TaskPending},
			},
		}
		task := domain.Task{
			ID:          "US-001-T1",
			StoryID:     "US-001",
			Title:       "Implement decoder",
			TargetFiles: []string{"src/decoder.py"},
		}

		passed, logMsg, valErr := v.ValidateTask(context.Background(), state, task)
		require.NoError(t, valErr)
		assert.True(t, passed)
		assert.NotContains(t, logMsg, "E2E")
		assert.Len(t, sb.calls, 1, "only unit tests should have been executed")
	})

	t.Run("when task is the final task in a story, E2E is validated", func(t *testing.T) {
		tmpDir := t.TempDir()
		makefilePath := filepath.Join(tmpDir, "Makefile")
		err := os.WriteFile(makefilePath, []byte("test:\n\tpytest\n\ne2e:\n\tpytest tests/e2e\n"), 0644)
		require.NoError(t, err)

		sb := &scriptedCommandSandbox{
			responses: map[string]struct {
				out string
				err error
			}{
				"": {
					out: "PASSED: 5 unit tests",
					err: nil,
				},
				"make e2e": {
					out: "PASSED: 2 e2e tests",
					err: nil,
				},
			},
		}

		v := NewTestValidator(sb, false, nil, nil)
		v.SetE2EConfig(config.E2EConfig{Mode: "native"})

		state := &domain.State{
			ProjectPath: tmpDir,
			Tasks: []domain.Task{
				{ID: "US-001-T1", StoryID: "US-001", Status: domain.TaskSuccess},
				{ID: "US-001-T2", StoryID: "US-001", Status: domain.TaskInProgress},
			},
		}
		task := domain.Task{
			ID:          "US-001-T2",
			StoryID:     "US-001",
			Title:       "Finalize story wiring",
			TargetFiles: []string{"src/main.py"},
		}

		passed, _, valErr := v.ValidateTask(context.Background(), state, task)
		require.NoError(t, valErr)
		assert.True(t, passed)
		assert.Contains(t, strings.Join(sb.calls, " "), "make e2e")
	})

	t.Run("when E2E fails on out-of-scope downstream feature, generator-tester loop ignores it and passes", func(t *testing.T) {
		tmpDir := t.TempDir()
		makefilePath := filepath.Join(tmpDir, "Makefile")
		err := os.WriteFile(makefilePath, []byte("test:\n\tpytest\n\ne2e:\n\tpytest tests/e2e\n"), 0644)
		require.NoError(t, err)

		sb := &scriptedCommandSandbox{
			responses: map[string]struct {
				out string
				err error
			}{
				"": {
					out: "PASSED: 5 unit tests",
					err: nil,
				},
				"make e2e": {
					out: `=================================== FAILURES ===================================
__________________________________ test_hset ___________________________________
FAILED tests/integration/test_hashes.py::test_hset - ConnectionRefusedError
FAILED tests/integration/test_lists.py::test_lpush - ConnectionRefusedError
=========================== 2 failed, 1 passed in 0.12s ===========================`,
					err: errors.New("exit status 1"),
				},
			},
		}

		v := NewTestValidator(sb, false, nil, nil)
		v.SetE2EConfig(config.E2EConfig{Mode: "native"})

		state := &domain.State{
			ProjectPath: tmpDir,
			Metadata: domain.StateMetadata{
				FeatureName: "US-001",
			},
			Tasks: []domain.Task{
				{ID: "US-001-T1", StoryID: "US-001", Status: domain.TaskSuccess},
				{ID: "US-001-T2", StoryID: "US-001", Status: domain.TaskInProgress},
			},
		}
		// Active task is for Connection/Ping (US-001), touching connection.py
		task := domain.Task{
			ID:          "US-001-T2",
			StoryID:     "US-001",
			Title:       "Implement Redis Ping and Connection",
			TargetFiles: []string{"src/commands/connection.py"},
		}

		passed, logMsg, valErr := v.ValidateTask(context.Background(), state, task)
		require.NoError(t, valErr)
		// Out-of-scope failures in hashes and lists must be ignored!
		assert.True(t, passed, "expected out-of-scope E2E failure to be ignored in generator-tester loop")
		assert.Contains(t, logMsg, "passed successfully")
	})

	t.Run("when E2E fails on in-scope active feature, generator-tester loop enforces failure", func(t *testing.T) {
		tmpDir := t.TempDir()
		makefilePath := filepath.Join(tmpDir, "Makefile")
		err := os.WriteFile(makefilePath, []byte("test:\n\tpytest\n\ne2e:\n\tpytest tests/e2e\n"), 0644)
		require.NoError(t, err)

		sb := &scriptedCommandSandbox{
			responses: map[string]struct {
				out string
				err error
			}{
				"": {
					out: "PASSED: 5 unit tests",
					err: nil,
				},
				"make e2e": {
					out: `=================================== FAILURES ===================================
__________________________________ test_ping ___________________________________
FAILED tests/integration/test_connection.py::test_ping - AssertionError: PONG != ERR
=========================== 1 failed, 2 passed in 0.10s ===========================`,
					err: errors.New("exit status 1"),
				},
			},
		}

		v := NewTestValidator(sb, false, nil, nil)
		v.SetE2EConfig(config.E2EConfig{Mode: "native"})

		state := &domain.State{
			ProjectPath: tmpDir,
			Metadata: domain.StateMetadata{
				FeatureName: "US-001",
			},
			Tasks: []domain.Task{
				{ID: "US-001-T1", StoryID: "US-001", Status: domain.TaskSuccess},
				{ID: "US-001-T2", StoryID: "US-001", Status: domain.TaskInProgress},
			},
		}
		// Active task is for Connection/Ping (US-001)
		task := domain.Task{
			ID:          "US-001-T2",
			StoryID:     "US-001",
			Title:       "Implement Redis Ping and Connection",
			TargetFiles: []string{"src/commands/connection.py", "tests/integration/test_connection.py"},
		}

		passed, logMsg, valErr := v.ValidateTask(context.Background(), state, task)
		require.NoError(t, valErr)
		assert.False(t, passed, "expected in-scope E2E failure to be enforced")
		assert.Contains(t, logMsg, "E2E test validation failed")
		assert.Contains(t, logMsg, "test_connection.py")
	})
}
