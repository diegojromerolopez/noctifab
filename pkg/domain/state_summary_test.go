package domain

import (
	"testing"
	"time"
)

func TestSummarizeState(t *testing.T) {
	now := time.Now()

	t.Run("when the state has tasks, it counts them by status and tracks timestamps", func(t *testing.T) {
		s := &State{
			ID:          "s1",
			Version:     3,
			StoryStatus: StoryRunning,
			BuildStatus: BuildFailing,
			StoryError:  "boom",
			Metadata: StateMetadata{
				FeatureName:       "feat",
				InputPath:         "spec.md",
				IntegrationBranch: "noctifab/feat",
				BaseBranch:        "main",
			},
			Tasks: []Task{
				{Status: TaskSuccess, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-time.Hour)},
				{Status: TaskSuccess, CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
				{Status: TaskPending, CreatedAt: now.Add(-30 * time.Minute), UpdatedAt: now.Add(-time.Minute)},
			},
		}
		got := SummarizeState(s)
		if got.ID != "s1" || got.Version != 3 || got.StoryStatus != "RUNNING" || got.BuildStatus != "FAILING" {
			t.Errorf("unexpected identifiers: %+v", got)
		}
		if got.StoryError != "boom" || got.FeatureName != "feat" || got.BaseBranch != "main" {
			t.Errorf("unexpected metadata: %+v", got)
		}
		if got.TotalTasks != 3 {
			t.Errorf("expected 3 total tasks, got %d", got.TotalTasks)
		}
		if got.TaskCounts["SUCCESS"] != 2 || got.TaskCounts["PENDING"] != 1 {
			t.Errorf("unexpected task counts: %v", got.TaskCounts)
		}
		if !got.CreatedAt.Equal(now.Add(-2 * time.Hour)) {
			t.Errorf("expected earliest CreatedAt, got %v", got.CreatedAt)
		}
		if !got.UpdatedAt.Equal(now) {
			t.Errorf("expected latest UpdatedAt, got %v", got.UpdatedAt)
		}
	})

	t.Run("when the state has no tasks, it returns an empty non-nil counts map", func(t *testing.T) {
		got := SummarizeState(&State{ID: "s2"})
		if got.TaskCounts == nil {
			t.Fatal("expected non-nil TaskCounts map")
		}
		if got.TotalTasks != 0 || len(got.TaskCounts) != 0 {
			t.Errorf("expected empty counts, got %+v", got)
		}
	})
}

func TestStoryStatusIsTerminal(t *testing.T) {
	t.Run("when the status is SUCCESS or FAILED, it is terminal", func(t *testing.T) {
		if !StorySuccess.IsTerminal() || !StoryFailed.IsTerminal() {
			t.Error("expected SUCCESS and FAILED to be terminal")
		}
	})

	t.Run("when the status is running, paused, cancelled, or idle, it is not terminal", func(t *testing.T) {
		for _, s := range []StoryStatus{StoryRunning, StoryPaused, StoryCancelled, StoryIdle} {
			if s.IsTerminal() {
				t.Errorf("expected %q not to be terminal", s)
			}
		}
	})
}
