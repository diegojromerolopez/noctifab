package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestLoad_Errors(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noctifab-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Create a directory at configPath so ReadFile fails
	dirPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.Mkdir(dirPath, 0755); err != nil {
		t.Fatalf("failed to create dummy dir: %v", err)
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("config", dirPath, "")
	_ = cmd.Flags().Set("config", dirPath)

	_, err = Load(cmd)
	if err == nil {
		t.Error("expected error loading directory as file")
	}

	// Create an invalid yaml file
	invalidYamlPath := filepath.Join(tmpDir, "invalid.yaml")
	if err := os.WriteFile(invalidYamlPath, []byte("invalid: [yaml: block"), 0644); err != nil {
		t.Fatalf("failed to write invalid yaml: %v", err)
	}

	cmd2 := &cobra.Command{Use: "test"}
	cmd2.Flags().String("config", invalidYamlPath, "")
	_ = cmd2.Flags().Set("config", invalidYamlPath)

	_, err = Load(cmd2)
	if err == nil {
		t.Error("expected error parsing invalid YAML")
	}
}

func TestLoad_NilCommand(t *testing.T) {
	_ = os.Setenv("NOCTIFAB_CONFIG", "nonexistent-config-path-12345.yaml")
	defer func() { _ = os.Unsetenv("NOCTIFAB_CONFIG") }()

	_, err := Load(nil)
	if err == nil {
		t.Error("expected validation error since no repo/token are set")
	}
}

func TestLoad_BadValues(t *testing.T) {
	// Bad boolean env
	_ = os.Setenv("NOCTIFAB_AUTO_COMMIT", "invalid-bool")
	cfg := &Config{}
	applyEnvOverrides(cfg)
	if cfg.AutoCommit {
		t.Error("expected AutoCommit to remain false on invalid env value")
	}
	_ = os.Unsetenv("NOCTIFAB_AUTO_COMMIT")

	// Bad int env
	_ = os.Setenv("NOCTIFAB_AGENTS_COUNT", "invalid-int")
	cfg = &Config{}
	applyEnvOverrides(cfg)
	if cfg.Agents.Generators.Number != 0 {
		t.Error("expected Concurrency to remain 0 on invalid env value")
	}
	_ = os.Unsetenv("NOCTIFAB_AGENTS_COUNT")

	// Bad duration env
	_ = os.Setenv("NOCTIFAB_INTERVAL", "invalid-duration")
	cfg = &Config{}
	applyEnvOverrides(cfg)
	if time.Duration(cfg.PollInterval) != 0 {
		t.Error("expected PollInterval to remain 0 on invalid env value")
	}
	_ = os.Unsetenv("NOCTIFAB_INTERVAL")

	// Bad float env
	_ = os.Setenv("NOCTIFAB_OCC_BACKOFF_FACTOR", "invalid-float")
	cfg = &Config{}
	applyEnvOverrides(cfg)
	if cfg.OCCBackoffFactor != 0.0 {
		t.Error("expected OCCBackoffFactor to remain 0.0 on invalid env value")
	}
	_ = os.Unsetenv("NOCTIFAB_OCC_BACKOFF_FACTOR")
}

func TestLoad_BadFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("auto-commit", "", "")
	cmd.Flags().String("agents", "", "")
	cmd.Flags().String("interval", "", "")
	cmd.Flags().String("occ-backoff-factor", "", "")

	_ = cmd.Flags().Set("auto-commit", "invalid-bool")
	_ = cmd.Flags().Set("agents", "invalid-int")
	_ = cmd.Flags().Set("interval", "invalid-duration")
	_ = cmd.Flags().Set("occ-backoff-factor", "invalid-float")

	cfg := &Config{}
	applyFlagOverrides(cfg, cmd)

	if cfg.AutoCommit {
		t.Error("expected AutoCommit to remain false on invalid flag value")
	}
	if cfg.Agents.Generators.Number != 0 {
		t.Error("expected Concurrency to remain 0 on invalid flag value")
	}
	if time.Duration(cfg.PollInterval) != 0 {
		t.Error("expected PollInterval to remain 0 on invalid flag value")
	}
	if cfg.OCCBackoffFactor != 0.0 {
		t.Error("expected OCCBackoffFactor to remain 0.0 on invalid flag value")
	}
}

