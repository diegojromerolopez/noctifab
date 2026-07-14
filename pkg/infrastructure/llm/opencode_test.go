package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenCodeProviderClient_Call(t *testing.T) {
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
			if req["model"] != "glm-5.2" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]any{
							"content": "opencode glm-5.2 response",
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewOpenAIProviderClient("opencode", server.URL, 0)
		res, err := client.Call(context.Background(), "glm-5.2", "test-key", "hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(res) != "opencode glm-5.2 response" {
			t.Errorf("expected 'opencode glm-5.2 response', got %s", string(res))
		}
	})

	t.Run("http error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("service unavailable"))
		}))
		defer server.Close()

		client := NewOpenAIProviderClient("opencode", server.URL, 0)
		_, err := client.Call(context.Background(), "glm-5.2", "test-key", "hello")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestOpenCodeProviderClient_GetAvailableModels(t *testing.T) {
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
				{"id": "glm-5.2"},
				{"id": "glm-5.1"},
				{"id": "kimi-k2.7-code"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOpenAIProviderClient("opencode", server.URL, 0)
	models, err := client.GetAvailableModels(context.Background(), "test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 3 || models[0] != "glm-5.2" || models[1] != "glm-5.1" || models[2] != "kimi-k2.7-code" {
		t.Errorf("unexpected models returned: %v", models)
	}
}
