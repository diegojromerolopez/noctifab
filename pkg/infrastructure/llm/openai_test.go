package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIProviderClient_Call(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req["model"] != "gpt-4o" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"content": "openai response text",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewOpenAIProviderClient("openai", server.URL, 0)
		res, err := client.Call(context.Background(), "gpt-4o", "test-key", "hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(res) != "openai response text" {
			t.Errorf("expected 'openai response text', got %s", string(res))
		}
	})

	t.Run("http error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		}))
		defer server.Close()

		client := NewOpenAIProviderClient("openai", server.URL, 0)
		_, err := client.Call(context.Background(), "gpt-4o", "test-key", "hello")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestOpenAIProviderClient_GetAvailableModels(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.URL.Path != "/models" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"data": []map[string]any{
					{"id": "gpt-4o"},
					{"id": "gpt-4o-mini"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewOpenAIProviderClient("openai", server.URL, 0)
		models, err := client.GetAvailableModels(context.Background(), "test-key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(models) != 2 || models[0] != "gpt-4o" || models[1] != "gpt-4o-mini" {
			t.Errorf("unexpected models returned: %v", models)
		}
	})
}
