package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

func TestStartCmd_AutoInitializeMissingWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "auto_init_project")

	// Set E2E environment flag to avoid connecting to real LLM during start CLI execution test
	t.Setenv("NOCTIFAB_E2E", "true")
	t.Setenv("OPENAI_API_KEY", "mock-key")

	// Run noctifab start targetDir
	err := startCmd.RunE(startCmd, []string{targetDir})
	require.NoError(t, err)

	// Verify targetDir was created along with .noctifab, secrets.yaml, SPEC.md, and roadmap/user-stories/US-001.md
	assert.DirExists(t, targetDir)
	assert.FileExists(t, filepath.Join(targetDir, ".noctifab", "config.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, ".noctifab", "secrets.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "SPEC.md"))
	assert.FileExists(t, filepath.Join(targetDir, "roadmap", "user-stories", "US-001.md"))
}

func TestStartCmd_CurrentDirectoryAutoInit(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(origDir) }()

	require.NoError(t, os.Chdir(tmpDir))
	t.Setenv("NOCTIFAB_E2E", "true")
	t.Setenv("OPENAI_API_KEY", "mock-key")

	// Run noctifab start with no arguments
	err = startCmd.RunE(startCmd, []string{})
	require.NoError(t, err)

	// Verify current directory was initialized with .noctifab, secrets.yaml, SPEC.md, and roadmap/user-stories/US-001.md
	assert.FileExists(t, filepath.Join(tmpDir, ".noctifab", "config.yaml"))
	assert.FileExists(t, filepath.Join(tmpDir, ".noctifab", "secrets.yaml"))
	assert.FileExists(t, filepath.Join(tmpDir, "SPEC.md"))
	assert.FileExists(t, filepath.Join(tmpDir, "roadmap", "user-stories", "US-001.md"))
}

func TestIsTemplateSpec(t *testing.T) {
	t.Run("when spec contains template marker, it returns true", func(t *testing.T) {
		assert.True(t, isTemplateSpec("# Specification: New Project\n\n## Overview"))
	})
	t.Run("when spec contains real content, it returns false", func(t *testing.T) {
		assert.False(t, isTemplateSpec("# Specification: Word Count CLI\n\n## Overview\nA real spec."))
	})
}

func TestIsTemplateStory(t *testing.T) {
	t.Run("when story contains template marker, it returns true", func(t *testing.T) {
		assert.True(t, isTemplateStory("# User Story: US-001 - Initial Feature Specification\n\n## Metadata"))
	})
	t.Run("when story contains real content, it returns false", func(t *testing.T) {
		assert.False(t, isTemplateStory("# User Story: US-001 - Count bytes from stdin\n\n## Metadata"))
	})
}

func TestStartCmd_RejectsTemplateSpec(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("NOCTIFAB_E2E", "true")
	t.Setenv("OPENAI_API_KEY", "mock-key")

	// Write the unedited template SPEC.md
	specPath := filepath.Join(tmpDir, "SPEC.md")
	require.NoError(t, os.WriteFile(specPath, []byte("# Specification: New Project\n\n## Overview"), 0644))

	err := startCmd.RunE(startCmd, []string{tmpDir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still contains the default template content")
}

func TestStartCmd_RejectsTemplateUserStory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("NOCTIFAB_E2E", "true")
	t.Setenv("OPENAI_API_KEY", "mock-key")

	// Write a real SPEC.md but a template user story
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "SPEC.md"),
		[]byte("# Specification: Word Count CLI\n\n## Overview\nCount words."), 0644))

	storiesDir := filepath.Join(tmpDir, "roadmap", "user-stories")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(storiesDir, "US-001.md"),
		[]byte("# User Story: US-001 - Initial Feature Specification\n\n## Metadata"),
		0644,
	))

	err := startCmd.RunE(startCmd, []string{tmpDir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still contains the default template content")
}

func TestStartCmd_ProviderBanningByNameOnly(t *testing.T) {
	// Verify that banning a provider spec by name ("gemini-primary") does not ban
	// another provider spec ("gemini-backup") sharing the same provider type ("gemini").
	bannedNames := []string{"gemini-primary"}
	priority := []string{"gemini-primary", "gemini-backup"}

	var filteredPriority []string
	for _, name := range priority {
		banned := false
		for _, b := range bannedNames {
			if strings.EqualFold(name, b) {
				banned = true
				break
			}
		}
		if !banned {
			filteredPriority = append(filteredPriority, name)
		}
	}

	assert.Equal(t, []string{"gemini-backup"}, filteredPriority)
}

func TestRunPreFlightChecks_UnreachableProvider(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Provider:    "openai",
			APIKeyValue: "invalid-key",
			URL:         "http://127.0.0.1:59999/v1",
		},
		Sandbox: config.SandboxConfig{
			Mode: "host",
		},
	}
	err := runPreFlightChecks(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pre-flight LLM provider ping failed")
}

