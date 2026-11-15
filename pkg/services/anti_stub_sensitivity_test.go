package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAntiStubSensitivity(t *testing.T) {
	t.Run("detects HTTP test suite with only 200 assertions and zero negative tests", func(t *testing.T) {
		lines := []string{
			"import requests",
			"def test_root():",
			"    r = requests.get('http://localhost:8080/')",
			"    assert r.status_code == 200",
			"def test_bucket():",
			"    r = requests.get('http://localhost:8080/b1')",
			"    assert r.status_code == 200",
			"def test_object():",
			"    r = requests.get('http://localhost:8080/b1/k1')",
			"    assert r.status_code == 200",
		}
		violations := checkTestSensitivityViolations("tests/e2e/test_api.py", lines)
		require.Len(t, violations, 1)
		assert.Equal(t, "lacks_negative_assertion_coverage", violations[0].Rule)
		assert.Contains(t, violations[0].Snippet, "3 positive status assertions")
	})

	t.Run("passes when negative assertion is included", func(t *testing.T) {
		lines := []string{
			"import requests",
			"def test_root():",
			"    r = requests.get('http://localhost:8080/')",
			"    assert r.status_code == 200",
			"def test_bucket():",
			"    r = requests.get('http://localhost:8080/b1')",
			"    assert r.status_code == 200",
			"def test_not_found():",
			"    r = requests.get('http://localhost:8080/missing')",
			"    assert r.status_code == 404",
		}
		violations := checkTestSensitivityViolations("tests/e2e/test_api.py", lines)
		assert.Empty(t, violations)
	})

	t.Run("ignores non-test files", func(t *testing.T) {
		lines := []string{
			"r = requests.get('http://localhost:8080/')",
			"assert r.status_code == 200",
		}
		violations := checkTestSensitivityViolations("src/client.py", lines)
		assert.Empty(t, violations)
	})
}
