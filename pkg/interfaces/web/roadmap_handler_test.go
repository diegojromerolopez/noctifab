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

func TestRoadmapHandler_Endpoints(t *testing.T) {
	tempDir := t.TempDir()
	storiesDir := filepath.Join(tempDir, "roadmap", "user-stories")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))

	story1 := `# User Story: US-001 - Database Layer

## Description
Implement SQLite storage layer with OCC.

## Definition of Done
- [x] Create schema tables
- [ ] Implement query transactions
- [ ] Add unit tests

### Acceptance Criteria
- Storage repository handles concurrency
- Zero data loss on conflict
`
	require.NoError(t, os.WriteFile(filepath.Join(storiesDir, "US-001-database-layer.md"), []byte(story1), 0644))

	repo := &mockRepo{
		state: &domain.State{
			ID:          "US-001",
			StoryStatus: domain.StoryRunning,
			ProjectPath: tempDir,
			Tasks: []domain.Task{
				{ID: "task-1", Title: "Create tables", Status: domain.TaskSuccess, Progress: 100},
				{ID: "task-2", Title: "Query transactions", Status: domain.TaskInProgress, Progress: 50},
			},
		},
	}

	ws := NewWebServer(WebServerConfig{Port: 0, Host: "127.0.0.1"}, repo, nil, nil)
	mux := ws.buildMux()

	t.Run("GET /api/v1/roadmap returns parsed stories with DoD and progress", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/roadmap", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "US-001")
		assert.Contains(t, rec.Body.String(), "Database Layer")
		assert.Contains(t, rec.Body.String(), "Definition of Done")
		assert.Contains(t, rec.Body.String(), "Create schema tables")
	})

	t.Run("GET /api/v1/states returns all states", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/states", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "US-001")
	})
}

func TestParseStoryMarkdown(t *testing.T) {
	md := `# User Story: US-002 - Authentication Service

## Description
Provide secure JWT token generation and validation.

## Definition of Done
- [x] Password hashing with bcrypt
- [x] JWT token issuer
- [x] Token verification middleware

### Acceptance Criteria
- Token expires in 24h
`
	story := parseStoryMarkdown("US-002-auth-service.md", md)
	assert.Equal(t, "US-002", story.ID)
	assert.Equal(t, "Authentication Service", story.Title)
	assert.Equal(t, 3, story.TotalCheckboxes)
	assert.Equal(t, 3, story.CompletedCheckboxes)
	assert.Equal(t, 100, story.Progress)
	assert.Contains(t, story.DefinitionOfDone, "Password hashing")
	assert.Len(t, story.AcceptanceCriteria, 4)
}
