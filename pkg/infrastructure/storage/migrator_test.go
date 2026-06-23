package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrate_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("when unsupported database type, it returns error", func(t *testing.T) {
		db, _, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		err = Migrate(ctx, db, "invalid_db")
		assert.ErrorContains(t, err, "unsupported database type")
	})

	t.Run("when table creation fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
			WillReturnError(errors.New("table creation error"))

		err = Migrate(ctx, db, "sqlite")
		assert.ErrorContains(t, err, "table creation error")
	})

	t.Run("when transaction begin fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectBegin().WillReturnError(errors.New("begin error"))

		err = Migrate(ctx, db, "sqlite")
		assert.ErrorContains(t, err, "begin error")
	})

	t.Run("when querying applied migrations fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM schema_migrations`).
			WillReturnError(errors.New("query error"))
		mock.ExpectRollback()

		err = Migrate(ctx, db, "sqlite")
		assert.ErrorContains(t, err, "query error")
	})

	t.Run("when scan applied migrations fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM schema_migrations`).
			WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow("not_an_int"))
		mock.ExpectRollback()

		err = Migrate(ctx, db, "sqlite")
		assert.ErrorContains(t, err, "scan migration version")
	})

	t.Run("when executing migration sql fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM schema_migrations`).
			WillReturnRows(sqlmock.NewRows([]string{"version"}))
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS state`).
			WillReturnError(errors.New("exec sql error"))
		mock.ExpectRollback()

		err = Migrate(ctx, db, "sqlite")
		assert.ErrorContains(t, err, "exec sql error")
	})

	t.Run("when inserting migration version fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM schema_migrations`).
			WillReturnRows(sqlmock.NewRows([]string{"version"}))
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS state`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`INSERT INTO schema_migrations`).
			WillReturnError(errors.New("insert version error"))
		mock.ExpectRollback()

		err = Migrate(ctx, db, "sqlite")
		assert.ErrorContains(t, err, "insert version error")
	})

	t.Run("when commit fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS schema_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectBegin()
		mock.ExpectQuery(`SELECT version FROM schema_migrations`).
			WillReturnRows(sqlmock.NewRows([]string{"version"}))
		mock.ExpectExec(`CREATE TABLE IF NOT EXISTS state`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`INSERT INTO schema_migrations`).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectCommit().WillReturnError(errors.New("commit error"))

		err = Migrate(ctx, db, "sqlite")
		assert.ErrorContains(t, err, "commit error")
	})
}
