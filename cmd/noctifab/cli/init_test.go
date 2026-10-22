package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitCmd_DefaultDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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

	// Assert roadmap/user-stories/US-001.md template exists
	assert.FileExists(t, filepath.Join(tmpDir, "roadmap", "user-stories", "US-001.md"))
	storyContent, err := os.ReadFile(filepath.Join(tmpDir, "roadmap", "user-stories", "US-001.md"))
	require.NoError(t, err)
	assert.Contains(t, string(storyContent), "User Story: US-001")
}

func TestInitCmd_TargetDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "new_project")

	WorkspaceDir = "."
	err := initCmd.RunE(initCmd, []string{targetDir})
	require.NoError(t, err)

	// Assert target directory was created with .noctifab, secrets.yaml, SPEC.md, and roadmap/user-stories/US-001.md
	assert.DirExists(t, targetDir)
	assert.FileExists(t, filepath.Join(targetDir, ".noctifab", "config.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, ".noctifab", "secrets.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "SPEC.md"))
	// US-001.md must exist because SPEC.md was also freshly created (new project)
	assert.FileExists(t, filepath.Join(targetDir, "roadmap", "user-stories", "US-001.md"))
}

func TestInitCmd_SkipSecretsWhenGlobalSecretsExist(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	homeNoctifab := filepath.Join(tempHome, ".noctifab")
	require.NoError(t, os.MkdirAll(homeNoctifab, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(homeNoctifab, "secrets.yaml"), []byte("OPENAI_API_KEY: \"test-key\"\n"), 0644))

	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "global_secrets_project")

	WorkspaceDir = "."
	err := initCmd.RunE(initCmd, []string{targetDir})
	require.NoError(t, err)

	// Assert target directory was created with .noctifab, config.yaml, SPEC.md, but NOT secrets.yaml
	assert.DirExists(t, targetDir)
	assert.FileExists(t, filepath.Join(targetDir, ".noctifab", "config.yaml"))
	assert.NoFileExists(t, filepath.Join(targetDir, ".noctifab", "secrets.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "SPEC.md"))
	assert.FileExists(t, filepath.Join(targetDir, "roadmap", "user-stories", "US-001.md"))
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
	assert.NoFileExists(t, filepath.Join(tmpDir, "roadmap", "user-stories", "US-001.md"))
}

func TestInitCmd_WithProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NOCTIFAB_E2E", "true")
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "profile_project")

	ProfileFlag = "ollama-qwen"
	defer func() { ProfileFlag = "" }()

	err := initCmd.RunE(initCmd, []string{targetDir})
	require.NoError(t, err)

	cfgPath := filepath.Join(targetDir, ".noctifab", "config.yaml")
	cfgContent, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, string(cfgContent), "ollama")
	assert.Contains(t, string(cfgContent), "qwen2.5-coder:32b")

	// Verify that the configuration file actually loads cleanly with config.Load
	cmd := &cobra.Command{}
	cmd.Flags().String("config", cfgPath, "")
	_ = cmd.Flags().Set("config", cfgPath)

	cfg, err := config.Load(cmd)
	require.NoError(t, err)
	assert.Equal(t, "ollama", cfg.LLM.Provider)
	assert.Equal(t, "qwen2.5-coder:32b", cfg.LLM.Model)
	assert.Equal(t, "http://localhost:11434/v1", cfg.LLM.URL)
}

func TestInitCmd_AdaptsToPythonProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NOCTIFAB_E2E", "true")
	t.Setenv("OPENAI_API_KEY", "test-key")
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "python_app")
	require.NoError(t, os.MkdirAll(filepath.Join(targetDir, ".git"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n"), 0644))

	WorkspaceDir = "."
	err := initCmd.RunE(initCmd, []string{targetDir})
	require.NoError(t, err)

	cfgPath := filepath.Join(targetDir, ".noctifab", "config.yaml")
	cmd := &cobra.Command{}
	cmd.Flags().String("config", cfgPath, "")
	_ = cmd.Flags().Set("config", cfgPath)

	cfg, err := config.Load(cmd)
	require.NoError(t, err)
	assert.Equal(t, "python3 -m unittest discover -s tests", cfg.Sandbox.TestCommand)
	assert.Empty(t, cfg.Sandbox.FormatterCommand)

	specContent, err := os.ReadFile(filepath.Join(targetDir, "SPEC.md"))
	require.NoError(t, err)
	assert.Contains(t, string(specContent), "Primary Language**: Python")
}

func TestInitCmd_AdaptsToRustProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NOCTIFAB_E2E", "true")
	t.Setenv("OPENAI_API_KEY", "test-key")
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "rust_app")
	require.NoError(t, os.MkdirAll(filepath.Join(targetDir, ".git"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "Cargo.toml"), []byte("[package]\nname = \"demo\"\n"), 0644))

	WorkspaceDir = "."
	err := initCmd.RunE(initCmd, []string{targetDir})
	require.NoError(t, err)

	cfgPath := filepath.Join(targetDir, ".noctifab", "config.yaml")
	cmd := &cobra.Command{}
	cmd.Flags().String("config", cfgPath, "")
	_ = cmd.Flags().Set("config", cfgPath)

	cfg, err := config.Load(cmd)
	require.NoError(t, err)
	assert.Equal(t, "cargo test", cfg.Sandbox.TestCommand)
	assert.Equal(t, "cargo fmt", cfg.Sandbox.FormatterCommand)

	specContent, err := os.ReadFile(filepath.Join(targetDir, "SPEC.md"))
	require.NoError(t, err)
	assert.Contains(t, string(specContent), "Primary Language**: Rust")
}

