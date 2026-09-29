package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureLanguagePackageMarkers_Python(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Python source tree (src/commands/strings.py without __init__.py)
	srcCommands := filepath.Join(tmpDir, "src", "commands")
	require.NoError(t, os.MkdirAll(srcCommands, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(srcCommands, "strings.py"), []byte("def ping(): return 'PONG'\n"), 0644))

	// 2. Python test tree (tests/unit/test_strings.py without __init__.py)
	testsUnit := filepath.Join(tmpDir, "tests", "unit")
	require.NoError(t, os.MkdirAll(testsUnit, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(testsUnit, "test_strings.py"), []byte("def test_ping(): pass\n"), 0644))

	err := EnsureLanguagePackageMarkers(tmpDir)
	require.NoError(t, err)

	// Verify tests/ has __init__.py
	assert.FileExists(t, filepath.Join(tmpDir, "tests", "__init__.py"))
	// Verify tests/unit/ has __init__.py
	assert.FileExists(t, filepath.Join(testsUnit, "__init__.py"))
	// Verify src/commands/ has __init__.py
	assert.FileExists(t, filepath.Join(srcCommands, "__init__.py"))
}

func TestEnsureLanguagePackageMarkers_Rust(t *testing.T) {
	tmpDir := t.TempDir()

	// Create tests/common/helpers.rs without mod.rs
	commonDir := filepath.Join(tmpDir, "tests", "common")
	require.NoError(t, os.MkdirAll(commonDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(commonDir, "helpers.rs"), []byte("pub fn setup() {}\n"), 0644))

	err := EnsureLanguagePackageMarkers(tmpDir)
	require.NoError(t, err)

	// Verify tests/common/mod.rs was created
	assert.FileExists(t, filepath.Join(commonDir, "mod.rs"))
}

func TestEnsureLanguagePackageMarkers_ShellPermissions(t *testing.T) {
	tmpDir := t.TempDir()

	testsDir := filepath.Join(tmpDir, "tests", "e2e")
	require.NoError(t, os.MkdirAll(testsDir, 0755))
	scriptPath := filepath.Join(testsDir, "run_tests.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\necho OK\n"), 0644))

	rootScript := filepath.Join(tmpDir, "run.sh")
	require.NoError(t, os.WriteFile(rootScript, []byte("#!/bin/sh\necho ROOT\n"), 0644))

	err := EnsureLanguagePackageMarkers(tmpDir)
	require.NoError(t, err)

	fi, err := os.Stat(scriptPath)
	require.NoError(t, err)
	assert.True(t, fi.Mode()&0111 != 0, "expected script to have executable bit set")

	fiRoot, err := os.Stat(rootScript)
	require.NoError(t, err)
	assert.True(t, fiRoot.Mode()&0111 != 0, "expected root script to have executable bit set")
}

func TestEnsureLanguagePackageMarkers_Empty(t *testing.T) {
	assert.NoError(t, EnsureLanguagePackageMarkers(""))
	assert.NoError(t, EnsureLanguagePackageMarkers("/non/existent/dir"))
}
