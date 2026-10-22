package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// UnblockIndependentTasks inspects blocked tasks waiting on failed upstream tasks.
// If a blocked task has no file-level mutation collisions with the failed task,
// it prunes the failed dependency edge, allowing independent downstream stories to progress.
func (o *Orchestrator) UnblockIndependentTasks(ctx context.Context, state *domain.State) int {
	if state == nil || len(state.Tasks) == 0 {
		return 0
	}

	failedTaskIDs := make(map[string]domain.Task)
	for _, t := range state.Tasks {
		if t.Status == domain.TaskFailed {
			failedTaskIDs[t.ID] = t
		}
	}

	if len(failedTaskIDs) == 0 {
		return 0
	}

	unblockedCount := 0
	_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
		for i := range st.Tasks {
			task := &st.Tasks[i]
			if task.Status != domain.TaskPending {
				continue
			}

			var newDeps []string
			prunedFailedDep := false

			for _, dep := range task.DependsOn {
				depClean := strings.TrimSpace(dep)
				failedTask, isFailed := failedTaskIDs[depClean]
				if isFailed {
					isCrossStory := (task.StoryID != "" && failedTask.StoryID != "" && task.StoryID != failedTask.StoryID) ||
						(strings.HasPrefix(task.ID, "US-") && strings.HasPrefix(failedTask.ID, "US-") && len(task.ID) >= 6 && len(failedTask.ID) >= 6 && task.ID[:6] != failedTask.ID[:6])

					if isCrossStory && len(task.TargetFiles) > 0 && len(failedTask.TargetFiles) > 0 {
						hasCollision := false
						for _, tf1 := range task.TargetFiles {
							for _, tf2 := range failedTask.TargetFiles {
								if strings.EqualFold(strings.TrimSpace(tf1), strings.TrimSpace(tf2)) {
									hasCollision = true
									break
								}
							}
							if hasCollision {
								break
							}
						}
						if !hasCollision {
							// Independent cross-story task! Prune failed dependency edge
							prunedFailedDep = true
							continue
						}
					}
				}
				newDeps = append(newDeps, dep)
			}

			if prunedFailedDep {
				task.DependsOn = newDeps
				task.UpdatedAt = time.Now()
				unblockedCount++
				fmt.Printf("✨ [Deadlock Self-Healing] Unblocked independent task %s (%s) by pruning failed upstream dependency\n", task.ID, task.Title)
			}
		}
		return nil
	})

	return unblockedCount
}
