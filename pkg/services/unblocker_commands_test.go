package services

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type inMemoryRepo struct {
	state *domain.State
}

func (r *inMemoryRepo) Load(_ context.Context) (*domain.State, error) {
	cp := *r.state
	tasks := make([]domain.Task, len(r.state.Tasks))
	copy(tasks, r.state.Tasks)
	cp.Tasks = tasks
	actions := make([]domain.Action, len(r.state.LastActions))
	copy(actions, r.state.LastActions)
	cp.LastActions = actions
	agents := make([]domain.Agent, len(r.state.ActiveAgents))
	copy(agents, r.state.ActiveAgents)
	cp.ActiveAgents = agents
	return &cp, nil
}

func (r *inMemoryRepo) Save(_ context.Context, s *domain.State) error {
	r.state = s
	return nil
}

func (r *inMemoryRepo) LoadByID(_ context.Context, _ string) (*domain.State, error) {
	return r.Load(context.Background())
}

func (r *inMemoryRepo) LoadAll(_ context.Context) ([]*domain.State, error) {
	return []*domain.State{r.state}, nil
}

func (r *inMemoryRepo) LoadAllSummaries(_ context.Context) ([]domain.StateSummary, error) {
	return []domain.StateSummary{domain.SummarizeState(r.state)}, nil
}

func (r *inMemoryRepo) PruneFinishedStates(_ context.Context, _ int) (int, error) {
	return 0, nil
}

func newTestState(tasks []domain.Task) *domain.State {
	return &domain.State{
		ID:           "test-state",
		Tasks:        tasks,
		LastActions:  []domain.Action{},
		ActiveAgents: []domain.Agent{},
		BuildStatus:  domain.BuildUnknown,
		StoryStatus:  domain.StoryRunning,
	}
}

func TestResetTaskCmd(t *testing.T) {
	t.Parallel()

	t.Run("when task is IN_PROGRESS, it resets to PENDING and increments Retries", func(t *testing.T) {
		t.Parallel()
		task := domain.Task{ID: "task-1", Status: domain.TaskInProgress, Progress: 50, Retries: 0, MaxRetries: 3, UpdatedAt: time.Now()}
		repo := &inMemoryRepo{state: newTestState([]domain.Task{task})}
		cmd := &ResetTaskCmd{TaskID: "task-1", Reason: "test reset"}

		err := cmd.Execute(context.Background(), repo)

		require.NoError(t, err)
		state, _ := repo.Load(context.Background())
		assert.Equal(t, domain.TaskPending, state.Tasks[0].Status)
		assert.Equal(t, 1, state.Tasks[0].Retries)
		assert.Equal(t, 0, state.Tasks[0].Progress)
	})

	t.Run("when task reaches max retries on reset, it marks task as FAILED", func(t *testing.T) {
		t.Parallel()
		task := domain.Task{ID: "task-1", Status: domain.TaskInProgress, Retries: 2, MaxRetries: 3, UpdatedAt: time.Now()}
		repo := &inMemoryRepo{state: newTestState([]domain.Task{task})}
		cmd := &ResetTaskCmd{TaskID: "task-1", Reason: "final retry reset"}

		err := cmd.Execute(context.Background(), repo)

		require.NoError(t, err)
		state, _ := repo.Load(context.Background())
		assert.Equal(t, domain.TaskFailed, state.Tasks[0].Status)
		assert.Equal(t, 3, state.Tasks[0].Retries)
		assert.Contains(t, state.Tasks[0].FailureLog, "task reset limit reached")
	})

	t.Run("race condition: when task already reached SUCCESS, it skips reset", func(t *testing.T) {
		t.Parallel()
		task := domain.Task{ID: "task-1", Status: domain.TaskSuccess, UpdatedAt: time.Now()}
		repo := &inMemoryRepo{state: newTestState([]domain.Task{task})}
		cmd := &ResetTaskCmd{TaskID: "task-1", Reason: "stale reset"}

		err := cmd.Execute(context.Background(), repo)

		require.NoError(t, err)
		state, _ := repo.Load(context.Background())
		assert.Equal(t, domain.TaskSuccess, state.Tasks[0].Status)
	})

	t.Run("cleans up active working agents assigned to task", func(t *testing.T) {
		t.Parallel()
		task := domain.Task{ID: "task-1", Status: domain.TaskInProgress, UpdatedAt: time.Now()}
		st := newTestState([]domain.Task{task})
		st.ActiveAgents = []domain.Agent{
			{ID: "agent-1", TaskID: "task-1", Status: domain.AgentWorking},
		}
		repo := &inMemoryRepo{state: st}
		cmd := &ResetTaskCmd{TaskID: "task-1", Reason: "reset"}

		err := cmd.Execute(context.Background(), repo)

		require.NoError(t, err)
		state, _ := repo.Load(context.Background())
		assert.Equal(t, domain.AgentCompleted, state.ActiveAgents[0].Status)
	})
}

func TestFailTaskCmd(t *testing.T) {
	t.Parallel()

	t.Run("when task exists, it marks it as FAILED", func(t *testing.T) {
		t.Parallel()
		task := domain.Task{ID: "task-1", Status: domain.TaskInProgress, UpdatedAt: time.Now()}
		repo := &inMemoryRepo{state: newTestState([]domain.Task{task})}
		cmd := &FailTaskCmd{TaskID: "task-1", Reason: "permanently stuck"}

		err := cmd.Execute(context.Background(), repo)

		require.NoError(t, err)
		state, _ := repo.Load(context.Background())
		assert.Equal(t, domain.TaskFailed, state.Tasks[0].Status)
		assert.Contains(t, state.Tasks[0].FailureLog, "[Unblocker]")
	})

	t.Run("race condition: when task already succeeded, it skips failure", func(t *testing.T) {
		t.Parallel()
		task := domain.Task{ID: "task-1", Status: domain.TaskSuccess, UpdatedAt: time.Now()}
		repo := &inMemoryRepo{state: newTestState([]domain.Task{task})}
		cmd := &FailTaskCmd{TaskID: "task-1", Reason: "stale failure"}

		err := cmd.Execute(context.Background(), repo)

		require.NoError(t, err)
		state, _ := repo.Load(context.Background())
		assert.Equal(t, domain.TaskSuccess, state.Tasks[0].Status)
	})
}

