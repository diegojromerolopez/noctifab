package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestSaveStateWithBackoff(t *testing.T) {
	t.Run("when the first save succeeds, it saves exactly once", func(t *testing.T) {
		repo := &mockConflictRepo{mockRepo: mockRepo{state: &domain.State{ID: "s1"}}, failSaves: 0}
		err := saveStateWithBackoff(context.Background(), repo, func(s *domain.State) {
			s.StoryStatus = domain.StoryPaused
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.saveCount != 1 {
			t.Errorf("expected 1 save, got %d", repo.saveCount)
		}
		if repo.state.StoryStatus != domain.StoryPaused {
			t.Errorf("expected paused story, got %q", repo.state.StoryStatus)
		}
	})

	t.Run("when version conflicts occur, it retries with backoff until success", func(t *testing.T) {
		repo := &mockConflictRepo{mockRepo: mockRepo{state: &domain.State{ID: "s1"}}, failSaves: 3}
		start := time.Now()
		err := saveStateWithBackoff(context.Background(), repo, func(s *domain.State) {
			s.StoryStatus = domain.StoryCancelled
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.saveCount != 4 {
			t.Errorf("expected 4 save attempts (3 conflicts + 1 success), got %d", repo.saveCount)
		}
		// Exponential backoff: at least ~50ms*0.8 + 100ms*0.8 + 200ms*0.8 total.
		if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
			t.Errorf("expected backoff sleeps, finished too fast: %v", elapsed)
		}
	})

	t.Run("when conflicts persist beyond the retry budget, it returns the conflict error", func(t *testing.T) {
		repo := &mockConflictRepo{mockRepo: mockRepo{state: &domain.State{ID: "s1"}}, failSaves: 100}
		err := saveStateWithBackoff(context.Background(), repo, func(s *domain.State) {})
		if err == nil {
			t.Fatal("expected error after exhausting retries")
		}
	})
}

// cannedSummariesRepo returns a fixed set of summaries so tests can prove
// the handler serves repository projections rather than full states.
type cannedSummariesRepo struct {
	mockRepo
	summaries []domain.StateSummary
}

func (r *cannedSummariesRepo) LoadAllSummaries(_ context.Context) ([]domain.StateSummary, error) {
	return r.summaries, nil
}

// failingSummariesRepo simulates a summary query failure.
type failingSummariesRepo struct {
	mockRepo
}

func (r *failingSummariesRepo) LoadAllSummaries(_ context.Context) ([]domain.StateSummary, error) {
	return nil, errors.New("summaries query failed")
}

func TestAPIStatusEndpointReturnsLightweightSummaries(t *testing.T) {
	now := time.Now()
	state := &domain.State{
		ID: "story-1",
		Tasks: []domain.Task{
			{ID: "t1", Title: "T1", Description: strings.Repeat("very long description ", 100), Status: domain.TaskSuccess, CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
			{ID: "t2", Title: "T2", Status: domain.TaskPending, CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now.Add(-time.Minute)},
			{ID: "t3", Title: "T3", Status: domain.TaskFailed, CreatedAt: now.Add(-20 * time.Minute), UpdatedAt: now.Add(-2 * time.Minute)},
		},
		Files:       []domain.FileInfo{{Path: "a.go", Size: 10}},
		LastActions: []domain.Action{{Tool: "evaluate", Result: strings.Repeat("log", 1000)}},
		StoryStatus: domain.StoryRunning,
		BuildStatus: domain.BuildFailing,
		Metadata:    domain.StateMetadata{FeatureName: "feat", IntegrationBranch: "noctifab/feature-x", BaseBranch: "main"},
	}
	repo := &mockRepo{state: state}
	mux := newDaemonMux(repo, NewCommandMailbox(repo), nil, &mockLLM{}, nil)

	t.Run("when GET /api/v1/status is called, it returns summaries without files, actions, or task bodies", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var summaries []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &summaries); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(summaries) != 1 {
			t.Fatalf("expected 1 summary, got %d", len(summaries))
		}
		s := summaries[0]
		if s["id"] != "story-1" || s["story_status"] != "RUNNING" || s["build_status"] != "FAILING" {
			t.Errorf("unexpected summary identifiers: %+v", s)
		}
		if s["total_tasks"] != float64(3) {
			t.Errorf("expected total_tasks=3, got %v", s["total_tasks"])
		}
		counts, _ := s["task_counts"].(map[string]any)
		if counts["SUCCESS"] != float64(1) || counts["PENDING"] != float64(1) || counts["FAILED"] != float64(1) {
			t.Errorf("unexpected task counts: %v", counts)
		}
		body := rec.Body.String()
		if strings.Contains(body, "very long description") {
			t.Error("summary must not include full task bodies")
		}
		if strings.Contains(body, "a.go") {
			t.Error("summary must not include the file index")
		}
		if strings.Contains(body, "logloglog") {
			t.Error("summary must not include LastActions results")
		}
	})

	t.Run("when GET /api/v1/status is called, it serves repository summaries instead of loading full states", func(t *testing.T) {
		summariesRepo := &cannedSummariesRepo{mockRepo: mockRepo{state: state}}
		summariesRepo.summaries = []domain.StateSummary{{ID: "canned-1", StoryStatus: "RUNNING", TaskCounts: map[string]int{}}}
		summariesMux := newDaemonMux(summariesRepo, NewCommandMailbox(summariesRepo), nil, &mockLLM{}, nil)

		rec := httptest.NewRecorder()
		summariesMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var summaries []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &summaries); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(summaries) != 1 || summaries[0]["id"] != "canned-1" {
			t.Errorf("expected the repository-provided summary, got %+v", summaries)
		}
	})

	t.Run("when the repository summary query fails, it returns 500", func(t *testing.T) {
		failRepo := &failingSummariesRepo{mockRepo: mockRepo{state: state}}
		failMux := newDaemonMux(failRepo, NewCommandMailbox(failRepo), nil, &mockLLM{}, nil)

		rec := httptest.NewRecorder()
		failMux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", rec.Code)
		}
	})

	t.Run("when a non-GET method is used on /api/v1/status, it returns 405", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/status", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("when GET /statusz is called, it strips the file index and caps the action log", func(t *testing.T) {
		manyActions := make([]domain.Action, 60)
		for i := range manyActions {
			manyActions[i] = domain.Action{Tool: "t", Result: "r"}
		}
		repo.state.Files = []domain.FileInfo{{Path: "hidden.go"}}
		repo.state.LastActions = manyActions

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/statusz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var got domain.State
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("failed to decode statusz: %v", err)
		}
		if len(got.Files) != 0 {
			t.Errorf("expected files stripped from /statusz, got %d entries", len(got.Files))
		}
		if len(got.LastActions) > 20 {
			t.Errorf("expected LastActions capped at 20 in /statusz, got %d", len(got.LastActions))
		}
	})
}

