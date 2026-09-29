package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestContractDiagnosticGuard(t *testing.T) {
	guard := NewTestContractDiagnosticGuard()

	t.Run("flags bare dispatch assertions without msg=", func(t *testing.T) {
		pyCode := `
import unittest

class TestCommands(unittest.TestCase):
    def test_set(self):
        self.assertEqual(b"+OK\r\n", self.dispatch("SET", "k", "v"))
`
		violations := guard.ValidateDiagnosticRichness("tests/unit/test_set.py", pyCode)
		require.Len(t, violations, 1)
		assert.Equal(t, 6, violations[0].LineNumber)
		assert.Contains(t, violations[0].Error(), "lacks descriptive msg=")
	})

	t.Run("accepts assertions with rich msg= parameter", func(t *testing.T) {
		pyCode := `
import unittest

class TestCommands(unittest.TestCase):
    def test_set(self):
        self.assertEqual(
            b"+OK\r\n",
            self.dispatch("SET", "k", "v"),
            msg="[CONTRACT VIOLATION] SET failed to return +OK"
        )
`
		violations := guard.ValidateDiagnosticRichness("tests/unit/test_set.py", pyCode)
		assert.Empty(t, violations)
	})

	t.Run("FormatRichContractMessage formats expected message", func(t *testing.T) {
		msg := guard.FormatRichContractMessage("SET", "('key', 'val', 'EX', '10')", "SET with EX must set TTL and return +OK")
		assert.Contains(t, msg, "[CONTRACT VIOLATION] Operation: SET")
		assert.Contains(t, msg, "Input Args: ('key', 'val', 'EX', '10')")
		assert.Contains(t, msg, "Contract Rule: SET with EX must set TTL and return +OK")
	})
}
