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
	cfg.Jira.Token = "secret:MY_JIRA_TOKEN"
	cfg.Jira.User = "secret:MY_JIRA_USER"
	cfg.Jira.URL = "secret:MY_JIRA_URL"

	secrets := map[string]string{
		"MY_API_KEY":    "resolved-api-key",
		"MY_LLM_URL":    "resolved-llm-url",
		"MY_VCS_TOKEN":  "resolved-vcs-token",
		"MY_JIRA_TOKEN": "resolved-jira-token",
		"MY_JIRA_USER":  "resolved-jira-user",
		"MY_JIRA_URL":   "resolved-jira-url",
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
	if cfg.Jira.Token != "resolved-jira-token" {
		t.Errorf("Jira.Token: expected resolved-jira-token, got %s", cfg.Jira.Token)
	}
	if cfg.Jira.User != "resolved-jira-user" {
		t.Errorf("Jira.User: expected resolved-jira-user, got %s", cfg.Jira.User)
	}
	if cfg.Jira.URL != "resolved-jira-url" {
		t.Errorf("Jira.URL: expected resolved-jira-url, got %s", cfg.Jira.URL)
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
