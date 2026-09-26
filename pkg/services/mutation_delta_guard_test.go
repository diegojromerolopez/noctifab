package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMutationDeltaGuard(t *testing.T) {
	guard := NewMutationDeltaGuard()

	t.Run("flags raw scalar assertion on mutating operation lacking delta context", func(t *testing.T) {
		content := `
def test_append():
    self.assertEqual(b":8\r\n", self.dispatch("APPEND", "blob", b"\xffcd"))
`
		violations := guard.ValidateMutationDeltaAssertions("tests/test_store.py", content)
		assert.Len(t, violations, 1)
		assert.Contains(t, violations[0].Reason, "state-delta invariant corroboration")
	})

	t.Run("accepts assertion with msg parameter explaining delta contract", func(t *testing.T) {
		content := `
def test_append():
    self.assertEqual(b":8\r\n", self.dispatch("APPEND", "blob", b"\xffcd"), msg="expected return equals new string length after append delta")
`
		violations := guard.ValidateMutationDeltaAssertions("tests/test_store.py", content)
		assert.Empty(t, violations)
	})

	t.Run("accepts assertion checking len or size delta", func(t *testing.T) {
		content := `
def test_append():
    res = self.dispatch("APPEND", "blob", b"\xffcd")
    self.assertEqual(len(self.store.get("blob")), 8)
`
		violations := guard.ValidateMutationDeltaAssertions("tests/test_store.py", content)
		assert.Empty(t, violations)
	})

	t.Run("FormatDeltaInvariantTemplate formats expected template", func(t *testing.T) {
		tmpl := guard.FormatDeltaInvariantTemplate("APPEND", "key1", "b'abc'")
		assert.Contains(t, tmpl, "pre_len")
		assert.Contains(t, tmpl, "post_len - pre_len == expected_delta")
	})
}
