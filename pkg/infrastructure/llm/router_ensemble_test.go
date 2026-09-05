package llm_test

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
)

func TestRouter_EnsembleResolution(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Provider: "openai",
			Model:    "gpt-4o",
			Providers: []config.ProviderSpec{
				{
					Name:        "claude",
					Provider:    "anthropic",
					Model:       "claude-3-5-sonnet",
					APIKeyValue: "test-claude-key",
				},
				{
					Name:        "gpt",
					Provider:    "openai",
					Model:       "gpt-4o",
					APIKeyValue: "test-openai-key",
				},
				{
					Name:        "gemini",
					Provider:    "gemini",
					Model:       "gemini-2.5-pro",
					APIKeyValue: "test-gemini-key",
				},
			},
		},
		Agents: config.AgentsConfig{
			ProductManager: config.AgentRoleConfig{
				Ensemble: config.EnsembleConfig{
					Strategy:           config.EnsembleStrategyParallel,
					TimeoutSeconds:     30,
					SoftTimeoutSeconds: 10,
					MinModels:          2,
					Models: []config.AgentProviderRef{
						{Name: "claude"},
						{Name: "gpt"},
					},
					Synthesizer: &config.AgentProviderRef{Name: "gemini"},
				},
			},
			Generators: config.AgentRoleConfig{
				Ensemble: config.EnsembleConfig{
					Strategy:        config.EnsembleStrategySerial,
					EarlyExitOnPass: true,
					Stages: []config.EnsembleStageSpec{
						{Name: "gpt"},
						{Name: "claude"},
					},
				},
			},
			QA: config.QAConfig{
				Ensemble: config.EnsembleConfig{
					Strategy: config.EnsembleStrategyConsensus,
					Voters: []config.AgentProviderRef{
						{Name: "claude"},
						{Name: "gpt"},
					},
					TieBreaker: &config.AgentProviderRef{Name: "gemini"},
				},
			},
		},
	}

	router := llm.NewResilientLLMRouter(cfg, nil)

	// 1. Resolve candidates for product_manager
	pmCandidates := router.ResolveCandidatesForRole("product_manager")
	if len(pmCandidates) == 0 {
		t.Fatalf("expected candidates for product_manager, got none")
	}
	if pmCandidates[0].Provider != "ensemble" || pmCandidates[0].Model != "parallel" {
		t.Errorf("expected top candidate to be parallel ensemble, got %+v", pmCandidates[0])
	}

	// 2. Resolve candidates for generator
	genCandidates := router.ResolveCandidatesForRole("generator")
	if len(genCandidates) == 0 {
		t.Fatalf("expected candidates for generator, got none")
	}
	if genCandidates[0].Provider != "ensemble" || genCandidates[0].Model != "serial" {
		t.Errorf("expected top candidate to be serial ensemble, got %+v", genCandidates[0])
	}

	// 3. Resolve candidates for auditor / qa
	qaCandidates := router.ResolveCandidatesForRole("auditor")
	if len(qaCandidates) == 0 {
		t.Fatalf("expected candidates for auditor, got none")
	}
	if qaCandidates[0].Provider != "ensemble" || qaCandidates[0].Model != "consensus" {
		t.Errorf("expected top candidate to be consensus ensemble, got %+v", qaCandidates[0])
	}
}

func TestRouter_UnlimitedTokenBudget(t *testing.T) {
	cfg := &config.Config{
		Runtime: config.RuntimeConfig{
			MaxTokens:         -1,
			MaxTokensPerStory: -1,
		},
		LLM: config.LLMConfig{
			Provider: "mock",
			Model:    "mock-model",
		},
	}

	router := llm.NewResilientLLMRouter(cfg, nil)
	ctx := llm.WithRoleContext(context.Background(), "generator")
	cand := router.ResolveCandidatesForRole("generator")
	if len(cand) == 0 {
		t.Fatalf("expected candidate for generator")
	}
	_ = ctx
}

func TestRouter_EnsembleCountSupport(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Providers: []config.ProviderSpec{
				{
					Name:        "claude",
					Provider:    "anthropic",
					Model:       "claude-3-5-sonnet",
					APIKeyValue: "test-claude-key",
				},
				{
					Name:        "gemini",
					Provider:    "gemini",
					Model:       "gemini-2.5-pro",
					APIKeyValue: "test-gemini-key",
				},
			},
		},
		Agents: config.AgentsConfig{
			Testers: config.AgentRoleConfig{
				Ensemble: config.EnsembleConfig{
					Strategy:       config.EnsembleStrategyBestOfNScored,
					TimeoutSeconds: 30,
					Models: []config.AgentProviderRef{
						{Name: "claude", Count: 3},
						{Name: "gemini", Count: 2},
					},
				},
			},
		},
	}

	router := llm.NewResilientLLMRouter(cfg, nil)
	candidates := router.ResolveCandidatesForRole("testers")
	if len(candidates) == 0 {
		t.Fatalf("expected candidates for testers")
	}
	if candidates[0].Provider != "ensemble" || candidates[0].Model != "best_of_n_scored" {
		t.Errorf("expected best_of_n_scored ensemble candidate, got %+v", candidates[0])
	}
}
