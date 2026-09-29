package config_test

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"gopkg.in/yaml.v3"
)

func TestEnsembleConfig_Unmarshal(t *testing.T) {
	yamlData := `
strategy: "parallel"
timeout_seconds: 45
soft_timeout_seconds: 15
min_models: 2
fallback_to_single: true
models:
  - name: "claude"
    max_tokens: 8192
    temperature: 0.2
  - name: "openai"
synthesizer:
  name: "gemini"
  max_tokens: 16384
`
	var cfg config.EnsembleConfig
	err := yaml.Unmarshal([]byte(yamlData), &cfg)
	if err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if cfg.Strategy != config.EnsembleStrategyParallel {
		t.Errorf("expected strategy 'parallel', got %q", cfg.Strategy)
	}
	if !cfg.IsEnabled() {
		t.Errorf("expected IsEnabled() to be true")
	}
	if cfg.TimeoutSeconds != 45 {
		t.Errorf("expected timeout 45, got %d", cfg.TimeoutSeconds)
	}
	if cfg.SoftTimeoutSeconds != 15 {
		t.Errorf("expected soft timeout 15, got %d", cfg.SoftTimeoutSeconds)
	}
	if cfg.MinModels != 2 {
		t.Errorf("expected min_models 2, got %d", cfg.MinModels)
	}
	if len(cfg.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(cfg.Models))
	}
	if cfg.Models[0].Name != "claude" || cfg.Models[0].MaxTokens == nil || *cfg.Models[0].MaxTokens != 8192 {
		t.Errorf("unexpected model 0 spec: %+v", cfg.Models[0])
	}
	if cfg.Synthesizer == nil || cfg.Synthesizer.Name != "gemini" {
		t.Errorf("unexpected synthesizer: %+v", cfg.Synthesizer)
	}
}

func TestEnsembleConfig_Strategies(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		strategy config.EnsembleStrategy
	}{
		{
			name:     "serial",
			yaml:     "strategy: serial\nearly_exit_on_pass: true\nstages:\n  - name: openai\n  - name: claude\n",
			strategy: config.EnsembleStrategySerial,
		},
		{
			name:     "consensus",
			yaml:     "strategy: consensus\nvoters:\n  - name: claude\n  - name: openai\ntie_breaker:\n  name: gemini\n",
			strategy: config.EnsembleStrategyConsensus,
		},
		{
			name:     "race",
			yaml:     "strategy: race\nmodels:\n  - name: cerebras\n  - name: gemini\n",
			strategy: config.EnsembleStrategyRace,
		},
		{
			name:     "decomposed",
			yaml:     "strategy: decomposed\ntargets:\n  - name: claude\n    role_prompt: types\n",
			strategy: config.EnsembleStrategyDecomposed,
		},
		{
			name:     "cascade",
			yaml:     "strategy: cascade\ntiers:\n  - name: gemini\n  - name: claude\n",
			strategy: config.EnsembleStrategyCascade,
		},
		{
			name:     "best_of_n_scored",
			yaml:     "strategy: best_of_n_scored\nmodels:\n  - name: claude\n",
			strategy: config.EnsembleStrategyBestOfNScored,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg config.EnsembleConfig
			if err := yaml.Unmarshal([]byte(tt.yaml), &cfg); err != nil {
				t.Fatalf("failed to unmarshal yaml: %v", err)
			}
			if cfg.Strategy != tt.strategy {
				t.Errorf("expected strategy %q, got %q", tt.strategy, cfg.Strategy)
			}
		})
	}
}

func TestAgentProviderRef_Count(t *testing.T) {
	yamlData := `
models:
  - name: "claude"
    count: 3
    temperature: 0.7
  - name: "gemini"
`
	var cfg config.EnsembleConfig
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(cfg.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(cfg.Models))
	}

	if cfg.Models[0].Count != 3 || cfg.Models[0].GetCount() != 3 {
		t.Errorf("expected count 3, got Count=%d, GetCount()=%d", cfg.Models[0].Count, cfg.Models[0].GetCount())
	}

	if cfg.Models[1].Count != 0 || cfg.Models[1].GetCount() != 1 {
		t.Errorf("expected default count 1, got Count=%d, GetCount()=%d", cfg.Models[1].Count, cfg.Models[1].GetCount())
	}

	zeroRef := config.AgentProviderRef{Count: 0}
	if zeroRef.GetCount() != 1 {
		t.Errorf("expected zero count to default to 1, got %d", zeroRef.GetCount())
	}

	negRef := config.AgentProviderRef{Count: -5}
	if negRef.GetCount() != 1 {
		t.Errorf("expected negative count to default to 1, got %d", negRef.GetCount())
	}
}
