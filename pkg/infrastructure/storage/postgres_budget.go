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

func (s *PostgresBudgetStore) GetDailyUsage(ctx context.Context, date string, provider string) (int64, error) {
	var tokens int64
	err := s.db.QueryRowContext(ctx,
		"SELECT tokens_used FROM budget_usage WHERE date = $1 AND provider = $2",
		date, provider,
	).Scan(&tokens)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get daily usage: %w", err)
	}
	return tokens, nil
}

func (s *PostgresBudgetStore) IncrementUsage(ctx context.Context, date string, provider string, tokens int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO budget_usage (date, provider, tokens_used) VALUES ($1, $2, $3)
		 ON CONFLICT(date, provider) DO UPDATE SET tokens_used = budget_usage.tokens_used + $3`,
		date, provider, tokens,
	)
	if err != nil {
		return fmt.Errorf("increment usage: %w", err)
	}
	return nil
}
