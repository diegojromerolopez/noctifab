package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadSecrets reads a flat key-value YAML secrets file from secretsPath.
// If the file does not exist, an empty map is returned with no error (secrets.yaml is optional).
// If the file exists but is malformed, an error is returned.
func LoadSecrets(secretsPath string) (map[string]string, error) {
	data, err := os.ReadFile(secretsPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("failed to read secrets file: %w", err)
	}

	secrets := map[string]string{}
	if err := yaml.Unmarshal(data, &secrets); err != nil {
		return nil, fmt.Errorf("failed to parse secrets file: %w", err)
	}
	return secrets, nil
}

// resolveSecretRef resolves a single string value against the secrets map.
// If the value starts with "secret:", the remainder is used as a key in the
// secrets map and the mapped value is returned (empty string if key not found).
// Otherwise the original value is returned unchanged.
func resolveSecretRef(value string, secrets map[string]string) string {
	const prefix = "secret:"
	if !strings.HasPrefix(value, prefix) {
		return value
	}
	key := strings.TrimPrefix(value, prefix)
	return secrets[key]
}

// applySecretsToConfig resolves all secret: references in the mutable string
// fields of cfg that are commonly used for credentials.
func applySecretsToConfig(cfg *Config, secrets map[string]string) {
	cfg.LLM.APIKey = resolveSecretRef(cfg.LLM.APIKey, secrets)
	cfg.LLM.URL = resolveSecretRef(cfg.LLM.URL, secrets)
	cfg.VCS.Token = resolveSecretRef(cfg.VCS.Token, secrets)
	cfg.Jira.Token = resolveSecretRef(cfg.Jira.Token, secrets)
	cfg.Jira.User = resolveSecretRef(cfg.Jira.User, secrets)
	cfg.Jira.URL = resolveSecretRef(cfg.Jira.URL, secrets)
}
