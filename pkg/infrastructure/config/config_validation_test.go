package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

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
config_version: "2.0"
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
		{"single_pass_co_synthesis", "single_pass"},
		{"co_synthesis", "single_pass"},
		{"single_pass_synthesis", "single_pass"},
		{"spcs", "single_pass"},
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
	if def.Agents.QA.Enabled || def.Agents.QA.Iterations != 1 {
		t.Errorf("unexpected QA defaults: %+v", def.Agents.QA)
	}

	tmpDir := t.TempDir()

	configYaml := `
config_version: "2.0"
agents:
  architecture: "code_first"
  generators:
    number: 4
    iterations: 6
  testers:
    number: 3
    iterations: 4
  qa:
    enabled: true
    iterations: 2
    validation_commands: ["./dist/example"]
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
	if !cfg.Agents.QA.Enabled {
		t.Error("expected Agents.QA.Enabled to be true")
	}
	if cfg.Agents.QA.Iterations != 2 {
		t.Errorf("expected Agents.QA.Iterations to be 2, got %d", cfg.Agents.QA.Iterations)
	}
}

func TestConfigVersionErrors(t *testing.T) {
	t.Setenv("NOCTIFAB_E2E", "true")
	for _, tc := range []struct {
		version string
		want    string
	}{
		{version: "1.0", want: `unsupported config_version "1.0": migrate to "2.0"`},
		{version: "3.0", want: `unsupported config_version "3.0": supported version is "2.0"`},
		{version: "", want: `unsupported config_version "": supported version is "2.0"`},
	} {
		t.Run(tc.version, func(t *testing.T) {
			cmd := writeConfigCommand(t, "config_version: \""+tc.version+"\"\n")
			_, err := Load(cmd)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestRemovedAgentRolesFailBeforeStrictDecode(t *testing.T) {
	for _, section := range []string{"agents", "roles"} {
		for _, role := range []string{"architect", "security", "performance", "docs", "devops"} {
			t.Run(section+"_"+role, func(t *testing.T) {
				cmd := writeConfigCommand(t, "config_version: \"2.0\"\n"+section+":\n  "+role+":\n    unknown: true\n")
				_, err := Load(cmd)
				want := `unsupported agent role "` + role + `": delete the ` + section + `.` + role + ` section`
				if err == nil || err.Error() != want {
					t.Fatalf("expected %q, got %v", want, err)
				}
			})
		}
	}
}

func writeConfigCommand(t *testing.T, content string) *cobra.Command {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("config", path, "")
	_ = cmd.Flags().Set("config", path)
	return cmd
}

func TestIsValidLLMProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     bool
	}{
		{"core provider", "openai", true},
		{"openrouter", "openrouter", true},
		{"grok alias", "grok", true},
		{"case insensitive", "OpenRouter", true},
		{"unknown provider", "super_ai_provider", false},
		{"empty provider", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidLLMProvider(tt.provider); got != tt.want {
				t.Errorf("IsValidLLMProvider(%q) = %v, want %v", tt.provider, got, tt.want)
			}
		})
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
			yamlContent: "config_version: \"2.0\"\nunknown_attribute: \"value\"\n",
			wantErr:     true,
			errContains: "failed to parse YAML config",
		},
		{
			name:        "Invalid agent architecture mode",
			yamlContent: "config_version: \"2.0\"\nagents:\n  architecture: \"invalid_mode\"\n",
			wantErr:     true,
			errContains: "invalid agent architecture mode",
		},
		{
			name:        "Negative generator agent count",
			yamlContent: "config_version: \"2.0\"\nagents:\n  generators:\n    number: -5\n",
			wantErr:     true,
			errContains: "agent role generators number must be non-negative",
		},
		{
			name:        "Negative tester agent iterations",
			yamlContent: "config_version: \"2.0\"\nagents:\n  testers:\n    iterations: -1\n",
			wantErr:     true,
			errContains: "agent role testers iterations must be non-negative",
		},
		{
			name:        "Invalid storage provider",
			yamlContent: "config_version: \"2.0\"\nstorage:\n  provider: \"oracle\"\n",
			wantErr:     true,
			errContains: "invalid storage provider",
		},
		{
			name:        "Invalid LLM provider name",
			yamlContent: "config_version: \"2.0\"\nllm:\n  provider: \"super_ai_provider\"\n",
			wantErr:     true,
			errContains: "invalid LLM provider",
		},
		{
			name:        "Valid registered providers accepted",
			yamlContent: "config_version: \"2.0\"\nllm:\n  provider: \"openrouter\"\n  api_key: \"sk-test\"\n",
			wantErr:     false,
		},
		{
			name:        "Valid grok provider accepted",
			yamlContent: "config_version: \"2.0\"\nllm:\n  provider: \"grok\"\n  api_key: \"sk-test\"\n",
			wantErr:     false,
		},
		{
			name:        "Valid provider in providers list accepted",
			yamlContent: "config_version: \"2.0\"\nllm:\n  providers:\n    - name: openrouter-backup\n      provider: openrouter\n      api_key: \"sk-test\"\n",
			wantErr:     false,
		},
		{
			name:        "Invalid provider in providers list rejected",
			yamlContent: "config_version: \"2.0\"\nllm:\n  providers:\n    - name: some-backup\n      provider: super_ai_provider\n      api_key: \"sk-test\"\n",
			wantErr:     true,
			errContains: "invalid LLM provider in providers list",
		},
		{
			name:        "Invalid VCS provider name",
			yamlContent: "config_version: \"2.0\"\nvcs:\n  provider: \"svn\"\n",
			wantErr:     true,
			errContains: "invalid VCS provider",
		},
		{
			name:        "Valid structured loop configuration",
			yamlContent: "config_version: \"2.0\"\nllm:\n  provider: \"ollama\"\nruntime:\n  loop:\n    count: 3\n",
			wantErr:     false,
		},
		{
			name:        "Valid matching loops and loop.count",
			yamlContent: "config_version: \"2.0\"\nllm:\n  provider: \"ollama\"\nruntime:\n  loops: 2\n  loop:\n    count: 2\n",
			wantErr:     false,
		},
		{
			name:        "Conflicting loops and loop.count rejected",
			yamlContent: "config_version: \"2.0\"\nruntime:\n  loops: 2\n  loop:\n    count: 3\n",
			wantErr:     true,
			errContains: "conflicting loop count settings",
		},
		{
			name:        "Negative runtime.loops rejected",
			yamlContent: "config_version: \"2.0\"\nruntime:\n  loops: -1\n",
			wantErr:     true,
			errContains: "runtime loops must be positive",
		},
		{
			name:        "Negative runtime.loop.count rejected",
			yamlContent: "config_version: \"2.0\"\nruntime:\n  loop:\n    count: -2\n",
			wantErr:     true,
			errContains: "runtime loop count must be positive",
		},
		{
			name:        "Negative product_manager max_user_stories rejected",
			yamlContent: "config_version: \"2.0\"\nagents:\n  product_manager:\n    max_user_stories: -1\n",
			wantErr:     true,
			errContains: "agent role product_manager max_user_stories must be non-negative",
		},
		{
			name:        "Negative product_manager passes rejected",
			yamlContent: "config_version: \"2.0\"\nagents:\n  product_manager:\n    passes: -1\n",
			wantErr:     true,
			errContains: "agent role product_manager passes must be non-negative",
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
