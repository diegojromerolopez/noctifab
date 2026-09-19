package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropic_MultiBlockPrefixCaching_WithCacheablePrefix(t *testing.T) {
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "{\"reasoning\": \"ok\", \"actions\": []}"},
			},
			"usage": map[string]any{
				"input_tokens":                100,
				"output_tokens":               20,
				"cache_read_input_tokens":     80,
				"cache_creation_input_tokens": 0,
			},
		})
	}))
	defer server.Close()

	// Build a prompt where static instructions >= 1000 chars and Task Details follows
	staticInstructions := strings.Repeat("MANDATORY RULE LINE: Do not do host installations.\n", 30) // ~1500 chars
	taskDetails := "\nTask Details:\nImplement feature X in src/foo.py.\n"
	contract := "\n{\"contract\": true}"
	body := staticInstructions + taskDetails
	fullPrompt := body + contract

	ctx := domain.WithCacheablePrefix(context.Background(), len(body))
	client := NewAnthropicProviderClient(server.URL, 0, 0, false)
	res, err := client.Call(ctx, "claude-3-5-sonnet-20241022", "test-key", fullPrompt, 4096, 0.0)
	require.NoError(t, err)
	require.NotNil(t, res)

	messages, ok := receivedPayload["messages"].([]any)
	require.True(t, ok)
	require.Len(t, messages, 1)

	firstMsg, ok := messages[0].(map[string]any)
	require.True(t, ok)

	contentBlocks, ok := firstMsg["content"].([]any)
	require.True(t, ok, "content should be an array of blocks")
	require.Len(t, contentBlocks, 3, "expected 3 blocks: static instructions, task details, and suffix contract")

	// Block 0: static instructions with cache_control
	b0 := contentBlocks[0].(map[string]any)
	assert.Equal(t, "text", b0["type"])
	assert.Equal(t, staticInstructions, b0["text"])
	assert.Equal(t, map[string]any{"type": "ephemeral"}, b0["cache_control"])

	// Block 1: task details with cache_control
	b1 := contentBlocks[1].(map[string]any)
	assert.Equal(t, "text", b1["type"])
	assert.Equal(t, taskDetails, b1["text"])
	assert.Equal(t, map[string]any{"type": "ephemeral"}, b1["cache_control"])

	// Block 2: contract without cache_control
	b2 := contentBlocks[2].(map[string]any)
	assert.Equal(t, "text", b2["type"])
	assert.Equal(t, contract, b2["text"])
	assert.Nil(t, b2["cache_control"])
}

func TestAnthropic_MultiBlockPrefixCaching_AutoDetectContinuationTurn(t *testing.T) {
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "{\"reasoning\": \"turn 2 ok\", \"actions\": []}"},
			},
		})
	}))
	defer server.Close()

	staticInstructions := strings.Repeat("RULE LINE FOR AGENT.\n", 60) // ~1320 chars
	turnOutputs := "\n\nTOOL OUTPUTS FROM PREVIOUS TURN (turn 1/6):\nTool run_tests executed successfully."
	fullPrompt := staticInstructions + turnOutputs

	// No domain.WithCacheablePrefix in ctx: test auto-detection of continuation turn
	client := NewAnthropicProviderClient(server.URL, 0, 0, false)
	res, err := client.Call(context.Background(), "claude-3-5-sonnet-20241022", "test-key", fullPrompt, 4096, 0.0)
	require.NoError(t, err)
	require.NotNil(t, res)

	messages := receivedPayload["messages"].([]any)
	firstMsg := messages[0].(map[string]any)
	contentBlocks := firstMsg["content"].([]any)
	require.Len(t, contentBlocks, 2)

	b0 := contentBlocks[0].(map[string]any)
	assert.Equal(t, staticInstructions, b0["text"])
	assert.Equal(t, map[string]any{"type": "ephemeral"}, b0["cache_control"])

	b1 := contentBlocks[1].(map[string]any)
	assert.Equal(t, turnOutputs, b1["text"])
	assert.Nil(t, b1["cache_control"])
}

func TestAnthropic_MultiBlockPrefixCaching_AutoDetectTurn1(t *testing.T) {
	var receivedPayload map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "{\"reasoning\": \"turn 1 ok\", \"actions\": []}"},
			},
		})
	}))
	defer server.Close()

	staticInstructions := strings.Repeat("CORE MANDATE DIRECTIVE.\n", 50) // ~1200 chars
	taskPart := "\nTask Details:\nUS-001\nContract"
	fullPrompt := staticInstructions + taskPart

	client := NewAnthropicProviderClient(server.URL, 0, 0, false)
	res, err := client.Call(context.Background(), "claude-3-5-sonnet-20241022", "test-key", fullPrompt, 4096, 0.0)
	require.NoError(t, err)
	require.NotNil(t, res)

	messages := receivedPayload["messages"].([]any)
	firstMsg := messages[0].(map[string]any)
	contentBlocks := firstMsg["content"].([]any)
	require.Len(t, contentBlocks, 2)

	b0 := contentBlocks[0].(map[string]any)
	assert.Equal(t, staticInstructions, b0["text"])
	assert.Equal(t, map[string]any{"type": "ephemeral"}, b0["cache_control"])

	b1 := contentBlocks[1].(map[string]any)
	assert.Equal(t, taskPart, b1["text"])
	assert.Nil(t, b1["cache_control"])
}
