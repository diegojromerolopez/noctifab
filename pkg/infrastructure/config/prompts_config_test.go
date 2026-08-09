package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func writePromptsTestConfig(t *testing.T, promptsSection string) *cobra.Command {
	t.Helper()
	tmpDir := t.TempDir()

	configYaml := `
config_version: "1.0"
vcs:
  repository: "myorg/myrepo"
  token: "secret:MY_VCS_TOKEN"
llm:
  provider: "openai"
  api_key: "secret:MY_API_KEY"
` + promptsSection
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYaml), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	secretsYaml := "MY_VCS_TOKEN: \"tok\"\nMY_API_KEY: \"key\"\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "secrets.yaml"), []byte(secretsYaml), 0600); err != nil {
		t.Fatalf("failed to write secrets: %v", err)
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("config", configPath, "")
	_ = cmd.Flags().Set("config", configPath)
	return cmd
}

func TestPromptsConfig(t *testing.T) {
	t.Run("when the prompts section declares valid keys the strict decoder accepts it", func(t *testing.T) {
		cmd := writePromptsTestConfig(t, `
prompts:
  tester:
    write:
      append: "Prefer table-driven tests."
  generator:
    implement:
      path: .noctifab/prompts/generator/implement.tmpl
`)
		cfg, err := Load(cmd)
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}
		if cfg.Prompts["tester"]["write"].Append != "Prefer table-driven tests." {
			t.Errorf("unexpected append: %+v", cfg.Prompts)
		}
		if cfg.Prompts["generator"]["implement"].Path != ".noctifab/prompts/generator/implement.tmpl" {
			t.Errorf("unexpected path: %+v", cfg.Prompts)
		}
	})

	t.Run("when the prompts section declares an unknown agent validation fails", func(t *testing.T) {
		cmd := writePromptsTestConfig(t, `
prompts:
  architect:
    design:
      append: "x"
`)
		_, err := Load(cmd)
		if err == nil || !strings.Contains(err.Error(), "architect") {
			t.Fatalf("expected unknown-agent error, got: %v", err)
		}
	})

	t.Run("when the prompts section declares an unknown action validation fails", func(t *testing.T) {
		cmd := writePromptsTestConfig(t, `
prompts:
  tester:
    nonexistent:
      append: "x"
`)
		_, err := Load(cmd)
		if err == nil || !strings.Contains(err.Error(), "nonexistent") {
			t.Fatalf("expected unknown-action error, got: %v", err)
		}
	})

	t.Run("when a prompt override declares an unknown field the strict decoder rejects it", func(t *testing.T) {
		cmd := writePromptsTestConfig(t, `
prompts:
  tester:
    write:
      prepend: "not a real field"
`)
		_, err := Load(cmd)
		if err == nil {
			t.Fatal("expected strict decoding error for unknown field")
		}
	})

	t.Run("when converting to prompt overrides the map round-trips", func(t *testing.T) {
		cfg := &Config{Prompts: map[string]map[string]PromptOverride{
			"tester": {"write": {Append: "extra"}},
		}}
		out := cfg.PromptOverrides()
		if out["tester"]["write"].Append != "extra" {
			t.Errorf("unexpected conversion: %+v", out)
		}
	})

	t.Run("when no prompts are configured the conversion returns nil", func(t *testing.T) {
		cfg := &Config{}
		if cfg.PromptOverrides() != nil {
			t.Error("expected nil overrides for empty prompts section")
		}
	})
}
