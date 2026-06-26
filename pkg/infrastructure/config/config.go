package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Load loads the configuration from file (if exists), environment variables, and CLI flags.
func Load(cmd *cobra.Command) (*Config, error) {
	cfg := DefaultConfig()

	// 1. Determine config path
	configPath := ".noctifab/config.yaml"
	if cmd != nil {
		if flag := cmd.Flags().Lookup("config"); flag != nil && flag.Changed {
			configPath = flag.Value.String()
		} else if envVal, exists := os.LookupEnv("NOCTIFAB_CONFIG"); exists {
			configPath = envVal
		}
	}

	// 2. Load YAML configuration if it exists
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config: %w", err)
		}
	}

	// 2a. Load secrets.yaml (optional) from the same directory as config.yaml.
	secretsPath := filepath.Join(filepath.Dir(configPath), "secrets.yaml")
	secrets, err := LoadSecrets(secretsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load secrets file: %w", err)
	}
	applySecretsToConfig(cfg, secrets)

	// 3. Override from environment variables
	applyEnvOverrides(cfg)

	// 4. Override from CLI flags
	if cmd != nil {
		applyFlagOverrides(cfg, cmd)
	}

	// 5. Resolve VCS and LLM secrets from env keys if not already provided
	resolveSecrets(cfg)

	// 6. Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

func resolveSecrets(cfg *Config) {
	if cfg.VCS.TokenValue == "" {
		if cfg.VCS.Token != "" {
			cfg.VCS.TokenValue = cfg.VCS.Token
		} else {
			envKey := cfg.VCS.TokenEnv
			if envKey == "" {
				envKey = "GITHUB_TOKEN"
			}
			cfg.VCS.TokenValue = os.Getenv(envKey)
		}
	}

	if cfg.LLM.APIKeyValue == "" {
		if cfg.LLM.APIKey != "" {
			cfg.LLM.APIKeyValue = cfg.LLM.APIKey
		} else {
			if cfg.LLM.APIKeyEnv != "" {
				cfg.LLM.APIKeyValue = os.Getenv(cfg.LLM.APIKeyEnv)
			}
			if cfg.LLM.APIKeyValue == "" {
				switch strings.ToLower(cfg.LLM.Provider) {
				case "openai":
					cfg.LLM.APIKeyValue = os.Getenv("OPENAI_API_KEY")
				case "anthropic":
					cfg.LLM.APIKeyValue = os.Getenv("ANTHROPIC_API_KEY")
				case "gemini":
					cfg.LLM.APIKeyValue = os.Getenv("GEMINI_API_KEY")
				}
			}
		}
	}
}

// Validate checks the configuration for correctness.
func (cfg *Config) Validate() error {
	p := strings.ToLower(cfg.Storage.Provider)
	if p != "sqlite" && p != "postgres" && p != "mysql" && p != "json" {
		return fmt.Errorf("invalid storage provider: %s", cfg.Storage.Provider)
	}

	lp := strings.ToLower(cfg.LLM.Provider)
	if lp != "openai" && lp != "anthropic" && lp != "gemini" && lp != "ollama" {
		return fmt.Errorf("invalid LLM provider: %s", cfg.LLM.Provider)
	}

	vp := strings.ToLower(cfg.VCS.Provider)
	if vp != "github" && vp != "gitlab" {
		return fmt.Errorf("invalid VCS provider: %s", cfg.VCS.Provider)
	}

	if cfg.VCS.Repository == "" {
		return fmt.Errorf("VCS repository is required")
	}

	if cfg.VCS.TokenValue == "" {
		return fmt.Errorf("VCS token is required")
	}

	if cfg.LLM.APIKeyValue == "" && lp != "ollama" {
		return fmt.Errorf("LLM API key is required")
	}

	return nil
}
