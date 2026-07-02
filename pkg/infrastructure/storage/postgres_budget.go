package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type PostgresBudgetStore struct {
	db *sql.DB
}

var _ domain.BudgetStore = (*PostgresBudgetStore)(nil)

func NewPostgresBudgetStore(db *sql.DB) *PostgresBudgetStore {
	return &PostgresBudgetStore{db: db}
}

func (s *PostgresBudgetStore) GetDailyUsage(ctx context.Context, date string, provider string) (float64, error) {
	var cost float64
	err := s.db.QueryRowContext(ctx,
		"SELECT cost_usd FROM budget_usage WHERE date = $1 AND provider = $2",
		date, provider,
	).Scan(&cost)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get daily usage: %w", err)
	}
	return cost, nil
}

func (s *PostgresBudgetStore) IncrementUsage(ctx context.Context, date string, provider string, costUSD float64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO budget_usage (date, provider, cost_usd) VALUES ($1, $2, $3)
		 ON CONFLICT(date, provider) DO UPDATE SET cost_usd = budget_usage.cost_usd + $3`,
		date, provider, costUSD,
	)
	if err != nil {
		return fmt.Errorf("increment usage: %w", err)
	}
	return nil
}
