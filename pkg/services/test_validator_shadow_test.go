package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectShadowTestFiles(t *testing.T) {
	t.Run("empty project path returns nil", func(t *testing.T) {
		assert.Nil(t, DetectShadowTestFiles(""))
	})

	t.Run("clean modular directory returns no collisions", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "tests", "unit"), 0755))
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "tests", "integration"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "tests", "unit", "test_store.py"), []byte("pass"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "tests", "integration", "test_server.py"), []byte("pass"), 0644))

		collisions := DetectShadowTestFiles(tmpDir)
		assert.Empty(t, collisions)
		assert.Empty(t, FormatShadowTestCollisions(collisions))
	})

	t.Run("detects duplicate test filename across root and unit directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "tests", "unit"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "tests", "test_store.py"), []byte("pass"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "tests", "unit", "test_store.py"), []byte("pass"), 0644))

		collisions := DetectShadowTestFiles(tmpDir)
		require.NotEmpty(t, collisions)
		assert.Equal(t, "tests/test_store.py", filepath.ToSlash(collisions[0].ShadowPath))
		assert.Equal(t, "tests/unit/test_store.py", filepath.ToSlash(collisions[0].Target))
		assert.Contains(t, collisions[0].Reason, "duplicate test filename")

		formatted := FormatShadowTestCollisions(collisions)
		assert.Contains(t, formatted, "Shadow test file collision detected")
		assert.Contains(t, formatted, "tests/test_store.py")
		assert.Contains(t, formatted, "delete_file")
	})

	t.Run("detects root-level test file when modular subdirectories exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "tests", "unit"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "tests", "test_legacy.py"), []byte("pass"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "tests", "unit", "test_modular.py"), []byte("pass"), 0644))

		collisions := DetectShadowTestFiles(tmpDir)
		require.NotEmpty(t, collisions)
		assert.Equal(t, "tests/test_legacy.py", filepath.ToSlash(collisions[0].ShadowPath))
		assert.Contains(t, collisions[0].Reason, "violates test partitioning")
	})

	t.Run("ValidateTask fails with clear diagnostics when shadow test files exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "tests", "unit"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "tests", "test_store.py"), []byte("def test_legacy(): assert True\n"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "tests", "unit", "test_store.py"), []byte("def test_modular(): assert True\n"), 0644))

		v := NewTestValidator(nil, false, nil, nil)
		state := &domain.State{ProjectPath: tmpDir}
		task := domain.Task{ID: "T-01", Title: "Store Task"}

		passed, log, err := v.ValidateTask(t.Context(), state, task)
		require.NoError(t, err)
		assert.False(t, passed)
		assert.Contains(t, log, "Shadow test file collision detected")
		assert.Contains(t, log, "delete_file")
	})
}
