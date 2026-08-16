package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type mockStateRepo struct {
	state *domain.State
}

func (m *mockStateRepo) Load(ctx context.Context) (*domain.State, error) {
	if m.state == nil {
		m.state = &domain.State{}
	}
	return m.state, nil
}

func (m *mockStateRepo) Save(ctx context.Context, s *domain.State) error {
	m.state = s
	return nil
}

func (m *mockStateRepo) LoadByID(ctx context.Context, id string) (*domain.State, error) {
	return m.Load(ctx)
}

func (m *mockStateRepo) LoadAll(ctx context.Context) ([]*domain.State, error) {
	st, err := m.Load(ctx)
	if err != nil {
		return nil, err
	}
	return []*domain.State{st}, nil
}

func (m *mockStateRepo) LoadAllSummaries(ctx context.Context) ([]domain.StateSummary, error) {
	return nil, nil
}

func (m *mockStateRepo) PruneFinishedStates(ctx context.Context, keepLast int) (int, error) {
	return 0, nil
}

func TestSteerCmd_Execute(t *testing.T) {
	repo := &mockStateRepo{
		state: &domain.State{
			Tasks: []domain.Task{
				{
					ID:     "task-1",
					Title:  "Setup DB",
					Status: domain.TaskInProgress,
				},
			},
		},
	}

	cmd := &SteerCmd{
		TaskID:    "task-1",
		Directive: "Use Postgres instead of SQLite",
	}

	err := cmd.Execute(context.Background(), repo)
	if err != nil {
		t.Fatalf("unexpected error executing SteerCmd: %v", err)
	}

	if len(repo.state.Tasks[0].UserDirectives) != 1 {
		t.Fatalf("expected 1 directive on task, got %d", len(repo.state.Tasks[0].UserDirectives))
	}
	if repo.state.Tasks[0].UserDirectives[0] != "Use Postgres instead of SQLite" {
		t.Errorf("got %q, want expected directive", repo.state.Tasks[0].UserDirectives[0])
	}
}

func TestSteerAndOrderRoutes(t *testing.T) {
	repo := &mockStateRepo{state: &domain.State{}}
	mailbox := NewCommandMailbox(repo)
	go mailbox.Start(context.Background())

	mux := newDaemonMux(repo, mailbox, nil, nil, nil)

	t.Run("POST /api/v1/steer accepted", func(t *testing.T) {
		body := `{"task_id":"task-1","directive":"Optimize query plan"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/steer", strings.NewReader(body))
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Errorf("expected 202 Accepted, got %d", rec.Code)
		}
	})

	t.Run("POST /api/v1/orders accepted", func(t *testing.T) {
		body := `{"prompt":"Add user authentication endpoints"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Errorf("expected 202 Accepted, got %d", rec.Code)
		}
	})
}
