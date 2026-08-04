package storage

import (
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// summaryAccumulator incrementally folds lightweight per-task rows
// (state_id, status, created_at, updated_at) into StateSummary projections.
// It is shared by the SQLite and PostgreSQL repositories so their summary
// semantics stay identical, and is unit-testable without a database.
type summaryAccumulator struct {
	summaries []domain.StateSummary
	indexByID map[string]int
}

// newSummaryAccumulator creates an accumulator seeded with summaries built
// from state rows (in their query order).
func newSummaryAccumulator() *summaryAccumulator {
	return &summaryAccumulator{indexByID: make(map[string]int)}
}

// addState registers a state-row projection. Task counts start empty.
func (a *summaryAccumulator) addState(summary domain.StateSummary) {
	if summary.TaskCounts == nil {
		summary.TaskCounts = make(map[string]int)
	}
	a.indexByID[summary.ID] = len(a.summaries)
	a.summaries = append(a.summaries, summary)
}

// addTask folds a single task's status and timestamps into its parent
// state summary. Tasks referencing unknown states are ignored.
func (a *summaryAccumulator) addTask(stateID, status string, createdAt, updatedAt time.Time) {
	idx, ok := a.indexByID[stateID]
	if !ok {
		return
	}
	summary := &a.summaries[idx]
	summary.TotalTasks++
	summary.TaskCounts[status]++
	if summary.CreatedAt.IsZero() || (!createdAt.IsZero() && createdAt.Before(summary.CreatedAt)) {
		summary.CreatedAt = createdAt
	}
	if updatedAt.After(summary.UpdatedAt) {
		summary.UpdatedAt = updatedAt
	}
}

// result returns the accumulated summaries (never nil).
func (a *summaryAccumulator) result() []domain.StateSummary {
	if a.summaries == nil {
		return []domain.StateSummary{}
	}
	return a.summaries
}
