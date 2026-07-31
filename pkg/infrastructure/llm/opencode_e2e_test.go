package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// TestOpenCodeProviderE2E exercises the full completion pipeline
// (Client.Complete dispatch -> openaiProviderClient.Call -> HTTP transport ->
// ExtractJSONBlock -> LenientUnmarshal -> domain.LLMResponse) for the opencode
// provider against an in-process OpenAI-compatible mock server. This is the
// e2e happy path per AGENTS.md; the only mocked boundary is the remote network
// endpoint (the real paid opencode.ai API is not hit from CI).
func TestOpenCodeProviderE2E(t *testing.T) {
	t.Run("when the opencode Go endpoint returns a full JSON action block, it Complete parses it into a domain.LLMResponse", func(t *testing.T) {
		const wantReasoning = "Planning two tasks for the auth feature."
		wantActions := []domain.LLMAction{
			{
				Tool: "add_task",
				Args: map[string]any{
					"title":       "Add login endpoint",
					"description": "Implement POST /login issuing a session token.",
					"change_type": "FEATURE",
					"depends_on":  []any{},
				},
			},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("unexpected method %q", r.Method)
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.Header.Get("Authorization") != "Bearer go-test-key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			// The url override is the full completions endpoint (httptest server root),
			// so we do not assert on r.URL.Path here; the production default-URL path
			// (/chat/completions) is exercised by the openai.go switch unit tests.
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req["model"] != "glm-5.2" {
				t.Errorf("expected model glm-5.2, got %v", req["model"])
			}
			if req["temperature"] != 0.0 {
				t.Errorf("expected temperature 0.0, got %v", req["temperature"])
			}

			content := `{"reasoning":"` + wantReasoning + `","actions":[{"tool":"add_task","args":{"title":"Add login endpoint","description":"Implement POST /login issuing a session token.","change_type":"FEATURE","depends_on":[]}}]}`
			resp := map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": content}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		c := NewClient("opencode", "glm-5.2", "go-test-key", 1, 0, server.URL)
		res, err := c.Complete(context.Background(), "Decompose specification into tasks: add an auth feature")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected non-nil LLMResponse")
			return
		}
		if res.Reasoning != wantReasoning {
			t.Errorf("reasoning: got %q, want %q", res.Reasoning, wantReasoning)
		}
		if len(res.Actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(res.Actions))
		}
		if res.Actions[0].Tool != wantActions[0].Tool {
			t.Errorf("tool: got %q, want %q", res.Actions[0].Tool, wantActions[0].Tool)
		}
		if res.Actions[0].Args["title"] != "Add login endpoint" {
			t.Errorf("title arg: got %v", res.Actions[0].Args["title"])
		}
	})

	t.Run("when the opencode Go endpoint returns transient 503 then 200, it Complete retries and succeeds on the next attempt", func(t *testing.T) {
		var calls int
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"message":"overloaded"}}`))
				return
			}
			content := `{"reasoning":"recovered","actions":[{"tool":"noop","args":{}}]}`
			resp := map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": content}},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		c := NewClient("opencode", "glm-5.2", "go-test-key", 5, 0, server.URL)
		res, err := c.Complete(context.Background(), "Execute task: implement feature")
		if err != nil {
			t.Fatalf("unexpected error after retry: %v", err)
		}
		if res == nil || len(res.Actions) != 1 || res.Actions[0].Tool != "noop" {
			t.Fatalf("unexpected response: %+v", res)
		}
		if calls < 2 {
			t.Errorf("expected at least 2 calls, got %d", calls)
		}
	})

	t.Run("when the opencode model glm-5.2 returns an error, it falls back to the next available lower model glm-5.1 and succeeds", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/models" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				resp := map[string]any{
					"data": []map[string]any{
						{"id": "glm-5.2"},
						{"id": "glm-5.1"},
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
				return
			}

			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			model, _ := req["model"].(string)
			if model == "glm-5.2" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"message":"internal error in glm-5.2"}}`))
				return
			}

			if model == "glm-5.1" {
				content := `{"reasoning":"fallback successful","actions":[{"tool":"noop","args":{}}]}`
				resp := map[string]any{
					"choices": []map[string]any{
						{"message": map[string]any{"content": content}},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
				return
			}

			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		c := NewClient("opencode", "glm-5.2", "go-test-key", 1, 0, server.URL)
		res, err := c.Complete(context.Background(), "Execute task: fallback test")
		if err != nil {
			t.Fatalf("unexpected error during fallback: %v", err)
		}
		if res == nil || res.Reasoning != "fallback successful" {
			t.Fatalf("expected fallback response, got: %+v", res)
		}
	})
}
