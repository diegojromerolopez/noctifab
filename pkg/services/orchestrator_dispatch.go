package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RunOnce runs a single cycle of the event loop.
// Returns a boolean indicating if any ready tasks were executed in this cycle, and any error.
//
// Task dispatch is continuous: ready tasks are launched as goroutines up to the
// configured concurrency limit, and every time a task completes the state is
// re-loaded and newly-ready tasks are dispatched into the freed slots. RunOnce
// returns once no tasks are running and none are ready.
func (o *Orchestrator) RunOnce(ctx context.Context) (bool, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "RunOnce",
		trace.WithAttributes(
			attribute.Int("concurrency", o.cfg.Concurrency),
			attribute.Int("occ_max_retries", o.cfg.OCCMaxRetries),
			attribute.String("poll_interval", o.cfg.PollInterval.String()),
		))
	defer span.End()

	state, err := o.repo.Load(ctx)
	if err != nil {
		return false, err
	}

	// 1. Observe Phase: File indexing and sync
	if err := o.syncWorkspaceFiles(ctx, state); err != nil {
		return false, err
	}

	// 2. Scheduler check: find ready tasks
	concurrencyLimit := o.cfg.Concurrency
	if concurrencyLimit <= 0 {
		concurrencyLimit = o.cfg.GeneratorsNumber
	}
	if concurrencyLimit <= 0 {
		concurrencyLimit = 3
	}
	ready := o.scheduler.GetReadyTasks(state, concurrencyLimit)

	// 2a. Global max_actions enforcement.
	currentActions := atomic.LoadInt64(&o.totalActions)
	if o.cfg.MaxActions > 0 && int(currentActions) >= o.cfg.MaxActions && state.StoryStatus == domain.StoryIdle {
		fmt.Printf("Orchestrator: story exceeded max_actions ceiling %d (executed %d); failing remaining tasks and aborting story.\n", o.cfg.MaxActions, currentActions)
		if err := o.updateStateWithRetry(ctx, func(st *domain.State) error {
			for i := range st.Tasks {
				if st.Tasks[i].Status != domain.TaskSuccess && st.Tasks[i].Status != domain.TaskFailed {
					st.Tasks[i].Status = domain.TaskFailed
					st.Tasks[i].FailureLog = fmt.Sprintf("story exceeded max_actions ceiling %d (executed %d)", st.Tasks[i].MaxRetries, currentActions)
					st.Tasks[i].UpdatedAt = time.Now()
				}
			}
			st.BuildStatus = domain.BuildFailing
			st.StoryStatus = domain.StoryFailed
			return nil
		}); err != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator: failed to persist max_actions abort: %v\n", err)
		}
		return false, nil
	}

	// 2b. Story-level wall clock enforcement. When max_duration is configured
	// (> 0) and the story has been running longer than the limit, fail every
	// non-finished task and mark the story as FAILED so the daemon stops
	// spending LLM tokens on a stuck story. The start time is the first cycle
	// in which any task became ready.
	if o.cfg.MaxDuration > 0 && len(state.Tasks) > 0 {
		startedAt := o.getStoryStartedAt()
		if startedAt.IsZero() && len(ready) > 0 {
			startedAt = time.Now()
			o.setStoryStartedAt(startedAt)
		}
		// Enforce the wall-clock cap regardless of the transient StoryStatus:
		// previously this only fired while StoryIdle, so a story that was stuck
		// mid-execution (StoryRunning) with a hung LLM/sandbox call never hit
		// the deadline and burned tokens indefinitely. Now we
		// abort as soon as the deadline elapses and any task is still pending.
		if !startedAt.IsZero() && time.Since(startedAt) > o.cfg.MaxDuration && !o.allTasksFinished(state) {
			elapsed := time.Since(startedAt)
			fmt.Printf("Orchestrator: story exceeded max_duration %s (elapsed %s); failing remaining tasks and aborting story.\n", o.cfg.MaxDuration, elapsed.Truncate(time.Second))
			if err := o.updateStateWithRetry(ctx, func(st *domain.State) error {
				for i := range st.Tasks {
					if st.Tasks[i].Status != domain.TaskSuccess && st.Tasks[i].Status != domain.TaskFailed {
						st.Tasks[i].Status = domain.TaskFailed
						st.Tasks[i].FailureLog = fmt.Sprintf("story exceeded max_duration %s (elapsed %s)", o.cfg.MaxDuration, elapsed.Truncate(time.Second))
						st.Tasks[i].UpdatedAt = time.Now()
					}
				}
				st.BuildStatus = domain.BuildFailing
				st.StoryStatus = domain.StoryFailed
				return nil
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Orchestrator: failed to persist max_duration abort: %v\n", err)
			}
			return false, nil
		}
	}

	if len(ready) == 0 {
		// Finalize exactly once: guarded by StoryStatus (not BuildStatus, which
		// may have been set to FAILING mid-run by a failing task and could
		// recover on retry). StoryStatus transitions Idle -> Success/Failed
		// exactly once when all tasks are finished.
		if o.allTasksFinished(state) && state.StoryStatus == domain.StoryIdle {
			// Run Post-Merge Repair Phase before release finalization
			_ = o.RunPostMergeRepairPhase(ctx, state)

			buildOK := o.allTasksSucceeded(state)
			if buildOK {
				if finalErr := o.FinalizeUserStory(ctx, state); finalErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: finalization failed: %v\n", finalErr)
				}
			} else {
				fmt.Printf("Orchestrator: one or more tasks failed test validation; marking build as FAILING and skipping release finalization.\n")
			}
			if err := o.updateStateWithRetry(ctx, func(st *domain.State) error {
				if buildOK {
					st.BuildStatus = domain.BuildPassing
					st.StoryStatus = domain.StorySuccess
				} else {
					st.BuildStatus = domain.BuildFailing
					st.StoryStatus = domain.StoryFailed
				}
				return nil
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Orchestrator: failed to persist story finalization status: %v\n", err)
			}
			return false, nil
		}

		// Deadlock detection: when tasks exist, none are finished, no active workers exist, and no task is in progress
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

		if !o.allTasksFinished(state) && activeWorkers == 0 && !hasInProgress && len(state.Tasks) > 0 {
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

		return false, nil
	}

	fmt.Printf("Orchestrator: Found %d ready task(s) to execute in this cycle\n", len(ready))

	// 3. Continuous dispatch: keep slots busy until nothing is running or ready.
	o.dispatchContinuously(ctx, state.ID, ready)
	return true, nil
}

// dispatchContinuously runs a continuous dispatch loop: the initial batch of
// ready tasks is launched as goroutines (capped to the concurrency limit), and
// on every task completion the state is re-loaded and newly-ready tasks are
// dispatched into the freed slots. It returns once no tasks are in flight and
// no further tasks are ready.
func (o *Orchestrator) dispatchContinuously(ctx context.Context, stateID string, initial []domain.Task) {
	concurrency := o.cfg.Concurrency
	if concurrency <= 0 {
		concurrency = o.cfg.GeneratorsNumber
	}
	if concurrency <= 0 {
		concurrency = 3
	}

	inflight := make(map[string]bool)
	completions := make(chan string)

	dispatch := func(tasks []domain.Task) {
		for _, t := range tasks {
			if inflight[t.ID] || len(inflight) >= concurrency {
				continue
			}
			inflight[t.ID] = true
			go func(taskID string) {
				o.executeTaskFn(ctx, stateID, taskID)
				completions <- taskID
			}(t.ID)
		}
	}

	dispatch(initial)

	for len(inflight) > 0 {
		taskID := <-completions
		delete(inflight, taskID)

		// Do not dispatch new work once the context is cancelled; keep
		// draining completions so no goroutine blocks forever.
		if ctx.Err() != nil {
			continue
		}

		freeSlots := concurrency - len(inflight)
		if freeSlots <= 0 {
			continue
		}

		state, err := o.repo.Load(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator: dispatch loop failed to re-load state: %v\n", err)
			continue
		}
		dispatch(o.readyTasksForFreeSlots(state, inflight, freeSlots))
	}
}

// readyTasksForFreeSlots asks the scheduler for the next ready tasks given the
// locally-known in-flight set and the number of free slots. In-flight tasks are
// marked IN_PROGRESS on a state snapshot so the scheduler never re-returns a
// task that is still running locally, even if its persisted status update has
// not landed yet (stateless-agent rule: the persisted state is eventually
// consistent with the local dispatch loop).
func (o *Orchestrator) readyTasksForFreeSlots(state *domain.State, inflight map[string]bool, freeSlots int) []domain.Task {
	snapshot := state.Clone()
	for i := range snapshot.Tasks {
		if inflight[snapshot.Tasks[i].ID] {
			snapshot.Tasks[i].Status = domain.TaskInProgress
		}
	}

	// GetReadyTasks computes available slots as limit minus the number of
	// WORKING agents in the state; compensate so exactly freeSlots tasks can
	// be returned.
	working := 0
	for _, a := range snapshot.ActiveAgents {
		if a.Status == domain.AgentWorking {
			working++
		}
	}

	ready := o.scheduler.GetReadyTasks(snapshot, freeSlots+working)
	out := make([]domain.Task, 0, len(ready))
	for _, t := range ready {
		if !inflight[t.ID] {
			out = append(out, t)
		}
	}
	return out
}
