package usecase

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonClient_All(t *testing.T) {
	t.Run("IsAlive returns true when status is 200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/healthz", r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		client := NewDaemonClientWithBase(srv.URL)
		assert.True(t, client.IsAlive())
	})

	t.Run("SendStartStory POSTs correct body", func(t *testing.T) {
		var received map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/stories", r.URL.Path)
			assert.Equal(t, "POST", r.Method)
			_ = json.NewDecoder(r.Body).Decode(&received)
			w.WriteHeader(http.StatusAccepted)
		}))
		defer srv.Close()

		client := NewDaemonClientWithBase(srv.URL)
		err := client.SendStartStory("roadmap/US-0001.md")
		assert.NoError(t, err)
		assert.Equal(t, "roadmap/US-0001.md", received["path"])
		assert.Equal(t, false, received["directory"])
	})

	t.Run("SendStartDirectory POSTs correct body", func(t *testing.T) {
		var received map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/stories", r.URL.Path)
			assert.Equal(t, "POST", r.Method)
			_ = json.NewDecoder(r.Body).Decode(&received)
			w.WriteHeader(http.StatusAccepted)
		}))
		defer srv.Close()

		client := NewDaemonClientWithBase(srv.URL)
		err := client.SendStartDirectory("/foo/bar")
		assert.NoError(t, err)
		assert.Equal(t, "/foo/bar", received["path"])
		assert.Equal(t, true, received["directory"])
	})

	t.Run("ResolveClarification POSTs correct body", func(t *testing.T) {
		var received map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v1/clarifications/c2/resolve", r.URL.Path)
			assert.Equal(t, "POST", r.Method)
			_ = json.NewDecoder(r.Body).Decode(&received)
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		client := NewDaemonClientWithBase(srv.URL)
		err := client.ResolveClarification("c2", "my answer")
		assert.NoError(t, err)
		assert.Equal(t, "my answer", received["answer"])
	})

	t.Run("GetStatus decodes state response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/statusz", r.URL.Path)
			st := &domain.State{
				BuildStatus: domain.BuildPassing,
			}
			_ = json.NewEncoder(w).Encode(st)
		}))
		defer srv.Close()

		client := NewDaemonClientWithBase(srv.URL)
		state, err := client.GetStatus()
		require.NoError(t, err)
		assert.Equal(t, domain.BuildPassing, state.BuildStatus)
	})
}
