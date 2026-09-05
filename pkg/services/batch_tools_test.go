package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteFilesTool_NameAndDescription(t *testing.T) {
	tool := &WriteFilesTool{}
	assert.Equal(t, "write_files", tool.Name())
	assert.NotEmpty(t, tool.Description())
}

func TestWriteFilesTool_Execute_MapFormat(t *testing.T) {
	tmpDir := t.TempDir()
	state := &domain.State{ProjectPath: tmpDir}
	tool := &WriteFilesTool{}

	args := map[string]any{
		"files": map[string]any{
			"pkg/a.txt": "hello a",
			"pkg/b.txt": "hello b",
		},
	}

	out, err := tool.Execute(context.Background(), state, args)
	require.NoError(t, err)
	assert.Contains(t, out, "Successfully wrote 2 files")

	aContent, err := os.ReadFile(filepath.Join(tmpDir, "pkg", "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello a", string(aContent))

	bContent, err := os.ReadFile(filepath.Join(tmpDir, "pkg", "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello b", string(bContent))
}

func TestWriteFilesTool_Execute_ListFormat(t *testing.T) {
	tmpDir := t.TempDir()
	state := &domain.State{ProjectPath: tmpDir}
	tool := &WriteFilesTool{}

	args := map[string]any{
		"files": []any{
			map[string]any{"path": "src/main.go", "content": "package main"},
			map[string]any{"path": "src/util.go", "content": "package main\n\nfunc Util() {}"},
		},
	}

	out, err := tool.Execute(context.Background(), state, args)
	require.NoError(t, err)
	assert.Contains(t, out, "Successfully wrote 2 files")

	mainContent, err := os.ReadFile(filepath.Join(tmpDir, "src", "main.go"))
	require.NoError(t, err)
	assert.Equal(t, "package main", string(mainContent))
}

func TestWriteFilesTool_Execute_DirectArgsFormat(t *testing.T) {
	tmpDir := t.TempDir()
	state := &domain.State{ProjectPath: tmpDir}
	tool := &WriteFilesTool{}

	args := map[string]any{
		"file1.txt": "content 1",
		"file2.txt": "content 2",
		"reasoning": "ignorable key",
	}

	out, err := tool.Execute(context.Background(), state, args)
	require.NoError(t, err)
	assert.Contains(t, out, "Successfully wrote 2 files")
}

func TestWriteFilesTool_Execute_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	state := &domain.State{ProjectPath: tmpDir}
	tool := &WriteFilesTool{}

	// Nil args
	_, err := tool.Execute(context.Background(), state, nil)
	assert.Error(t, err)

	// Empty args
	_, err = tool.Execute(context.Background(), state, map[string]any{})
	assert.Error(t, err)

	// Invalid content type
	_, err = tool.Execute(context.Background(), state, map[string]any{
		"files": map[string]any{"invalid.txt": 12345},
	})
	assert.Error(t, err)

	// Invalid item in slice
	_, err = tool.Execute(context.Background(), state, map[string]any{
		"files": []any{"not a map"},
	})
	assert.Error(t, err)

	// Missing path in item
	_, err = tool.Execute(context.Background(), state, map[string]any{
		"files": []any{map[string]any{"content": "abc"}},
	})
	assert.Error(t, err)

	// Sandbox boundary violation
	_, err = tool.Execute(context.Background(), state, map[string]any{
		"files": map[string]any{"../escape.txt": "escaped"},
	})
	assert.Error(t, err)
}
