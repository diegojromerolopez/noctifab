package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractFilePathFromContext(t *testing.T) {
	assert.Equal(t, "src/main.go", extractFilePathFromContext("File src/main.go:\n```\npackage main\n```"))
	assert.Equal(t, "pkg/util.py", extractFilePathFromContext("File pkg/util.py (Diff Window +/- 10 lines):\n```diff\n"))
	assert.Equal(t, "", extractFilePathFromContext("Workspace file structure:\nsrc/main.go"))
	assert.Equal(t, "", extractFilePathFromContext("Project Manifest (go.mod):\nmodule foo"))
}

func TestDeduplicateFileAndReaderContexts(t *testing.T) {
	fileCtx := []string{
		"File src/main.go:\n```\npackage main\n```",
		"File src/model.go:\n```\ntype Model struct{}\n```",
	}

	readerCtx := []string{
		"Workspace file structure:\nsrc/main.go\nsrc/model.go\nsrc/other.go",
		"Project Manifest (go.mod):\nmodule foo",
		"File src/main.go:\n```\n// duplicate in reader\n```",
		"File src/other.go:\n```\npackage other\n```",
	}

	cleanFile, cleanReader := DeduplicateFileAndReaderContexts(fileCtx, readerCtx)

	assert.Len(t, cleanFile, 2)
	assert.Len(t, cleanReader, 3) // workspace structure, manifest, and src/other.go (main.go was skipped)

	// Verify src/main.go is NOT present in cleanReader
	for _, rc := range cleanReader {
		assert.False(t, strings.HasPrefix(rc, "File src/main.go:"), "expected duplicated src/main.go to be omitted from readerContexts")
	}
}

func TestPruneAndWindowToolOutputs(t *testing.T) {
	history := []string{
		"--- Turn 1 ---\nTool run_tests failed:\nError: assertion failure on line 42\nStack trace line 1\nStack trace line 2\nTool write_file executed successfully.",
		"--- Turn 2 ---\nTool edit_file executed successfully.\nTool run_tests executed successfully.\nAll 15 tests passed.",
	}

	pruned := PruneAndWindowToolOutputs(history, 2)
	assert.Len(t, pruned, 2)

	// Turn 1's run_tests should be marked superseded because Turn 2 has a newer run_tests
	assert.Contains(t, pruned[0], "superseded by subsequent test run")
	assert.NotContains(t, pruned[0], "Stack trace line 1")

	// Turn 2's run_tests should retain full details
	assert.Contains(t, pruned[1], "All 15 tests passed.")
}

func TestFormatWorkspaceFileTree(t *testing.T) {
	// Small repo <= 60 files
	small := []string{"main.go", "go.mod", "pkg/util.go"}
	resSmall := formatWorkspaceFileTree(small, []string{"main.go"}, 60)
	assert.Contains(t, resSmall, "Workspace file structure:\nmain.go\ngo.mod\npkg/util.go")

	// Large repo > threshold
	var large []string
	large = append(large, "Makefile", "README.md", "src/auth/login.py", "src/auth/token.py")
	for i := 0; i < 70; i++ {
		large = append(large, "tests/legacy/test_"+string(rune('a'+i%26))+".py")
	}

	resLarge := formatWorkspaceFileTree(large, []string{"src/auth/login.py"}, 50)
	assert.Contains(t, resLarge, "Workspace file structure (relevant subtrees):")
	assert.Contains(t, resLarge, "Makefile")
	assert.Contains(t, resLarge, "README.md")
	assert.Contains(t, resLarge, "src/auth/login.py")
	assert.Contains(t, resLarge, "src/auth/token.py")
	assert.Contains(t, resLarge, "tests/legacy/ (70 files omitted)")
	assert.Contains(t, resLarge, "74 total files in workspace")
}
