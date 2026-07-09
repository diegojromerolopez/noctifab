package domain

import "context"

// StateRepository defines the contract for loading and saving system state.
type StateRepository interface {
	Load(ctx context.Context) (*State, error)
	LoadByID(ctx context.Context, id string) (*State, error)
	LoadAll(ctx context.Context) ([]*State, error)
	Save(ctx context.Context, state *State) error
}
