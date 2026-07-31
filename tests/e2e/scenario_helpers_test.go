package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	if os.Getenv("NOCTIFAB_E2E") != "true" {
		fmt.Println("Skipping tests in tests/e2e package because NOCTIFAB_E2E is not set to true")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func setupRepo(t *testing.T, ctx context.Context, tempDir, subDir, sessionID string) (domain.StateRepository, func()) {
	dbProvider := os.Getenv("NOCTIFAB_STORAGE_PROVIDER")
	if dbProvider == "postgres" {
		dsn := os.Getenv("NOCTIFAB_TEST_DB_DSN")
		if dsn == "" {
			dsn = "postgres://noctifab:noctifab_password@db:5432/noctifab_test?sslmode=disable"
		}
		repo, err := storage.NewPostgresRepository(ctx, dsn, 10, 10)
		require.NoError(t, err)

		// Clean up previous test state for isolation
		db, err := sql.Open("pgx", dsn)
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "TRUNCATE TABLE state CASCADE")
		require.NoError(t, err)
		_ = db.Close()

		return repo, func() { _ = repo.Close() }
	}

	// Fallback to SQLite
	dbDir := filepath.Join(tempDir, subDir, ".noctifab", "config")
	err := os.MkdirAll(dbDir, 0755)
	require.NoError(t, err)

	dbPath := filepath.Join(dbDir, "noctifab.db")
	repo, err := storage.NewSQLiteRepository(ctx, dbPath)
	require.NoError(t, err)
	return repo, func() { _ = repo.Close() }
}
