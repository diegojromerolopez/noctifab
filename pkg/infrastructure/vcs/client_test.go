package vcs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVCSClient(t *testing.T) {
	t.Run("mock provider success", func(t *testing.T) {
		client := NewClient("mock", "owner/repo", "")
		pr, err := client.CreatePullRequest(context.Background(), "title", "body", "head", "base")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "https://github.com/owner/repo/pull/123"
		if pr != expected {
			t.Errorf("expected %s, got %s", expected, pr)
		}

		err = client.MergePullRequest(context.Background(), pr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("github provider REST API success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer token-xyz" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			if r.Method == "POST" && r.URL.Path == "/repos/owner/repo/pulls" {
				w.WriteHeader(http.StatusCreated)
				response := map[string]string{
					"html_url": "https://github.com/owner/repo/pull/99",
				}
				_ = json.NewEncoder(w).Encode(response)
				return
			}

			if r.Method == "PUT" && r.URL.Path == "/repos/owner/repo/pulls/99/merge" {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"merged": true}`))
				return
			}

			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient("github", "owner/repo", "token-xyz")
		client.BaseURL = server.URL

		pr, err := client.CreatePullRequest(context.Background(), "Implement views", "description", "feature/views", "main")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := "https://github.com/owner/repo/pull/99"
		if pr != expected {
			t.Errorf("expected %s, got %s", expected, pr)
		}

		err = client.MergePullRequest(context.Background(), pr)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
