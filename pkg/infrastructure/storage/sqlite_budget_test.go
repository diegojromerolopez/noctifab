package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func newSQLiteBudgetTestRepo(t *testing.T) (*SQLiteRepository, string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "noctifab-sqlite-budget-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	dsn := filepath.Join(tmpDir, "test.db")
	repo, err := NewSQLiteRepository(context.Background(), dsn)
	if err != nil {
		t.Fatalf("failed to create sqlite repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo, dsn
}

func TestSQLiteBudgetStore(t *testing.T) {
	repo, _ := newSQLiteBudgetTestRepo(t)
	store := NewSQLiteBudgetStore(repo.db)

	t.Run("GetDailyUsage returns zero for new date/provider", func(t *testing.T) {
		usage, err := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if usage != 0 {
			t.Errorf("expected 0, got %.2f", usage)
		}
	})

	t.Run("IncrementUsage increases value", func(t *testing.T) {
		err := store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 1.50)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		usage, err := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if usage != 1.50 {
			t.Errorf("expected 1.50, got %.2f", usage)
		}
	})

	t.Run("IncrementUsage accumulates multiple calls", func(t *testing.T) {
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 2.00)
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 3.00)
		usage, err := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if usage != 6.50 {
			t.Errorf("expected 6.50, got %.2f", usage)
		}
	})

	t.Run("different dates tracked separately", func(t *testing.T) {
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 1.00)
		_ = store.IncrementUsage(context.Background(), "2026-07-03", "gpt-4o", 2.00)
		day1, _ := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		day2, _ := store.GetDailyUsage(context.Background(), "2026-07-03", "gpt-4o")
		if day1 != 7.50 {
			t.Errorf("expected day1 7.50, got %.2f", day1)
		}
		if day2 != 2.00 {
			t.Errorf("expected day2 2.00, got %.2f", day2)
		}
	})
}
