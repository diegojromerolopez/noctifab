package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyntaxValidator(t *testing.T) {
	v := NewSyntaxValidator()
	tmpDir := t.TempDir()

	t.Run("valid Go file passes", func(t *testing.T) {
		p := filepath.Join(tmpDir, "valid.go")
		err := os.WriteFile(p, []byte("package main\n\nfunc main() {}\n"), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		assert.Nil(t, violation)
	})

	t.Run("invalid Go file fails with violation", func(t *testing.T) {
		p := filepath.Join(tmpDir, "invalid.go")
		err := os.WriteFile(p, []byte("package main\n\nfunc main( {\n"), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		require.NotNil(t, violation)
		assert.Contains(t, violation.Message, "Go syntax error")
	})

	t.Run("valid JSON file passes", func(t *testing.T) {
		p := filepath.Join(tmpDir, "valid.json")
		err := os.WriteFile(p, []byte(`{"key": "value", "num": 42}`), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		assert.Nil(t, violation)
	})

	t.Run("invalid JSON file fails with violation", func(t *testing.T) {
		p := filepath.Join(tmpDir, "invalid.json")
		err := os.WriteFile(p, []byte(`{"key": "value", trailing`), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		require.NotNil(t, violation)
		assert.Contains(t, violation.Message, "JSON syntax error")
	})

	t.Run("valid Python file passes if python3 available", func(t *testing.T) {
		p := filepath.Join(tmpDir, "valid.py")
		err := os.WriteFile(p, []byte("def hello():\n    return 42\n"), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		assert.Nil(t, violation)
	})

	t.Run("invalid Python file fails if python3 available", func(t *testing.T) {
		p := filepath.Join(tmpDir, "invalid.py")
		err := os.WriteFile(p, []byte("def hello(\n    return 42\n"), 0600)
		require.NoError(t, err)

		violation, err := v.ValidateFile(context.Background(), p)
		require.NoError(t, err)
		if violation != nil {
			assert.Contains(t, violation.Message, "Python syntax error")
		}
	})
}
