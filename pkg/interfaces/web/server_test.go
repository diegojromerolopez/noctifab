package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/require"
)

type mockRepo struct {
	mu    sync.RWMutex
	state *domain.State
}

func (m *mockRepo) Load(ctx context.Context) (*domain.State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.state == nil {
		return &domain.State{
			StoryStatus: domain.StoryRunning,
			Tasks: []domain.Task{
				{ID: "task-1", Title: "Initialize Schema", Status: domain.TaskSuccess},
			},
		}, nil
	}
	return m.state.Clone(), nil
}

func (m *mockRepo) Save(ctx context.Context, s *domain.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s == nil {
		m.state = nil
		return nil
	}
	m.state = s.Clone()
	return nil
}

func (m *mockRepo) MutateState(fn func(s *domain.State)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == nil {
		m.state = &domain.State{
			StoryStatus: domain.StoryRunning,
		}
	}
	fn(m.state)
}

func (m *mockRepo) LoadByID(ctx context.Context, id string) (*domain.State, error) {
	return m.Load(ctx)
}

func (m *mockRepo) LoadAll(ctx context.Context) ([]*domain.State, error) {
	st, err := m.Load(ctx)
	if err != nil {
		return nil, err
	}
	return []*domain.State{st}, nil
}

func (m *mockRepo) LoadAllSummaries(ctx context.Context) ([]domain.StateSummary, error) {
	return nil, nil
}

func (m *mockRepo) PruneFinishedStates(ctx context.Context, keepLast int) (int, error) {
	return 0, nil
}

func TestWebServer_Endpoints(t *testing.T) {
	tempDir := t.TempDir()
	repo := &mockRepo{
		state: &domain.State{
			StoryStatus: domain.StoryRunning,
			ProjectPath: tempDir,
			Tasks: []domain.Task{
				{ID: "task-1", Title: "Initialize Schema", Status: domain.TaskSuccess},
			},
		},
	}
	mailboxCtx, cancelMailbox := context.WithCancel(context.Background())
	defer cancelMailbox()

	mailbox := services.NewCommandMailbox(repo)
	go mailbox.Start(mailboxCtx)

	ws := NewWebServer(WebServerConfig{Port: 0, Host: "127.0.0.1"}, repo, mailbox, nil)
	mux := ws.buildMux()

	t.Run("GET / serves index.html", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Noctifab") {
			t.Errorf("expected body to contain Noctifab, got: %s", rec.Body.String())
		}
	})

	t.Run("GET /api/v1/state returns state snapshot", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/state", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Initialize Schema") {
			t.Errorf("expected state to contain tasks, got: %s", rec.Body.String())
		}
	})

	t.Run("GET /api/v1/state includes fallback_used, last_resort_used, and active agents", func(t *testing.T) {
		repo.MutateState(func(s *domain.State) {
			s.Tasks = []domain.Task{
				{ID: "task-99", Title: "Deadlocked Task", Status: domain.TaskSuccess, FallbackUsed: true, LastResortUsed: true},
			}
			s.ActiveAgents = []domain.Agent{
				{ID: "agent-fallback-task-99", Role: domain.AgentRoleFallback, Status: domain.AgentWorking, TaskID: "task-99"},
			}
		})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/state", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"fallback_used":true`) {
			t.Errorf("expected state to contain fallback_used: true, got: %s", body)
		}
		if !strings.Contains(body, `"last_resort_used":true`) {
			t.Errorf("expected state to contain last_resort_used: true, got: %s", body)
		}
		if !strings.Contains(body, `"role":"FALLBACK"`) {
			t.Errorf("expected state to contain role: FALLBACK, got: %s", body)
		}
	})

	t.Run("POST /api/v1/steer submits steering directive", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/steer", strings.NewReader(`{"directive":"Use Redis"}`))
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Errorf("expected 202 Accepted, got %d", rec.Code)
		}
	})

	t.Run("POST /api/v1/orders submits new order", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(`{"prompt":"Build API"}`))
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Errorf("expected 202 Accepted, got %d", rec.Code)
		}
		require.Eventually(t, func() bool {
			st, err := repo.Load(context.Background())
			return err == nil && st != nil && len(st.Orders) > 0
		}, 2*time.Second, 5*time.Millisecond, "expected order to be persisted into state by mailbox")
	})

	t.Run("POST /api/v1/pause pauses story", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pause", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /api/v1/resume resumes story", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/resume", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("GET /api/v1/clarifications lists clarifications", func(t *testing.T) {
		repo.MutateState(func(s *domain.State) {
			s.Clarifications = []domain.Clarification{
				{ID: "c-1", Question: "Use SQLite or PostgreSQL?", Resolved: false},
			}
		})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/clarifications?pending=true", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Use SQLite or PostgreSQL?") {
			t.Errorf("expected clarification in response, got: %s", rec.Body.String())
		}
	})

	t.Run("POST /api/v1/clarifications/{id}/resolve resolves question", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/clarifications/c-1/resolve", strings.NewReader(`{"answer":"Use SQLite"}`))
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Errorf("expected 202 Accepted, got %d", rec.Code)
		}
	})

	t.Run("GET /api/v1/orders/list returns orders", func(t *testing.T) {
		repo.MutateState(func(s *domain.State) {
			s.Orders = []domain.StoryOrder{
				{ID: "order-1", Prompt: "Build Auth", Status: "PENDING"},
			}
		})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/list", nil)
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Build Auth") {
			t.Errorf("expected order in response, got: %s", rec.Body.String())
		}
	})
}
