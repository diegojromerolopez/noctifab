package llm

import (
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

func TestBuildFailoverClient_Disabled(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Provider:     "openai",
			Model:        "gpt-4o",
			MaxRetries:   3,
			RetryBackoff: config.Duration(100 * time.Millisecond),
			Failover: config.FailoverConfig{
				Enabled: false,
			},
		},
	}
	client := BuildFailoverClient(cfg, nil)
	if _, ok := client.(*Client); !ok {
		t.Errorf("expected *Client when failover disabled, got %T", client)
	}
}

func TestBuildFailoverClient_Enabled(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
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
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Provider:     "openai",
			Model:        "gpt-4o",
			MaxRetries:   3,
			RetryBackoff: config.Duration(100 * time.Millisecond),
			Failover: config.FailoverConfig{
				Enabled:  true,
				Backends: nil,
			},
		},
	}
	client := BuildFailoverClient(cfg, nil)
	if _, ok := client.(*Client); !ok {
		t.Errorf("expected *Client when failover enabled but no backends, got %T", client)
	}
}

func TestBuildFailoverClient_EnabledZeroBackends(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Provider:     "openai",
			Model:        "gpt-4o",
			MaxRetries:   3,
			RetryBackoff: config.Duration(100 * time.Millisecond),
			Failover: config.FailoverConfig{
				Enabled:  true,
				Backends: []config.FailoverBackend{},
			},
		},
	}
	client := BuildFailoverClient(cfg, nil)
	if _, ok := client.(*Client); !ok {
		t.Errorf("expected *Client when failover enabled with empty backends slice, got %T", client)
	}
}

func TestBuildFailoverClient_MultipleLLMs(t *testing.T) {
	cfg := &config.Config{
		LLMs: []config.LLMConfig{
			{
				Provider:     "opencode",
				Model:        "glm-5.2",
				MaxRetries:   3,
				RetryBackoff: config.Duration(100 * time.Millisecond),
			},
			{
				Provider:     "gemini",
				Model:        "gemini-2.5-pro",
				MaxRetries:   5,
				RetryBackoff: config.Duration(200 * time.Millisecond),
			},
		},
	}
	client := BuildFailoverClient(cfg, nil)
	fc, ok := client.(*FailoverClient)
	if !ok {
		t.Fatalf("expected *FailoverClient, got %T", client)
	}
	if len(fc.backends) != 2 {
		t.Errorf("expected 2 backends, got %d", len(fc.backends))
	}
	if fc.backends[0].Name != "opencode/glm-5.2" {
		t.Errorf("expected first backend opencode/glm-5.2, got %s", fc.backends[0].Name)
	}
	if fc.backends[1].Name != "gemini/gemini-2.5-pro" {
		t.Errorf("expected second backend gemini/gemini-2.5-pro, got %s", fc.backends[1].Name)
	}
}
