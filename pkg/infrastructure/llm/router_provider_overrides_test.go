package llm

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRouter_ProviderOverrides(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	int64Ptr := func(i int64) *int64 { return &i }

	t.Run("when only one provider enables thinking, its client thinks and the others do not", func(t *testing.T) {
		cfg := &config.Config{LLM: config.LLMConfig{
			Priority: []string{"claude", "gemini"},
			Thinking: &config.ThinkingConfig{Enabled: boolPtr(false)},
			Providers: []config.ProviderSpec{
				{Name: "claude", Provider: "anthropic", Model: "m", Thinking: &config.ThinkingConfig{Enabled: boolPtr(true)}},
				{Name: "gemini", Provider: "gemini", Model: "m"},
			},
		}}
		router := NewResilientLLMRouter(cfg, nil)

		claude, ok := router.buildClientForSpec(cfg.LLM.Providers[0], "").(*Client)
		require.True(t, ok)
		require.NotNil(t, claude.EnableThinking)
		assert.True(t, *claude.EnableThinking)
		require.NotNil(t, claude.ThinkingBudget)
		assert.Equal(t, 2048, *claude.ThinkingBudget)

		gemini, ok := router.buildClientForSpec(cfg.LLM.Providers[1], "").(*Client)
		require.True(t, ok)
		assert.Nil(t, gemini.EnableThinking)
		assert.Nil(t, gemini.ThinkingBudget)
	})

	t.Run("when a provider overrides json_reminder caps, its client uses them", func(t *testing.T) {
		cfg := &config.Config{LLM: config.LLMConfig{
			JSONReminder: config.JSONReminderConfig{Task: config.JSONReminderCapConfig{Cap: 1500}},
			Providers: []config.ProviderSpec{{
				Name: "p", Provider: "openai", Model: "m",
				JSONReminder: &config.JSONReminderConfig{Task: config.JSONReminderCapConfig{Cap: 300}},
			}},
		}}
		router := NewResilientLLMRouter(cfg, nil)
		c, ok := router.buildClientForSpec(cfg.LLM.Providers[0], "").(*Client)
		require.True(t, ok)
		assert.Equal(t, 300, c.JSONReminderTaskCap)
		assert.Equal(t, 12000, c.JSONReminderBodyCap)
	})

	t.Run("when the primary provider disables hedging, no speculative request is sent", func(t *testing.T) {
		cfg := &config.Config{LLM: config.LLMConfig{
			Hedging: config.HedgingConfig{Delay: config.Duration(10 * time.Millisecond)},
			Providers: []config.ProviderSpec{
				{Name: "slow", Provider: "openai", Model: "m", Hedging: &config.HedgingConfig{Enabled: boolPtr(false)}},
				{Name: "backup", Provider: "openai", Model: "m"},
			},
		}}
		router := NewResilientLLMRouter(cfg, nil)
		var backupCalls int32
		seedRouterCandidates(router, "", []RouterCandidate{
			{Name: "slow", Provider: "openai", Client: &mockLLMClient{completeFn: func(ctx context.Context, _ string) (*domain.LLMResponse, error) {
				time.Sleep(60 * time.Millisecond)
				return &domain.LLMResponse{Reasoning: "slow"}, nil
			}}},
			{Name: "backup", Provider: "openai", Client: &mockLLMClient{completeFn: func(ctx context.Context, _ string) (*domain.LLMResponse, error) {
				atomic.AddInt32(&backupCalls, 1)
				return &domain.LLMResponse{Reasoning: "backup"}, nil
			}}},
		})

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, err := router.Complete(ctx, "prompt")
		require.NoError(t, err)
		assert.Equal(t, "slow", resp.Reasoning)
		assert.Equal(t, int32(0), atomic.LoadInt32(&backupCalls))
	})

	t.Run("when the global hedging is off and the primary enables it, the provider delay is used", func(t *testing.T) {
		cfg := &config.Config{LLM: config.LLMConfig{
			Hedging: config.HedgingConfig{HeavyDelay: config.Duration(90 * time.Second)},
			Providers: []config.ProviderSpec{
				{Name: "p", Provider: "openai", Model: "m", Hedging: &config.HedgingConfig{
					Enabled:    boolPtr(true),
					Delay:      config.Duration(7 * time.Second),
					HeavyDelay: config.Duration(120 * time.Second),
				}},
			},
		}}
		router := NewResilientLLMRouter(cfg, nil)
		router.SetHedgeDelay(-1)
		assert.Equal(t, 7*time.Second, router.hedgeDelayFor("p"))
		assert.Equal(t, 120*time.Second, router.heavyHedgeDelayFor("p"))
		assert.Equal(t, time.Duration(-1), router.hedgeDelayFor("unknown"))
		assert.Equal(t, 90*time.Second, router.heavyHedgeDelayFor("unknown"))
	})

	t.Run("when a provider reaches its own token_usage_limit, the next provider serves the request", func(t *testing.T) {
		store := newMockBudgetStore()
		cfg := &config.Config{LLM: config.LLMConfig{
			Providers: []config.ProviderSpec{
				{Name: "capped", Provider: "openai", Model: "m", TokenUsageLimit: int64Ptr(100)},
				{Name: "free", Provider: "openai", Model: "m"},
			},
		}}
		router := NewResilientLLMRouter(cfg, store)
		router.SetHedgeDelay(-1)
		today := time.Now().UTC().Format("2006-01-02")
		require.NoError(t, store.IncrementUsage(context.Background(), today, namedUsageKey("capped"), 100))

		var cappedCalls int32
		seedRouterCandidates(router, "", []RouterCandidate{
			{Name: "capped", Provider: "openai", Client: &mockLLMClient{completeFn: func(ctx context.Context, _ string) (*domain.LLMResponse, error) {
				atomic.AddInt32(&cappedCalls, 1)
				return &domain.LLMResponse{Reasoning: "capped"}, nil
			}}},
			{Name: "free", Provider: "openai", Client: &mockLLMClient{}},
		})

		resp, err := router.Complete(context.Background(), "prompt")
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Reasoning)
		assert.Equal(t, int32(0), atomic.LoadInt32(&cappedCalls))

		freeUsage, err := store.GetDailyUsage(context.Background(), today, namedUsageKey("free"))
		require.NoError(t, err)
		assert.Greater(t, freeUsage, int64(0), "usage must be recorded per named provider")
	})

	t.Run("when every provider reached its token_usage_limit, it returns ErrBudgetExhausted", func(t *testing.T) {
		store := newMockBudgetStore()
		cfg := &config.Config{LLM: config.LLMConfig{
			Providers: []config.ProviderSpec{{Name: "capped", Provider: "openai", Model: "m", TokenUsageLimit: int64Ptr(10)}},
		}}
		router := NewResilientLLMRouter(cfg, store)
		today := time.Now().UTC().Format("2006-01-02")
		require.NoError(t, store.IncrementUsage(context.Background(), today, namedUsageKey("capped"), 10))
		seedRouterCandidates(router, "", []RouterCandidate{{Name: "capped", Provider: "openai", Client: &mockLLMClient{}}})

		_, err := router.Complete(context.Background(), "prompt")
		require.Error(t, err)
		assert.True(t, errors.Is(err, domain.ErrBudgetExhausted))
	})

	t.Run("when global transport settings are defined, it applies them to inheriting providers and honors per-provider overrides", func(t *testing.T) {
		yamlContent := `
llm:
  max_retries: 4
  retry_backoff: 250ms
  max_timeout: 45s
  idle_timeout: 30s
  max_tokens: 4096
  temperature: 0.7
  streaming: false
  providers:
  - name: inheriting
    provider: openai
    model: gpt-4o
  - name: overriding
    provider: openai
    model: gpt-4o-mini
    max_retries: 1
    retry_backoff: 100ms
    max_timeout: 10s
    idle_timeout: 5s
    max_tokens: -1
    temperature: 0.0
    streaming: true
    disable_json_mode: true
    extra_params:
      custom_header: special_val
`
		var cfg config.Config
		err := yaml.Unmarshal([]byte(yamlContent), &cfg)
		require.NoError(t, err)

		router := NewResilientLLMRouter(&cfg, nil)

		inhClient, ok := router.buildClientForSpec(cfg.LLM.Providers[0], "").(*Client)
		require.True(t, ok)
		assert.Equal(t, 4, inhClient.MaxRetries)
		assert.Equal(t, 250*time.Millisecond, inhClient.Backoff)
		assert.Equal(t, 45*time.Second, inhClient.Timeout)
		assert.Equal(t, 30*time.Second, inhClient.IdleTimeout)
		assert.Equal(t, 4096, inhClient.MaxTokens)
		assert.Equal(t, 0.7, inhClient.Temperature)
		assert.False(t, inhClient.Streaming)
		assert.False(t, inhClient.DisableJSONMode)
		assert.Empty(t, inhClient.ExtraParams)

		ovrClient, ok := router.buildClientForSpec(cfg.LLM.Providers[1], "").(*Client)
		require.True(t, ok)
		assert.Equal(t, 1, ovrClient.MaxRetries)
		assert.Equal(t, 100*time.Millisecond, ovrClient.Backoff)
		assert.Equal(t, 10*time.Second, ovrClient.Timeout)
		assert.Equal(t, 5*time.Second, ovrClient.IdleTimeout)
		assert.Equal(t, 0, ovrClient.MaxTokens, "max_tokens: -1 should resolve to 0 (unlimited)")
		assert.Equal(t, 0.0, ovrClient.Temperature, "explicit temperature: 0.0 should override global 0.7")
		assert.True(t, ovrClient.Streaming)
		assert.True(t, ovrClient.DisableJSONMode)
		assert.Equal(t, "special_val", ovrClient.ExtraParams["custom_header"])
	})
}
