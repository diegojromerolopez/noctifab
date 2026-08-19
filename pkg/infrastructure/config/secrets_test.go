package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSecrets_FileNotExist(t *testing.T) {
	secrets, err := LoadSecrets("/nonexistent/path/secrets.yaml")
	if err != nil {
		t.Fatalf("expected no error for nonexistent secrets file, got: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("expected empty map, got: %v", secrets)
	}
}

func TestLoadSecrets_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	secretsPath := filepath.Join(tmpDir, "secrets.yaml")

	content := "MY_TOKEN: \"abc123\"\nANOTHER_KEY: \"xyz789\"\n"
	if err := os.WriteFile(secretsPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write secrets file: %v", err)
	}

	secrets, err := LoadSecrets(secretsPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secrets["MY_TOKEN"] != "abc123" {
		t.Errorf("expected abc123, got %s", secrets["MY_TOKEN"])
	}
	if secrets["ANOTHER_KEY"] != "xyz789" {
		t.Errorf("expected xyz789, got %s", secrets["ANOTHER_KEY"])
	}
}

func TestLoadSecrets_MalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	secretsPath := filepath.Join(tmpDir, "secrets.yaml")

	content := "invalid: [yaml: block"
	if err := os.WriteFile(secretsPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write secrets file: %v", err)
	}

	_, err := LoadSecrets(secretsPath)
	if err == nil {
		t.Error("expected error for malformed YAML secrets file")
	}
}

func TestResolveSecretRef_FoundInMap(t *testing.T) {
	secrets := map[string]string{"MY_KEY": "super-secret-value"}
	result := resolveSecretRef("secret:MY_KEY", secrets)
	if result != "super-secret-value" {
		t.Errorf("expected super-secret-value, got %s", result)
	}
}

func TestResolveSecretRef_NotFoundInMap(t *testing.T) {
	secrets := map[string]string{}
	result := resolveSecretRef("secret:MISSING_KEY", secrets)
	if result != "" {
		t.Errorf("expected empty string for missing key, got %s", result)
	}
}

func TestResolveSecretRef_NoPrefix(t *testing.T) {
	secrets := map[string]string{"FOO": "bar"}
	result := resolveSecretRef("plain-value", secrets)
	if result != "plain-value" {
		t.Errorf("expected plain-value passthrough, got %s", result)
	}
}

func TestApplySecretsToConfig(t *testing.T) {
	cfg := &Config{}
	cfg.LLM.APIKey = "secret:MY_API_KEY"
	cfg.LLM.URL = "secret:MY_LLM_URL"
	cfg.VCS.Token = "secret:MY_VCS_TOKEN"

	secrets := map[string]string{
		"MY_API_KEY":   "resolved-api-key",
		"MY_LLM_URL":   "resolved-llm-url",
		"MY_VCS_TOKEN": "resolved-vcs-token",
	}

	applySecretsToConfig(cfg, secrets)

	if cfg.LLM.APIKey != "resolved-api-key" {
		t.Errorf("LLM.APIKey: expected resolved-api-key, got %s", cfg.LLM.APIKey)
	}
	if cfg.LLM.URL != "resolved-llm-url" {
		t.Errorf("LLM.URL: expected resolved-llm-url, got %s", cfg.LLM.URL)
	}
	if cfg.VCS.Token != "resolved-vcs-token" {
		t.Errorf("VCS.Token: expected resolved-vcs-token, got %s", cfg.VCS.Token)
	}
}

func TestApplySecretsToConfig_PlainValuesUnchanged(t *testing.T) {
	cfg := &Config{}
	cfg.LLM.APIKey = "plain-api-key"
	cfg.VCS.Token = "plain-vcs-token"

	secrets := map[string]string{"plain-api-key": "should-not-be-used"}

	applySecretsToConfig(cfg, secrets)

	if cfg.LLM.APIKey != "plain-api-key" {
		t.Errorf("expected plain-api-key unchanged, got %s", cfg.LLM.APIKey)
	}
	if cfg.VCS.Token != "plain-vcs-token" {
		t.Errorf("expected plain-vcs-token unchanged, got %s", cfg.VCS.Token)
	}
}

func TestResolveProviderSpecSecret_CanonicalEnvNames(t *testing.T) {
	t.Setenv("HUGGINGFACE_API_KEY", "hf-canonical-key")
	t.Setenv("HERMES_API_KEY", "hermes-canonical-key")
	t.Setenv("NOUS_API_KEY", "should-not-be-used")
	t.Setenv("HF_TOKEN", "should-not-be-used")

	tests := []struct {
		name        string
		provider    string
		wantPrimary string
	}{
		{"huggingface uses HUGGINGFACE_API_KEY", "huggingface", "hf-canonical-key"},
		{"hermes uses HERMES_API_KEY", "hermes", "hermes-canonical-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ProviderSpec{Name: tt.provider, Provider: tt.provider}
			resolveProviderSpecSecret(p)

			if len(p.APIKeyPool) != 1 || p.APIKeyValue != tt.wantPrimary {
				t.Errorf("APIKeyPool=%v APIKeyValue=%q; want primary=%q pool of 1", p.APIKeyPool, p.APIKeyValue, tt.wantPrimary)
			}
		})
	}
}

func TestLoad_GlobalHomeSecretsFallback(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("NOCTIFAB_E2E", "true")

	homeNoctifab := filepath.Join(tempHome, ".noctifab")
	if err := os.MkdirAll(homeNoctifab, 0755); err != nil {
		t.Fatalf("failed to create home .noctifab dir: %v", err)
	}

	homeSecretsPath := filepath.Join(homeNoctifab, "secrets.yaml")
	if err := os.WriteFile(homeSecretsPath, []byte("GLOBAL_TOKEN: \"global-secret-123\"\n"), 0600); err != nil {
		t.Fatalf("failed to write home secrets: %v", err)
	}

	// Create project directory with config.yaml referencing secret:GLOBAL_TOKEN
	tempProj := t.TempDir()
	projNoctifab := filepath.Join(tempProj, ".noctifab")
	if err := os.MkdirAll(projNoctifab, 0755); err != nil {
		t.Fatalf("failed to create proj .noctifab dir: %v", err)
	}

	configContent := "config_version: \"2.0\"\nvcs:\n  provider: \"github\"\n  repository: \"test/repo\"\n  token: \"secret:GLOBAL_TOKEN\"\nllm:\n  providers:\n    - name: test\n      provider: ollama\n"
	if err := os.WriteFile(filepath.Join(projNoctifab, "config.yaml"), []byte(configContent), 0600); err != nil {
		t.Fatalf("failed to write config.yaml: %v", err)
	}

	t.Setenv("NOCTIFAB_CONFIG", filepath.Join(projNoctifab, "config.yaml"))

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.VCS.Token != "global-secret-123" {
		t.Errorf("expected global-secret-123 from HOME secrets, got %q", cfg.VCS.Token)
	}

	// Now add an unpopulated local secrets.yaml with GLOBAL_TOKEN: ""
	projSecretsPath := filepath.Join(projNoctifab, "secrets.yaml")
	if err := os.WriteFile(projSecretsPath, []byte("GLOBAL_TOKEN: \"\"\n"), 0600); err != nil {
		t.Fatalf("failed to write proj secrets.yaml: %v", err)
	}

	cfg2, err := Load(nil)
	if err != nil {
		t.Fatalf("Load with empty proj secrets failed: %v", err)
	}

	if cfg2.VCS.Token != "global-secret-123" {
		t.Errorf("expected empty local secret to NOT wipe home secret global-secret-123, got %q", cfg2.VCS.Token)
	}
}

