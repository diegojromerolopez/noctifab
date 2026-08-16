package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSteerCmd_Validation(t *testing.T) {
	err := steerCmd.RunE(steerCmd, []string{"   "})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestOrderCmd_Validation(t *testing.T) {
	err := orderCmd.RunE(orderCmd, []string{"   "})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestDaemonClient_SteerAndOrder(t *testing.T) {
	var lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	}))
	defer srv.Close()

	client := services.NewDaemonClientWithBase(srv.URL)

	err := client.SendSteerDirective(context.Background(), "task-1", "Use SQLite")
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/steer", lastPath)

	err = client.SendOrderPrompt(context.Background(), "Add REST endpoints")
	require.NoError(t, err)
	assert.Equal(t, "/api/v1/orders", lastPath)
}
