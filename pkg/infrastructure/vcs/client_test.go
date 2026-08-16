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

	t.Run("github provider fallback when token is empty with mock server", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.Method == "POST" && r.URL.Path == "/repos/owner/repo/pulls" {
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"html_url": "https://github.com/owner/repo/pull/55",
				})
				return
			}
			if r.Method == "PUT" && r.URL.Path == "/repos/owner/repo/pulls/55/merge" {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		client := NewClient("github", "owner/repo", "")
		client.BaseURL = server.URL

		pr, err := client.CreatePullRequest(context.Background(), "Title", "Body", "feat", "main")
		if err != nil {
			t.Fatalf("unexpected error on fallback: %v", err)
		}
		if pr == "" {
			t.Errorf("expected non-empty PR URL")
		}

		err = client.MergePullRequest(context.Background(), pr)
		if err != nil {
			t.Fatalf("unexpected error on merge fallback: %v", err)
		}
	})

	t.Run("github provider REST API failure returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message": "Forbidden"}`))
		}))
		defer server.Close()

		client := NewClient("github", "invalid/repo", "bad-token")
		client.BaseURL = server.URL

		_, err := client.CreatePullRequest(context.Background(), "t", "b", "h", "m")
		if err == nil {
			t.Fatalf("expected error on REST API 403 failure")
		}

		err = client.MergePullRequest(context.Background(), "https://github.com/invalid/repo/pull/1")
		if err == nil {
			t.Fatalf("expected error on REST API 403 merge failure")
		}
	})

	t.Run("unsupported provider error", func(t *testing.T) {
		client := NewClient("unknown_vcs", "owner/repo", "token")
		_, err := client.CreatePullRequest(context.Background(), "t", "b", "h", "b")
		if err == nil {
			t.Errorf("expected error for unsupported provider")
		}

		err = client.MergePullRequest(context.Background(), "123")
		if err == nil {
			t.Errorf("expected error for unsupported provider merge")
		}
	})
}