func TestInitCmd_DetectsGitOrigin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NOCTIFAB_E2E", "true")
	t.Setenv("OPENAI_API_KEY", "test-key")
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "git_origin_app")
	gitDir := filepath.Join(targetDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0755))
	configData := `[remote "origin"]
	url = git@github.com:octocat/hello-world.git
`
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte(configData), 0644))

	WorkspaceDir = "."
	err := initCmd.RunE(initCmd, []string{targetDir})
	require.NoError(t, err)

	cfgPath := filepath.Join(targetDir, ".noctifab", "config.yaml")
	cmd := &cobra.Command{}
	cmd.Flags().String("config", cfgPath, "")
	_ = cmd.Flags().Set("config", cfgPath)

	cfg, err := config.Load(cmd)
	require.NoError(t, err)
	assert.Equal(t, "octocat/hello-world", cfg.VCS.Repository)
}

func TestInitCmd_ProtectsGitInfoExclude(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "git_project")
	require.NoError(t, os.MkdirAll(filepath.Join(targetDir, ".git", "info"), 0755))

	_, err := EnsureWorkspaceInitialized(targetDir)
	require.NoError(t, err)

	excludeContent, err := os.ReadFile(filepath.Join(targetDir, ".git", "info", "exclude"))
	require.NoError(t, err)
	assert.Contains(t, string(excludeContent), ".noctifab/data/")
	assert.Contains(t, string(excludeContent), ".noctifab/logs/")
	assert.Contains(t, string(excludeContent), ".noctifab/worktrees/")
}

func TestInitCmd_MinimalConfigAndObeysDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NOCTIFAB_E2E", "true")
	t.Setenv("OPENAI_API_KEY", "test-key")
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "minimal_project")

	WorkspaceDir = "."
	err := initCmd.RunE(initCmd, []string{targetDir})
	require.NoError(t, err)

	cfgPath := filepath.Join(targetDir, ".noctifab", "config.yaml")
	assert.FileExists(t, cfgPath)

	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	content := string(data)

	// 1. Assert config is minimal: under 30 lines
	lines := strings.Split(strings.TrimSpace(content), "\n")
	assert.LessOrEqual(t, len(lines), 30, "expected minimal config under 30 lines, got %d", len(lines))

	// 2. Assert header comments exist
	assert.Contains(t, content, "# Noctifab Project Configuration")
	assert.Contains(t, content, "inherit defaults from Noctifab runtime")

	// 3. Assert boilerplate runtime/storage/agents settings are omitted
	assert.NotContains(t, content, "occ:")
	assert.NotContains(t, content, "allowed_commands:")
	assert.NotContains(t, content, "package_managers:")
	assert.NotContains(t, content, "product_manager:")
	assert.NotContains(t, content, "clarification_timeout_action:")
	assert.NotContains(t, content, "max_silent_stall_duration:")
	assert.NotContains(t, content, "sovereign_rescue:")

	// 4. Assert config.Load obeys all default values
	cmd := &cobra.Command{}
	cmd.Flags().String("config", cfgPath, "")
	_ = cmd.Flags().Set("config", cfgPath)

	cfg, err := config.Load(cmd)
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	def := config.DefaultConfig()
	assert.Equal(t, "2.0", cfg.ConfigVersion)
	assert.Equal(t, def.VCS.Provider, cfg.VCS.Provider)
	assert.Equal(t, def.VCS.BaseBranch, cfg.VCS.BaseBranch)
	assert.Equal(t, def.VCS.UseWorktrees, cfg.VCS.UseWorktrees)
	assert.Equal(t, def.Sandbox.Mode, cfg.Sandbox.Mode)
	assert.Equal(t, def.Sandbox.TestCommand, cfg.Sandbox.TestCommand)
	assert.Equal(t, def.Sandbox.FormatterCommand, cfg.Sandbox.FormatterCommand)
	assert.Equal(t, def.LLM.Provider, cfg.LLM.Provider)
	assert.Equal(t, def.LLM.Model, cfg.LLM.Model)
	assert.Equal(t, def.LLM.Temperature, cfg.LLM.Temperature)

	// Omitted fields strictly obey defaults
	assert.Equal(t, def.Storage.Provider, cfg.Storage.Provider)
	assert.Equal(t, def.Storage.OCC.MaxRetries, cfg.Storage.OCC.MaxRetries)
	assert.Equal(t, def.Runtime.GetLoops(), cfg.Runtime.GetLoops())
	assert.Equal(t, def.Agents.Architecture, cfg.Agents.Architecture)
	assert.Equal(t, def.Agents.ProductManager.Passes, cfg.Agents.ProductManager.Passes)
	assert.Equal(t, def.Agents.Generators.Number, cfg.Agents.Generators.Number)
	assert.Equal(t, def.Agents.Testers.Number, cfg.Agents.Testers.Number)
	assert.Equal(t, def.Fallback.Enabled, cfg.Fallback.Enabled)
	assert.Equal(t, def.Unblocker.Enabled, cfg.Unblocker.Enabled)
	assert.Equal(t, def.Context.Mode, cfg.Context.Mode)
	assert.Equal(t, len(def.Sandbox.AllowedCommands), len(cfg.Sandbox.AllowedCommands))
}
