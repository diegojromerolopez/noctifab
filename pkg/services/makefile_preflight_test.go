package services_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakefileInspector_HasTarget(t *testing.T) {
	tempDir := t.TempDir()
	makefileContent := `
.PHONY: all clean test

all:
	@echo "building..."

test:
	@echo "testing..."

clean:
	@echo "cleaning..."
`
	err := os.WriteFile(filepath.Join(tempDir, "Makefile"), []byte(makefileContent), 0644)
	require.NoError(t, err)

	inspector := services.NewMakefileInspector()
	assert.True(t, inspector.HasTarget(tempDir, "all"))
	assert.True(t, inspector.HasTarget(tempDir, "test"))
	assert.True(t, inspector.HasTarget(tempDir, "clean"))
	assert.False(t, inspector.HasTarget(tempDir, "lint"))
	assert.False(t, inspector.HasTarget(tempDir, "build"))
}

func TestMakefileInspector_ResolveSafeMakeCommand_TargetPresent(t *testing.T) {
	tempDir := t.TempDir()
	makefileContent := "test:\n\t@echo ok\n"
	err := os.WriteFile(filepath.Join(tempDir, "Makefile"), []byte(makefileContent), 0644)
	require.NoError(t, err)

	inspector := services.NewMakefileInspector()
	cmd, err := inspector.ResolveSafeMakeCommand(tempDir, "make test")
	require.NoError(t, err)
	assert.Equal(t, "make test", cmd)
}

func TestMakefileInspector_ResolveSafeMakeCommand_MissingTestWithNativeFallback(t *testing.T) {
	tempDir := t.TempDir()
	// Makefile only has clean, but project has Cargo.toml
	makefileContent := "clean:\n\t@rm -rf target\n"
	err := os.WriteFile(filepath.Join(tempDir, "Makefile"), []byte(makefileContent), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "Cargo.toml"), []byte("[package]\nname = \"test\"\n"), 0644)
	require.NoError(t, err)

	inspector := services.NewMakefileInspector()
	cmd, err := inspector.ResolveSafeMakeCommand(tempDir, "make test")
	require.NoError(t, err)
	assert.Equal(t, "cargo test", cmd)
}

func TestMakefileInspector_ResolveSafeMakeCommand_MissingLintBypassed(t *testing.T) {
	tempDir := t.TempDir()
	makefileContent := "test:\n\t@echo ok\n"
	err := os.WriteFile(filepath.Join(tempDir, "Makefile"), []byte(makefileContent), 0644)
	require.NoError(t, err)

	inspector := services.NewMakefileInspector()
	cmd, err := inspector.ResolveSafeMakeCommand(tempDir, "make lint")
	require.NoError(t, err)
	assert.Equal(t, "", cmd, "missing optional make lint should be gracefully bypassed")
}
