package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicProviderClient_Call(t *testing.T) {
	t.Run("success single text block", func(t *testing.T) {
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
			if _, exists := req["temperature"]; exists {
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

		client := NewAnthropicProviderClient(server.URL, 0, 0, false)
		res, err := client.Call(context.Background(), "claude-3-opus-20240229", "test-key", "hello", 4096, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(res) != "anthropic response text" {
			t.Errorf("expected 'anthropic response text', got %s", string(res))
		}
	})

	t.Run("thinking block preceding text block", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"content": []map[string]any{
					{
						"type":     "thinking",
						"thinking": "analyzing user request...",
					},
					{
						"type": "text",
						"text": `{"reasoning": "done", "actions": []}`,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewAnthropicProviderClient(server.URL, 0, 0, false)
		res, err := client.Call(context.Background(), "claude-3-5-sonnet-latest", "test-key", "hello", 4096, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := `{"reasoning": "done", "actions": []}`
		if string(res) != expected {
			t.Errorf("expected '%s', got '%s'", expected, string(res))
		}
	})

	t.Run("multi-block text concatenation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"content": []map[string]any{
					{
						"type": "text",
						"text": "part 1",
					},
					{
						"type": "text",
						"text": "part 2",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewAnthropicProviderClient(server.URL, 0, 0, false)
		res, err := client.Call(context.Background(), "claude-3-opus-20240229", "test-key", "hello", 4096, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "part 1\npart 2"
		if string(res) != expected {
			t.Errorf("expected '%s', got '%s'", expected, string(res))
		}
	})

	t.Run("temperature parameter set when greater than zero", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			temp, exists := req["temperature"].(float64)
			if !exists || temp != 0.7 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "ok"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewAnthropicProviderClient(server.URL, 0, 0, false)
		res, err := client.Call(context.Background(), "claude-3-opus-20240229", "test-key", "hello", 4096, 0.7)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(res) != "ok" {
			t.Errorf("expected 'ok', got '%s'", string(res))
		}
	})

	t.Run("http error status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal server error"))
		}))
		defer server.Close()

		client := NewAnthropicProviderClient(server.URL, 0, 0, false)
		_, err := client.Call(context.Background(), "claude-3-opus-20240229", "test-key", "hello", 4096, 0.0)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("empty or missing content array", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"content": []map[string]any{},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewAnthropicProviderClient(server.URL, 0, 0, false)
		_, err := client.Call(context.Background(), "claude-3-opus-20240229", "test-key", "hello", 4096, 0.0)
		if err == nil {
			t.Fatal("expected error for empty content array, got nil")
		}
	})
}

func TestAnthropicProviderClient_GetAvailableModels(t *testing.T) {
	t.Run("success fetching models", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.Header.Get("X-API-Key") != "test-key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := map[string]any{
				"data": []map[string]any{
					{"id": "claude-3-5-sonnet-20241022"},
					{"id": "claude-3-opus-20240229"},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := NewAnthropicProviderClient(server.URL, 0, 0, false)
		models, err := client.GetAvailableModels(context.Background(), "test-key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(models) != 2 || models[0] != "claude-3-5-sonnet-20241022" {
			t.Errorf("unexpected models returned: %v", models)
		}
	})

	t.Run("http error on models endpoint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("Forbidden"))
		}))
		defer server.Close()

		client := NewAnthropicProviderClient(server.URL, 0, 0, false)
		_, err := client.GetAvailableModels(context.Background(), "test-key")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
