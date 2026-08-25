package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// PromptOverrides converts the config prompts section into the prompts
// package's override map, ready for prompts.NewRenderer.
func (cfg *Config) PromptOverrides() map[string]map[string]prompts.Override {
	if len(cfg.Prompts) == 0 {
		return nil
	}
	out := make(map[string]map[string]prompts.Override, len(cfg.Prompts))
	for agent, actions := range cfg.Prompts {
		byAction := make(map[string]prompts.Override, len(actions))
		for action, ov := range actions {
			byAction[action] = prompts.Override{Path: ov.Path, Append: ov.Append}
		}
		out[agent] = byAction
	}
	return out
}

// Load loads the configuration from file (if exists), environment variables, and CLI flags.
func Load(cmd *cobra.Command) (*Config, error) {
	cfg := DefaultConfig()

	// 1. Determine config path
	configPath := ".noctifab/config.yaml"
	if cmd != nil {
		if flag := cmd.Flags().Lookup("config"); flag != nil && flag.Changed {
			configPath = flag.Value.String()
		}
	}
	if envVal, exists := os.LookupEnv("NOCTIFAB_CONFIG"); exists && envVal != "" {
		configPath = envVal
	}

	// 2. Load YAML configuration if it exists
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		if len(bytes.TrimSpace(data)) > 0 {
			if err := validateYAMLContract(data); err != nil {
				return nil, err
			}
			decoder := yaml.NewDecoder(bytes.NewReader(data))
			decoder.KnownFields(true)
			if err := decoder.Decode(cfg); err != nil {
				return nil, fmt.Errorf("failed to parse YAML config: %w", err)
			}
		}
		if len(cfg.LLMs) > 0 {
			cfg.LLM = cfg.LLMs[0]
		}
	}
	if err := validateConfigVersion(cfg.ConfigVersion); err != nil {
		return nil, err
	}

	// 2a. Load global secrets from $HOME/.noctifab/secrets.yaml if present.
	mergedSecrets := make(map[string]string)
	if homeSecretsPath := GlobalSecretsPath(); homeSecretsPath != "" {
		if homeSecrets, err := LoadSecrets(homeSecretsPath); err == nil {
			for k, v := range homeSecrets {
				mergedSecrets[k] = v
			}
		}
	}

	// 2b. Load project-level secrets.yaml (optional) from the same directory as config.yaml.
	secretsPath := filepath.Join(filepath.Dir(configPath), "secrets.yaml")
	projectSecrets, err := LoadSecrets(secretsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load secrets file: %w", err)
	}
	for k, v := range projectSecrets {
		if strings.TrimSpace(v) != "" {
			mergedSecrets[k] = v
		}
	}

	applySecretsToConfig(cfg, mergedSecrets)

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
	for i := range cfg.LLM.Providers {
		resolveProviderSpecSecret(&cfg.LLM.Providers[i])
	}
}

func resolveProviderSpecSecret(p *ProviderSpec) {
	if len(p.APIKeyPool) == 0 {
		if p.APIKey != "" && !strings.HasPrefix(p.APIKey, "secret:") {
			p.APIKeyValue = p.APIKey
			p.APIKeyPool = []string{p.APIKey}
		} else {
			var keySources []string
			if len(p.APIKeys) > 0 {
				keySources = append(keySources, p.APIKeys...)
			}
			defaultEnv := strings.ToUpper(p.Provider) + "_API_KEY"

			pool, primary := resolveSecretKeys(keySources, defaultEnv, nil)
			p.APIKeyPool = pool
			p.APIKeyValue = primary
		}
	}
}

func resolveSingleLLMSecret(llm *LLMConfig) {
	if len(llm.APIKeyPool) == 0 {
		if llm.APIKey != "" && !strings.HasPrefix(llm.APIKey, "secret:") {
			llm.APIKeyValue = llm.APIKey
			llm.APIKeyPool = []string{llm.APIKey}
		} else {
			var keySources []string
			if len(llm.APIKeys) > 0 {
				keySources = append(keySources, llm.APIKeys...)
			}
			defaultEnv := strings.ToUpper(llm.Provider) + "_API_KEY"

			pool, primary := resolveSecretKeys(keySources, defaultEnv, nil)
			llm.APIKeyPool = pool
			llm.APIKeyValue = primary
		}
	}
}