func TestResolveSecrets(t *testing.T) {
	t.Run("OpenAI fallback", func(t *testing.T) {
		_ = os.Setenv("OPENAI_API_KEY", "openai-fallback")
		defer func() { _ = os.Unsetenv("OPENAI_API_KEY") }()

		cfg := &Config{}
		cfg.LLM.Provider = "openai"
		resolveSecrets(cfg)

		if cfg.LLM.APIKeyValue != "openai-fallback" {
			t.Errorf("expected openai-fallback, got %s", cfg.LLM.APIKeyValue)
		}
	})

	t.Run("Anthropic fallback", func(t *testing.T) {
		_ = os.Setenv("ANTHROPIC_API_KEY", "anthropic-fallback")
		defer func() { _ = os.Unsetenv("ANTHROPIC_API_KEY") }()

		cfg := &Config{}
		cfg.LLM.Provider = "anthropic"
		resolveSecrets(cfg)

		if cfg.LLM.APIKeyValue != "anthropic-fallback" {
			t.Errorf("expected anthropic-fallback, got %s", cfg.LLM.APIKeyValue)
		}
	})

	t.Run("Gemini fallback", func(t *testing.T) {
		_ = os.Setenv("GEMINI_API_KEY", "gemini-fallback")
		defer func() { _ = os.Unsetenv("GEMINI_API_KEY") }()

		cfg := &Config{}
		cfg.LLM.Provider = "gemini"
		resolveSecrets(cfg)

		if cfg.LLM.APIKeyValue != "gemini-fallback" {
			t.Errorf("expected gemini-fallback, got %s", cfg.LLM.APIKeyValue)
		}
	})

	t.Run("Custom LLM API Key Env", func(t *testing.T) {
		_ = os.Setenv("CUSTOM_LLM_KEY", "custom-llm-val")
		defer func() { _ = os.Unsetenv("CUSTOM_LLM_KEY") }()

		cfg := &Config{}
		cfg.LLM.APIKeyEnv = "CUSTOM_LLM_KEY"
		resolveSecrets(cfg)

		if cfg.LLM.APIKeyValue != "custom-llm-val" {
			t.Errorf("expected custom-llm-val, got %s", cfg.LLM.APIKeyValue)
		}
	})

	t.Run("Direct APIKey takes precedence over env", func(t *testing.T) {
		_ = os.Setenv("GEMINI_API_KEY", "gemini-env-val")
		defer func() { _ = os.Unsetenv("GEMINI_API_KEY") }()

		cfg := &Config{}
		cfg.LLM.Provider = "gemini"
		cfg.LLM.APIKey = "direct-api-key"
		resolveSecrets(cfg)

		if cfg.LLM.APIKeyValue != "direct-api-key" {
			t.Errorf("expected direct-api-key, got %s", cfg.LLM.APIKeyValue)
		}
	})

	t.Run("Direct Token takes precedence over env", func(t *testing.T) {
		_ = os.Setenv("GITHUB_TOKEN", "github-env-val")
		defer func() { _ = os.Unsetenv("GITHUB_TOKEN") }()

		cfg := &Config{}
		cfg.VCS.Token = "direct-token"
		resolveSecrets(cfg)

		if cfg.VCS.TokenValue != "direct-token" {
			t.Errorf("expected direct-token, got %s", cfg.VCS.TokenValue)
		}
	})
}

