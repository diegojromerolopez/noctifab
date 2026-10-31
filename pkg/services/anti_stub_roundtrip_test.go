package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAntiStubRoundTrip(t *testing.T) {
	t.Run("detects static dummy test writing and reading hardcoded 'foo', 'bar'", func(t *testing.T) {
		lines := []string{
			"def test_set_get(client):",
			"    client.set('foo', 'bar')",
			"    assert client.get('foo') == 'bar'",
		}
		violations := checkDynamicRoundTripViolations("tests/e2e/test_kv.py", lines)
		require.Len(t, violations, 1)
		assert.Equal(t, "e2e_hardcoded_static_test", violations[0].Rule)
		assert.Contains(t, violations[0].Snippet, "test_set_get")
	})

	t.Run("passes when test uses dynamic random uuid value", func(t *testing.T) {
		lines := []string{
			"import uuid",
			"def test_set_get_dynamic(client):",
			"    val = str(uuid.uuid4())",
			"    client.set('key', val)",
			"    assert client.get('key') == val",
		}
		violations := checkDynamicRoundTripViolations("tests/e2e/test_kv.py", lines)
		assert.Empty(t, violations)
	})

	t.Run("passes when test uses random int or timestamp", func(t *testing.T) {
		lines := []string{
			"import random",
			"def test_store_dynamic(client):",
			"    token = f'val_{random.randint(1000, 9999)}'",
			"    client.set('k1', token)",
			"    assert client.get('k1') == token",
		}
		violations := checkDynamicRoundTripViolations("tests/e2e/test_kv.py", lines)
		assert.Empty(t, violations)
	})

	t.Run("ignores non-test files", func(t *testing.T) {
		lines := []string{
			"client.set('foo', 'bar')",
			"assert client.get('foo') == 'bar'",
		}
		violations := checkDynamicRoundTripViolations("src/app.py", lines)
		assert.Empty(t, violations)
	})
}
