package llm

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPromptSize(t *testing.T) {
	t.Run("when the prompt is under the limit it passes", func(t *testing.T) {
		assert.NoError(t, checkPromptSize("short prompt", 100))
	})

	t.Run("when the prompt estimate exceeds the limit it fails with ErrPromptTooLarge", func(t *testing.T) {
		// 100 chars ≈ 25 tokens > limit of 10.
		err := checkPromptSize(strings.Repeat("a", 100), 10)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPromptTooLarge)
	})

	t.Run("when the limit is zero it applies the built-in default", func(t *testing.T) {
		assert.NoError(t, checkPromptSize("short prompt", 0))
		oversized := strings.Repeat("a", (defaultMaxPromptTokens+1)*estCharsPerToken)
		assert.ErrorIs(t, checkPromptSize(oversized, 0), ErrPromptTooLarge)
	})

	t.Run("when the limit is negative the check is disabled", func(t *testing.T) {
		oversized := strings.Repeat("a", (defaultMaxPromptTokens+1)*estCharsPerToken)
		assert.NoError(t, checkPromptSize(oversized, -1))
	})
}

func TestClientPreSendPromptGuard(t *testing.T) {
	t.Run("when the prompt exceeds max_prompt_tokens it fails fast before any network call", func(t *testing.T) {
		client := NewClient("openai", "gpt-4o", "test-key", 1, 0, "http://127.0.0.1:0")
		client.MaxPromptTokens = 10

		_, err := client.Complete(context.Background(), strings.Repeat("a", 400))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPromptTooLarge)
	})
}
