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
			t.Errorf("expected 0, got %d", usage)
		}
	})

	t.Run("IncrementUsage increases value", func(t *testing.T) {
		err := store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 1500)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		usage, err := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if usage != 1500 {
			t.Errorf("expected 1500, got %d", usage)
		}
	})

	t.Run("IncrementUsage accumulates multiple calls", func(t *testing.T) {
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 2000)
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 3000)
		usage, err := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if usage != 6500 {
			t.Errorf("expected 6500, got %d", usage)
		}
	})

	t.Run("different dates tracked separately", func(t *testing.T) {
		_ = store.IncrementUsage(context.Background(), "2026-07-02", "gpt-4o", 1000)
		_ = store.IncrementUsage(context.Background(), "2026-07-03", "gpt-4o", 2000)
		day1, _ := store.GetDailyUsage(context.Background(), "2026-07-02", "gpt-4o")
		day2, _ := store.GetDailyUsage(context.Background(), "2026-07-03", "gpt-4o")
		if day1 != 7500 {
			t.Errorf("expected day1 7500, got %d", day1)
		}
		if day2 != 2000 {
			t.Errorf("expected day2 2000, got %d", day2)
		}
	})
}
