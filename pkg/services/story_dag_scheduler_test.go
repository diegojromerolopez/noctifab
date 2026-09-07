package services_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStoryDependencies(t *testing.T) {
	t.Run("extracts YAML array depends_on format", func(t *testing.T) {
		md := "# Story 2\n**depends_on: [\"roadmap/user-stories/US-001-scaffolding.md\"]**\n"
		deps := services.ParseStoryDependencies(md)
		assert.Equal(t, []string{"US-001"}, deps)
	})

	t.Run("extracts plain array depends_on format", func(t *testing.T) {
		md := "depends_on: [US-001, US-002]\n"
		deps := services.ParseStoryDependencies(md)
		assert.Equal(t, []string{"US-001", "US-002"}, deps)
	})

	t.Run("returns empty when depends_on is empty array", func(t *testing.T) {
		md := "depends_on: []\n"
		deps := services.ParseStoryDependencies(md)
		assert.Empty(t, deps)
	})

	t.Run("returns empty when depends_on is absent", func(t *testing.T) {
		md := "# Story 1\nNo dependencies specified.\n"
		deps := services.ParseStoryDependencies(md)
		assert.Empty(t, deps)
	})
}

func TestExtractStoryID(t *testing.T) {
	assert.Equal(t, "US-001", services.ExtractStoryID("roadmap/user-stories/US-001-scaffolding.md"))
	assert.Equal(t, "US-002", services.ExtractStoryID("US-2.md"))
	assert.Equal(t, "", services.ExtractStoryID("feature.md"))
}

