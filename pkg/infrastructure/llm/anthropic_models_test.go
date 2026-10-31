package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetiredAnthropicModel_Detection(t *testing.T) {
	retired := []string{
		"claude-3-5-haiku-20241022",
		"claude-3-5-haiku-latest",
		"claude-3-5-haiku",
		"CLAUDE-3-5-HAIKU-20241022",
		"claude-3-haiku-20240307",
		"claude-3-haiku",
	}
	for _, m := range retired {
		assert.True(t, isRetiredAnthropicModel(m), "expected %s to be recognized as retired", m)
	}

	active := []string{
		"claude-haiku-4-5-20251001",
		"claude-haiku-5-5",
		"claude-sonnet-5-5",
		"claude-opus-5-5",
		"claude-sonnet-5",
		"claude-opus-5",
	}
	for _, m := range active {
		assert.False(t, isRetiredAnthropicModel(m), "expected %s to NOT be recognized as retired", m)
	}
}

func TestRetiredAnthropicModel_Reroute(t *testing.T) {
	rerouted := rerouteRetiredAnthropicModel("claude-3-5-haiku-20241022")
	assert.Equal(t, ActiveClaudeHaikuModel, rerouted)

	reroutedHaiku3 := rerouteRetiredAnthropicModel("claude-3-haiku-20240307")
	assert.Equal(t, ActiveClaudeHaikuModel, reroutedHaiku3)

	reroutedLatest := rerouteRetiredAnthropicModel("claude-3-5-haiku-latest")
	assert.Equal(t, ActiveClaudeHaikuModel, reroutedLatest)

	kept := rerouteRetiredAnthropicModel("claude-sonnet-5")
	assert.Equal(t, "claude-sonnet-5", kept)
}

func TestRetiredAnthropicModel_ParseAnthropicModelExclusion(t *testing.T) {
	_, okRetired := parseAnthropicModel("claude-3-5-haiku-20241022")
	assert.False(t, okRetired, "retired claude-3-5-haiku-20241022 must be excluded from parseAnthropicModel")

	_, okRetired3 := parseAnthropicModel("claude-3-haiku-20240307")
	assert.False(t, okRetired3, "retired claude-3-haiku-20240307 must be excluded from parseAnthropicModel")

	_, okRetiredLatest := parseAnthropicModel("claude-3-5-haiku-latest")
	assert.False(t, okRetiredLatest, "retired claude-3-5-haiku-latest must be excluded from parseAnthropicModel")

	infoHaiku45, okHaiku45 := parseAnthropicModel("claude-haiku-4-5-20251001")
	assert.True(t, okHaiku45)
	assert.Equal(t, "haiku", infoHaiku45.Tier)

	infoHaiku55, okHaiku55 := parseAnthropicModel("claude-haiku-5-5")
	assert.True(t, okHaiku55)
	assert.Equal(t, "haiku", infoHaiku55.Tier)
}

func TestRetiredAnthropicModel_GetAvailableModelsFiltersRetired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"data": []map[string]any{
				{"id": "claude-sonnet-5-5"},
				{"id": "claude-3-5-haiku-20241022"}, // Retired: should be filtered out
				{"id": "claude-haiku-4-5-20251001"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewAnthropicProviderClient(server.URL, 0, 0, false)
	models, err := client.GetAvailableModels(context.Background(), "test-key")
	require.NoError(t, err)

	assert.Equal(t, []string{"claude-sonnet-5-5", "claude-haiku-4-5-20251001"}, models)
	for _, m := range models {
		assert.NotContains(t, m, "claude-3-5-haiku")
	}
}

func TestRetiredAnthropicModel_CallReroutesTransparently(t *testing.T) {
	var requestedModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		requestedModel, _ = req["model"].(string)

		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id":   "msg_123",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "{\"actions\":[]}"},
			},
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewAnthropicProviderClient(server.URL, 0, 0, false)
	// Calling with retired model claude-3-5-haiku-20241022
	res, err := client.Call(context.Background(), "claude-3-5-haiku-20241022", "test-key", "hello", 1024, 0.0)
	require.NoError(t, err)
	require.NotNil(t, res)

	// Verify that the request sent to upstream was redirected to ActiveClaudeHaikuModel
	assert.Equal(t, ActiveClaudeHaikuModel, requestedModel)
}

func TestRetiredAnthropicModel_BlacklistVerification(t *testing.T) {
	ResetModelBlacklist()
	assert.True(t, IsModelBlacklisted("claude-3-5-haiku-20241022"))
	assert.True(t, IsModelBlacklisted("claude-3-5-haiku"))
	assert.True(t, IsModelBlacklisted("claude-3-5-haiku-latest"))
	assert.True(t, IsModelBlacklisted("claude-3-haiku-20240307"))
	assert.True(t, IsModelBlacklisted("claude-3-haiku"))

	// Ensure active models are not blacklisted
	assert.False(t, IsModelBlacklisted("claude-haiku-5-5"))
	assert.False(t, IsModelBlacklisted("claude-sonnet-5"))
}
