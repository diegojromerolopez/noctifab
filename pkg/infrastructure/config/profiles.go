package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProfilePreset represents a pre-tuned configuration template for specific LLM providers.
type ProfilePreset struct {
	Name        string
	Description string
	ConfigYAML  string
}

// AvailableProfiles contains all pre-tuned configuration profiles for 1-click initialization.
var AvailableProfiles = map[string]ProfilePreset{
	"ollama-qwen": {
		Name:        "ollama-qwen",
		Description: "Pre-tuned for Qwen2.5-Coder (32b/14b/7b) running locally via Ollama with 32k context and aggressive compaction",
		ConfigYAML: `# Noctifab Configuration Profile: Ollama Qwen2.5-Coder
config_version: "2.0"
llm:
  provider: "ollama"
  url: "http://localhost:11434/v1"
  model: "qwen2.5-coder:32b"
  temperature: 0.1
  max_timeout: 300s
context:
  compaction: "aggressive"
`,
	},
	"ollama-deepseek": {
		Name:        "ollama-deepseek",
		Description: "Pre-tuned for DeepSeek-R1 reasoning models via Ollama with reasoning tag stripping and extended timeout",
		ConfigYAML: `# Noctifab Configuration Profile: Ollama DeepSeek-R1
config_version: "2.0"
llm:
  provider: "ollama"
  url: "http://localhost:11434/v1"
  model: "deepseek-r1:32b"
  temperature: 0.2
  max_timeout: 360s
context:
  compaction: "aggressive"
`,
	},
	"vllm-local": {
		Name:        "vllm-local",
		Description: "Pre-tuned for vLLM local OpenAI-compatible inference servers",
		ConfigYAML: `# Noctifab Configuration Profile: vLLM Local Server
config_version: "2.0"
llm:
  provider: "openai"
  url: "http://localhost:8000/v1"
  model: "Qwen/Qwen2.5-Coder-32B-Instruct"
  temperature: 0.1
  max_timeout: 300s
context:
  compaction: "aggressive"
`,
	},
	"openai-compat": {
		Name:        "openai-compat",
		Description: "Generic OpenAI-compatible local or self-hosted endpoint",
		ConfigYAML: `# Noctifab Configuration Profile: OpenAI-Compatible Endpoint
config_version: "2.0"
llm:
  provider: "openai"
  url: "http://localhost:1234/v1"
  model: "default-model"
  temperature: 0.1
  max_timeout: 300s
`,
	},
}

// GetProfile retrieves a configuration profile preset by name.
func GetProfile(name string) (ProfilePreset, error) {
	norm := strings.ToLower(strings.TrimSpace(name))
	preset, exists := AvailableProfiles[norm]
	if !exists {
		var valid []string
		for k := range AvailableProfiles {
			valid = append(valid, k)
		}
		return ProfilePreset{}, fmt.Errorf("unknown profile %q (available profiles: %s)", name, strings.Join(valid, ", "))
	}
	return preset, nil
}

// ApplyProfile loads and applies the named profile preset onto cfg.
func ApplyProfile(cfg *Config, profileName string) error {
	preset, err := GetProfile(profileName)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(strings.NewReader(preset.ConfigYAML))
	decoder.KnownFields(true)
	if err := decoder.Decode(cfg); err != nil {
		return fmt.Errorf("failed to apply profile %q: %w", profileName, err)
	}
	return nil
}

// ListProfiles returns all available profile names and descriptions.
func ListProfiles() []ProfilePreset {
	var list []ProfilePreset
	for _, p := range AvailableProfiles {
		list = append(list, p)
	}
	return list
}
