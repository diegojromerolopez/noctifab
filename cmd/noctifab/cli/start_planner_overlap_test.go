package cli

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type mockPlannerRepo struct {
	mu    sync.Mutex
	state *domain.State
}

func (m *mockPlannerRepo) Save(ctx context.Context, state *domain.State) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = state
	return nil
}

func (m *mockPlannerRepo) Load(ctx context.Context) (*domain.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, nil
}

func (m *mockPlannerRepo) LoadByID(ctx context.Context, id string) (*domain.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, nil
}

func (m *mockPlannerRepo) LoadAll(ctx context.Context) ([]*domain.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state == nil {
		return nil, nil
	}
	return []*domain.State{m.state}, nil
}

func (m *mockPlannerRepo) LoadAllSummaries(ctx context.Context) ([]domain.StateSummary, error) {
	return nil, nil
}

func (m *mockPlannerRepo) PruneFinishedStates(ctx context.Context, keepLast int) (int, error) {
	return 0, nil
}

func (m *mockPlannerRepo) SaveAction(ctx context.Context, action domain.Action) error {
	return nil
}

func (m *mockPlannerRepo) LoadActions(ctx context.Context, limit int) ([]domain.Action, error) {
	return nil, nil
}

func (m *mockPlannerRepo) ClearActions(ctx context.Context) error {
	return nil
}

func TestSpeculativePlanner(t *testing.T) {
	t.Run("when upcoming stories exist it speculatively triggers background planning for queued stories", func(t *testing.T) {
		repo := &mockPlannerRepo{
			state: &domain.State{
				Tasks: []domain.Task{
					{ID: "task-1", StoryID: "US-001"},
				},
			},
		}

		var plannedMu sync.Mutex
		plannedFiles := make(map[string]bool)

		planFn := func(ctx context.Context, storyFile string) error {
			plannedMu.Lock()
			plannedFiles[storyFile] = true
			plannedMu.Unlock()
			return nil
		}

		planner := NewSpeculativePlanner(repo, planFn)
		storyFiles := []string{"roadmap/US-001.md", "roadmap/US-002.md", "roadmap/US-003.md"}

		planner.PrePlanQueuedStories(context.Background(), storyFiles, "roadmap/US-001.md")

		// Allow background goroutine to execute
		time.Sleep(50 * time.Millisecond)

		plannedMu.Lock()
		defer plannedMu.Unlock()

		if plannedFiles["roadmap/US-001.md"] {
			t.Errorf("it should not pre-plan the active running story US-001")
		}
		if !plannedFiles["roadmap/US-002.md"] {
			t.Errorf("it should pre-plan downstream story US-002")
		}
		if !plannedFiles["roadmap/US-003.md"] {
			t.Errorf("it should pre-plan downstream story US-003")
		}
		if !planner.IsPlanned("roadmap/US-002.md") {
			t.Errorf("it should mark US-002 as planned")
		}
	})

	t.Run("when a story is already planned in repository it skips redundant background planning", func(t *testing.T) {
		repo := &mockPlannerRepo{
			state: &domain.State{
				Tasks: []domain.Task{
					{ID: "task-1", StoryID: "US-001"},
					{ID: "task-2", StoryID: "US-002"},
				},
			},
		}

		var planCount int
		planFn := func(ctx context.Context, storyFile string) error {
			planCount++
			return nil
		}

		planner := NewSpeculativePlanner(repo, planFn)
		storyFiles := []string{"roadmap/US-001.md", "roadmap/US-002.md"}

		planner.PrePlanQueuedStories(context.Background(), storyFiles, "roadmap/US-001.md")

		time.Sleep(50 * time.Millisecond)

		if planCount != 0 {
			t.Errorf("expected 0 planning invocations for already planned US-002, got %d", planCount)
		}
		if !planner.IsPlanned("roadmap/US-002.md") {
			t.Errorf("it should recognize US-002 as already planned")
		}
	})

	t.Run("when pre-planning encounters an error it handles gracefully without panic", func(t *testing.T) {
		repo := &mockPlannerRepo{
			state: &domain.State{},
		}

		planFn := func(ctx context.Context, storyFile string) error {
			return errors.New("transient LLM planning failure")
		}

		planner := NewSpeculativePlanner(repo, planFn)
		storyFiles := []string{"roadmap/US-001.md", "roadmap/US-002.md"}

		planner.PrePlanQueuedStories(context.Background(), storyFiles, "roadmap/US-001.md")

		time.Sleep(50 * time.Millisecond)
		// No panic occurred, test passes
	})

	t.Run("when planner is nil or planFunc is nil it safely no-ops", func(t *testing.T) {
		var nilPlanner *SpeculativePlanner
		nilPlanner.PrePlanQueuedStories(context.Background(), []string{"a.md"}, "a.md")
		nilPlanner.MarkPlanned("a.md")
		if nilPlanner.IsPlanned("a.md") {
			t.Errorf("nil planner should return false")
		}

		emptyPlanner := NewSpeculativePlanner(nil, nil)
		emptyPlanner.PrePlanQueuedStories(context.Background(), []string{"a.md"}, "a.md")
	})
}
