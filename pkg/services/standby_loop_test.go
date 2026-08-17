package services

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/notifier"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStandbyStateRepo struct {
	state *domain.State
}

func (m *mockStandbyStateRepo) Load(ctx context.Context) (*domain.State, error) {
	if m.state == nil {
		m.state = &domain.State{
			ID:          "mock-state",
			StoryStatus: domain.StoryRunning,
		}
	}
	return m.state, nil
}

func (m *mockStandbyStateRepo) Save(ctx context.Context, state *domain.State) error {
	m.state = state
	return nil
}

func (m *mockStandbyStateRepo) LoadAll(ctx context.Context) ([]*domain.State, error) {
	st, err := m.Load(ctx)
	if err != nil {
		return nil, err
	}
	return []*domain.State{st}, nil
}

func (m *mockStandbyStateRepo) LoadByID(ctx context.Context, id string) (*domain.State, error) {
	return m.Load(ctx)
}

func (m *mockStandbyStateRepo) LoadAllSummaries(ctx context.Context) ([]domain.StateSummary, error) {
	st, err := m.Load(ctx)
	if err != nil {
		return nil, err
	}
	return []domain.StateSummary{domain.SummarizeState(st)}, nil
}

func (m *mockStandbyStateRepo) PruneFinishedStates(ctx context.Context, keepLast int) (int, error) {
	return 0, nil
}

func TestStandbyEngine_LifecycleAndOrderExecution(t *testing.T) {
	repo := &mockStandbyStateRepo{}
	mockNotif := notifier.NewMockNotifier()

	var executionCount int32
	executor := func(ctx context.Context, storyFile string) error {
		atomic.AddInt32(&executionCount, 1)
		assert.Equal(t, "roadmap/user-stories/US-001.md", storyFile)
		return nil
	}

	engine := NewStandbyEngine(StandbyEngineConfig{
		Repo:     repo,
		Notifier: mockNotif,
		Executor: executor,
		QueueCap: 10,
	})

	assert.Equal(t, DaemonStatusIdle, engine.Status())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- engine.Run(ctx)
	}()

	// Wait for engine to start and set idle state
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, DaemonStatusIdle, engine.Status())
	assert.Equal(t, domain.StoryIdle, repo.state.StoryStatus)

	// Enqueue a story order
	enqueued := engine.Enqueue(StoryWorkItem{Path: "roadmap/user-stories/US-001.md"})
	require.True(t, enqueued)

	// Wait for execution to process
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(1), atomic.LoadInt32(&executionCount))
	assert.Equal(t, DaemonStatusIdle, engine.Status())

	// Verify desktop notification was dispatched
	require.Len(t, mockNotif.Notifications, 1)
	assert.Equal(t, notifier.NotifyStoryCompleted, mockNotif.Notifications[0].Kind)
	assert.Contains(t, mockNotif.Notifications[0].Message, "US-001.md")

	// Cancel context to stop engine
	cancel()
	err := <-errCh
	assert.ErrorIs(t, err, context.Canceled)
}

func TestStandbyEngine_FailureNotification(t *testing.T) {
	repo := &mockStandbyStateRepo{}
	mockNotif := notifier.NewMockNotifier()

	executor := func(ctx context.Context, storyFile string) error {
		return errors.New("compilation failed on 3rd attempt")
	}

	engine := NewStandbyEngine(StandbyEngineConfig{
		Repo:     repo,
		Notifier: mockNotif,
		Executor: executor,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = engine.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	engine.Enqueue(StoryWorkItem{Path: "roadmap/user-stories/US-002.md"})
	time.Sleep(100 * time.Millisecond)

	require.Len(t, mockNotif.Notifications, 1)
	assert.Equal(t, notifier.NotifyBuildFailed, mockNotif.Notifications[0].Kind)
	assert.Contains(t, mockNotif.Notifications[0].Message, "compilation failed")

	assert.Equal(t, DaemonStatusIdle, engine.Status())
}
