package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
		if string(res.Body) != "anthropic response text" {
			t.Errorf("expected 'anthropic response text', got %s", string(res.Body))
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
		if string(res.Body) != expected {
			t.Errorf("expected '%s', got '%s'", expected, string(res.Body))
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
		if string(res.Body) != expected {
			t.Errorf("expected '%s', got '%s'", expected, string(res.Body))
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
		if string(res.Body) != "ok" {
			t.Errorf("expected 'ok', got '%s'", string(res.Body))
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

	t.Run("adaptive retry on deprecated temperature error", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if _, hasTemp := req["temperature"]; hasTemp && callCount == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"\` + "`" + `temperature\` + "`" + ` is deprecated for this model."}}`))
				return
			}
			// Attempt 2: temperature should be omitted
			if _, hasTemp := req["temperature"]; hasTemp {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"unexpected temperature"}}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "success without temperature"},
				},
			})
		}))
		defer server.Close()

		client := NewAnthropicProviderClient(server.URL, 0, 0, false)
		res, err := client.Call(context.Background(), "claude-opus-5", "test-key", "hello", 4096, 0.3)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(res.Body) != "success without temperature" {
			t.Errorf("expected 'success without temperature', got %s", string(res.Body))
		}
		if callCount != 2 {
			t.Errorf("expected 2 calls, got %d", callCount)
		}
	})

	t.Run("adaptive retry on max_tokens rejection", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			maxTok, _ := req["max_tokens"].(float64)
			if maxTok > 8192 && callCount == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens: 32768 is greater than maximum allowed 8192"}}`))
				return
			}
			if maxTok != 4096 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"message":"unexpected max_tokens"}}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "success with max_tokens=4096"},
				},
			})
		}))
		defer server.Close()

		client := NewAnthropicProviderClient(server.URL, 0, 0, false)
		res, err := client.Call(context.Background(), "claude-sonnet-5", "test-key", "hello", 32768, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(res.Body) != "success with max_tokens=4096" {
			t.Errorf("expected 'success with max_tokens=4096', got %s", string(res.Body))
		}
		if callCount != 2 {
			t.Errorf("expected 2 calls, got %d", callCount)
		}
	})

	t.Run("adaptive retry on cache_control rejection", func(t *testing.T) {
		callCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			msgs, _ := req["messages"].([]any)
			firstMsg, _ := msgs[0].(map[string]any)
			if _, isSlice := firstMsg["content"].([]any); isSlice && callCount == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"cache_control is not supported"}}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "success without prompt caching"},
				},
			})
		}))
		defer server.Close()

		client := NewAnthropicProviderClient(server.URL, 0, 0, false)
		longPrompt := strings.Repeat("A", 3000)
		res, err := client.Call(context.Background(), "claude-sonnet-5", "test-key", longPrompt, 4096, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(res.Body) != "success without prompt caching" {
			t.Errorf("expected 'success without prompt caching', got %s", string(res.Body))
		}
		if callCount != 2 {
			t.Errorf("expected 2 calls, got %d", callCount)
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
