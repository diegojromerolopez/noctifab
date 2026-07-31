package domain

import (
	"context"
	"testing"
)

type mockBudgetStore struct {
	usage   int64
	records map[string]int64
}

func (m *mockBudgetStore) GetDailyUsage(_ context.Context, date string, provider string) (int64, error) {
	if m.records != nil {
		return m.records[date+"|"+provider], nil
	}
	return m.usage, nil
}

func (m *mockBudgetStore) IncrementUsage(_ context.Context, date string, provider string, tokens int64) error {
	if m.records == nil {
		m.records = make(map[string]int64)
	}
	m.records[date+"|"+provider] += tokens
	return nil
}

func TestBudgetStoreInterface(t *testing.T) {
	t.Run("GetDailyUsage returns zero when no records exist", func(t *testing.T) {
		store := &mockBudgetStore{records: make(map[string]int64)}
		usage, err := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if usage != 0 {
			t.Errorf("expected 0, got %d", usage)
		}
	})

	t.Run("IncrementUsage accumulates correctly", func(t *testing.T) {
		store := &mockBudgetStore{records: make(map[string]int64)}
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 1500)
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 2500)
		usage, _ := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		if usage != 4000 {
			t.Errorf("expected 4000, got %d", usage)
		}
	})

	t.Run("different providers tracked separately", func(t *testing.T) {
		store := &mockBudgetStore{records: make(map[string]int64)}
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 1000)
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "claude-3", 2000)
		gptUsage, _ := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		claudeUsage, _ := store.GetDailyUsage(context.Background(), "2026-07-02", "claude-3")
		if gptUsage != 1000 {
			t.Errorf("expected gpt-4o usage 1000, got %d", gptUsage)
		}
		if claudeUsage != 2000 {
			t.Errorf("expected claude-3 usage 2000, got %d", claudeUsage)
		}
	})
}