func TestValidate(t *testing.T) {
	baseCfg := func() *Config {
		cfg := DefaultConfig()
		cfg.VCS.Repository = "owner/repo"
		cfg.VCS.TokenValue = "vcs-token"
		cfg.LLM.APIKeyValue = "llm-key"
		return cfg
	}

	t.Run("Valid default", func(t *testing.T) {
		cfg := baseCfg()
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected valid config, got error: %v", err)
		}
	})

	t.Run("Invalid storage provider", func(t *testing.T) {
		cfg := baseCfg()
		cfg.Storage.Provider = "invalid"
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for invalid storage provider")
		}
	})

	t.Run("Invalid LLM provider", func(t *testing.T) {
		cfg := baseCfg()
		cfg.LLM.Provider = "invalid"
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for invalid LLM provider")
		}
	})

	t.Run("Invalid VCS provider", func(t *testing.T) {
		cfg := baseCfg()
		cfg.VCS.Provider = "invalid"
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for invalid VCS provider")
		}
	})

	t.Run("Missing VCS repository", func(t *testing.T) {
		t.Setenv("NOCTIFAB_E2E", "")
		t.Setenv("NOCTIFAB_VCS_REPO", "")
		cfg := baseCfg()
		cfg.VCS.Repository = ""
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for missing VCS repository")
		}
	})

	t.Run("Missing VCS token", func(t *testing.T) {
		t.Setenv("NOCTIFAB_E2E", "")
		t.Setenv("GITHUB_TOKEN", "")
		t.Setenv("NOCTIFAB_VCS_TOKEN", "")
		cfg := baseCfg()
		cfg.VCS.TokenValue = ""
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for missing VCS token")
		}
	})

	t.Run("Missing LLM API Key for non-ollama", func(t *testing.T) {
		cfg := baseCfg()
		cfg.LLM.APIKeyValue = ""
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for missing LLM API Key")
		}
	})

	t.Run("Missing LLM API Key is OK for ollama", func(t *testing.T) {
		cfg := baseCfg()
		cfg.LLM.Provider = "ollama"
		cfg.LLM.APIKeyValue = ""
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected valid config for ollama without key, got error: %v", err)
		}
	})

	t.Run("New LLM providers are valid", func(t *testing.T) {
		for _, provider := range []string{"hermes", "huggingface", "mistral", "deepseek"} {
			cfg := baseCfg()
			cfg.LLM.Provider = provider
			if err := cfg.Validate(); err != nil {
				t.Errorf("expected provider %s to be valid, got error: %v", provider, err)
			}
		}
	})
}

func TestLoad_SecretsFile(t *testing.T) {
	tmpDir := t.TempDir()

	configYaml := `
config_version: "1.0"
vcs:
  provider: "github"
  repository: "owner/repo"
  token: "secret:MY_VCS_TOKEN"
llm:
  provider: "openai"
  api_key: "secret:MY_API_KEY"
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYaml), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	secretsYaml := "MY_VCS_TOKEN: \"resolved-vcs-token\"\nMY_API_KEY: \"resolved-api-key\"\n"
	secretsPath := filepath.Join(tmpDir, "secrets.yaml")
	if err := os.WriteFile(secretsPath, []byte(secretsYaml), 0600); err != nil {
		t.Fatalf("failed to write secrets: %v", err)
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("config", configPath, "")
	_ = cmd.Flags().Set("config", configPath)

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// resolveSecrets propagates Token -> TokenValue
	if cfg.VCS.TokenValue != "resolved-vcs-token" {
		t.Errorf("VCS.TokenValue: expected resolved-vcs-token, got %q", cfg.VCS.TokenValue)
	}
	if cfg.LLM.APIKeyValue != "resolved-api-key" {
		t.Errorf("LLM.APIKeyValue: expected resolved-api-key, got %q", cfg.LLM.APIKeyValue)
	}
}

func TestOrchestratorArchitectureConfig(t *testing.T) {
	def := DefaultConfig()
	if def.Agents.Architecture != "code_first" {
		t.Errorf("expected default Architecture to be code_first, got %q", def.Agents.Architecture)
	}

	tmpDir, err := os.MkdirTemp("", "noctifab-arch-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	configYaml := `
config_version: "1.0"
agents:
  architecture: "single_pass"
vcs:
  repository: "myorg/myrepo"
  token: "secret:MY_VCS_TOKEN"
llm:
  provider: "openai"
  api_key: "secret:MY_API_KEY"
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYaml), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	secretsYaml := "MY_VCS_TOKEN: \"resolved-vcs-token\"\nMY_API_KEY: \"resolved-api-key\"\n"
	secretsPath := filepath.Join(tmpDir, "secrets.yaml")
	if err := os.WriteFile(secretsPath, []byte(secretsYaml), 0600); err != nil {
		t.Fatalf("failed to write secrets: %v", err)
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("config", configPath, "")
	_ = cmd.Flags().Set("config", configPath)

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Agents.Architecture != "single_pass" {
		t.Errorf("expected Architecture to be single_pass, got %q", cfg.Agents.Architecture)
	}
}

