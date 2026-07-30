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

func (s *SQLiteBudgetStore) GetDailyUsage(ctx context.Context, date string, provider string) (int64, error) {
	var tokens int64
	err := s.db.QueryRowContext(ctx,
		"SELECT tokens_used FROM budget_usage WHERE date = ? AND provider = ?",
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

func (s *SQLiteBudgetStore) IncrementUsage(ctx context.Context, date string, provider string, tokens int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO budget_usage (date, provider, tokens_used) VALUES (?, ?, ?)
		 ON CONFLICT(date, provider) DO UPDATE SET tokens_used = tokens_used + ?`,
		date, provider, tokens, tokens,
	)
	if err != nil {
		return fmt.Errorf("increment usage: %w", err)
	}
	return nil
}
