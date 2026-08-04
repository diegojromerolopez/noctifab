package services

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func newDispatchTestOrchestrator(t *testing.T, state *domain.State, concurrency int) (*Orchestrator, *mockRepo) {
	t.Helper()
	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(state.ProjectPath)
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(NewHostSandbox(nil, "", 0, nil), false, nil, nil)
	cfg := OrchestratorConfig{
		PollInterval: 10 * time.Millisecond,
		Concurrency:  concurrency,
	}
	orch := NewOrchestrator(repo, reg, &mockLLM{}, validator, scheduler, git, queue, evaluator, &mockVCS{}, cfg, nil, nil)
	return orch, repo
}

// taskEventRecorder records start/finish times of stubbed task executions.
type taskEventRecorder struct {
	mu         sync.Mutex
	startedAt  map[string]time.Time
	finishedAt map[string]time.Time
	running    int
	maxRunning int
}

func newTaskEventRecorder() *taskEventRecorder {
	return &taskEventRecorder{
		startedAt:  make(map[string]time.Time),
		finishedAt: make(map[string]time.Time),
	}
}

func (r *taskEventRecorder) start(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startedAt[taskID] = time.Now()
	r.running++
	if r.running > r.maxRunning {
		r.maxRunning = r.running
	}
}

func (r *taskEventRecorder) finish(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishedAt[taskID] = time.Now()
	r.running--
}

func markTaskSuccessInRepo(t *testing.T, repo *mockRepo, taskID string) {
	t.Helper()
	state, err := repo.Load(context.Background())
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	for i := range state.Tasks {
		if state.Tasks[i].ID == taskID {
			state.Tasks[i].Status = domain.TaskSuccess
			state.Tasks[i].UpdatedAt = time.Now()
		}
	}
	if err := repo.Save(context.Background(), state); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}
}

// TestRunOnce_ContinuousDispatch verifies the top-priority fix: with
// concurrency 2 and tasks A, B (slow), C (depends on A), C must be dispatched
// as soon as A completes, while B is still running — instead of waiting for
// the whole batch to finish.
func TestRunOnce_ContinuousDispatch(t *testing.T) {
	t.Run("when a slot frees up, it dispatches newly-ready tasks while a slow task is still running", func(t *testing.T) {
		state := &domain.State{
			ID:          "story-continuous",
			ProjectPath: t.TempDir(),
			Tasks: []domain.Task{
				{ID: "A", Title: "Task A", Description: "quick task", Status: domain.TaskPending, MaxRetries: 3},
				{ID: "B", Title: "Task B", Description: "slow task", Status: domain.TaskPending, MaxRetries: 3},
				{ID: "C", Title: "Task C", Description: "depends on A", Status: domain.TaskPending, MaxRetries: 3, DependsOn: []string{"A"}},
			},
		}
		orch, repo := newDispatchTestOrchestrator(t, state, 2)

		rec := newTaskEventRecorder()
		orch.executeTaskFn = func(ctx context.Context, stateID, taskID string) {
			rec.start(taskID)
			defer rec.finish(taskID)
			switch taskID {
			case "A":
				time.Sleep(20 * time.Millisecond)
			case "B":
				time.Sleep(400 * time.Millisecond)
			case "C":
				time.Sleep(20 * time.Millisecond)
			}
			markTaskSuccessInRepo(t, repo, taskID)
		}

		hasWork, err := orch.RunOnce(context.Background())
		if err != nil {
			t.Fatalf("RunOnce returned error: %v", err)
		}
		if !hasWork {
			t.Fatal("expected RunOnce to report work executed")
		}

		rec.mu.Lock()
		defer rec.mu.Unlock()
		cStart, cRan := rec.startedAt["C"]
		bFinish, bRan := rec.finishedAt["B"]
		if !cRan || !bRan {
			t.Fatalf("expected all tasks to run; started=%v finished=%v", rec.startedAt, rec.finishedAt)
		}
		if !cStart.Before(bFinish) {
			t.Errorf("expected C to start (%v) while slow task B was still running (finished %v)", cStart, bFinish)
		}
		if len(rec.finishedAt) != 3 {
			t.Errorf("expected 3 tasks executed, got %d", len(rec.finishedAt))
		}
	})

	t.Run("when the concurrency limit is 1, it never runs more than one task at a time", func(t *testing.T) {
		state := &domain.State{
			ID:          "story-limit",
			ProjectPath: t.TempDir(),
			Tasks: []domain.Task{
				{ID: "A", Title: "Task A", Description: "task a", Status: domain.TaskPending, MaxRetries: 3},
				{ID: "B", Title: "Task B", Description: "task b", Status: domain.TaskPending, MaxRetries: 3},
				{ID: "C", Title: "Task C", Description: "task c", Status: domain.TaskPending, MaxRetries: 3},
			},
		}
		orch, repo := newDispatchTestOrchestrator(t, state, 1)

		rec := newTaskEventRecorder()
		orch.executeTaskFn = func(ctx context.Context, stateID, taskID string) {
			rec.start(taskID)
			defer rec.finish(taskID)
			time.Sleep(20 * time.Millisecond)
			markTaskSuccessInRepo(t, repo, taskID)
		}

		if _, err := orch.RunOnce(context.Background()); err != nil {
			t.Fatalf("RunOnce returned error: %v", err)
		}

		rec.mu.Lock()
		defer rec.mu.Unlock()
		if rec.maxRunning > 1 {
			t.Errorf("expected at most 1 concurrent task, observed %d", rec.maxRunning)
		}
		if len(rec.finishedAt) != 3 {
			t.Errorf("expected all 3 tasks to execute across the continuous loop, got %d", len(rec.finishedAt))
		}
	})

	t.Run("when a task does not complete its dependents, it returns without dispatching them", func(t *testing.T) {
		state := &domain.State{
			ID:          "story-blocked-dep",
			ProjectPath: t.TempDir(),
			Tasks: []domain.Task{
				{ID: "A", Title: "Task A", Description: "task a fails", Status: domain.TaskPending, MaxRetries: 0},
				{ID: "C", Title: "Task C", Description: "depends on A", Status: domain.TaskPending, MaxRetries: 3, DependsOn: []string{"A"}},
			},
		}
		orch, repo := newDispatchTestOrchestrator(t, state, 2)

		rec := newTaskEventRecorder()
		orch.executeTaskFn = func(ctx context.Context, stateID, taskID string) {
			rec.start(taskID)
			defer rec.finish(taskID)
			// A fails permanently: C must never run.
			st, _ := repo.Load(context.Background())
			for i := range st.Tasks {
				if st.Tasks[i].ID == taskID {
					st.Tasks[i].Status = domain.TaskFailed
					st.Tasks[i].UpdatedAt = time.Now()
				}
			}
			_ = repo.Save(context.Background(), st)
		}

		if _, err := orch.RunOnce(context.Background()); err != nil {
			t.Fatalf("RunOnce returned error: %v", err)
		}

		rec.mu.Lock()
		defer rec.mu.Unlock()
		if _, ran := rec.startedAt["C"]; ran {
			t.Error("task C must not run when its dependency A failed")
		}
	})
}
