package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestLoadValidationProjectConfigs(t *testing.T) {
	for _, key := range []string{
		"CLAUDE_API_KEY", "GEMINI_API_KEY", "OPENAI_API_KEY", "QWENCLOUD_API_KEY",
		"OPENCODE_ZEN_API_KEY", "OPENROUTER_API_KEY", "GITHUB_TOKEN",
	} {
		t.Setenv(key, "test-key")
	}
	projectsDir := filepath.Join("..", "..", "..", "validation", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		t.Fatalf("failed to read projects directory: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		project := entry.Name()
		t.Run(project, func(t *testing.T) {
			projectDir := filepath.Join("..", "..", "..", "validation", "projects", project, ".noctifab")
			sourcePath := filepath.Join(projectDir, "config.yaml")
			data, err := os.ReadFile(sourcePath)
			if os.IsNotExist(err) {
				t.Skipf("config not present: %v", err)
			}
			if err != nil {
				t.Fatalf("read config: %v", err)
			}
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(configPath, data, 0600); err != nil {
				t.Fatalf("copy config: %v", err)
			}
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("config", configPath, "")
			_ = cmd.Flags().Set("config", configPath)

			t.Setenv("NOCTIFAB_E2E", "true")
			cfg, err := Load(cmd)
			if err != nil {
				t.Fatalf("Load(%s) failed: %v", project, err)
			}
			if cfg == nil {
				t.Fatal("Load returned nil config")
			}
		})
	}
}
