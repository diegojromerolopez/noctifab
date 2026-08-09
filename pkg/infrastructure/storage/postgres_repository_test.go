package storage

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresRepository_NewAndClose(t *testing.T) {
	ctx := context.Background()

	t.Run("when initializing with invalid connection string, it returns error", func(t *testing.T) {
		repo, err := NewPostgresRepository(ctx, "postgres://invalid:invalid@localhost:54321/db?sslmode=disable", 1, 1)
		assert.Error(t, err)
		assert.Nil(t, repo)
	})

	t.Run("when closing repository, it closes the db connection", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		mock.ExpectClose()
		repo := &PostgresRepository{db: db}
		err = repo.Close()
		assert.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
