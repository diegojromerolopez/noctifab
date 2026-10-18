package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestLLMConfig_ResolveTransport(t *testing.T) {
	streamTrue, streamFalse := true, false

	global := LLMConfig{
		MaxRetries:   3,
		RetryBackoff: Duration(500 * time.Millisecond),
		MaxTimeout:   Duration(60 * time.Second),
		IdleTimeout:  Duration(60 * time.Second),
		MaxTokens:    32768,
		Temperature:  0.3,
		Streaming:    &streamTrue,
	}

	t.Run("when a provider sets nothing, every setting is inherited from llm.*", func(t *testing.T) {
		tr := global.ResolveTransport(ProviderSpec{Name: "p"})
		assert.Equal(t, 3, tr.MaxRetries)
		assert.Equal(t, 500*time.Millisecond, tr.RetryBackoff)
		assert.Equal(t, 60*time.Second, tr.MaxTimeout)
		assert.Equal(t, 60*time.Second, tr.IdleTimeout)
		assert.Equal(t, 32768, tr.MaxTokens)
		assert.True(t, tr.TemperatureSet)
		assert.InDelta(t, 0.3, tr.Temperature, 1e-9)
		assert.True(t, tr.StreamingSet)
		assert.True(t, tr.Streaming)
	})

	t.Run("when a provider sets values, they override llm.*", func(t *testing.T) {
		tr := global.ResolveTransport(ProviderSpec{
			MaxRetries: 1, RetryBackoff: Duration(time.Second), MaxTimeout: Duration(120 * time.Second),
			IdleTimeout: Duration(5 * time.Second), MaxTokens: 4096, Temperature: 0.9, Streaming: &streamFalse,
		})
		assert.Equal(t, 1, tr.MaxRetries)
		assert.Equal(t, time.Second, tr.RetryBackoff)
		assert.Equal(t, 120*time.Second, tr.MaxTimeout)
		assert.Equal(t, 5*time.Second, tr.IdleTimeout)
		assert.Equal(t, 4096, tr.MaxTokens)
		assert.InDelta(t, 0.9, tr.Temperature, 1e-9)
		assert.False(t, tr.Streaming)
	})

	t.Run("when a provider sets max_tokens to -1, it means unlimited even if llm.max_tokens is set", func(t *testing.T) {
		assert.Equal(t, 0, global.ResolveTransport(ProviderSpec{MaxTokens: -1}).MaxTokens)
	})

	t.Run("when YAML writes explicit zeros, they override non-zero globals", func(t *testing.T) {
		var l LLMConfig
		require.NoError(t, yaml.Unmarshal([]byte(`
providers:
  - name: deterministic
    provider: openai
    temperature: 0
    max_retries: 0
  - name: inherit
    provider: openai
`), &l))
		require.Len(t, l.Providers, 2)

		explicit := global.ResolveTransport(l.Providers[0])
		assert.True(t, explicit.TemperatureSet)
		assert.Equal(t, 0.0, explicit.Temperature)
		assert.Equal(t, 0, explicit.MaxRetries)

		inherited := global.ResolveTransport(l.Providers[1])
		assert.InDelta(t, 0.3, inherited.Temperature, 1e-9)
		assert.Equal(t, 3, inherited.MaxRetries)
	})

	t.Run("when a provider entry has an unknown key, decoding fails", func(t *testing.T) {
		var l LLMConfig
		err := yaml.Unmarshal([]byte("providers:\n  - name: p\n    provider: openai\n    temprature: 0.2\n"), &l)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temprature")
	})

	t.Run("when a provider entry is not a mapping, decoding fails", func(t *testing.T) {
		var l LLMConfig
		require.Error(t, yaml.Unmarshal([]byte("providers:\n  - just-a-string\n"), &l))
	})
}
