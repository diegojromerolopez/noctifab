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

	t.Run("Check on valid and invalid files", func(t *testing.T) {
		validP := filepath.Join(tmpDir, "check_valid.go")
		err := os.WriteFile(validP, []byte("package main\n"), 0600)
		require.NoError(t, err)
		assert.NoError(t, v.Check(context.Background(), validP))

		invalidP := filepath.Join(tmpDir, "check_invalid.go")
		err = os.WriteFile(invalidP, []byte("package main\nfunc {"), 0600)
		require.NoError(t, err)
		assert.Error(t, v.Check(context.Background(), invalidP))
	})

	t.Run("Check on directory walks and flags invalid files", func(t *testing.T) {
		subDir := filepath.Join(tmpDir, "subdir")
		require.NoError(t, os.MkdirAll(subDir, 0755))
		validFile := filepath.Join(subDir, "ok.json")
		require.NoError(t, os.WriteFile(validFile, []byte(`{"a": 1}`), 0600))
		assert.NoError(t, v.Check(context.Background(), subDir))

		badFile := filepath.Join(subDir, "broken.json")
		require.NoError(t, os.WriteFile(badFile, []byte(`{"a": `), 0600))
		assert.Error(t, v.Check(context.Background(), subDir))
	})
}
