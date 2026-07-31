package domain

import "context"

// BudgetRecord tracks daily token usage for an LLM provider.
type BudgetRecord struct {
	Date       string
	Provider   string
	TokensUsed int64
}

// BudgetStore persists daily token usage per provider.
type BudgetStore interface {
	GetDailyUsage(ctx context.Context, date string, provider string) (int64, error)
	IncrementUsage(ctx context.Context, date string, provider string, tokens int64) error
}
