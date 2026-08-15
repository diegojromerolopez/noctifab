package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelBlacklist(t *testing.T) {
	t.Cleanup(ResetModelBlacklist)
	ResetModelBlacklist()

	t.Run("when model is blacklisted, IsModelBlacklisted returns true", func(t *testing.T) {
		assert.False(t, IsModelBlacklisted("gpt-4-deprecated"))
		BlacklistModel("gpt-4-deprecated")
		assert.True(t, IsModelBlacklisted("gpt-4-deprecated"))
		assert.True(t, IsModelBlacklisted("GPT-4-DEPRECATED"))
	})

	t.Run("when ResetModelBlacklist is called, all blacklisted models are cleared", func(t *testing.T) {
		BlacklistModel("claude-2.0")
		assert.True(t, IsModelBlacklisted("claude-2.0"))
		ResetModelBlacklist()
		assert.False(t, IsModelBlacklisted("claude-2.0"))
	})

	t.Run("when fallback selects lower model, blacklisted models are skipped", func(t *testing.T) {
		ResetModelBlacklist()
		defer ResetModelBlacklist()

		models := []string{"claude-3-opus-20240229", "claude-3-5-sonnet-latest", "claude-3-5-haiku-latest"}
		parsed := parsedModelsFor(models, parseAnthropicModel)

		// Under normal conditions, fallback from opus is sonnet
		next := selectLowerModelFromParsed("claude-3-opus-20240229", parsed)
		assert.Equal(t, "claude-3-5-sonnet-latest", next)

		// Blacklist sonnet
		BlacklistModel("claude-3-5-sonnet-latest")

		// Next fallback should skip sonnet and return haiku
		nextAfterBlacklist := selectLowerModelFromParsed("claude-3-opus-20240229", parsed)
		assert.Equal(t, "claude-3-5-haiku-latest", nextAfterBlacklist)

		// After reset, sonnet becomes selectable again
		ResetModelBlacklist()
		nextAfterReset := selectLowerModelFromParsed("claude-3-opus-20240229", parsed)
		assert.Equal(t, "claude-3-5-sonnet-latest", nextAfterReset)
	})
}
