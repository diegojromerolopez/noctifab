package services

import (
	"context"
	"sync"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// FileLockRegistry prevents concurrent tasks from writing to overlapping files.
type FileLockRegistry struct {
	mu    sync.Mutex
	locks map[string]string // maps file path -> task ID
}

func NewFileLockRegistry() *FileLockRegistry {
	return &FileLockRegistry{
		locks: make(map[string]string),
	}
}

// TryAcquireLocks attempts to lock all target files for a given task.
// If any file is locked by another task, it rolls back and returns false.
func (r *FileLockRegistry) TryAcquireLocks(taskID string, files []string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if any file is already locked
	for _, file := range files {
		if holder, locked := r.locks[file]; locked && holder != taskID {
			return false
		}
	}

	// Acquire locks
	for _, file := range files {
		r.locks[file] = taskID
	}
	return true
}

// ReleaseLocks releases all locks held by a task.
func (r *FileLockRegistry) ReleaseLocks(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for file, holder := range r.locks {
		if holder == taskID {
			delete(r.locks, file)
		}
	}
}

// Scheduler handles task topological checks and parallel task selection.
type Scheduler struct {
	lockRegistry *FileLockRegistry
}

func NewScheduler(registry *FileLockRegistry) *Scheduler {
	return &Scheduler{
		lockRegistry: registry,
	}
}

// GetReadyTasks returns tasks that are PENDING or FAILED (eligible for retry)
// and whose upstream dependencies are all SUCCESS, respecting file locks and agent concurrency.
func (s *Scheduler) GetReadyTasks(state *domain.State, concurrencyLimit int) []domain.Task {
	_, span := telemetry.Tracer().Start(context.Background(), "GetReadyTasks",
		trace.WithAttributes(
			attribute.Int("concurrency_limit", concurrencyLimit),
			attribute.Int("task_count", len(state.Tasks)),
		))
	defer span.End()
	// Build map of task ID / Title to status
	statusMap := make(map[string]domain.TaskStatus)
	idMap := make(map[string]string)
	titleMap := make(map[string]string)
	for _, t := range state.Tasks {
		statusMap[t.ID] = t.Status
		statusMap[t.Title] = t.Status
		idMap[t.Title] = t.ID
		idMap[t.ID] = t.ID
		titleMap[t.Title] = t.ID
	}

	// Compute blocked tasks transitively from unresolved clarifications
	blocked := make(map[string]bool)
	for _, c := range state.Clarifications {
		if !c.Resolved {
			if c.TaskID == "" {
				// Global clarification blocks everything
				return nil
			}
			blocked[c.TaskID] = true
			if id, exists := titleMap[c.TaskID]; exists {
				blocked[id] = true
			}
		}
	}

	// Propagate blocks downstream (iteratively)
	changed := true
	for changed {
		changed = false
		for _, t := range state.Tasks {
			if blocked[t.ID] {
				continue
			}
			for _, dep := range t.DependsOn {
				depID := dep
				if id, exists := titleMap[dep]; exists {
					depID = id
				}
				if blocked[depID] {
					blocked[t.ID] = true
					changed = true
					break
				}
			}
		}
	}

	// Count active agents
	activeCount := 0
	for _, agent := range state.ActiveAgents {
		if agent.Status == domain.AgentWorking {
			activeCount++
		}
	}

	availableSlots := concurrencyLimit - activeCount
	if availableSlots <= 0 {
		return nil
	}

	var ready []domain.Task
	for i, t := range state.Tasks {
		if t.Status != domain.TaskPending && t.Status != domain.TaskFailed {
			continue
		}

		// Check if max retries reached
		if t.Status == domain.TaskFailed && t.Retries >= t.MaxRetries {
			continue
		}

		// Skip if transitively blocked by unresolved clarification
		if blocked[t.ID] {
			continue
		}

		// Check dependencies
		depsMet := true
		for _, dep := range t.DependsOn {
			status, ok := statusMap[dep]
			if !ok || status != domain.TaskSuccess {
				depsMet = false
				break
			}
		}

		if depsMet {
			// Check file locks
			if s.lockRegistry.TryAcquireLocks(t.ID, t.TargetFiles) {
				ready = append(ready, state.Tasks[i])
				if len(ready) >= availableSlots {
					break
				}
			}
		}
	}

	return ready
}

func (s *Scheduler) ReleaseLocks(taskID string) {
	s.lockRegistry.ReleaseLocks(taskID)
}
