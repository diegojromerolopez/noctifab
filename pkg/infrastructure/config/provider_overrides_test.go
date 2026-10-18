package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLLMConfig_ProviderOverrides(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	intPtr := func(i int) *int { return &i }

	t.Run("when the global thinking is disabled and one provider enables it, only that provider thinks", func(t *testing.T) {
		l := LLMConfig{Thinking: &ThinkingConfig{Enabled: boolPtr(false)}}
		lone := ProviderSpec{Name: "claude", Thinking: &ThinkingConfig{Enabled: boolPtr(true), Budget: intPtr(8192)}}
		other := ProviderSpec{Name: "gemini"}

		th := l.ResolveThinkingEnabled(lone)
		require.NotNil(t, th)
		assert.True(t, *th)
		assert.Equal(t, 8192, *l.ResolveThinkingBudget(lone, true))

		assert.Nil(t, l.ResolveThinkingEnabled(other))
		assert.Nil(t, l.ResolveThinkingBudget(other, false))
	})

	t.Run("when the global thinking is enabled and one provider disables it, the provider wins", func(t *testing.T) {
		l := LLMConfig{Thinking: &ThinkingConfig{Enabled: boolPtr(true), Budget: intPtr(1024)}}
		off := ProviderSpec{Name: "openai", EnableThinking: boolPtr(false)}
		inherit := ProviderSpec{Name: "qwen"}

		assert.False(t, *l.ResolveThinkingEnabled(off))
		require.NotNil(t, l.ResolveThinkingEnabled(inherit))
		assert.True(t, *l.ResolveThinkingEnabled(inherit))
		assert.Equal(t, 1024, *l.ResolveThinkingBudget(inherit, true))
	})

	t.Run("when thinking is enabled without any budget, it uses default_budget", func(t *testing.T) {
		l := LLMConfig{Thinking: &ThinkingConfig{Enabled: boolPtr(true)}}
		assert.Equal(t, 2048, *l.ResolveThinkingBudget(ProviderSpec{}, true))
	})

	t.Run("when a provider sets only some hedging fields, the rest are inherited", func(t *testing.T) {
		l := LLMConfig{Hedging: HedgingConfig{Delay: Duration(25 * time.Second), HeavyDelay: Duration(90 * time.Second)}}
		p := ProviderSpec{Hedging: &HedgingConfig{Delay: Duration(5 * time.Second)}}

		h := l.ResolveHedging(p)
		assert.True(t, h.IsEnabled())
		assert.Equal(t, 5*time.Second, h.GetDelay())
		assert.Equal(t, 90*time.Second, h.GetHeavyDelay())

		disabled := l.ResolveHedging(ProviderSpec{Hedging: &HedgingConfig{Enabled: boolPtr(false)}})
		assert.False(t, disabled.IsEnabled())
		assert.True(t, l.Hedging.IsEnabled(), "global config must not be mutated")
	})

	t.Run("when a provider sets one json_reminder cap, the other cap is inherited", func(t *testing.T) {
		l := LLMConfig{JSONReminder: JSONReminderConfig{Task: JSONReminderCapConfig{Cap: 1500}, Body: JSONReminderCapConfig{Cap: 12000}}}
		r := l.ResolveJSONReminder(ProviderSpec{JSONReminder: &JSONReminderConfig{Body: JSONReminderCapConfig{Cap: 4000}}})
		assert.Equal(t, 1500, r.GetTaskCap())
		assert.Equal(t, 4000, r.GetBodyCap())
		assert.Equal(t, 12000, l.ResolveJSONReminder(ProviderSpec{}).GetBodyCap())
	})

	t.Run("when a provider sets token_usage_limit, it is a per-provider cap; otherwise there is none", func(t *testing.T) {
		limit := int64(500000)
		l := LLMConfig{TokenUsageLimit: 10_000_000}
		assert.Equal(t, int64(500000), l.ResolveTokenUsageLimit(ProviderSpec{TokenUsageLimit: &limit}))
		assert.Equal(t, int64(0), l.ResolveTokenUsageLimit(ProviderSpec{}))
	})

	t.Run("when parsing YAML, provider override blocks are decoded", func(t *testing.T) {
		raw := `
providers:
  - name: claude
    provider: anthropic
    token_usage_limit: 750000
    hedging:
      enabled: false
    json_reminder:
      task:
        cap: 800
    thinking:
      enabled: true
      budget: 4096
`
		var l LLMConfig
		require.NoError(t, yaml.Unmarshal([]byte(raw), &l))
		require.Len(t, l.Providers, 1)
		p := l.Providers[0]
		require.NotNil(t, p.TokenUsageLimit)
		assert.Equal(t, int64(750000), *p.TokenUsageLimit)
		assert.False(t, l.ResolveHedging(p).IsEnabled())
		assert.Equal(t, 800, l.ResolveJSONReminder(p).GetTaskCap())
		assert.True(t, *l.ResolveThinkingEnabled(p))
	})
}
