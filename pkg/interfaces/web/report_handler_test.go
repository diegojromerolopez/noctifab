package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportHandler_Endpoints(t *testing.T) {
	tempDir := t.TempDir()
	reportsDir := filepath.Join(tempDir, ".noctifab", "reports")
	require.NoError(t, os.MkdirAll(reportsDir, 0755))

	reportContent := "# Execution Report\n\nRun succeeded with 100% test coverage."
	require.NoError(t, os.WriteFile(filepath.Join(reportsDir, "20260822_120000_report.md"), []byte(reportContent), 0644))

	sampleFile := "package main\n\nfunc main() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(sampleFile), 0644))

	repo := &mockRepo{
		state: &domain.State{
			ID:          "US-001",
			StoryStatus: domain.StorySuccess,
			BuildStatus: domain.BuildPassing,
			ProjectPath: tempDir,
			Metadata: domain.StateMetadata{
				TotalTokensUsed: 15000,
				FeatureName:     "US-001",
			},
			Tasks: []domain.Task{
				{ID: "task-1", Title: "Initialize Schema", Status: domain.TaskSuccess, Progress: 100},
			},
		},
	}

	ws := NewWebServer(WebServerConfig{Port: 0, Host: "127.0.0.1"}, repo, nil, nil)
	mux := ws.buildMux()

	t.Run("GET /api/v1/report returns report JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/report", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "Execution Report")
	})

	t.Run("GET /api/v1/report?download=true returns markdown attachment", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/report?download=true", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")
		assert.Contains(t, rec.Body.String(), "Execution Report")
	})

	t.Run("GET /api/v1/files/content reads valid workspace file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?path=main.go", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "package main")
	})

	t.Run("GET /api/v1/files/content rejects path traversal outside workspace", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?path=../../etc/passwd", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("GET /api/v1/metrics returns aggregated telemetry", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "total_tokens")
	})
}
