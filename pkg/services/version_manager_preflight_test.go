package services_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionManagerResolver_ResolveBinary(t *testing.T) {
	tempHome := t.TempDir()
	shimDir := filepath.Join(tempHome, ".asdf", "shims")
	err := os.MkdirAll(shimDir, 0755)
	require.NoError(t, err)

	// Create a mock tool binary in shimDir
	mockTool := filepath.Join(shimDir, "mycustomcompiler")
	err = os.WriteFile(mockTool, []byte("#!/bin/sh\necho custom 1.0\n"), 0755)
	require.NoError(t, err)

	resolver := &services.VersionManagerResolver{HomeDir: tempHome}
	resolved, err := resolver.ResolveBinary("mycustomcompiler")
	require.NoError(t, err)
	assert.Equal(t, mockTool, resolved)
}

func TestVersionManagerResolver_MissingBinary(t *testing.T) {
	tempHome := t.TempDir()
	resolver := &services.VersionManagerResolver{HomeDir: tempHome}
	_, err := resolver.ResolveBinary("nonexistenttool9999")
	assert.Error(t, err)
}

func TestEnsureVenvTestRunner_AlreadyInstalled(t *testing.T) {
	tempProject := t.TempDir()
	venvBin := filepath.Join(tempProject, ".venv", "bin")
	err := os.MkdirAll(venvBin, 0755)
	require.NoError(t, err)

	pytestMock := filepath.Join(venvBin, "pytest")
	err = os.WriteFile(pytestMock, []byte("#!/bin/sh\nexit 0\n"), 0755)
	require.NoError(t, err)

	err = services.EnsureVenvTestRunner(tempProject, "pytest")
	assert.NoError(t, err)
}
