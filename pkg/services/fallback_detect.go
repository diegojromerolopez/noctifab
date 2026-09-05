package services

import (
	"path/filepath"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func getLogWindowLines(stallCount int) int {
	switch stallCount {
	case 0:
		return 50
	case 1:
		return 500
	default:
		return 5000
	}
}

func fetchTaskLogSnippet(taskID string, stallCount int) []string {
	lineCount := getLogWindowLines(stallCount)
	logPath := filepath.Join(".noctifab", "logs", "tasks", taskID+".log")
	lines, err := TailLogFile(logPath, lineCount)
	if err != nil || len(lines) == 0 {
		return nil
	}
	sanitized := make([]string, len(lines))
	for i, l := range lines {
		sanitized[i] = SanitizeLog(l)
	}
	return sanitized
}

// stallCooldownKey identifies a stall for LLM-assessment throttling purposes.
func stallCooldownKey(taskID string, status domain.TaskStatus) string {
	return taskID + "|" + string(status)
}

// filterStallsForLLMAssessment prunes cooldown entries whose task status has
// changed (or whose task disappeared) and returns only the stalls that have
// not been LLM-assessed within the cooldown window. Returned stalls are marked
// as assessed now.
func (u *FallbackAgent) filterStallsForLLMAssessment(state *domain.State, stalls []StalledTask) []StalledTask {
	cooldown := u.llmCooldown
	if cooldown <= 0 {
		cooldown = defaultLLMAssessmentCooldown
	}

	u.cooldownMu.Lock()
	defer u.cooldownMu.Unlock()
	if u.llmAssessedAt == nil {
		u.llmAssessedAt = make(map[string]time.Time)
	}

	// Prune entries for tasks whose status changed since the assessment (the
	// key embeds the status) or that no longer exist in the state.
	current := make(map[string]bool, len(state.Tasks))
	for _, t := range state.Tasks {
		current[stallCooldownKey(t.ID, t.Status)] = true
	}
	for key := range u.llmAssessedAt {
		if !current[key] {
			delete(u.llmAssessedAt, key)
		}
	}

	now := time.Now()
	var fresh []StalledTask
	for _, s := range stalls {
		key := stallCooldownKey(s.Task.ID, s.Task.Status)
		if assessedAt, ok := u.llmAssessedAt[key]; ok && now.Sub(assessedAt) < cooldown {
			continue
		}
		u.llmAssessedAt[key] = now
		fresh = append(fresh, s)
	}
	return fresh
}

// detectStalledTasks inspects the state and returns all detected stalls.
func (u *FallbackAgent) detectStalledTasks(state *domain.State) []StalledTask {
	var stalls []StalledTask

	// Build a lookup map from taskID -> agent for quick access.
	agentByTaskID := make(map[string]*domain.Agent)
	for i := range state.ActiveAgents {
		ag := &state.ActiveAgents[i]
		if ag.TaskID != "" {
			agentByTaskID[ag.TaskID] = ag
		}
	}

	// Build a lookup map from taskID -> task status.
	statusByID := make(map[string]domain.TaskStatus)
	for _, t := range state.Tasks {
		statusByID[t.ID] = t.Status
	}

	for i := range state.Tasks {
		task := state.Tasks[i]
		// Edge case guard: ignore uninitialized UpdatedAt timestamps to prevent false stalls.
		if task.UpdatedAt.IsZero() {
			continue
		}

		now := time.Now()

		switch task.Status {
		case domain.TaskInProgress:
			stalledFor := now.Sub(task.UpdatedAt)
			// 1. Frozen progress: IN_PROGRESS with no update beyond threshold.
			if stalledFor > u.stallThreshold {
				agent := agentByTaskID[task.ID]
				stalls = append(stalls, StalledTask{
					Task:          task,
					Agent:         agent,
					Reason:        StallReasonFrozenProgress,
					ReasonStr:     StallReasonFrozenProgress.String(),
					StalledFor:    stalledFor,
					StalledForStr: stalledFor.Round(time.Second).String(),
					RecentLogs:    fetchTaskLogSnippet(task.ID, task.StallCount),
				})
				continue
			}
			// 2. Orphaned: IN_PROGRESS but no WORKING agent is assigned.
			if stalledFor > u.stallThreshold/2 {
				agent := agentByTaskID[task.ID]
				if agent == nil || agent.Status != domain.AgentWorking {
					stalls = append(stalls, StalledTask{
						Task:          task,
						Agent:         agent,
						Reason:        StallReasonOrphanedTask,
						ReasonStr:     StallReasonOrphanedTask.String(),
						StalledFor:    stalledFor,
						StalledForStr: stalledFor.Round(time.Second).String(),
						RecentLogs:    fetchTaskLogSnippet(task.ID, task.StallCount),
					})
				}
			}

		case domain.TaskConflictBlocked:
			stalledFor := now.Sub(task.UpdatedAt)
			// 3. CONFLICT_BLOCKED for too long.
			if stalledFor > u.conflictThreshold {
				stalls = append(stalls, StalledTask{
					Task:          task,
					Agent:         agentByTaskID[task.ID],
					Reason:        StallReasonConflictBlocked,
					ReasonStr:     StallReasonConflictBlocked.String(),
					StalledFor:    stalledFor,
					StalledForStr: stalledFor.Round(time.Second).String(),
					RecentLogs:    fetchTaskLogSnippet(task.ID, task.StallCount),
				})
			}
		}
	}

	// 4. Agent inconsistency: WORKING agent whose task is not IN_PROGRESS.
	gracePeriod := u.inconsistencyGracePeriod
	if gracePeriod <= 0 {
		gracePeriod = 30 * time.Second
	}

	for i := range state.ActiveAgents {
		ag := &state.ActiveAgents[i]
		if ag.Status != domain.AgentWorking || ag.TaskID == "" {
			continue
		}
		taskStatus, exists := statusByID[ag.TaskID]
		if !exists || taskStatus != domain.TaskInProgress {
			var affectedTask domain.Task
			for _, t := range state.Tasks {
				if t.ID == ag.TaskID {
					affectedTask = t
					break
				}
			}

			// Grace period check: allow a grace period for in-flight transitions
			// (e.g. Generator -> Tester handoff, SQLite commit window, worktree locking).
			now := time.Now()
			if !ag.StartedAt.IsZero() && now.Sub(ag.StartedAt) < gracePeriod {
				continue
			}
			if !affectedTask.UpdatedAt.IsZero() && now.Sub(affectedTask.UpdatedAt) < gracePeriod {
				continue
			}

			stalledFor := time.Since(ag.StartedAt)
			if ag.StartedAt.IsZero() {
				stalledFor = time.Minute
			}
			stalls = append(stalls, StalledTask{
				Task:          affectedTask,
				Agent:         ag,
				Reason:        StallReasonAgentInconsistency,
				ReasonStr:     StallReasonAgentInconsistency.String(),
				StalledFor:    stalledFor,
				StalledForStr: stalledFor.Round(time.Second).String(),
				RecentLogs:    fetchTaskLogSnippet(affectedTask.ID, affectedTask.StallCount),
			})
		}
	}

	return stalls
}
