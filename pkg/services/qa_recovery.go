package services

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

const restartRecoveryReason = "restart_recovery"

// QARecoveryService atomically terminates QA work that cannot safely resume after restart.
type QARecoveryService struct {
	repository domain.StateRepository
	clock      QAClock
}

// NewQARecoveryService creates a restart recovery service with injected persistence and time.
func NewQARecoveryService(repository domain.StateRepository, clock QAClock) *QARecoveryService {
	return &QARecoveryService{repository: repository, clock: clock}
}

// Recover interrupts expired or orphaned QA phases. The returned count is from
// the state version that was successfully persisted.
func (s *QARecoveryService) Recover(ctx context.Context) (int, error) {
	var lastErr error
	for attempt := 0; attempt < occRetryMaxAttempts; attempt++ {
		state, err := s.repository.Load(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, nil
			}
			return 0, err
		}
		recovered := recoverQAState(state, s.clock.Now())
		if recovered == 0 {
			return 0, nil
		}
		if err = s.repository.Save(ctx, state); err == nil {
			return recovered, nil
		}
		if !errors.Is(err, domain.ErrVersionConflict) {
			return 0, err
		}
		lastErr = err
	}
	return 0, lastErr
}

func recoverQAState(state *domain.State, now time.Time) int {
	recovered := 0
	for phaseIndex := range state.ReviewPhases {
		phase := &state.ReviewPhases[phaseIndex]
		if phase.Status != domain.ReviewWorking || !strings.EqualFold(phase.Role, "qa") {
			continue
		}

		agentIndexes := matchingWorkingQAAgents(state.ActiveAgents, phase.TaskID)
		if !phase.DeadlineAt.Before(now) && len(agentIndexes) > 0 {
			continue
		}

		phase.Status = domain.ReviewInterrupted
		phase.TerminalReason = restartRecoveryReason
		phase.CompletedAt = now
		for _, agentIndex := range agentIndexes {
			state.ActiveAgents[agentIndex].Status = domain.AgentCompleted
			state.ActiveAgents[agentIndex].CompletedAt = now
			state.ActiveAgents[agentIndex].LastError = "restart recovery"
		}
		for taskIndex := range state.Tasks {
			if state.Tasks[taskIndex].ID == phase.TaskID {
				state.Tasks[taskIndex].Status = domain.TaskInterrupted
				state.Tasks[taskIndex].UpdatedAt = now
				break
			}
		}
		recovered++
	}
	return recovered
}

func matchingWorkingQAAgents(agents []domain.Agent, taskID string) []int {
	var matches []int
	for index := range agents {
		if agents[index].TaskID == taskID && agents[index].Status == domain.AgentWorking &&
			strings.EqualFold(string(agents[index].Role), "qa") {
			matches = append(matches, index)
		}
	}
	return matches
}
