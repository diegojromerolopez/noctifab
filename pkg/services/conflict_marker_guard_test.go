package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConflictMarkerGuard_CheckLines(t *testing.T) {
	t.Run("detects standard git conflict markers", func(t *testing.T) {
		lines := []string{
			"int add(int a, int b) {",
			"<<<<<<< HEAD",
			"    return a + b;",
			"=======",
			"    return b + a;",
			">>>>>>> branch-feat",
			"}",
		}
		violations := checkConflictMarkerViolations("src/calc.c", lines)
		require.Len(t, violations, 3)
		assert.Equal(t, "git_merge_conflict_marker", violations[0].Rule)
		assert.Equal(t, 2, violations[0].Line)
		assert.Equal(t, "<<<<<<< HEAD", violations[0].Snippet)

		assert.Equal(t, "git_merge_conflict_marker", violations[1].Rule)
		assert.Equal(t, 4, violations[1].Line)
		assert.Equal(t, "=======", violations[1].Snippet)

		assert.Equal(t, "git_merge_conflict_marker", violations[2].Rule)
		assert.Equal(t, 6, violations[2].Line)
		assert.Equal(t, ">>>>>>> branch-feat", violations[2].Snippet)
	})

	t.Run("detects diff3 base markers", func(t *testing.T) {
		lines := []string{
			"<<<<<<< HEAD",
			"||||||| merged common ancestors",
			"=======",
			">>>>>>> main",
		}
		violations := checkConflictMarkerViolations("pkg/app.go", lines)
		require.Len(t, violations, 4)
		assert.Equal(t, "||||||| merged common ancestors", violations[1].Snippet)
	})

	t.Run("ignores markdown documentation files", func(t *testing.T) {
		lines := []string{
			"# Git Conflict Guide",
			"When you see <<<<<<< HEAD, resolve it.",
			"=======",
			">>>>>>> main",
		}
		violations := checkConflictMarkerViolations("docs/git.md", lines)
		assert.Empty(t, violations)
	})

	t.Run("clean source code produces zero violations", func(t *testing.T) {
		lines := []string{
			"package main",
			"func main() { println(\"hello\") }",
		}
		violations := checkConflictMarkerViolations("main.go", lines)
		assert.Empty(t, violations)
	})
}

func TestConflictMarkerGuard_DetectInWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0755))

	cleanFile := filepath.Join(srcDir, "clean.c")
	require.NoError(t, os.WriteFile(cleanFile, []byte("int ok() { return 1; }\n"), 0644))

	conflictFile := filepath.Join(srcDir, "http.c")
	conflictContent := `
#include <stdio.h>
<<<<<<< HEAD
int parse_http() { return 1; }
=======
int parse_http() { return 2; }
>>>>>>> feat/http
`
	require.NoError(t, os.WriteFile(conflictFile, []byte(conflictContent), 0644))

	violations, err := DetectConflictMarkersInWorkspace(tmpDir)
	require.NoError(t, err)
	require.Len(t, violations, 3)
	assert.Equal(t, filepath.Join("src", "http.c"), violations[0].Path)
}
