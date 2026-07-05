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
		if len(cfg.LLMs) > 0 {
			cfg.LLM = cfg.LLMs[0]
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

	resolveSingleLLMSecret(&cfg.LLM)
	for i := range cfg.LLMs {
		resolveSingleLLMSecret(&cfg.LLMs[i])
	}
}

func resolveSingleLLMSecret(llm *LLMConfig) {
	if llm.APIKeyValue == "" {
		if llm.APIKey != "" && !strings.HasPrefix(llm.APIKey, "secret:") {
			llm.APIKeyValue = llm.APIKey
		} else {
			if llm.APIKeyEnv != "" {
				llm.APIKeyValue = os.Getenv(llm.APIKeyEnv)
			}
			if llm.APIKeyValue == "" {
				switch strings.ToLower(llm.Provider) {
				case "openai":
					llm.APIKeyValue = os.Getenv("OPENAI_API_KEY")
				case "anthropic":
					llm.APIKeyValue = os.Getenv("ANTHROPIC_API_KEY")
				case "gemini":
					llm.APIKeyValue = os.Getenv("GEMINI_API_KEY")
				case "hermes":
					llm.APIKeyValue = os.Getenv("NOUS_API_KEY")
				case "huggingface":
					val := os.Getenv("HF_TOKEN")
					if val == "" {
						val = os.Getenv("HUGGINGFACE_API_KEY")
					}
					llm.APIKeyValue = val
				case "mistral":
					llm.APIKeyValue = os.Getenv("MISTRAL_API_KEY")
				case "deepseek":
					llm.APIKeyValue = os.Getenv("DEEPSEEK_API_KEY")
				case "ollama":
					llm.APIKeyValue = os.Getenv("OLLAMA_API_KEY")
				case "opencode":
					llm.APIKeyValue = os.Getenv("OPENCODE_API_KEY")
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

	validLLM := map[string]bool{
		"openai":      true,
		"anthropic":   true,
		"gemini":      true,
		"ollama":      true,
		"hermes":      true,
		"huggingface": true,
		"mistral":     true,
		"deepseek":    true,
		"opencode":    true,
	}

	if len(cfg.LLMs) > 0 {
		for _, llm := range cfg.LLMs {
			lp := strings.ToLower(llm.Provider)
			if !validLLM[lp] {
				return fmt.Errorf("invalid LLM provider in llms list: %s", llm.Provider)
			}
		}
	} else {
		lp := strings.ToLower(cfg.LLM.Provider)
		if !validLLM[lp] {
			return fmt.Errorf("invalid LLM provider: %s", cfg.LLM.Provider)
		}
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

	if len(cfg.LLMs) > 0 {
		for _, llm := range cfg.LLMs {
			if llm.APIKeyValue == "" && strings.ToLower(llm.Provider) != "ollama" {
				return fmt.Errorf("LLM API key is required for provider %s", llm.Provider)
			}
		}
	} else {
		if cfg.LLM.APIKeyValue == "" && strings.ToLower(cfg.LLM.Provider) != "ollama" {
			return fmt.Errorf("LLM API key is required")
		}
	}

	return nil
}
