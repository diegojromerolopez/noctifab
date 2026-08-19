package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebServer_SpecEndpoints(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spec-web-test-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	specPath := filepath.Join(tempDir, "SPEC.md")
	require.NoError(t, os.WriteFile(specPath, []byte("# Initial Test Spec"), 0644))

	// Pre-seed snapshots v1 and v2
	snapshots := storage.NewSpecSnapshotManager(tempDir)
	_, _, err = snapshots.SaveSnapshot(1, "# Initial Test Spec v1", "")
	require.NoError(t, err)
	_, _, err = snapshots.SaveSnapshot(2, "# Initial Test Spec v2 with TLS", "+ TLS")
	require.NoError(t, err)

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	require.NoError(t, os.Chdir(tempDir))

	server := NewWebServer(WebServerConfig{Host: "127.0.0.1", Port: 8080}, nil, nil, nil)
	mux := server.buildMux()

	t.Run("GET /api/v1/spec returns specification content and model roles", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/spec", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "Initial Test Spec")
		assert.Contains(t, rec.Body.String(), "Product Manager")
		assert.Contains(t, rec.Body.String(), `"available_versions":[1,2]`)
	})

	t.Run("POST /api/v1/spec/checkout time-travels to revision v1", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/spec/checkout", strings.NewReader(`{"version":1}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "Initial Test Spec v1")
		assert.Contains(t, rec.Body.String(), `"active_version":1`)
	})

	t.Run("POST /api/v1/spec/approve finalizes specification", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/spec/approve", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "approved")
	})

	t.Run("POST /api/v1/spec/refine handles empty prompt rejection", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/spec/refine", strings.NewReader(`{"prompt":""}`))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestWebServer_SpecGETIsReadonlyAndLoadsYAMLConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "spec-web-yaml-*")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	noctifabDir := filepath.Join(tempDir, ".noctifab")
	require.NoError(t, os.MkdirAll(noctifabDir, 0755))

	configYAML := `config_version: "2.0"
vcs:
  provider: "github"
  repository: "owner/repo"
agents:
  product_manager:
    providers:
      - name: "custom-pm-provider"
`
	require.NoError(t, os.WriteFile(filepath.Join(noctifabDir, "config.yaml"), []byte(configYAML), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "SPEC.md"), []byte("# Spec for YAML test"), 0644))

	oldWd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldWd) }()
	require.NoError(t, os.Chdir(tempDir))

	server := NewWebServer(WebServerConfig{Host: "127.0.0.1", Port: 8080}, nil, nil, nil)
	mux := server.buildMux()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/spec", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "custom-pm-provider")

	// Verify GET did NOT write any snapshot files to disk
	snapshots := storage.NewSpecSnapshotManager(tempDir)
	versions, err := snapshots.ListSnapshots()
	require.NoError(t, err)
	assert.Empty(t, versions, "GET /api/v1/spec must be idempotent and not write snapshots to disk")
}