func TestStoryDAGScheduler_Execute(t *testing.T) {
	t.Run("executes independent stories in parallel and child stories after parents", func(t *testing.T) {
		scheduler := services.NewStoryDAGScheduler(4)

		item1 := services.StoryWorkItem{Path: "US-001.md", Spec: "depends_on: []"}
		item2 := services.StoryWorkItem{Path: "US-002.md", Spec: "depends_on: [\"US-001\"]"}
		item3 := services.StoryWorkItem{Path: "US-003.md", Spec: "depends_on: [\"US-001\"]"}
		item4 := services.StoryWorkItem{Path: "US-004.md", Spec: "depends_on: [\"US-002\", \"US-003\"]"}

		scheduler.AddStory(item1)
		scheduler.AddStory(item2)
		scheduler.AddStory(item3)
		scheduler.AddStory(item4)

		var executedMu sync.Mutex
		var executedOrder []string
		executionTimes := make(map[string]time.Time)

		err := scheduler.Execute(context.Background(), func(ctx context.Context, item services.StoryWorkItem) error {
			id := services.ExtractStoryID(item.Path)

			executedMu.Lock()
			executionTimes[id] = time.Now()
			executedMu.Unlock()

			time.Sleep(50 * time.Millisecond)

			executedMu.Lock()
			executedOrder = append(executedOrder, id)
			executedMu.Unlock()
			return nil
		})

		require.NoError(t, err)

		executedMu.Lock()
		defer executedMu.Unlock()

		assert.Len(t, executedOrder, 4)
		// US-001 must complete before US-002 and US-003
		assert.Equal(t, "US-001", executedOrder[0])
		// US-004 must be last
		assert.Equal(t, "US-004", executedOrder[3])

		// US-002 and US-003 should start at roughly the same time (parallel execution)
		diff := executionTimes["US-003"].Sub(executionTimes["US-002"])
		if diff < 0 {
			diff = -diff
		}
		assert.True(t, diff < 30*time.Millisecond, "US-002 and US-003 should start concurrently")
	})

	t.Run("detects deadlock if dependencies are unsatisfied", func(t *testing.T) {
		scheduler := services.NewStoryDAGScheduler(2)

		item1 := services.StoryWorkItem{Path: "US-001.md", Spec: "depends_on: [\"US-099\"]"}
		scheduler.AddStory(item1)

		err := scheduler.Execute(context.Background(), func(ctx context.Context, item services.StoryWorkItem) error {
			return nil
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "deadlock detected")
	})

	t.Run("pipelined mode dispatches dependent stories concurrently once parent is running", func(t *testing.T) {
		scheduler := services.NewStoryDAGScheduler(4)
		scheduler.SetPipelined(true)
		assert.True(t, scheduler.IsPipelined())

		item1 := services.StoryWorkItem{Path: "US-001.md", Spec: "depends_on: []"}
		item2 := services.StoryWorkItem{Path: "US-002.md", Spec: "depends_on: [\"US-001\"]"}

		scheduler.AddStory(item1)
		scheduler.AddStory(item2)

		var startMu sync.Mutex
		startTimes := make(map[string]time.Time)
		bothRunning := make(chan struct{})
		var once sync.Once

		err := scheduler.Execute(context.Background(), func(ctx context.Context, item services.StoryWorkItem) error {
			id := services.ExtractStoryID(item.Path)
			startMu.Lock()
			startTimes[id] = time.Now()
			if len(startTimes) == 2 {
				once.Do(func() { close(bothRunning) })
			}
			startMu.Unlock()

			if id == "US-001" {
				// Wait until child story has also started running before completing
				select {
				case <-bothRunning:
				case <-time.After(1 * time.Second):
					t.Error("timed out waiting for child story to start in pipelined mode")
				}
			}
			return nil
		})

		require.NoError(t, err)
		startMu.Lock()
		defer startMu.Unlock()
		assert.Len(t, startTimes, 2)
	})
}

func TestStoryDAGScheduler_InputValidationAndEdgeCases(t *testing.T) {
	t.Run("concurrency less than or equal to zero defaults to 4", func(t *testing.T) {
		s0 := services.NewStoryDAGScheduler(0)
		require.NotNil(t, s0)

		sNeg := services.NewStoryDAGScheduler(-5)
		require.NotNil(t, sNeg)
	})

	t.Run("execute on empty scheduler returns nil", func(t *testing.T) {
		scheduler := services.NewStoryDAGScheduler(2)
		err := scheduler.Execute(context.Background(), func(ctx context.Context, item services.StoryWorkItem) error {
			return nil
		})
		assert.NoError(t, err)
	})

	t.Run("execute with cancelled context aborts and returns error", func(t *testing.T) {
		scheduler := services.NewStoryDAGScheduler(2)
		scheduler.AddStory(services.StoryWorkItem{Path: "US-001.md", Spec: "depends_on: []"})

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		err := scheduler.Execute(ctx, func(c context.Context, item services.StoryWorkItem) error {
			return nil
		})
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("marks story as completed and unblocks downstream story without executing upstream", func(t *testing.T) {
		scheduler := services.NewStoryDAGScheduler(2)
		scheduler.AddStory(services.StoryWorkItem{Path: "US-001.md", Spec: "depends_on: []"})
		scheduler.AddStory(services.StoryWorkItem{Path: "US-002.md", Spec: "depends_on: [\"US-001\"]"})

		// Pre-mark US-001 as completed
		scheduler.MarkStoryCompleted("US-001")
		// Non-existent story mark completed should not panic
		scheduler.MarkStoryCompleted("US-999")

		var executed []string
		err := scheduler.Execute(context.Background(), func(ctx context.Context, item services.StoryWorkItem) error {
			executed = append(executed, services.ExtractStoryID(item.Path))
			return nil
		})

		require.NoError(t, err)
		// Only US-002 should have been executed by processFunc because US-001 was already completed
		assert.Equal(t, []string{"US-002"}, executed)
	})

	t.Run("handles path without US-XXX prefix by falling back to base name", func(t *testing.T) {
		scheduler := services.NewStoryDAGScheduler(2)
		scheduler.AddStory(services.StoryWorkItem{Path: "custom-story.md", Spec: "depends_on: []"})

		var executed []string
		err := scheduler.Execute(context.Background(), func(ctx context.Context, item services.StoryWorkItem) error {
			executed = append(executed, item.Path)
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, []string{"custom-story.md"}, executed)
	})
}
