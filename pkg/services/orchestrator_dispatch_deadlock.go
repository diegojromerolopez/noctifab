package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// handleDeadlockOrRescue evaluates a stalled dispatch state where no tasks are ready.
// If isolated task-level sovereign rescue is possible, it attempts to unblock the pipeline.
// Otherwise, it records deadlock and aborts the story.
func (o *Orchestrator) handleDeadlockOrRescue(ctx context.Context, state *domain.State) (bool, error) {
	activeWorkers := 0
	for _, agent := range state.ActiveAgents {
		if agent.Status == domain.AgentWorking {
			activeWorkers++
		}
	}

	hasInProgress := false
	for _, t := range state.Tasks {
		if t.Status == domain.TaskInProgress {
			hasInProgress = true
			break
		}
	}

	if o.allTasksFinished(state) || activeWorkers > 0 || hasInProgress || len(state.Tasks) == 0 {
		return false, nil
	}

	// 1. Isolated Task-Level Sovereign Rescue
	// Find the earliest failed task that has not exhausted sovereign rescue.
	var rescueCandidate *domain.Task
	for i := range state.Tasks {
		t := &state.Tasks[i]
		if t.Status == domain.TaskFailed && !t.LastResortUsed {
			rescueCandidate = t
			break
		}
	}

	fbCfg := o.cfg.GetFallback()
	if rescueCandidate != nil && fbCfg.Enabled {
		fmt.Printf("⚡ [Isolated Sovereign Rescue] Deadlock averted: activating task-scoped sovereign rescue for failed task %s (%s)...\n", rescueCandidate.ID, rescueCandidate.Title)

		triggerReason := fmt.Sprintf("pipeline_deadlock_on_task_%s", rescueCandidate.ID)
		failureLog := rescueCandidate.FailureLog
		if strings.TrimSpace(failureLog) == "" {
			failureLog = "Task failed previous attempts and is blocking downstream story dependencies."
		}

		fbPassed, fbLog := o.RunFallbackAgent(ctx, rescueCandidate, state, o.git, failureLog, triggerReason)
		if fbPassed {
			fmt.Printf("✨ [Isolated Sovereign Rescue] Task %s successfully rescued! Resuming DAG execution...\n", rescueCandidate.ID)
			_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
				for i := range st.Tasks {
					if st.Tasks[i].ID == rescueCandidate.ID {
						st.Tasks[i].Status = domain.TaskSuccess
						st.Tasks[i].Progress = 100
						st.Tasks[i].FailureLog = ""
						st.Tasks[i].LastResortUsed = true
						st.Tasks[i].UpdatedAt = time.Now()
						break
					}
				}
				return nil
			})
			return true, nil
		}

		// Rescue failed: mark LastResortUsed to prevent recurring loops
		_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
			for i := range st.Tasks {
				if st.Tasks[i].ID == rescueCandidate.ID {
					st.Tasks[i].LastResortUsed = true
					st.Tasks[i].FailureLog = fbLog
					break
				}
			}
			return nil
		})
	}

	// 2. Full Deadlock Abort
	var blockedInfo []string
	for _, t := range state.Tasks {
		if t.Status != domain.TaskSuccess {
			blockedInfo = append(blockedInfo, fmt.Sprintf("task %s (%s, status=%s, deps=%v)", t.ID, t.Title, t.Status, t.DependsOn))
		}
	}
	diagMsg := fmt.Sprintf("deadlock detected: 0 ready tasks and 0 active workers; blocked tasks:\n - %s", strings.Join(blockedInfo, "\n - "))
	fmt.Fprintf(os.Stderr, "Orchestrator: %s\n", diagMsg)

	if err := o.updateStateWithRetry(ctx, func(st *domain.State) error {
		for i := range st.Tasks {
			if st.Tasks[i].Status != domain.TaskSuccess && st.Tasks[i].Status != domain.TaskFailed {
				st.Tasks[i].Status = domain.TaskFailed
				st.Tasks[i].FailureLog = diagMsg
				st.Tasks[i].UpdatedAt = time.Now()
			}
		}
		st.BuildStatus = domain.BuildFailing
		st.StoryStatus = domain.StoryFailed
		st.StoryError = diagMsg
		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: failed to persist deadlock abort status: %v\n", err)
	}

	return false, fmt.Errorf("%s", diagMsg)
}
