package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGenerateMinimalConfigYAML_Default(t *testing.T) {
	def := DefaultConfig()
	yamlBytes, err := GenerateMinimalConfigYAML(def)
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// 1. Verify header comment exists
	assert.Contains(t, yamlStr, "# Noctifab Project Configuration")
	assert.Contains(t, yamlStr, "inherit defaults from Noctifab runtime")

	// 2. Verify minimal fields are present
	assert.Contains(t, yamlStr, "config_version: \"2.0\"")
	assert.Contains(t, yamlStr, "provider: github")
	assert.Contains(t, yamlStr, "repository: local/repo")
	assert.Contains(t, yamlStr, "mode: host")
	assert.Contains(t, yamlStr, "test_command: go test -v ./...")
	assert.Contains(t, yamlStr, "formatter_command: go fmt ./...")
	assert.Contains(t, yamlStr, "provider: openai")
	assert.Contains(t, yamlStr, "model: latest")

	// 3. Verify internal engine boilerplates are OMITTED
	assert.NotContains(t, yamlStr, "occ:")
	assert.NotContains(t, yamlStr, "allowed_commands:")
	assert.NotContains(t, yamlStr, "package_managers:")
	assert.NotContains(t, yamlStr, "product_manager:")
	assert.NotContains(t, yamlStr, "clarification_timeout_action:")
	assert.NotContains(t, yamlStr, "max_silent_stall_duration:")
	assert.NotContains(t, yamlStr, "sovereign_rescue:")
	assert.NotContains(t, yamlStr, "workspace_cache:")

	// 4. Verify line count is minimal (< 30 lines)
	lines := strings.Split(strings.TrimSpace(yamlStr), "\n")
	assert.LessOrEqual(t, len(lines), 30, "minimal config should be concise, got %d lines", len(lines))

	// 5. Verify decoding into DefaultConfig() preserves ALL default values
	targetCfg := DefaultConfig()
	err = yaml.Unmarshal(yamlBytes, targetCfg)
	require.NoError(t, err)

	assert.Equal(t, "2.0", targetCfg.ConfigVersion)
	assert.Equal(t, "github", targetCfg.VCS.Provider)
	assert.Equal(t, "local/repo", targetCfg.VCS.Repository)
	assert.Equal(t, "auto", targetCfg.VCS.BaseBranch)
	assert.True(t, targetCfg.VCS.UseWorktrees)
	assert.Equal(t, "host", targetCfg.Sandbox.Mode)
	assert.Equal(t, "go test -v ./...", targetCfg.Sandbox.TestCommand)
	assert.Equal(t, "go fmt ./...", targetCfg.Sandbox.FormatterCommand)
	assert.Equal(t, "openai", targetCfg.LLM.Provider)
	assert.Equal(t, "latest", targetCfg.LLM.Model)
	assert.Equal(t, 0.0, targetCfg.LLM.Temperature)

	// Omitted defaults preserved
	assert.Equal(t, "sqlite", targetCfg.Storage.Provider)
	assert.Equal(t, 20, targetCfg.Storage.OCC.MaxRetries)
	assert.Equal(t, 1, targetCfg.Runtime.GetLoops())
	assert.Equal(t, "code_first", targetCfg.Agents.Architecture)
	assert.Equal(t, 2, targetCfg.Agents.ProductManager.Passes)
	assert.True(t, targetCfg.Fallback.Enabled)
	assert.True(t, targetCfg.Unblocker.Enabled)
	assert.Equal(t, "tree_sitter", targetCfg.Context.Mode)
	assert.Len(t, targetCfg.Sandbox.AllowedCommands, len(def.Sandbox.AllowedCommands))
}

func TestGenerateMinimalConfigYAML_WithProfile(t *testing.T) {
	cfg := DefaultConfig()
	err := ApplyProfile(cfg, "ollama-qwen")
	require.NoError(t, err)

	yamlBytes, err := GenerateMinimalConfigYAML(cfg)
	require.NoError(t, err)

	yamlStr := string(yamlBytes)
	assert.Contains(t, yamlStr, "provider: ollama")
	assert.Contains(t, yamlStr, "model: qwen2.5-coder:32b")
	assert.Contains(t, yamlStr, "url: http://localhost:11434/v1")
	assert.Contains(t, yamlStr, "compaction: aggressive")

	// Validate decoding
	targetCfg := DefaultConfig()
	err = yaml.Unmarshal(yamlBytes, targetCfg)
	require.NoError(t, err)
	assert.Equal(t, "ollama", targetCfg.LLM.Provider)
	assert.Equal(t, "qwen2.5-coder:32b", targetCfg.LLM.Model)
	assert.Equal(t, "http://localhost:11434/v1", targetCfg.LLM.URL)
	assert.Equal(t, "aggressive", targetCfg.Context.Compaction)
}

