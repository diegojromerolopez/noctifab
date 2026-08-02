package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestLoadValidationProjectConfigs(t *testing.T) {
	projects := []string{"calculator", "echo", "fortune", "frontpunch", "todo-cli", "wc"}
	for _, project := range projects {
		t.Run(project, func(t *testing.T) {
			projectDir := filepath.Join("..", "..", "..", "validation", "projects", project, ".noctifab")
			configPath := filepath.Join(projectDir, "config.yaml")
			if _, err := os.Stat(configPath); err != nil {
				t.Skipf("config not present: %v", err)
			}
			// secrets.yaml is gitignored and mounted only at runtime, so the
			// full load (VCS/LLM key resolution) only runs locally.
			if _, err := os.Stat(filepath.Join(projectDir, "secrets.yaml")); err != nil {
				t.Skipf("secrets.yaml not present, skipping full load: %v", err)
			}

			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("config", configPath, "")
			_ = cmd.Flags().Set("config", configPath)

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
