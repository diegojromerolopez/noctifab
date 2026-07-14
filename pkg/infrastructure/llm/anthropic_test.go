package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicProviderClient_Call(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.Header.Get("X-API-Key") != "test-key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req["model"] != "claude-3-opus-20240229" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"content": []map[string]any{
					{
						"type": "text",
						"text": "anthropic response text",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewAnthropicProviderClient(server.URL, 0)
		res, err := client.Call(context.Background(), "claude-3-opus-20240229", "test-key", "hello")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(res) != "anthropic response text" {
			t.Errorf("expected 'anthropic response text', got %s", string(res))
		}
	})

	t.Run("http error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		}))
		defer server.Close()

		client := NewAnthropicProviderClient(server.URL, 0)
		_, err := client.Call(context.Background(), "claude-3-opus-20240229", "test-key", "hello")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("GetAvailableModels unsupported", func(t *testing.T) {
		client := NewAnthropicProviderClient("", 0)
		_, err := client.GetAvailableModels(context.Background(), "test-key")
		if err == nil {
			t.Fatal("expected error for GetAvailableModels on anthropic client, got nil")
		}
	})
}
