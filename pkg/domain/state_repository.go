package domain

import "context"

// StateRepository defines the contract for loading and saving system state.
type StateRepository interface {
	Load(ctx context.Context) (*State, error)
	LoadByID(ctx context.Context, id string) (*State, error)
	LoadAll(ctx context.Context) ([]*State, error)
	// LoadAllSummaries returns lightweight per-story projections (state row
	// plus task status counts) without loading actions, files, or task bodies.
	LoadAllSummaries(ctx context.Context) ([]StateSummary, error)
	Save(ctx context.Context, state *State) error
	// PruneFinishedStates deletes terminal (SUCCESS/FAILED) states beyond the
	// most recent keepLast, cascading deletion of all their relation rows.
	// It returns the number of pruned states.
	PruneFinishedStates(ctx context.Context, keepLast int) (int, error)
}
