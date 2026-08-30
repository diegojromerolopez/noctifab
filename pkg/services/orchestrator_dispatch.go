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
					st.Tasks[i].FailureLog = fmt.Sprintf("story exceeded max_actions ceiling %d (executed %d)", o.cfg.MaxActions, currentActions)
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

	// 2c. Story-level token ceiling enforcement (Circuit Breaker).
	if o.cfg.MaxTokensPerStory > 0 && len(state.Tasks) > 0 {
		storyTokens := int64(0)
		for _, t := range state.Tasks {
			storyTokens += t.TokensUsed
		}
		if storyTokens >= o.cfg.MaxTokensPerStory && !o.allTasksFinished(state) {
			fmt.Printf("Orchestrator: story exceeded max_tokens_per_story ceiling %d (consumed %d); failing remaining tasks and aborting story.\n", o.cfg.MaxTokensPerStory, storyTokens)
			if err := o.updateStateWithRetry(ctx, func(st *domain.State) error {
				for i := range st.Tasks {
					if st.Tasks[i].Status != domain.TaskSuccess && st.Tasks[i].Status != domain.TaskFailed {
						st.Tasks[i].Status = domain.TaskFailed
						st.Tasks[i].FailureLog = fmt.Sprintf("story exceeded max_tokens_per_story ceiling %d (consumed %d tokens)", o.cfg.MaxTokensPerStory, storyTokens)
						st.Tasks[i].UpdatedAt = time.Now()
					}
				}
				st.BuildStatus = domain.BuildFailing
				st.StoryStatus = domain.StoryFailed
				return nil
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Orchestrator: failed to persist max_tokens_per_story abort: %v\n", err)
			}
			return false, nil
		}
	}

	// 2c2. Global-level token ceiling enforcement (runtime.max_tokens).
	if o.cfg.MaxTokens > 0 && len(state.Tasks) > 0 {
		globalTokens := int64(state.Metadata.TotalTokensUsed)
		if globalTokens >= o.cfg.MaxTokens && !o.allTasksFinished(state) {
			fmt.Printf("Orchestrator: execution exceeded global max_tokens ceiling %d (consumed %d); failing remaining tasks and aborting run.\n", o.cfg.MaxTokens, globalTokens)
			if err := o.updateStateWithRetry(ctx, func(st *domain.State) error {
				for i := range st.Tasks {
					if st.Tasks[i].Status != domain.TaskSuccess && st.Tasks[i].Status != domain.TaskFailed {
						st.Tasks[i].Status = domain.TaskFailed
						st.Tasks[i].FailureLog = fmt.Sprintf("execution exceeded global max_tokens ceiling %d (consumed %d tokens)", o.cfg.MaxTokens, globalTokens)
						st.Tasks[i].UpdatedAt = time.Now()
					}
				}
				st.BuildStatus = domain.BuildFailing
				st.StoryStatus = domain.StoryFailed
				return nil
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Orchestrator: failed to persist max_tokens abort: %v\n", err)
			}
			return false, nil
		}
	}

	// 2d. Story-level silent stall enforcement (30-Minute Livelock Ceiling).
	if o.cfg.MaxSilentStallDuration > 0 && len(state.Tasks) > 0 {
		var lastActivity time.Time
		for _, a := range state.LastActions {
			if a.Timestamp.After(lastActivity) {
				lastActivity = a.Timestamp
			}
		}
		for _, t := range state.Tasks {
			if t.UpdatedAt.After(lastActivity) {
				lastActivity = t.UpdatedAt
			}
		}
		if lastActivity.IsZero() {
			lastActivity = o.getStoryStartedAt()
		}
		if !lastActivity.IsZero() && time.Since(lastActivity) > o.cfg.MaxSilentStallDuration && !o.allTasksFinished(state) {
			elapsed := time.Since(lastActivity)
			fmt.Printf("Orchestrator: story exceeded max_silent_stall_duration %s without progress (elapsed %s); failing remaining tasks and aborting story.\n", o.cfg.MaxSilentStallDuration, elapsed.Truncate(time.Second))
			if err := o.updateStateWithRetry(ctx, func(st *domain.State) error {
				for i := range st.Tasks {
					if st.Tasks[i].Status != domain.TaskSuccess && st.Tasks[i].Status != domain.TaskFailed {
						st.Tasks[i].Status = domain.TaskFailed
						st.Tasks[i].FailureLog = fmt.Sprintf("story exceeded max_silent_stall_duration %s without progress (elapsed %s)", o.cfg.MaxSilentStallDuration, elapsed.Truncate(time.Second))
						st.Tasks[i].UpdatedAt = time.Now()
					}
				}
				st.BuildStatus = domain.BuildFailing
				st.StoryStatus = domain.StoryFailed
				return nil
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Orchestrator: failed to persist max_silent_stall_duration abort: %v\n", err)
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
				// Story-Level QA Feature Completeness Gate:
				// Review the user story requirements vs generated codebase.
				// If features are missing and remediation attempts remain, queue remediation task to trigger another Generator-Tester cycle.
				if o.shouldAuditStoryCompleteness(state) {
					storyQAResult, qaErr := o.AuditStoryCompleteness(ctx, state)
					if qaErr == nil && storyQAResult != nil && !storyQAResult.Passed && len(storyQAResult.MissingFeatures) > 0 {
						if o.queueStoryRemediationTask(ctx, state, storyQAResult) {
							// Return true to continue the task processing loop with the newly added remediation task.
							return true, nil
						}
						// If remediation attempts are exhausted, the story fails Definition of Done review
						buildOK = false
						fmt.Printf("❌ Story %s: Definition of Done audit failed with %d missing feature(s):\n", state.Metadata.FeatureName, len(storyQAResult.MissingFeatures))
						for _, f := range storyQAResult.MissingFeatures {
							fmt.Printf(" - %s\n", f)
						}
					}
				}

				if buildOK {
					if finalErr := o.FinalizeUserStory(ctx, state); finalErr != nil {
						fmt.Fprintf(os.Stderr, "Orchestrator: finalization failed: %v\n", finalErr)
					}
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
