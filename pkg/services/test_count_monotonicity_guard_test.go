package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestCountMonotonicityGuard(t *testing.T) {
	guard := NewTestCountMonotonicityGuard()

	t.Run("CountTestsInContent correctly counts Python tests", func(t *testing.T) {
		pyCode := `
import unittest
class TestSuite(unittest.TestCase):
    def test_one(self):
        pass
    def test_two(self):
        pass
    # def test_commented(self): pass
    def helper_function(self):
        pass
`
		count := guard.CountTestsInContent("test_app.py", pyCode)
		assert.Equal(t, 2, count)
	})

	t.Run("CountTestsInContent correctly counts Go tests", func(t *testing.T) {
		goCode := `
package main
import "testing"
func TestAdd(t *testing.T) {}
func TestSubtract(t *testing.T) {}
func helper() {}
`
		count := guard.CountTestsInContent("math_test.go", goCode)
		assert.Equal(t, 2, count)
	})

	t.Run("CountTestsInContent correctly counts JS/TS tests", func(t *testing.T) {
		tsCode := `
describe("service", () => {
    it("handles login", () => {});
    test("handles logout", () => {});
    // it("commented", () => {});
});
`
		count := guard.CountTestsInContent("service.test.ts", tsCode)
		assert.Equal(t, 2, count)
	})

	t.Run("CountTestsInContent correctly counts Rust tests", func(t *testing.T) {
		rsCode := `
#[test]
fn test_encode() {}

#[test]
fn test_decode() {}
`
		count := guard.CountTestsInContent("tests/parser.rs", rsCode)
		assert.Equal(t, 2, count)
	})

	t.Run("ValidateMonotonicity succeeds when count stays same or grows", func(t *testing.T) {
		assert.NoError(t, guard.ValidateMonotonicity(5, 5, "turn 1 to 2"))
		assert.NoError(t, guard.ValidateMonotonicity(5, 7, "turn 2 to 3"))
	})

	t.Run("ValidateMonotonicity fails when count decreases", func(t *testing.T) {
		err := guard.ValidateMonotonicity(5, 4, "turn 2 to 3")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "test count monotonicity violation")
		assert.Contains(t, err.Error(), "shrank from 5 to 4 test cases")
	})

	t.Run("CountTestsInDir scans directory correctly", func(t *testing.T) {
		tempDir := t.TempDir()
		testDir := filepath.Join(tempDir, "tests")
		require.NoError(t, os.MkdirAll(testDir, 0o755))

		require.NoError(t, os.WriteFile(filepath.Join(testDir, "test_1.py"), []byte("def test_a(): pass\ndef test_b(): pass\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(testDir, "test_2.py"), []byte("def test_c(): pass\n"), 0o644))

		total, err := guard.CountTestsInDir(tempDir)
		require.NoError(t, err)
		assert.Equal(t, 3, total)
	})
}
