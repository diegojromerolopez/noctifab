package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredOutputs_JSONSchemaConfigured(t *testing.T) {
	model := "test-schema-model-unique"
	opts := completionOptions{
		enforceJSON: true,
	}

	params := buildChatParams(model, "hello", opts)
	require.NotNil(t, params.ResponseFormat.OfJSONSchema)
	assert.Equal(t, "noctifab_response", params.ResponseFormat.OfJSONSchema.JSONSchema.Name)
	assert.Nil(t, params.ResponseFormat.OfJSONObject)

	// Mark JSON schema unsupported for this model
	globalCapabilityCache.markJSONSchemaUnsupported(model)

	// Now buildChatParams must fallback to OfJSONObject
	paramsFallback := buildChatParams(model, "hello", opts)
	assert.Nil(t, paramsFallback.ResponseFormat.OfJSONSchema)
	require.NotNil(t, paramsFallback.ResponseFormat.OfJSONObject)
}

func TestGemini_ResponseSchemaConfigured(t *testing.T) {
	var receivedBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates": [{
				"content": {
					"parts": [{"text": "{\"reasoning\":\"ok\",\"actions\":[]}"}]
				}
			}],
			"usageMetadata": {
				"promptTokenCount": 10,
				"candidatesTokenCount": 5,
				"cachedContentTokenCount": 2
			}
		}`))
	}))
	defer srv.Close()

	client := NewGeminiProviderClient(srv.URL, 5*time.Second, 5*time.Second, false)
	res, err := client.Call(context.Background(), "gemini-2.5-flash", "testkey", "test prompt", 100, 0.7)
	require.NoError(t, err)
	require.NotNil(t, res)

	genConfig, ok := receivedBody["generationConfig"].(map[string]any)
	require.True(t, ok, "expected generationConfig in payload")
	assert.Equal(t, "application/json", genConfig["responseMimeType"])

	respSchema, ok := genConfig["responseSchema"].(map[string]any)
	require.True(t, ok, "expected responseSchema in generationConfig")
	assert.Equal(t, "OBJECT", respSchema["type"])
}
