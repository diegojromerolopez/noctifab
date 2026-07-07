package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type SQLiteBudgetStore struct {
	db *sql.DB
}

var _ domain.BudgetStore = (*SQLiteBudgetStore)(nil)

func NewSQLiteBudgetStore(db *sql.DB) *SQLiteBudgetStore {
	return &SQLiteBudgetStore{db: db}
}

func (s *SQLiteBudgetStore) GetDailyUsage(ctx context.Context, date string, provider string) (float64, error) {
	var cost float64
	err := s.db.QueryRowContext(ctx,
		"SELECT cost_usd FROM budget_usage WHERE date = ? AND provider = ?",
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

func (s *SQLiteBudgetStore) IncrementUsage(ctx context.Context, date string, provider string, costUSD float64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO budget_usage (date, provider, cost_usd) VALUES (?, ?, ?)
		 ON CONFLICT(date, provider) DO UPDATE SET cost_usd = cost_usd + ?`,
		date, provider, costUSD, costUSD,
	)
	if err != nil {
		return fmt.Errorf("increment usage: %w", err)
	}
	return nil
}