func TestClearInconsistentAgentCmd(t *testing.T) {
	t.Parallel()

	t.Run("when agent is WORKING, it marks agent as COMPLETED", func(t *testing.T) {
		t.Parallel()
		st := newTestState(nil)
		st.ActiveAgents = []domain.Agent{
			{ID: "agent-99", Status: domain.AgentWorking, TaskID: "t-done"},
		}
		repo := &inMemoryRepo{state: st}
		cmd := &ClearInconsistentAgentCmd{AgentID: "agent-99", Reason: "task no longer in progress"}

		err := cmd.Execute(context.Background(), repo)

		require.NoError(t, err)
		state, _ := repo.Load(context.Background())
		assert.Equal(t, domain.AgentCompleted, state.ActiveAgents[0].Status)
	})
}

func TestBypassToLastResortCmd(t *testing.T) {
	t.Parallel()

	t.Run("when task exists and in-progress, it sets stall count to 2, flags LastResortUsed, and resets to PENDING", func(t *testing.T) {
		t.Parallel()
		task := domain.Task{ID: "task-1", Status: domain.TaskInProgress, Progress: 50, UpdatedAt: time.Now()}
		st := newTestState([]domain.Task{task})
		st.ActiveAgents = []domain.Agent{
			{ID: "agent-1", TaskID: "task-1", Status: domain.AgentWorking},
		}
		repo := &inMemoryRepo{state: st}
		cmd := &BypassToLastResortCmd{TaskID: "task-1", Reason: "persistent compilation stall"}

		err := cmd.Execute(context.Background(), repo)

		require.NoError(t, err)
		state, _ := repo.Load(context.Background())
		assert.Equal(t, domain.TaskPending, state.Tasks[0].Status)
		assert.Equal(t, 2, state.Tasks[0].StallCount)
		assert.True(t, state.Tasks[0].LastResortUsed)
		assert.Contains(t, state.Tasks[0].RecoveryDirective, "SOVEREIGN REPAIR DIRECTIVE")
		assert.Equal(t, domain.AgentCompleted, state.ActiveAgents[0].Status)
	})

	t.Run("when task already in terminal state, it skips bypass", func(t *testing.T) {
		t.Parallel()
		task := domain.Task{ID: "task-1", Status: domain.TaskSuccess, UpdatedAt: time.Now()}
		repo := &inMemoryRepo{state: newTestState([]domain.Task{task})}
		cmd := &BypassToLastResortCmd{TaskID: "task-1", Reason: "late bypass"}

		err := cmd.Execute(context.Background(), repo)

		require.NoError(t, err)
		state, _ := repo.Load(context.Background())
		assert.Equal(t, domain.TaskSuccess, state.Tasks[0].Status)
	})
}

func TestScopeTriageCmd(t *testing.T) {
	t.Parallel()

	t.Run("defers downstream stories beyond keep limit and their associated tasks", func(t *testing.T) {
		t.Parallel()
		stories := []domain.Story{
			{ID: "US-001", Status: domain.StorySuccess},
			{ID: "US-002", Status: domain.StoryRunning},
			{ID: "US-003", Status: domain.StoryPending},
			{ID: "US-004", Status: domain.StoryPending},
		}
		tasks := []domain.Task{
			{ID: "t-1", StoryID: "US-001", Status: domain.TaskSuccess},
			{ID: "t-2", StoryID: "US-002", Status: domain.TaskInProgress},
			{ID: "t-3", StoryID: "US-003", Status: domain.TaskPending},
			{ID: "t-4", StoryID: "US-004", Status: domain.TaskPending},
		}
		st := newTestState(tasks)
		st.Stories = stories
		repo := &inMemoryRepo{state: st}

		cmd := &ScopeTriageCmd{Reason: "approaching 50% budget cliff", KeepStories: 2}
		err := cmd.Execute(context.Background(), repo)

		require.NoError(t, err)
		state, _ := repo.Load(context.Background())

		// US-001 and US-002 should not be deferred
		assert.Equal(t, domain.StorySuccess, state.Stories[0].Status)
		assert.Equal(t, domain.StoryRunning, state.Stories[1].Status)
		// US-003 and US-004 should be DEFERRED
		assert.Equal(t, domain.StoryDeferred, state.Stories[2].Status)
		assert.Equal(t, domain.StoryDeferred, state.Stories[3].Status)

		// Tasks for US-003 and US-004 should be DEFERRED
		assert.Equal(t, domain.TaskSuccess, state.Tasks[0].Status)
		assert.Equal(t, domain.TaskInProgress, state.Tasks[1].Status)
		assert.Equal(t, domain.TaskDeferred, state.Tasks[2].Status)
		assert.Equal(t, domain.TaskDeferred, state.Tasks[3].Status)

		// Check action logged
		require.NotEmpty(t, state.LastActions)
		assert.Equal(t, "scope_triage", state.LastActions[len(state.LastActions)-1].Tool)
	})
}
