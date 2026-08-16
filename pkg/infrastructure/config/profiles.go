package config

import (
	"fmt"
	"strings"
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
llm:
  provider: "ollama"
  base_url: "http://localhost:11434/v1"
  model: "qwen2.5-coder:32b"
  temperature: 0.1
  timeout: 300s
  context:
    max_tokens: 32768
    compaction: "aggressive"
  parser:
    resilient_json_fallback: true

orchestrator:
  concurrency: 2
  max_retries: 3
  timeout: 600s
  execution_mode: "breadth_first"

vcs:
  type: "git"
  pull_request:
    auto_create: false
    auto_merge: false
`,
	},
	"ollama-deepseek": {
		Name:        "ollama-deepseek",
		Description: "Pre-tuned for DeepSeek-R1 reasoning models via Ollama with reasoning tag stripping and extended timeout",
		ConfigYAML: `# Noctifab Configuration Profile: Ollama DeepSeek-R1
llm:
  provider: "ollama"
  base_url: "http://localhost:11434/v1"
  model: "deepseek-r1:32b"
  temperature: 0.2
  timeout: 360s
  context:
    max_tokens: 32768
    compaction: "aggressive"
  parser:
    strip_reasoning_tags: true
    resilient_json_fallback: true

orchestrator:
  concurrency: 2
  max_retries: 3
  timeout: 600s
  execution_mode: "breadth_first"

vcs:
  type: "git"
  pull_request:
    auto_create: false
    auto_merge: false
`,
	},
	"vllm-local": {
		Name:        "vllm-local",
		Description: "Pre-tuned for vLLM local OpenAI-compatible inference servers",
		ConfigYAML: `# Noctifab Configuration Profile: vLLM Local Server
llm:
  provider: "openai"
  base_url: "http://localhost:8000/v1"
  model: "Qwen/Qwen2.5-Coder-32B-Instruct"
  temperature: 0.1
  timeout: 300s
  context:
    max_tokens: 32768
    compaction: "aggressive"

orchestrator:
  concurrency: 2
  max_retries: 3
  timeout: 600s

vcs:
  type: "git"
  pull_request:
    auto_create: false
    auto_merge: false
`,
	},
	"openai-compat": {
		Name:        "openai-compat",
		Description: "Generic OpenAI-compatible local or self-hosted endpoint",
		ConfigYAML: `# Noctifab Configuration Profile: OpenAI-Compatible Endpoint
llm:
  provider: "openai"
  base_url: "http://localhost:1234/v1"
  model: "default-model"
  temperature: 0.1
  timeout: 300s

orchestrator:
  concurrency: 2
  max_retries: 3

vcs:
  type: "git"
  pull_request:
    auto_create: false
    auto_merge: false
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

// ListProfiles returns all available profile names and descriptions.
func ListProfiles() []ProfilePreset {
	var list []ProfilePreset
	for _, p := range AvailableProfiles {
		list = append(list, p)
	}
	return list
}