func TestNormalizeArchitecture(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"code_first", "code_first"},
		{"code_first_verification_loop", "code_first"},
		{"cfv", "code_first"},
		{"dfv", "code_first"},
		{"single_pass", "single_pass"},
		{"single_pass_execution", "single_pass"},
		{"spe", "single_pass"},
		{"breadth_first", "breadth_first"},
		{"breadth_first_generation", "breadth_first"},
		{"bfg", "breadth_first"},
		{"big", "breadth_first"},
		{"  BREADTH_FIRST  ", "breadth_first"},
	}

	for _, tc := range tests {
		got := NormalizeArchitecture(tc.input)
		if got != tc.expected {
			t.Errorf("NormalizeArchitecture(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestAgentRolesConfig(t *testing.T) {
	def := DefaultConfig()
	if def.Agents.Architect.Number != 1 || def.Agents.Architect.Iterations != 2 {
		t.Errorf("unexpected Architect defaults: %+v", def.Agents.Architect)
	}
	if def.Agents.QA.Number != 1 || def.Agents.QA.Iterations != 2 {
		t.Errorf("unexpected QA defaults: %+v", def.Agents.QA)
	}
	if def.Agents.Security.Number != 1 || def.Agents.Security.Iterations != 2 {
		t.Errorf("unexpected Security defaults: %+v", def.Agents.Security)
	}
	if def.Agents.Performance.Number != 1 || def.Agents.Performance.Iterations != 2 {
		t.Errorf("unexpected Performance defaults: %+v", def.Agents.Performance)
	}
	if def.Agents.Docs.Number != 1 || def.Agents.Docs.Iterations != 2 {
		t.Errorf("unexpected Docs defaults: %+v", def.Agents.Docs)
	}
	if def.Agents.DevOps.Number != 1 || def.Agents.DevOps.Iterations != 2 {
		t.Errorf("unexpected DevOps defaults: %+v", def.Agents.DevOps)
	}

	tmpDir := t.TempDir()

	configYaml := `
config_version: "1.0"
agents:
  architecture: "single_pass_execution"
  architect:
    number: 1
    iterations: 2
  generators:
    number: 4
    iterations: 6
  testers:
    number: 3
    iterations: 4
  qa:
    number: 2
    iterations: 2
  security:
    number: 1
    iterations: 2
  performance:
    number: 1
    iterations: 2
  docs:
    number: 1
    iterations: 2
  devops:
    number: 1
    iterations: 2
vcs:
  repository: "myorg/myrepo"
  token: "secret:MY_VCS_TOKEN"
llm:
  provider: "openai"
  api_key: "secret:MY_API_KEY"
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configYaml), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	secretsYaml := "MY_VCS_TOKEN: \"resolved-vcs-token\"\nMY_API_KEY: \"resolved-api-key\"\n"
	secretsPath := filepath.Join(tmpDir, "secrets.yaml")
	if err := os.WriteFile(secretsPath, []byte(secretsYaml), 0600); err != nil {
		t.Fatalf("failed to write secrets: %v", err)
	}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("config", configPath, "")
	_ = cmd.Flags().Set("config", configPath)

	cfg, err := Load(cmd)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Agents.Architect.Number != 1 || cfg.Agents.Architect.Iterations != 2 {
		t.Errorf("expected Agents.Architect number=1, iterations=2, got %+v", cfg.Agents.Architect)
	}
	if cfg.Agents.Generators.Number != 4 {
		t.Errorf("expected Agents.Generators.Number to be 4, got %d", cfg.Agents.Generators.Number)
	}
	if cfg.Agents.Generators.Iterations != 6 {
		t.Errorf("expected Agents.Generators.Iterations to be 6, got %d", cfg.Agents.Generators.Iterations)
	}
	if cfg.Agents.Testers.Number != 3 {
		t.Errorf("expected Agents.Testers.Number to be 3, got %d", cfg.Agents.Testers.Number)
	}
	if cfg.Agents.Testers.Iterations != 4 {
		t.Errorf("expected Agents.Testers.Iterations to be 4, got %d", cfg.Agents.Testers.Iterations)
	}
	if cfg.Agents.QA.Number != 2 {
		t.Errorf("expected Agents.QA.Number to be 2, got %d", cfg.Agents.QA.Number)
	}
	if cfg.Agents.QA.Iterations != 2 {
		t.Errorf("expected Agents.QA.Iterations to be 2, got %d", cfg.Agents.QA.Iterations)
	}
	if cfg.Agents.Security.Number != 1 || cfg.Agents.Security.Iterations != 2 {
		t.Errorf("expected Agents.Security number=1, iterations=2, got %+v", cfg.Agents.Security)
	}
	if cfg.Agents.Performance.Number != 1 || cfg.Agents.Performance.Iterations != 2 {
		t.Errorf("expected Agents.Performance number=1, iterations=2, got %+v", cfg.Agents.Performance)
	}
	if cfg.Agents.Docs.Number != 1 || cfg.Agents.Docs.Iterations != 2 {
		t.Errorf("expected Agents.Docs number=1, iterations=2, got %+v", cfg.Agents.Docs)
	}
	if cfg.Agents.DevOps.Number != 1 || def.Agents.DevOps.Iterations != 2 {
		t.Errorf("expected Agents.DevOps number=1, iterations=2, got %+v", cfg.Agents.DevOps)
	}
}

func TestConfigValidation_Comprehensive(t *testing.T) {
	_ = os.Setenv("NOCTIFAB_E2E", "true")
	defer func() { _ = os.Unsetenv("NOCTIFAB_E2E") }()

	tests := []struct {
		name        string
		yamlContent string
		wantErr     bool
		errContains string
	}{
		{
			name:        "Unknown/Misspelled YAML attribute",
			yamlContent: "config_version: \"1.0\"\nunknown_attribute: \"value\"\n",
			wantErr:     true,
			errContains: "failed to parse YAML config",
		},
		{
			name:        "Invalid agent architecture mode",
			yamlContent: "config_version: \"1.0\"\nagents:\n  architecture: \"invalid_mode\"\n",
			wantErr:     true,
			errContains: "invalid agent architecture mode",
		},
		{
			name:        "Negative generator agent count",
			yamlContent: "config_version: \"1.0\"\nagents:\n  generators:\n    number: -5\n",
			wantErr:     true,
			errContains: "agent role generators number must be non-negative",
		},
		{
			name:        "Negative tester agent iterations",
			yamlContent: "config_version: \"1.0\"\nagents:\n  testers:\n    iterations: -1\n",
			wantErr:     true,
			errContains: "agent role testers iterations must be non-negative",
		},
		{
			name:        "Negative max tools per response",
			yamlContent: "config_version: \"1.0\"\nagents:\n  max_tools_per_response: -1\n",
			wantErr:     true,
			errContains: "max_tools_per_response must be non-negative",
		},
		{
			name:        "Invalid storage provider",
			yamlContent: "config_version: \"1.0\"\nstorage:\n  provider: \"oracle\"\n",
			wantErr:     true,
			errContains: "invalid storage provider",
		},
		{
			name:        "Invalid LLM provider name",
			yamlContent: "config_version: \"1.0\"\nllm:\n  provider: \"super_ai_provider\"\n",
			wantErr:     true,
			errContains: "invalid LLM provider",
		},
		{
			name:        "Invalid VCS provider name",
			yamlContent: "config_version: \"1.0\"\nvcs:\n  provider: \"svn\"\n",
			wantErr:     true,
			errContains: "invalid VCS provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp("", "noctifab-schema-test-*")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			cfgFile := filepath.Join(tmpDir, "config.yaml")
			if err := os.WriteFile(cfgFile, []byte(tt.yamlContent), 0644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("config", cfgFile, "")
			_ = cmd.Flags().Set("config", cfgFile)

			_, err = Load(cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("expected error to contain %q, got %q", tt.errContains, err.Error())
			}
		})
	}
}

func TestValidationProjectsConfigs_StrictSchemaValidation(t *testing.T) {
	_ = os.Setenv("NOCTIFAB_E2E", "true")
	defer func() { _ = os.Unsetenv("NOCTIFAB_E2E") }()

	projects := []string{"calculator", "echo", "fortune", "frontpunch", "todo-cli", "wc"}

	for _, project := range projects {
		t.Run(project, func(t *testing.T) {
			configPath, err := filepath.Abs(filepath.Join("..", "..", "..", "validation", "projects", project, ".noctifab", "config.yaml"))
			if err != nil {
				t.Fatalf("failed to resolve path: %v", err)
			}

			if _, err := os.Stat(configPath); err != nil {
				t.Fatalf("config file for project %s does not exist at %s: %v", project, configPath, err)
			}

			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().String("config", configPath, "")
			_ = cmd.Flags().Set("config", configPath)

			cfg, err := Load(cmd)
			if err != nil {
				t.Errorf("validation project %s failed strict schema validation: %v", project, err)
			}
			if cfg == nil {
				t.Errorf("expected non-nil config for validation project %s", project)
			}
		})
	}
}