func TestStoryStatusHandlers(t *testing.T) {
	newMux := func(t *testing.T) (*http.ServeMux, *mockRepo) {
		t.Helper()
		repo := &mockRepo{state: &domain.State{ID: "s1", StoryStatus: domain.StoryRunning}}
		return newDaemonMux(repo, NewCommandMailbox(repo), nil, &mockLLM{}, nil), repo
	}

	cases := []struct {
		path   string
		expect domain.StoryStatus
	}{
		{"/api/v1/pause", domain.StoryPaused},
		{"/api/v1/resume", domain.StoryRunning},
		{"/api/v1/cancel", domain.StoryCancelled},
	}
	for _, tc := range cases {
		t.Run("when POST "+tc.path+" is called, it transitions the story status", func(t *testing.T) {
			mux, repo := newMux(t)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tc.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			if repo.state.StoryStatus != tc.expect {
				t.Errorf("expected story status %q, got %q", tc.expect, repo.state.StoryStatus)
			}
		})

		t.Run("when GET "+tc.path+" is called, it returns 405", func(t *testing.T) {
			mux, _ := newMux(t)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected 405, got %d", rec.Code)
			}
		})
	}
}

func TestDaemonServerHasTimeouts(t *testing.T) {
	t.Run("when the daemon server is built, it has read/write/idle timeouts", func(t *testing.T) {
		repo := &mockRepo{state: &domain.State{ID: "s1"}}
		server := StartDaemonServer(repo, NewCommandMailbox(repo), nil, &mockLLM{}, nil)
		defer func() { _ = server.Close() }()

		if server.ReadHeaderTimeout != 5*time.Second {
			t.Errorf("expected ReadHeaderTimeout=5s, got %v", server.ReadHeaderTimeout)
		}
		if server.ReadTimeout != 30*time.Second {
			t.Errorf("expected ReadTimeout=30s, got %v", server.ReadTimeout)
		}
		if server.WriteTimeout != 60*time.Second {
			t.Errorf("expected WriteTimeout=60s, got %v", server.WriteTimeout)
		}
		if server.IdleTimeout != 120*time.Second {
			t.Errorf("expected IdleTimeout=120s, got %v", server.IdleTimeout)
		}
	})
}
