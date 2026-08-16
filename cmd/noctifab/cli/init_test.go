package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitCmd_DefaultDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	require.NoError(t, os.Chdir(tmpDir))

	// Reset WorkspaceDir
	WorkspaceDir = "."
	err = initCmd.RunE(initCmd, []string{})
	require.NoError(t, err)

	// Assert .noctifab files exist
	assert.FileExists(t, filepath.Join(tmpDir, ".noctifab", "config.yaml"))
	assert.FileExists(t, filepath.Join(tmpDir, ".noctifab", "secrets.yaml"))
	assert.FileExists(t, filepath.Join(tmpDir, ".noctifab", "data", "noctifab.db"))
	assert.FileExists(t, filepath.Join(tmpDir, ".noctifab", ".gitignore"))

	// Assert SPEC.md template exists
	assert.FileExists(t, filepath.Join(tmpDir, "SPEC.md"))
	content, err := os.ReadFile(filepath.Join(tmpDir, "SPEC.md"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "Specification: New Project")

	// Assert roadmap/US-001.md template exists
	assert.FileExists(t, filepath.Join(tmpDir, "roadmap", "US-001.md"))
	storyContent, err := os.ReadFile(filepath.Join(tmpDir, "roadmap", "US-001.md"))
	require.NoError(t, err)
	assert.Contains(t, string(storyContent), "User Story: US-001")
}

func TestInitCmd_TargetDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "new_project")

	WorkspaceDir = "."
	err := initCmd.RunE(initCmd, []string{targetDir})
	require.NoError(t, err)

	// Assert target directory was created with .noctifab, secrets.yaml, SPEC.md, and roadmap/US-001.md
	assert.DirExists(t, targetDir)
	assert.FileExists(t, filepath.Join(targetDir, ".noctifab", "config.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, ".noctifab", "secrets.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "SPEC.md"))
	// US-001.md must exist because SPEC.md was also freshly created (new project)
	assert.FileExists(t, filepath.Join(targetDir, "roadmap", "US-001.md"))
}

func TestEnsureWorkspaceInitialized_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "SPEC.md")
	require.NoError(t, os.WriteFile(specPath, []byte("# Custom Spec"), 0644))

	createdSpec, err := EnsureWorkspaceInitialized(tmpDir)
	require.NoError(t, err)
	assert.False(t, createdSpec)

	content, err := os.ReadFile(specPath)
	require.NoError(t, err)
	assert.Equal(t, "# Custom Spec", string(content))

	// US-001.md must NOT be created when SPEC.md already existed
	assert.NoFileExists(t, filepath.Join(tmpDir, "roadmap", "US-001.md"))
}

func TestInitCmd_WithProfile(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "profile_project")

	ProfileFlag = "ollama-qwen"
	defer func() { ProfileFlag = "" }()

	err := initCmd.RunE(initCmd, []string{targetDir})
	require.NoError(t, err)

	cfgContent, err := os.ReadFile(filepath.Join(targetDir, ".noctifab", "config.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(cfgContent), "ollama")
	assert.Contains(t, string(cfgContent), "qwen2.5-coder:32b")
}
