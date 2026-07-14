package services_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteFileTool_Execute(t *testing.T) {
	t.Run("when the file exists in the sandbox workspace, it is successfully deleted", func(t *testing.T) {
		tempDir := t.TempDir()
		state := &domain.State{
			ProjectPath: tempDir,
		}

		filePath := filepath.Join(tempDir, "test.txt")
		err := os.WriteFile(filePath, []byte("hello"), 0644)
		require.NoError(t, err)

		tool := &services.DeleteFileTool{}
		args := map[string]any{
			"path": "test.txt",
		}

		out, err := tool.Execute(context.Background(), state, args)
		require.NoError(t, err)
		assert.Equal(t, "File deleted successfully", out)

		_, statErr := os.Stat(filePath)
		assert.True(t, os.IsNotExist(statErr))
	})

	t.Run("when the file does not exist, it returns 'File does not exist' and no error", func(t *testing.T) {
		tempDir := t.TempDir()
		state := &domain.State{
			ProjectPath: tempDir,
		}

		tool := &services.DeleteFileTool{}
		args := map[string]any{
			"path": "nonexistent.txt",
		}

		out, err := tool.Execute(context.Background(), state, args)
		require.NoError(t, err)
		assert.Equal(t, "File does not exist", out)
	})

	t.Run("when the path violates the sandbox jailer, it returns a sandbox violation error", func(t *testing.T) {
		tempDir := t.TempDir()
		state := &domain.State{
			ProjectPath: tempDir,
		}

		tool := &services.DeleteFileTool{}
		args := map[string]any{
			"path": "../outside.txt",
		}

		_, err := tool.Execute(context.Background(), state, args)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Sandbox violation")
	})

	t.Run("when the path targets .noctifab folder, it returns a sandbox violation error", func(t *testing.T) {
		tempDir := t.TempDir()
		state := &domain.State{
			ProjectPath: tempDir,
		}

		tool := &services.DeleteFileTool{}
		args := map[string]any{
			"path": ".noctifab/config.yaml",
		}

		_, err := tool.Execute(context.Background(), state, args)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Sandbox violation")
	})
}
