package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectTestCouplingViolations(t *testing.T) {
	t.Run("when projectPath is empty or has no test directory, returns nil", func(t *testing.T) {
		assert.Nil(t, DetectTestCouplingViolations(""))
		tempDir := t.TempDir()
		assert.Nil(t, DetectTestCouplingViolations(tempDir))
	})

	t.Run("when test suites are properly hermetic, returns no violations", func(t *testing.T) {
		tempDir := t.TempDir()
		unitDir := filepath.Join(tempDir, "tests", "unit")
		e2eDir := filepath.Join(tempDir, "tests", "e2e")
		require.NoError(t, os.MkdirAll(unitDir, 0755))
		require.NoError(t, os.MkdirAll(e2eDir, 0755))

		// Clean unit test importing only from production src
		require.NoError(t, os.WriteFile(filepath.Join(unitDir, "test_store.py"), []byte(`
import unittest
from src.store import Store

class TestStore(unittest.TestCase):
    def test_basic(self):
        s = Store()
        self.assertIsNotNone(s)
`), 0644))

		// Clean e2e test running independently
		require.NoError(t, os.WriteFile(filepath.Join(e2eDir, "run_tests.py"), []byte(`
import socket
print("e2e passed")
`), 0644))

		violations := DetectTestCouplingViolations(tempDir)
		assert.Empty(t, violations)
		assert.Empty(t, FormatTestCouplingViolations(violations))
	})

	t.Run("when unit test imports from tests.e2e, flags cross-layer coupling", func(t *testing.T) {
		tempDir := t.TempDir()
		unitDir := filepath.Join(tempDir, "tests", "unit")
		require.NoError(t, os.MkdirAll(unitDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(unitDir, "test_e2e_protocol.py"), []byte(`
import unittest
from tests.e2e.run_tests import frame

class TestProtocol(unittest.TestCase):
    def test_frame(self):
        self.assertEqual(frame("PING"), b"*1\r\n$4\r\nPING\r\n")
`), 0644))

		violations := DetectTestCouplingViolations(tempDir)
		require.Len(t, violations, 1)
		assert.Equal(t, "unit", violations[0].SourceCategory)
		assert.Equal(t, "e2e", violations[0].ImportedCategory)
		assert.Contains(t, violations[0].FilePath, "test_e2e_protocol.py")
		assert.Contains(t, violations[0].MatchedLine, "from tests.e2e.run_tests import frame")

		formatted := FormatTestCouplingViolations(violations)
		assert.Contains(t, formatted, "Cross-layer test coupling violation detected (1 violation(s)):")
		assert.Contains(t, formatted, "Hermetic Test Isolation Mandate")
		assert.Contains(t, formatted, "[unit -> e2e]")
	})

	t.Run("when unit test imports from integration or JS relative path, flags violations", func(t *testing.T) {
		tempDir := t.TempDir()
		unitDir := filepath.Join(tempDir, "tests", "unit")
		require.NoError(t, os.MkdirAll(unitDir, 0755))

		// Python relative import
		require.NoError(t, os.WriteFile(filepath.Join(unitDir, "test_helper.py"), []byte(`
from ..integration.fixtures import db_session
`), 0644))

		// TS relative import
		require.NoError(t, os.WriteFile(filepath.Join(unitDir, "test_client.ts"), []byte(`
import { harness } from '../e2e/harness';
`), 0644))

		violations := DetectTestCouplingViolations(tempDir)
		require.Len(t, violations, 2)
	})

	t.Run("when integration test imports from unit or e2e, flags violations", func(t *testing.T) {
		tempDir := t.TempDir()
		integDir := filepath.Join(tempDir, "tests", "integration")
		require.NoError(t, os.MkdirAll(integDir, 0755))

		require.NoError(t, os.WriteFile(filepath.Join(integDir, "test_server.py"), []byte(`
import unittest
from tests.unit.test_store import FakeStore
import tests.e2e.client
`), 0644))

		violations := DetectTestCouplingViolations(tempDir)
		require.Len(t, violations, 2)
	})

	t.Run("ValidateTask fails with clear diagnostics when cross-layer coupling exists", func(t *testing.T) {
		tempDir := t.TempDir()
		unitDir := filepath.Join(tempDir, "tests", "unit")
		require.NoError(t, os.MkdirAll(unitDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(unitDir, "test_e2e_protocol.py"), []byte(`
import unittest
from tests.e2e.run_tests import frame
`), 0644))

		v := NewTestValidator(nil, false, nil, nil)
		state := &domain.State{ProjectPath: tempDir}
		task := domain.Task{ID: "T-02", Title: "Protocol Task"}

		passed, log, err := v.ValidateTask(t.Context(), state, task)
		require.NoError(t, err)
		assert.False(t, passed)
		assert.Contains(t, log, "Cross-layer test coupling violation detected")
		assert.Contains(t, log, "Hermetic Test Isolation Mandate")
		assert.Contains(t, log, "tests/unit/test_e2e_protocol.py")
	})
}