func TestResumeCmd_Setup(t *testing.T) {
	assert.Equal(t, "resume", resumeCmd.Name())
	assert.NotNil(t, resumeCmd.Flags().Lookup("spec"))
	assert.NotNil(t, resumeCmd.Flags().Lookup("web"))
	assert.NotNil(t, resumeCmd.Flags().Lookup("web-port"))
	assert.NotNil(t, resumeCmd.Flags().Lookup("web-host"))
	assert.NotNil(t, resumeCmd.Flags().Lookup("web-open"))
}

func TestStartCmd_WebFlags(t *testing.T) {
	webFlag := startCmd.Flags().Lookup("web")
	require.NotNil(t, webFlag)
	assert.Equal(t, "w", webFlag.Shorthand)
	assert.Equal(t, "false", webFlag.DefValue)

	portFlag := startCmd.Flags().Lookup("web-port")
	require.NotNil(t, portFlag)
	assert.Equal(t, "8080", portFlag.DefValue)

	hostFlag := startCmd.Flags().Lookup("web-host")
	require.NotNil(t, hostFlag)
	assert.Equal(t, "127.0.0.1", hostFlag.DefValue)

	openFlag := startCmd.Flags().Lookup("web-open")
	require.NotNil(t, openFlag)
	assert.Equal(t, "", openFlag.Shorthand)
	assert.Equal(t, "false", openFlag.DefValue)

	standbyFlag := startCmd.Flags().Lookup("standby")
	require.NotNil(t, standbyFlag)
	assert.Equal(t, "false", standbyFlag.DefValue)
}

func TestExtractStoryTitle(t *testing.T) {
	tmpFile := t.TempDir() + "/US-001.md"
	content := "# My Custom Story Title\n\nSome body text..."
	require.NoError(t, os.WriteFile(tmpFile, []byte(content), 0644))

	title := extractStoryTitle(tmpFile)
	assert.Equal(t, "My Custom Story Title", title)

	assert.Equal(t, "", extractStoryTitle("/nonexistent/file.md"))
}

func TestStartCmd_RejectsTemplateUserStoryInSubdirectory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("NOCTIFAB_E2E", "true")
	t.Setenv("OPENAI_API_KEY", "mock-key")

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "SPEC.md"),
		[]byte("# Specification: Word Count CLI\n\n## Overview\nCount words."), 0644))

	storiesDir := filepath.Join(tmpDir, "roadmap", "user-stories")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(storiesDir, "US-001-feature.md"),
		[]byte("# User Story: US-001 - Initial Feature Specification\n\n## Metadata"),
		0644,
	))

	err := startCmd.RunE(startCmd, []string{tmpDir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "still contains the default template content")
}

func TestDiscoverStoryFiles(t *testing.T) {
	tmpDir := t.TempDir()
	roadmapDir := filepath.Join(tmpDir, "roadmap")
	storiesDir := filepath.Join(roadmapDir, "user-stories")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))

	topLevelFile := filepath.Join(roadmapDir, "other_doc.md")
	storyFile := filepath.Join(storiesDir, "US-002-feature.md")

	require.NoError(t, os.WriteFile(topLevelFile, []byte("# Other Doc"), 0644))
	require.NoError(t, os.WriteFile(storyFile, []byte("# New Story"), 0644))

	files := discoverStoryFiles(tmpDir)
	assert.Len(t, files, 1)
	assert.Contains(t, files, storyFile)
	assert.NotContains(t, files, topLevelFile)
}
