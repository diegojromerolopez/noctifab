package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPing_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The OpenAI-compatible GetAvailableModels derives the /models path
		// from the configured base URL: when o.url ends with /chat/completions
		// it strips that suffix, otherwise it appends /models. We accept
		// any path ending in /models so both derivation paths pass here.
		if r.URL.Path == "/models" || r.URL.Path == "/v1/models" || r.URL.Path == "/chat/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"glm-5.2"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Pass the completions endpoint; ping's provider client will derive
	// /models from it the same way Client.Complete does in production.
	if _, err := Ping(context.Background(), "opencode", "test-key", server.URL+"/chat/completions"); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestPingAndResolveModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" || r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o"},{"id":"gpt-4o-mini"},{"id":"gpt-3.5-turbo"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Run("exact model match", func(t *testing.T) {
		latency, resolved, err := PingAndResolveModel(context.Background(), "openai", "test-key", server.URL+"/v1", "gpt-4o-mini")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if latency <= 0 {
			t.Errorf("expected positive latency")
		}
		if resolved != "gpt-4o-mini" {
			t.Errorf("expected gpt-4o-mini, got %s", resolved)
		}
	})

	t.Run("missing model falls back to best available", func(t *testing.T) {
		_, resolved, err := PingAndResolveModel(context.Background(), "openai", "test-key", server.URL+"/v1", "gemini-3.6-pro-deprecated")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved != "gpt-4o" {
			t.Errorf("expected best model gpt-4o, got %s", resolved)
		}
	})

	t.Run("empty alias resolves to top model", func(t *testing.T) {
		_, resolved, err := PingAndResolveModel(context.Background(), "openai", "test-key", server.URL+"/v1", "latest")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolved != "gpt-4o" {
			t.Errorf("expected latest model gpt-4o, got %s", resolved)
		}
	})
}

func TestPing_UnsupportedProvider(t *testing.T) {
	_, err := Ping(context.Background(), "bogus", "key", "")
	if err == nil {
		t.Fatal("expected error for unsupported provider, got nil")
	}
}

func TestPing_RegistryProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"deepseek/deepseek-v4-flash"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := Ping(context.Background(), "openrouter", "test-key", server.URL+"/v1"); err != nil {
		t.Fatalf("expected nil error for registry-only provider openrouter, got: %v", err)
	}
}

func TestPing_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"type":"invalid_api_key"}}`))
	}))
	defer server.Close()

	_, err := Ping(context.Background(), "opencode", "bad-key", server.URL+"/models")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !contains(err.Error(), "rejected the API key") && !contains(err.Error(), "401") {
		t.Errorf("expected auth-classified error, got: %v", err)
	}
}

func TestPing_QuotaFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`RESOURCE_EXHAUSTED`))
	}))
	defer server.Close()

	_, err := Ping(context.Background(), "opencode", "key", server.URL+"/models")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !contains(err.Error(), "quota") {
		t.Errorf("expected quota-classified error, got: %v", err)
	}
}

func TestPing_Timeout(t *testing.T) {
	// Point at a non-routable address to trigger a network/timeout error
	// quickly. Using a short context timeout keeps the test fast.
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	_, err := Ping(ctx, "opencode", "key", "http://192.0.2.1:1/models")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (indexOf(haystack, needle) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
