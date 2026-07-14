package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

		client := NewGeminiProviderClient(server.URL, 0)
		res, err := client.Call(context.Background(), "gemini-2.5-pro", "test-key", "hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(res) != "gemini response text" {
			t.Errorf("expected 'gemini response text', got %s", string(res))
		}
	})

	t.Run("http error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		}))
		defer server.Close()

		client := NewGeminiProviderClient(server.URL, 0)
		_, err := client.Call(context.Background(), "gemini-2.5-pro", "test-key", "hello")
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

		client := NewGeminiProviderClient(server.URL, 0)
		models, err := client.GetAvailableModels(context.Background(), "test-key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(models) != 2 || models[0] != "gemini-2.5-pro" || models[1] != "gemini-2.5-flash" {
			t.Errorf("unexpected models returned: %v", models)
		}
	})
}
