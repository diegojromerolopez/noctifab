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

func TestStructuredOutputs_DefaultJSONObject(t *testing.T) {
	model := "test-default-json-model"
	opts := completionOptions{
		enforceJSON: true,
	}

	params := buildChatParams(model, "generate plan with tasks or actions", opts)
	// Default JSON enforcement must use OfJSONObject so dynamic tool arguments
	// (path, content, title, etc.) and planner task structures are not stripped or constrained.
	require.Nil(t, params.ResponseFormat.OfJSONSchema)
	require.NotNil(t, params.ResponseFormat.OfJSONObject)
}

func TestStructuredOutputs_ExplicitJSONSchemaConfigured(t *testing.T) {
	model := "test-schema-model-unique"
	opts := completionOptions{
		enforceJSON: true,
		jsonSchema:  &NoctifabResponseSchema,
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

func TestAdaptOptionsForError_JSONSchemaRejectionFallback(t *testing.T) {
	model := "test-schema-rejection-model"
	opts := completionOptions{
		enforceJSON: true,
		jsonSchema:  &NoctifabResponseSchema,
	}

	err := &httpError{
		StatusCode: http.StatusBadRequest,
		Body:       `{"error": {"message": "Invalid parameter: response_format of type json_schema is not supported"}}`,
	}

	adapted, ok := adaptOptionsForError(opts, err, model)
	require.True(t, ok, "expected adaptOptionsForError to handle json_schema rejection")
	assert.True(t, adapted.enforceJSON)
	assert.Nil(t, adapted.jsonSchema, "expected jsonSchema to be reset to nil for fallback")
	assert.True(t, globalCapabilityCache.isJSONSchemaUnsupported(model))

	// Rebuilding params with adapted options must yield OfJSONObject
	params := buildChatParams(model, "test prompt", adapted)
	assert.Nil(t, params.ResponseFormat.OfJSONSchema)
	require.NotNil(t, params.ResponseFormat.OfJSONObject)
}

func TestGemini_DefaultResponseMimeTypeWithoutSchema(t *testing.T) {
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
					"parts": [{"text": "{\"tasks\":[{\"id\":\"T1\"}]}"}]
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

	// Default call must NOT restrict responseSchema so arbitrary tools and tasks can be generated
	_, hasSchema := genConfig["responseSchema"]
	assert.False(t, hasSchema, "expected no responseSchema in default generationConfig")
}

func TestGemini_ExplicitResponseSchemaConfigured(t *testing.T) {
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
	if gc, ok := client.(*geminiProviderClient); ok {
		gc.SetResponseSchema(map[string]any{
			"type": "OBJECT",
			"properties": map[string]any{
				"reasoning": map[string]any{"type": "STRING"},
			},
		})
	}

	res, err := client.Call(context.Background(), "gemini-2.5-flash", "testkey", "test prompt", 100, 0.7)
	require.NoError(t, err)
	require.NotNil(t, res)

	genConfig, ok := receivedBody["generationConfig"].(map[string]any)
	require.True(t, ok, "expected generationConfig in payload")
	assert.Equal(t, "application/json", genConfig["responseMimeType"])

	respSchema, ok := genConfig["responseSchema"].(map[string]any)
	require.True(t, ok, "expected responseSchema in generationConfig when explicitly set")
	assert.Equal(t, "OBJECT", respSchema["type"])
}
