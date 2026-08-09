package services

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type fixedQAClock struct{ now time.Time }

func (c fixedQAClock) Now() time.Time { return c.now }

func TestQARecoveryService(t *testing.T) {
	now := time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC)

	t.Run("when the deadline passed, it atomically interrupts the phase agent and task", func(t *testing.T) {
		repo := &mockRepo{state: recoveryState(now.Add(-time.Second), true)}
		recovered, err := NewQARecoveryService(repo, fixedQAClock{now: now}).Recover(context.Background())
		if err != nil {
			t.Fatalf("recover: %v", err)
		}
		if recovered != 1 {
			t.Fatalf("expected one recovered phase, got %d", recovered)
		}

		state, _ := repo.Load(context.Background())
		phase := state.ReviewPhases[0]
		if phase.Status != domain.ReviewInterrupted || phase.TerminalReason != restartRecoveryReason || !phase.CompletedAt.Equal(now) {
			t.Errorf("unexpected recovered phase: %+v", phase)
		}
		agent := state.ActiveAgents[0]
		if agent.Status != domain.AgentCompleted || agent.LastError != "restart recovery" || !agent.CompletedAt.Equal(now) {
			t.Errorf("unexpected recovered agent: %+v", agent)
		}
		if state.Tasks[0].Status != domain.TaskInterrupted || !state.Tasks[0].UpdatedAt.Equal(now) {
			t.Errorf("unexpected recovered task: %+v", state.Tasks[0])
		}
	})

	t.Run("when the matching agent is absent, it interrupts the orphaned phase and task", func(t *testing.T) {
		repo := &mockRepo{state: recoveryState(now.Add(time.Minute), false)}
		recovered, err := NewQARecoveryService(repo, fixedQAClock{now: now}).Recover(context.Background())
		if err != nil {
			t.Fatalf("recover: %v", err)
		}
		state, _ := repo.Load(context.Background())
		if recovered != 1 || state.ReviewPhases[0].Status != domain.ReviewInterrupted {
			t.Fatalf("expected orphaned phase recovery, got count=%d phase=%+v", recovered, state.ReviewPhases[0])
		}
		if state.Tasks[0].Status != domain.TaskInterrupted {
			t.Errorf("expected interrupted task, got %s", state.Tasks[0].Status)
		}
	})

	t.Run("when the phase is healthy, it preserves state", func(t *testing.T) {
		repo := &mockRepo{state: recoveryState(now.Add(time.Minute), true)}
		recovered, err := NewQARecoveryService(repo, fixedQAClock{now: now}).Recover(context.Background())
		if err != nil {
			t.Fatalf("recover: %v", err)
		}
		state, _ := repo.Load(context.Background())
		if recovered != 0 || state.ReviewPhases[0].Status != domain.ReviewWorking || state.Tasks[0].Status != domain.TaskInProgress {
			t.Fatalf("healthy state changed: count=%d state=%+v", recovered, state)
		}
		if state.Version != 0 {
			t.Errorf("healthy recovery should not save state, got version %d", state.Version)
		}
	})

	t.Run("when save has an OCC conflict, it reloads and retries recovery", func(t *testing.T) {
		repo := &mockConflictRepo{
			mockRepo:  mockRepo{state: recoveryState(now.Add(-time.Second), true)},
			failSaves: 1,
		}
		recovered, err := NewQARecoveryService(repo, fixedQAClock{now: now}).Recover(context.Background())
		if err != nil {
			t.Fatalf("recover: %v", err)
		}
		state, _ := repo.Load(context.Background())
		if repo.saveCount != 2 || recovered != 1 || state.ReviewPhases[0].Status != domain.ReviewInterrupted {
			t.Fatalf("expected successful OCC retry, saves=%d recovered=%d phase=%+v", repo.saveCount, recovered, state.ReviewPhases[0])
		}
	})
}

func recoveryState(deadline time.Time, withAgent bool) *domain.State {
	state := &domain.State{
		ID:    "state-1",
		Tasks: []domain.Task{{ID: "task-1", Status: domain.TaskInProgress}},
		ReviewPhases: []domain.ReviewPhase{{
			ID: "phase-1", StoryID: "US-001", TaskID: "task-1", Role: "qa",
			Status: domain.ReviewWorking, DeadlineAt: deadline,
		}},
	}
	if withAgent {
		state.ActiveAgents = []domain.Agent{{
			ID: "agent-qa-task-1", Role: domain.AgentRole("QA"), Status: domain.AgentWorking, TaskID: "task-1",
		}}
	}
	return state
}