func TestGenerateMinimalConfigYAML_WithTemplate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Sandbox.TestCommand = "cargo test --workspace"
	cfg.Sandbox.FormatterCommand = "cargo fmt"
	cfg.Sandbox.ForbiddenPatterns = []string{`\bunsafe\s*\{`}
	cfg.Sandbox.TimeoutSeconds = 600
	linterCmd := "cargo clippy"
	maxIssues := 50
	cfg.Sandbox.Linter = LinterConfig{
		Command:   &linterCmd,
		MaxIssues: &maxIssues,
	}

	yamlBytes, err := GenerateMinimalConfigYAML(cfg)
	require.NoError(t, err)

	yamlStr := string(yamlBytes)
	assert.Contains(t, yamlStr, "test_command: cargo test --workspace")
	assert.Contains(t, yamlStr, "formatter_command: cargo fmt")
	assert.Contains(t, yamlStr, "timeout_seconds: 600")
	assert.Contains(t, yamlStr, `\bunsafe\s*\{`)
	assert.Contains(t, yamlStr, "command: cargo clippy")
	assert.Contains(t, yamlStr, "max_issues: 50")

	// Validate decoding
	targetCfg := DefaultConfig()
	err = yaml.Unmarshal(yamlBytes, targetCfg)
	require.NoError(t, err)
	assert.Equal(t, "cargo test --workspace", targetCfg.Sandbox.TestCommand)
	assert.Equal(t, "cargo fmt", targetCfg.Sandbox.FormatterCommand)
	assert.Equal(t, 600, targetCfg.Sandbox.TimeoutSeconds)
	assert.Equal(t, []string{`\bunsafe\s*\{`}, targetCfg.Sandbox.ForbiddenPatterns)
	require.NotNil(t, targetCfg.Sandbox.Linter.Command)
	assert.Equal(t, "cargo clippy", *targetCfg.Sandbox.Linter.Command)
	require.NotNil(t, targetCfg.Sandbox.Linter.MaxIssues)
	assert.Equal(t, 50, *targetCfg.Sandbox.Linter.MaxIssues)
}

func TestGenerateMinimalConfigYAML_NilSafe(t *testing.T) {
	yamlBytes, err := GenerateMinimalConfigYAML(nil)
	require.NoError(t, err)
	assert.Contains(t, string(yamlBytes), "config_version: \"2.0\"")
}

func TestGenerateMinimalConfigYAML_CustomQAAndLoops(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.QA.BuildCommand = []string{"cargo", "build"}
	cfg.Agents.Architecture = "single_pass"
	cfg.Runtime.Loops = 3
	cfg.Runtime.Loop.Count = 3
	cfg.Storage.Provider = "postgres"
	cfg.Storage.ConnString = "postgres://localhost/noctifab"

	yamlBytes, err := GenerateMinimalConfigYAML(cfg)
	require.NoError(t, err)

	yamlStr := string(yamlBytes)
	assert.Contains(t, yamlStr, "cargo")
	assert.Contains(t, yamlStr, "build")
	assert.Contains(t, yamlStr, "architecture: single_pass")
	assert.Contains(t, yamlStr, "loops: 3")
	assert.Contains(t, yamlStr, "provider: postgres")
	assert.Contains(t, yamlStr, "conn_string: postgres://localhost/noctifab")

	targetCfg := DefaultConfig()
	err = yaml.Unmarshal(yamlBytes, targetCfg)
	require.NoError(t, err)
	assert.Equal(t, []string{"cargo", "build"}, targetCfg.Agents.QA.BuildCommand)
	assert.Equal(t, "single_pass", targetCfg.Agents.Architecture)
	assert.Equal(t, 3, targetCfg.Runtime.GetLoops())
	assert.Equal(t, "postgres", targetCfg.Storage.Provider)
}

func TestWriteDefaultConfig_ProducesMinimalValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	err := WriteDefaultConfig(cfgPath)
	require.NoError(t, err)

	content, err := os.ReadFile(cfgPath)
	require.NoError(t, err)

	// Verify it's minimal (< 30 lines)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	assert.LessOrEqual(t, len(lines), 30)

	// Verify config.Load loads it cleanly
	cmd := &cobra.Command{}
	cmd.Flags().String("config", cfgPath, "")
	_ = cmd.Flags().Set("config", cfgPath)

	t.Setenv("NOCTIFAB_E2E", "true")
	t.Setenv("OPENAI_API_KEY", "test-key")
	loaded, err := Load(cmd)
	require.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Equal(t, "2.0", loaded.ConfigVersion)
	assert.Equal(t, "github", loaded.VCS.Provider)
	assert.Equal(t, "local/repo", loaded.VCS.Repository)
	assert.Equal(t, "host", loaded.Sandbox.Mode)
	assert.Equal(t, "openai", loaded.LLM.Provider)
	assert.Equal(t, "latest", loaded.LLM.Model)
	assert.Equal(t, Duration(5*time.Minute), loaded.PollInterval)
	assert.Equal(t, "sqlite", loaded.Storage.Provider)
}
