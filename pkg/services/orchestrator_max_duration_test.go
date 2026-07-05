package services

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// TestRunOnce_MaxDurationExceeded verifies that when the configured
// max_duration has elapsed since the first ready-task cycle, RunOnce fails
// every non-finished task and marks the story as FAILED without dispatching
// any new work.
func TestRunOnce_MaxDurationExceeded(t *testing.T) {
	state := &domain.State{
		ID:          "story-maxdur",
		ProjectPath: t.TempDir(),
		Tasks: []domain.Task{
			{ID: "t1", Title: "T1", Description: "desc one", Status: domain.TaskInProgress, MaxRetries: 3},
			{ID: "t2", Title: "T2", Description: "desc two", Status: domain.TaskPending, MaxRetries: 3},
			{ID: "t3", Title: "T3", Description: "desc three", Status: domain.TaskSuccess, MaxRetries: 3},
		},
		StoryStatus: domain.StoryIdle,
	}
	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(state.ProjectPath)
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(NewHostSandbox(nil, "", 0, nil), false, nil, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{
		PollInterval: 10 * time.Millisecond,
		Concurrency:  1,
		MaxDuration:  1 * time.Nanosecond, // effectively immediate
	}
	orch := NewOrchestrator(repo, reg, &mockLLM{}, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil)
	// Pre-seed the start time so the very next RunOnce is past the limit.
	orch.storyStartedAt = time.Now().Add(-1 * time.Hour)

	if err := orch.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}

	final := repo.state
	if final.StoryStatus != domain.StoryFailed {
		t.Errorf("expected StoryStatus=StoryFailed, got %q", final.StoryStatus)
	}
	if final.BuildStatus != domain.BuildFailing {
		t.Errorf("expected BuildStatus=BuildFailing, got %q", final.BuildStatus)
	}
	for i, tk := range final.Tasks {
		if tk.ID == "t3" {
			if tk.Status != domain.TaskSuccess {
				t.Errorf("t3 should remain TaskSuccess, got %q", tk.Status)
			}
			continue
		}
		if tk.Status != domain.TaskFailed {
			t.Errorf("task %s (idx %d) expected TaskFailed, got %q", tk.ID, i, tk.Status)
		}
		if tk.FailureLog == "" {
			t.Errorf("task %s expected non-empty FailureLog", tk.ID)
		}
	}
}

// TestRunOnce_MaxDurationZero_Disabled verifies that a MaxDuration of 0
// disables the wall-clock check (the historical default behaviour): a story
// with running tasks and an old storyStartedAt stays Idle and is not aborted.
func TestRunOnce_MaxDurationZero_Disabled(t *testing.T) {
	state := &domain.State{
		ID:          "story-nomax",
		ProjectPath: t.TempDir(),
		Tasks: []domain.Task{
			{ID: "t1", Title: "T1", Description: "desc one", Status: domain.TaskInProgress, MaxRetries: 3},
		},
		StoryStatus: domain.StoryIdle,
	}
	repo := &mockRepo{state: state}
	reg := NewToolRegistry()
	validator := NewPolicyValidator(nil, "main", nil)
	scheduler := NewScheduler(NewFileLockRegistry())
	git := NewGitClient(state.ProjectPath)
	queue := NewRebaseQueue(git)
	evaluator := NewTestValidator(NewHostSandbox(nil, "", 0, nil), false, nil, nil)
	vcsClient := &mockVCS{}
	cfg := OrchestratorConfig{
		PollInterval: 10 * time.Millisecond,
		Concurrency:  1,
		MaxDuration:  0, // disabled
	}
	orch := NewOrchestrator(repo, reg, &mockLLM{}, validator, scheduler, git, queue, evaluator, vcsClient, cfg, nil, nil)
	orch.storyStartedAt = time.Now().Add(-1 * time.Hour)

	_ = orch.RunOnce(context.Background())

	// Story should NOT have been aborted by max_duration (it's disabled).
	// The InProgress task will yield 0 ready tasks; finalization runs because
	// not all tasks are finished (TaskInProgress is not finished), so
	// StoryStatus stays Idle. We assert it is NOT StoryFailed.
	if repo.state.StoryStatus == domain.StoryFailed {
		t.Errorf("expected StoryStatus to NOT be StoryFailed when MaxDuration=0, got %q", repo.state.StoryStatus)
	}
}
