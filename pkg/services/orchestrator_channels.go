package services

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func (o *Orchestrator) markTaskFailed(ctx context.Context, taskID, reason string) error {
	err := o.updateStateWithRetry(ctx, func(st *domain.State) error {
		for i := range st.Tasks {
			if st.Tasks[i].ID == taskID {
				st.Tasks[i].Status = domain.TaskFailed
				st.Tasks[i].UpdatedAt = time.Now()
				break
			}
		}
		domain.AppendAction(st, domain.Action{
			Timestamp: time.Now(),
			Tool:      "execute",
			Success:   false,
			Result:    reason,
		})
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: failed to persist FAILED status for task %s: %v\n", taskID, err)
	}
	return err
}

// TaskCompletedChan returns the read-only channel signaled whenever a task completes.
func (o *Orchestrator) TaskCompletedChan() <-chan struct{} {
	if o == nil {
		return nil
	}
	return o.taskCompletedChan
}

// StoryCompletedChan returns the read-only channel signaled whenever a user story finishes.
func (o *Orchestrator) StoryCompletedChan() <-chan struct{} {
	if o == nil {
		return nil
	}
	return o.storyCompletedChan
}

// NotifyStoryCompleted signals that a user story has reached terminal state (success or failure),
// instantly waking up any waiting orchestrator or runner event loops.
func (o *Orchestrator) NotifyStoryCompleted() {
	if o == nil || o.storyCompletedChan == nil {
		return
	}
	select {
	case o.storyCompletedChan <- struct{}{}:
	default:
	}
}
