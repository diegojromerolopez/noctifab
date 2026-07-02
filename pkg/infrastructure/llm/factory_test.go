package llm

import (
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

func TestBuildFailoverClient_Disabled(t *testing.T) {
	cfg := &config.LLMConfig{
		Provider:     "openai",
		Model:        "gpt-4o",
		MaxRetries:   3,
		RetryBackoff: config.Duration(100 * time.Millisecond),
		Failover: config.FailoverConfig{
			Enabled: false,
		},
	}
	client := BuildFailoverClient(cfg, nil)
	if _, ok := client.(*Client); !ok {
		t.Errorf("expected *Client when failover disabled, got %T", client)
	}
}

func TestBuildFailoverClient_Enabled(t *testing.T) {
	cfg := &config.LLMConfig{
		Provider:     "openai",
		Model:        "gpt-4o",
		MaxRetries:   3,
		RetryBackoff: config.Duration(100 * time.Millisecond),
		Failover: config.FailoverConfig{
			Enabled:      true,
			Cooldown:     config.Duration(5 * time.Minute),
			MaxCallLimit: 100,
			Backends: []config.FailoverBackend{
				{Provider: "gemini", Model: "gemini-2.5-flash", APIKeyEnv: "GEMINI_API_KEY"},
				{Provider: "anthropic", Model: "claude-3-haiku", APIKeyEnv: "ANTHROPIC_API_KEY"},
			},
		},
	}
	client := BuildFailoverClient(cfg, nil)
	fc, ok := client.(*FailoverClient)
	if !ok {
		t.Fatalf("expected *FailoverClient when failover enabled, got %T", client)
	}
	if fc.maxCallBudget != 100 {
		t.Errorf("expected maxCallBudget 100, got %d", fc.maxCallBudget)
	}
}

func TestBuildFailoverClient_EmptyBackends(t *testing.T) {
	cfg := &config.LLMConfig{
		Provider:     "openai",
		Model:        "gpt-4o",
		MaxRetries:   3,
		RetryBackoff: config.Duration(100 * time.Millisecond),
		Failover: config.FailoverConfig{
			Enabled:  true,
			Backends: nil,
		},
	}
	client := BuildFailoverClient(cfg, nil)
	if _, ok := client.(*Client); !ok {
		t.Errorf("expected *Client when failover enabled but no backends, got %T", client)
	}
}

func TestBuildFailoverClient_EnabledZeroBackends(t *testing.T) {
	cfg := &config.LLMConfig{
		Provider:     "openai",
		Model:        "gpt-4o",
		MaxRetries:   3,
		RetryBackoff: config.Duration(100 * time.Millisecond),
		Failover: config.FailoverConfig{
			Enabled:  true,
			Backends: []config.FailoverBackend{},
		},
	}
	client := BuildFailoverClient(cfg, nil)
	if _, ok := client.(*Client); !ok {
		t.Errorf("expected *Client when failover enabled with empty backends slice, got %T", client)
	}
}
