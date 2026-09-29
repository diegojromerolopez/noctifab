package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failSyntaxChecker struct {
	err error
}

func (f *failSyntaxChecker) Check(_ context.Context, _ string) error {
	return f.err
}

func TestWriteFileTool_RollbackOnSyntaxError(t *testing.T) {
	tmpDir := t.TempDir()
	state := &domain.State{ProjectPath: tmpDir}
	sc := &failSyntaxChecker{err: errors.New("syntax check failed: bad syntax")}
	tool := &WriteFileTool{SyntaxChecker: sc}

	// 1. New file should not exist on disk after syntax error
	newFile := filepath.Join(tmpDir, "new_file.py")
	_, err := tool.Execute(context.Background(), state, map[string]any{
		"path":    "new_file.py",
		"content": "invalid python code",
	})
	assert.Error(t, err)
	assert.NoFileExists(t, newFile)

	// 2. Existing file should be restored to original content after syntax error
	origFile := filepath.Join(tmpDir, "existing.py")
	require.NoError(t, os.WriteFile(origFile, []byte("original content"), 0644))

	_, err = tool.Execute(context.Background(), state, map[string]any{
		"path":    "existing.py",
		"content": "broken new content",
	})
	assert.Error(t, err)
	restored, readErr := os.ReadFile(origFile)
	require.NoError(t, readErr)
	assert.Equal(t, "original content", string(restored))
}

func TestEditFileTool_RollbackOnSyntaxError(t *testing.T) {
	tmpDir := t.TempDir()
	state := &domain.State{ProjectPath: tmpDir}
	sc := &failSyntaxChecker{err: errors.New("syntax check failed: bad syntax")}
	tool := &EditFileTool{SyntaxChecker: sc}

	origFile := filepath.Join(tmpDir, "mod.py")
	require.NoError(t, os.WriteFile(origFile, []byte("hello world"), 0644))

	_, err := tool.Execute(context.Background(), state, map[string]any{
		"path": "mod.py",
		"edits": []any{
			map[string]any{
				"start_line":          1,
				"end_line":            1,
				"target_content":      "hello world",
				"replacement_content": "broken world",
			},
		},
	})
	assert.Error(t, err)
	restored, readErr := os.ReadFile(origFile)
	require.NoError(t, readErr)
	assert.Equal(t, "hello world", string(restored))
}

func TestWriteFilesTool_RollbackOnSyntaxError(t *testing.T) {
	tmpDir := t.TempDir()
	state := &domain.State{ProjectPath: tmpDir}
	sc := &failSyntaxChecker{err: errors.New("syntax check failed: bad syntax")}
	tool := &WriteFilesTool{SyntaxChecker: sc}

	origFile := filepath.Join(tmpDir, "f1.py")
	require.NoError(t, os.WriteFile(origFile, []byte("orig f1"), 0644))
	newFile := filepath.Join(tmpDir, "f2.py")

	_, err := tool.Execute(context.Background(), state, map[string]any{
		"files": map[string]any{
			"f1.py": "new f1",
			"f2.py": "new f2",
		},
	})
	assert.Error(t, err)
	assert.NoFileExists(t, newFile)
	restored, readErr := os.ReadFile(origFile)
	require.NoError(t, readErr)
	assert.Equal(t, "orig f1", string(restored))
}

func TestApplyPatchTool_RollbackOnSyntaxError(t *testing.T) {
	tmpDir := t.TempDir()
	state := &domain.State{ProjectPath: tmpDir}
	sc := &failSyntaxChecker{err: errors.New("syntax check failed: bad syntax")}
	tool := &ApplyPatchTool{SyntaxChecker: sc}

	origFile := filepath.Join(tmpDir, "patch_target.py")
	require.NoError(t, os.WriteFile(origFile, []byte("line1\nline2\n"), 0644))

	patch := `--- a/patch_target.py
+++ b/patch_target.py
@@ -1,2 +1,2 @@
 line1
-line2
+broken_line
`
	_, err := tool.Execute(context.Background(), state, map[string]any{
		"patch": patch,
	})
	assert.Error(t, err)
	restored, readErr := os.ReadFile(origFile)
	require.NoError(t, readErr)
	assert.Equal(t, "line1\nline2\n", string(restored))
}
