package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestClarificationPoller(t *testing.T) {
	t.Run("when there are clarifications, it prints them and resolves them", func(t *testing.T) {
		var resolvedID, resolvedAnswer string
		var resolveCalls int

		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/clarifications", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]PendingClarification{
				{
					ID:       "c1",
					Question: "What is your favorite color?",
					Resolved: false,
				},
			})
		})
		mux.HandleFunc("/api/v1/clarifications/c1/resolve", func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			resolvedID = "c1"
			resolvedAnswer = body["answer"].(string)
			resolveCalls++
			w.WriteHeader(http.StatusOK)
		})

		srv := httptest.NewServer(mux)
		defer srv.Close()

		client := NewDaemonClientWithBase(srv.URL)
		var out bytes.Buffer
		in := bytes.NewBufferString("blue\n")

		poller := NewClarificationPoller(client, 10*time.Millisecond, in, &out)

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		poller.checkAndPrompt(ctx)

		assert.Contains(t, out.String(), "CLARIFICATION NEEDED")
		assert.Contains(t, out.String(), "What is your favorite color?")
		assert.Contains(t, out.String(), "Answer sent.")
		assert.Equal(t, 1, resolveCalls)
		assert.Equal(t, "c1", resolvedID)
		assert.Equal(t, "blue", resolvedAnswer)
	})
}
