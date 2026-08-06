package storage

import (
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummaryAccumulator(t *testing.T) {
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

	t.Run("when tasks are folded in, it counts by status and tracks min/max timestamps", func(t *testing.T) {
		acc := newSummaryAccumulator()
		acc.addState(domain.StateSummary{ID: "s1"})

		acc.addTask("s1", "SUCCESS", now.Add(-2*time.Hour), now.Add(-time.Hour))
		acc.addTask("s1", "SUCCESS", now.Add(-time.Hour), now)
		acc.addTask("s1", "PENDING", now.Add(-30*time.Minute), now.Add(-time.Minute))

		result := acc.result()
		require.Len(t, result, 1)
		s := result[0]
		assert.Equal(t, 3, s.TotalTasks)
		assert.Equal(t, 2, s.TaskCounts["SUCCESS"])
		assert.Equal(t, 1, s.TaskCounts["PENDING"])
		assert.True(t, s.CreatedAt.Equal(now.Add(-2*time.Hour)))
		assert.True(t, s.UpdatedAt.Equal(now))
	})

	t.Run("when a task references an unknown state, it is ignored", func(t *testing.T) {
		acc := newSummaryAccumulator()
		acc.addState(domain.StateSummary{ID: "s1"})
		acc.addTask("ghost", "PENDING", now, now)
		result := acc.result()
		require.Len(t, result, 1)
		assert.Equal(t, 0, result[0].TotalTasks)
	})

	t.Run("when a state has no tasks, its counts map is empty but non-nil", func(t *testing.T) {
		acc := newSummaryAccumulator()
		acc.addState(domain.StateSummary{ID: "s1"})
		result := acc.result()
		require.Len(t, result, 1)
		assert.NotNil(t, result[0].TaskCounts)
		assert.Empty(t, result[0].TaskCounts)
	})

	t.Run("when no states are added, it returns an empty non-nil slice", func(t *testing.T) {
		acc := newSummaryAccumulator()
		assert.NotNil(t, acc.result())
		assert.Empty(t, acc.result())
	})
}
