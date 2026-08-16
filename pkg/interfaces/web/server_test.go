package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

type mockRepo struct {
	state *domain.State
}

func (m *mockRepo) Load(ctx context.Context) (*domain.State, error) {
	if m.state == nil {
		m.state = &domain.State{
			StoryStatus: domain.StoryRunning,
			Tasks: []domain.Task{
				{ID: "task-1", Title: "Initialize Schema", Status: domain.TaskSuccess},
			},
		}
	}
	return m.state, nil
}

func (m *mockRepo) Save(ctx context.Context, s *domain.State) error {
	m.state = s
	return nil
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
	repo := &mockRepo{}
	mailbox := services.NewCommandMailbox(repo)
	go mailbox.Start(context.Background())

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
	})
}
