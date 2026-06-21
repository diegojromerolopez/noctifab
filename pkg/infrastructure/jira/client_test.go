package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJiraClient_FetchIssueDescription(t *testing.T) {
	t.Run("successful fetch and ADF parse", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/rest/api/3/issue/KEY-123" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			username, password, ok := r.BasicAuth()
			if !ok || username != "user@company.com" || password != "api-token-xyz" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			response := map[string]any{
				"key": "KEY-123",
				"fields": map[string]any{
					"summary": "Implement Contact CRUD",
					"description": map[string]any{
						"type": "doc",
						"content": []map[string]any{
							{
								"type": "paragraph",
								"content": []map[string]any{
									{"type": "text", "text": "Spec description text"},
								},
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(server.URL, "user@company.com", "api-token-xyz", 3, 10*time.Millisecond)
		desc, err := client.FetchIssueDescription(context.Background(), "KEY-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := "Spec description text"
		if desc != expected {
			t.Errorf("expected description %q, got %q", expected, desc)
		}
	})

	t.Run("fallback to summary on empty description", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := map[string]any{
				"key": "KEY-123",
				"fields": map[string]any{
					"summary": "Default Summary Text",
				},
			}
			_ = json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(server.URL, "", "", 1, 5*time.Millisecond)
		desc, err := client.FetchIssueDescription(context.Background(), "KEY-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := "Default Summary Text"
		if desc != expected {
			t.Errorf("expected fallback to summary %q, got %q", expected, desc)
		}
	})

	t.Run("retry logic on transient errors", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := map[string]any{
				"key": "KEY-123",
				"fields": map[string]any{
					"summary": "Success after retries",
				},
			}
			_ = json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewClient(server.URL, "", "", 3, 5*time.Millisecond)
		desc, err := client.FetchIssueDescription(context.Background(), "KEY-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
		if desc != "Success after retries" {
			t.Errorf("expected description %q, got %q", "Success after retries", desc)
		}
	})
}
