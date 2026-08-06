package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTailLogFile(t *testing.T) {
	t.Run("tails requested number of lines from file", func(t *testing.T) {
		tmpDir := t.TempDir()
		logPath := filepath.Join(tmpDir, "test.log")

		content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
		err := os.WriteFile(logPath, []byte(content), 0644)
		require.NoError(t, err)

		lines, err := TailLogFile(logPath, 3)
		require.NoError(t, err)
		assert.Equal(t, []string{"line 3", "line 4", "line 5"}, lines)
	})

	t.Run("returns error when log file does not exist", func(t *testing.T) {
		_, err := TailLogFile("/non/existent/log/file.log", 10)
		assert.Error(t, err)
	})
}

func TestSanitizeLog(t *testing.T) {
	t.Run("redacts API keys and tokens", func(t *testing.T) {
		raw := "DEBUG: OPENAI_API_KEY=sk-1234567890abcdef1234567890abcdef fetching model list"
		sanitized := SanitizeLog(raw)
		assert.NotContains(t, sanitized, "sk-1234567890abcdef1234567890abcdef")
		assert.Contains(t, sanitized, "[REDACTED_SECRET]")
	})

	t.Run("redacts GitHub tokens", func(t *testing.T) {
		raw := "Authorization: Bearer ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
		sanitized := SanitizeLog(raw)
		assert.NotContains(t, sanitized, "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890")
		assert.Contains(t, sanitized, "[REDACTED_SECRET]")
	})

	t.Run("leaves safe logs untouched", func(t *testing.T) {
		raw := "Info: compilation succeeded in 1.2s"
		sanitized := SanitizeLog(raw)
		assert.Equal(t, raw, sanitized)
	})
}
