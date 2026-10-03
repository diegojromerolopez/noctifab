package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestGeminiProviderClient_Call(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			contents, ok := req["contents"].([]any)
			if !ok || len(contents) == 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{
								{"text": "gemini response text"},
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewGeminiProviderClient(server.URL, 0, 0, false)
		res, err := client.Call(context.Background(), "gemini-2.5-pro", "test-key", "hello", 4096, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(res.Body) != "gemini response text" {
			t.Errorf("expected 'gemini response text', got %s", string(res.Body))
		}
	})

	t.Run("http error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		}))
		defer server.Close()

		client := NewGeminiProviderClient(server.URL, 0, 0, false)
		_, err := client.Call(context.Background(), "gemini-2.5-pro", "test-key", "hello", 4096, 0.0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestGeminiProviderClient_GetAvailableModels(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"models": []map[string]any{
					{
						"name":                       "models/gemini-2.5-pro",
						"supportedGenerationMethods": []string{"generateContent"},
					},
					{
						"name":                       "models/gemini-2.5-flash",
						"supportedGenerationMethods": []string{"generateContent"},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewGeminiProviderClient(server.URL, 0, 0, false)
		models, err := client.GetAvailableModels(context.Background(), "test-key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(models) != 2 || models[0] != "gemini-2.5-pro" || models[1] != "gemini-2.5-flash" {
			t.Errorf("unexpected models returned: %v", models)
		}
	})
}

func TestGeminiProviderClient_CacheParameters(t *testing.T) {
	t.Run("cachedContent parameter passed in payload", func(t *testing.T) {
		var receivedReq map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&receivedReq)
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{{"text": "cached response"}},
						},
					},
				},
				"usageMetadata": map[string]any{
					"promptTokenCount":        float64(500),
					"candidatesTokenCount":    float64(50),
					"cachedContentTokenCount": float64(400),
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewGeminiProviderClient(server.URL, 0, 0, false)
		if gc, ok := client.(*geminiProviderClient); ok {
			gc.SetExtraBody(map[string]interface{}{
				"cachedContent": "cachedContents/my-cache-resource-123",
			})
		}
		res, err := client.Call(context.Background(), "gemini-2.5-flash", "test-key", "query text", 100, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if receivedReq["cachedContent"] != "cachedContents/my-cache-resource-123" {
			t.Errorf("expected cachedContent in payload, got: %v", receivedReq["cachedContent"])
		}
		if res.Usage.CachedTokens != 400 {
			t.Errorf("expected 400 cached tokens, got %d", res.Usage.CachedTokens)
		}
	})

	t.Run("systemInstruction prefix caching with alternative totalCachedTokens field", func(t *testing.T) {
		var receivedReq map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&receivedReq)
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{{"text": "ok"}},
						},
					},
				},
				"usageMetadata": map[string]any{
					"promptTokenCount":     float64(300),
					"candidatesTokenCount": float64(30),
					"totalCachedTokens":    float64(250),
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewGeminiProviderClient(server.URL, 0, 0, false)
		prompt := "PREFIX INSTRUCTIONS HERE\nDYNAMIC QUESTION"
		ctx := domain.WithCacheablePrefix(context.Background(), len("PREFIX INSTRUCTIONS HERE"))
		res, err := client.Call(ctx, "gemini-2.5-flash", "test-key", prompt, 100, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if receivedReq["systemInstruction"] == nil {
			t.Errorf("expected systemInstruction in payload for cacheable prefix")
		}
		if res.Usage.CachedTokens != 250 {
			t.Errorf("expected 250 cached tokens, got %d", res.Usage.CachedTokens)
		}
	})

	t.Run("thinking-only models omit thinkingConfig with budget 0", func(t *testing.T) {
		var receivedReq map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&receivedReq)
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{{"text": "ok"}},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewGeminiProviderClient(server.URL, 0, 0, false)
		if setter, ok := client.(extraBodySetter); ok {
			setter.SetExtraBody(map[string]interface{}{
				"thinkingConfig": map[string]any{"thinkingBudget": 0},
			})
		} else {
			t.Fatalf("client does not implement extraBodySetter")
		}

		_, err := client.Call(context.Background(), "gemini-3.1-pro-preview", "test-key", "hello", 100, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		genConfig, ok := receivedReq["generationConfig"].(map[string]any)
		if !ok {
			t.Fatalf("expected generationConfig in request payload")
		}
		if _, hasThinkingConfig := genConfig["thinkingConfig"]; hasThinkingConfig {
			t.Errorf("expected thinkingConfig to be omitted for thinking-only model with budget 0, but was present: %v", genConfig["thinkingConfig"])
		}
	})
}
