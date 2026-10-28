package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// GlobalSecretsPath returns the default location of the global user secrets file ($HOME/.noctifab/secrets.yaml).
// If the user home directory cannot be determined, it returns an empty string.
func GlobalSecretsPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return ""
	}
	return filepath.Join(homeDir, ".noctifab", "secrets.yaml")
}

// HasGlobalSecrets returns true if $HOME/.noctifab/secrets.yaml exists on the host filesystem.
func HasGlobalSecrets() bool {
	p := GlobalSecretsPath()
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

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

// isAPIKeyName returns true if s matches or ends with "API_KEY" or "API_KEYS" (case-insensitive).
func isAPIKeyName(s string) bool {
	upper := strings.ToUpper(strings.TrimSpace(s))
	return upper == "API_KEY" || upper == "API_KEYS" ||
		strings.HasSuffix(upper, "_API_KEY") || strings.HasSuffix(upper, "_API_KEYS")
}

// resolveSecretRef resolves a single string value against the secrets map and OS environment.
// If the value starts with "secret:", the remainder is used as a secret key name.
// If the value does not start with "secret:", but ends with "API_KEY" or "API_KEYS" (case-insensitive),
// it is also treated as a secret key reference.
// The key is looked up in the secrets map first. If not found or empty, it falls back to
// os.Getenv(key) only when the key ends with "API_KEY" or "API_KEYS".
// Otherwise, plain literal values are returned unchanged.
func resolveSecretRef(value string, secrets map[string]string) string {
	const prefix = "secret:"
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}

	isExplicitSecret := strings.HasPrefix(trimmed, prefix)
	var key string
	if isExplicitSecret {
		key = strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	} else if isAPIKeyName(trimmed) {
		key = trimmed
	} else {
		return value
	}

	if val, ok := secrets[key]; ok && strings.TrimSpace(val) != "" {
		return val
	}

	if isAPIKeyName(key) {
		if envVal := os.Getenv(key); envVal != "" {
			return envVal
		}
	}

	return ""
}

// resolveSecretKeys resolves secret key names against secrets map and OS environment.
// Supports comma-separated keys and automatic "S" suffix fallback (e.g. OPENCODE_API_KEY -> OPENCODE_API_KEYS).
func resolveSecretKeys(keyNames []string, fallbackEnv string, secrets map[string]string) ([]string, string) {
	var pool []string
	seen := make(map[string]bool)

	addVal := func(val string) {
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		if strings.Contains(val, ",") {
			for _, part := range strings.Split(val, ",") {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" && !seen[trimmed] {
					seen[trimmed] = true
					pool = append(pool, trimmed)
				}
			}
		} else if !seen[val] {
			seen[val] = true
			pool = append(pool, val)
		}
	}

	for _, name := range keyNames {
		if val, ok := secrets[name]; ok && val != "" {
			addVal(val)
		} else if val := os.Getenv(name); val != "" {
			addVal(val)
		}

		if !strings.HasSuffix(name, "S") && !strings.HasSuffix(name, "s") {
			pluralName := name + "S"
			if val, ok := secrets[pluralName]; ok && val != "" {
				addVal(val)
			} else if val := os.Getenv(pluralName); val != "" {
				addVal(val)
			}
		}
	}

	if len(pool) == 0 && fallbackEnv != "" {
		if val, ok := secrets[fallbackEnv]; ok && val != "" {
			addVal(val)
		} else if val := os.Getenv(fallbackEnv); val != "" {
			addVal(val)
		}
		if !strings.HasSuffix(fallbackEnv, "S") && !strings.HasSuffix(fallbackEnv, "s") {
			pluralFallback := fallbackEnv + "S"
			if val, ok := secrets[pluralFallback]; ok && val != "" {
				addVal(val)
			} else if val := os.Getenv(pluralFallback); val != "" {
				addVal(val)
			}
		}
	}

	primary := ""
	if len(pool) > 0 {
		primary = pool[0]
	}
	return pool, primary
}

// applySecretsToConfig resolves all secret: references in the mutable string
// fields of cfg that are commonly used for credentials.
func applySecretsToConfig(cfg *Config, secrets map[string]string) {
	cfg.LLM.APIKey = resolveSecretRef(cfg.LLM.APIKey, secrets)
	cfg.LLM.URL = resolveSecretRef(cfg.LLM.URL, secrets)
	cfg.VCS.Token = resolveSecretRef(cfg.VCS.Token, secrets)

	for i := range cfg.LLMs {
		cfg.LLMs[i].APIKey = resolveSecretRef(cfg.LLMs[i].APIKey, secrets)
		cfg.LLMs[i].URL = resolveSecretRef(cfg.LLMs[i].URL, secrets)
	}

	for i := range cfg.LLM.Providers {
		cfg.LLM.Providers[i].APIKey = resolveSecretRef(cfg.LLM.Providers[i].APIKey, secrets)
		cfg.LLM.Providers[i].URL = resolveSecretRef(cfg.LLM.Providers[i].URL, secrets)

		var keySources []string
		if len(cfg.LLM.Providers[i].APIKeys) > 0 {
			keySources = append(keySources, cfg.LLM.Providers[i].APIKeys...)
		}
		defaultEnv := strings.ToUpper(cfg.LLM.Providers[i].Provider) + "_API_KEY"

		pool, primary := resolveSecretKeys(keySources, defaultEnv, secrets)
		if len(pool) > 0 {
			cfg.LLM.Providers[i].APIKeyPool = pool
			cfg.LLM.Providers[i].APIKeyValue = primary
		}
	}
}