// Validate checks the configuration for correctness.
func (cfg *Config) Validate() error {
	if err := validateConfigVersion(cfg.ConfigVersion); err != nil {
		return err
	}

	p := strings.ToLower(cfg.Storage.Provider)
	if p != "sqlite" && p != "postgres" && p != "mysql" && p != "json" {
		return fmt.Errorf("invalid storage provider: %s", cfg.Storage.Provider)
	}

	if cfg.Agents.Architecture != "" {
		norm := NormalizeArchitecture(cfg.Agents.Architecture)
		if norm != "code_first" && norm != "single_pass" && norm != "breadth_first" {
			return fmt.Errorf("invalid agent architecture mode: %s (must be code_first, single_pass, or breadth_first)", cfg.Agents.Architecture)
		}
		cfg.Agents.Architecture = norm
	}
	if err := validateQAConfig(cfg); err != nil {
		return err
	}

	roles := map[string]AgentRoleConfig{
		"orchestrator":    cfg.Agents.Orchestrator,
		"product_manager": cfg.Agents.ProductManager,
		"planner":         cfg.Agents.Planner,
		"generators":      cfg.Agents.Generators,
		"testers":         cfg.Agents.Testers,
		"auditor":         cfg.Agents.Auditor,
		"unblocker":       cfg.Agents.Unblocker,
	}
	for roleName, roleCfg := range roles {
		if roleCfg.Number < 0 {
			return fmt.Errorf("agent role %s number must be non-negative, got %d", roleName, roleCfg.Number)
		}
		if roleCfg.Iterations < 0 {
			return fmt.Errorf("agent role %s iterations must be non-negative, got %d", roleName, roleCfg.Iterations)
		}
	}

	if len(cfg.LLM.Providers) > 0 {
		for _, p := range cfg.LLM.Providers {
			if !IsValidLLMProvider(p.Provider) {
				return fmt.Errorf("invalid LLM provider in providers list: %s", p.Provider)
			}
		}
	} else if len(cfg.LLMs) > 0 {
		for _, llmItem := range cfg.LLMs {
			if !IsValidLLMProvider(llmItem.Provider) {
				return fmt.Errorf("invalid LLM provider in llms list: %s", llmItem.Provider)
			}
		}
	} else {
		if !IsValidLLMProvider(cfg.LLM.Provider) {
			return fmt.Errorf("invalid LLM provider: %s", cfg.LLM.Provider)
		}
	}

	for agent, actions := range cfg.Prompts {
		for action := range actions {
			if err := prompts.ValidateKey(agent, action); err != nil {
				return fmt.Errorf("invalid prompts configuration: %w", err)
			}
		}
	}

	vp := strings.ToLower(cfg.VCS.Provider)
	if vp != "github" && vp != "gitlab" {
		return fmt.Errorf("invalid VCS provider: %s", cfg.VCS.Provider)
	}

	if cfg.VCS.Repository == "" && os.Getenv("NOCTIFAB_E2E") != "true" {
		return fmt.Errorf("VCS repository is required")
	}

	if cfg.VCS.TokenValue == "" && os.Getenv("NOCTIFAB_E2E") != "true" {
		return fmt.Errorf("VCS token is required")
	}

	if len(cfg.LLM.Providers) > 0 {
		for _, p := range cfg.LLM.Providers {
			if p.APIKeyValue == "" && strings.ToLower(p.Provider) != "ollama" {
				return fmt.Errorf("LLM API key is required for provider %s", p.Name)
			}
		}
	} else if len(cfg.LLMs) > 0 {
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

func validateConfigVersion(version string) error {
	if version == "1.0" {
		return fmt.Errorf("unsupported config_version %q: migrate to %q", version, "2.0")
	}
	if version != "2.0" {
		return fmt.Errorf("unsupported config_version %q: supported version is %q", version, "2.0")
	}
	return nil
}

var removedAgentRoles = map[string]struct{}{
	"architect":   {},
	"security":    {},
	"performance": {},
	"docs":        {},
	"devops":      {},
}

// IsRemovedAgentRole reports whether role was deleted from the version 2 schema.
func IsRemovedAgentRole(role string) bool {
	_, removed := removedAgentRoles[strings.ToLower(strings.TrimSpace(role))]
	return removed
}

func validateYAMLContract(data []byte) error {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	root := document.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		section := root.Content[i].Value
		if section != "agents" && section != "roles" {
			continue
		}
		mapping := root.Content[i+1]
		if mapping.Kind != yaml.MappingNode {
			continue
		}
		for j := 0; j+1 < len(mapping.Content); j += 2 {
			role := mapping.Content[j].Value
			if IsRemovedAgentRole(role) {
				return fmt.Errorf("unsupported agent role %q: delete the %s.%s section", role, section, role)
			}
		}
	}
	return nil
}

// validLLMProviders is the allowlist of LLM provider names accepted by
// configuration validation. It must stay in sync with the provider registry in
// the llm package (pkg/infrastructure/llm/provider_registry.go); the drift
// guard test in that package asserts this. The list is duplicated here because
// the config package cannot import llm (llm already imports config).
var validLLMProviders = map[string]bool{
	"openai":      true,
	"anthropic":   true,
	"gemini":      true,
	"ollama":      true,
	"hermes":      true,
	"huggingface": true,
	"mistral":     true,
	"deepseek":    true,
	"opencode":    true,
	"openrouter":  true,
	"groq":        true,
	"qwen":        true,
	"dashscope":   true,
	"together":    true,
	"llama":       true,
	"meta":        true,
	"xai":         true,
	"grok":        true,
	"perplexity":  true,
	"fireworks":   true,
	"sambanova":   true,
	"cohere":      true,
	"cerebras":    true,
	"nvidia":      true,
	"ai21":        true,
	"upstage":     true,
	"kimi":        true,
	"qwencloud":   true,
	"moonshot":    true,
}

// IsValidLLMProvider reports whether name is a known LLM provider accepted by
// configuration validation. Comparison is case-insensitive.
func IsValidLLMProvider(name string) bool {
	return validLLMProviders[strings.ToLower(name)]
}

// NormalizeArchitecture normalizes architecture mode strings, short names, and legacy aliases to canonical forms.
func NormalizeArchitecture(arch string) string {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "code_first", "code_first_verification_loop", "cfv", "dfv":
		return "code_first"
	case "single_pass", "single_pass_execution", "spe":
		return "single_pass"
	case "breadth_first", "breadth_first_generation", "bfg", "big":
		return "breadth_first"
	default:
		return arch
	}
}
