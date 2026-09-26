package services

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRescueRepo struct {
	state *domain.State
}

func (m *mockRescueRepo) Load(ctx context.Context) (*domain.State, error) {
	if m.state == nil {
		m.state = &domain.State{}
	}
	return m.state, nil
}

func (m *mockRescueRepo) Save(ctx context.Context, st *domain.State) error {
	m.state = st
	return nil
}

func (m *mockRescueRepo) LoadByID(ctx context.Context, id string) (*domain.State, error) {
	return m.Load(ctx)
}

func (m *mockRescueRepo) LoadAll(ctx context.Context) ([]*domain.State, error) {
	st, err := m.Load(ctx)
	if err != nil {
		return nil, err
	}
	return []*domain.State{st}, nil
}

func (m *mockRescueRepo) LoadAllSummaries(ctx context.Context) ([]domain.StateSummary, error) {
	return nil, nil
}

func (m *mockRescueRepo) PruneFinishedStates(ctx context.Context, keepLast int) (int, error) {
	return 0, nil
}

func TestOrchestrator_HandleDeadlockOrRescue(t *testing.T) {
	t.Run("when all tasks finished returns false nil", func(t *testing.T) {
		st := &domain.State{
			ID: "test-state",
			Tasks: []domain.Task{
				{ID: "T1", Status: domain.TaskSuccess},
			},
		}
		repo := &mockRescueRepo{state: st}
		orch := &Orchestrator{
			cfg:  OrchestratorConfig{},
			repo: repo,
		}

		resumed, err := orch.handleDeadlockOrRescue(context.Background(), st)
		assert.False(t, resumed)
		assert.NoError(t, err)
	})

	t.Run("when workers active returns false nil", func(t *testing.T) {
		st := &domain.State{
			ID: "test-state",
			Tasks: []domain.Task{
				{ID: "T1", Status: domain.TaskPending},
			},
			ActiveAgents: []domain.Agent{
				{ID: "a1", Role: domain.AgentRoleGenerator, Status: domain.AgentWorking},
			},
		}
		repo := &mockRescueRepo{state: st}
		orch := &Orchestrator{
			cfg:  OrchestratorConfig{},
			repo: repo,
		}

		resumed, err := orch.handleDeadlockOrRescue(context.Background(), st)
		assert.False(t, resumed)
		assert.NoError(t, err)
	})

	t.Run("when tasks in progress returns false nil", func(t *testing.T) {
		st := &domain.State{
			ID: "test-state",
			Tasks: []domain.Task{
				{ID: "T1", Status: domain.TaskInProgress},
			},
		}
		repo := &mockRescueRepo{state: st}
		orch := &Orchestrator{
			cfg:  OrchestratorConfig{},
			repo: repo,
		}

		resumed, err := orch.handleDeadlockOrRescue(context.Background(), st)
		assert.False(t, resumed)
		assert.NoError(t, err)
	})

	t.Run("when failed task exists and fallback disabled aborts with deadlock", func(t *testing.T) {
		st := &domain.State{
			ID: "test-state",
			Tasks: []domain.Task{
				{ID: "T1", Title: "Task 1", Status: domain.TaskFailed, FailureLog: "compile error"},
				{ID: "T2", Title: "Task 2", Status: domain.TaskPending, DependsOn: []string{"T1"}},
			},
		}
		repo := &mockRescueRepo{state: st}
		orch := &Orchestrator{
			cfg: OrchestratorConfig{
				Fallback: config.FallbackAgentConfig{
					Enabled: false,
				},
			},
			repo: repo,
		}

		resumed, err := orch.handleDeadlockOrRescue(context.Background(), st)
		assert.False(t, resumed)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "deadlock detected")
		assert.Equal(t, domain.StoryFailed, st.StoryStatus)
	})
}
