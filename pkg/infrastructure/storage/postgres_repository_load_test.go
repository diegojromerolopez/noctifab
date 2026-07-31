package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresRepository_Load_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("when main state query fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		mock.ExpectQuery("SELECT id, project_path").WillReturnError(errors.New("db error"))

		_, err = repo.Load(ctx)
		assert.ErrorContains(t, err, "db error")
	})

	t.Run("when query validations fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		stateRows := sqlmock.NewRows([]string{
			"id", "project_path", "version", "build_status", "story_status", "story_error", "input_source", "input_path", "integration_branch", "feature_name", "base_branch", "project_version", "total_tokens_used", "total_cost_usd",
		}).AddRow(
			"state-1", "/src", 2, "PASSING", "", "", "jira", "https://jira.com/1", "feature/foo", "Foo", "main", "1.0.0", 100, 0.00300,
		)

		mock.ExpectQuery("SELECT id, project_path").WillReturnRows(stateRows)
		mock.ExpectQuery("SELECT id, title").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectQuery("SELECT question").WillReturnRows(sqlmock.NewRows([]string{"question"}))
		mock.ExpectQuery("SELECT timestamp").WillReturnRows(sqlmock.NewRows([]string{"timestamp"}))
		mock.ExpectQuery("SELECT path").WillReturnRows(sqlmock.NewRows([]string{"path"}))
		mock.ExpectQuery("SELECT id, type").WillReturnError(errors.New("validation error"))

		_, err = repo.Load(ctx)
		assert.ErrorContains(t, err, "validation error")
	})

	t.Run("when query active_agents fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		stateRows := sqlmock.NewRows([]string{
			"id", "project_path", "version", "build_status", "story_status", "story_error", "input_source", "input_path", "integration_branch", "feature_name", "base_branch", "project_version", "total_tokens_used", "total_cost_usd",
		}).AddRow(
			"state-1", "/src", 2, "PASSING", "", "", "jira", "https://jira.com/1", "feature/foo", "Foo", "main", "1.0.0", 100, 0.00300,
		)

		mock.ExpectQuery("SELECT id, project_path").WillReturnRows(stateRows)
		mock.ExpectQuery("SELECT id, title").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectQuery("SELECT question").WillReturnRows(sqlmock.NewRows([]string{"question"}))
		mock.ExpectQuery("SELECT timestamp").WillReturnRows(sqlmock.NewRows([]string{"timestamp"}))
		mock.ExpectQuery("SELECT path").WillReturnRows(sqlmock.NewRows([]string{"path"}))
		mock.ExpectQuery("SELECT id, type").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectQuery("SELECT id, name").WillReturnError(errors.New("agents error"))

		_, err = repo.Load(ctx)
		assert.ErrorContains(t, err, "agents error")
	})
}

func TestPostgresRepository_Load(t *testing.T) {
	ctx := context.Background()

	t.Run("when state exists, it constructs model using SQL queries", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		now := time.Now()

		stateRows := sqlmock.NewRows([]string{
			"id", "project_path", "version", "build_status", "story_status", "story_error", "input_source", "input_path", "integration_branch", "feature_name", "base_branch", "project_version", "total_tokens_used", "total_cost_usd",
		}).AddRow(
			"state-1", "/src", 2, "PASSING", "RUNNING", "", "jira", "https://jira.com/1", "feature/foo", "Foo", "main", "1.0.0", 100, 0.00300,
		)

		mock.ExpectQuery("SELECT id, project_path").WillReturnRows(stateRows)

		taskRows := sqlmock.NewRows([]string{
			"id", "title", "description", "status", "change_type", "assigned_to", "progress", "depends_on", "target_files", "partial_changelog", "retries", "max_retries", "failure_log", "created_at", "updated_at",
		}).AddRow(
			"task-1", "Task Title", "Desc", "PENDING", "FIX", "agent-1", 45, `["task-0"]`, `["foo.go"]`, `["Changelog"]`, 0, 3, "Test Failure Log", now, now,
		)
		mock.ExpectQuery("SELECT id, title").WithArgs("state-1").WillReturnRows(taskRows)

		mock.ExpectQuery("SELECT question").WithArgs("state-1").WillReturnRows(sqlmock.NewRows([]string{"question"}))
		mock.ExpectQuery("SELECT timestamp").WithArgs("state-1").WillReturnRows(sqlmock.NewRows([]string{"timestamp"}))
		mock.ExpectQuery("SELECT path").WithArgs("state-1").WillReturnRows(sqlmock.NewRows([]string{"path"}))
		mock.ExpectQuery("SELECT id, type").WithArgs("state-1").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectQuery("SELECT id, name").WithArgs("state-1").WillReturnRows(sqlmock.NewRows([]string{"id"}))

		state, err := repo.Load(ctx)
		assert.NoError(t, err)
		require.NotNil(t, state)
		assert.Equal(t, "state-1", state.ID)
		require.Len(t, state.Tasks, 1)
		assert.Equal(t, "task-1", state.Tasks[0].ID)
		assert.Equal(t, 45, state.Tasks[0].Progress)
	})

	t.Run("when state does not exist, it returns sql.ErrNoRows", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		mock.ExpectQuery("SELECT id, project_path").WillReturnError(sql.ErrNoRows)

		_, err = repo.Load(ctx)
		assert.ErrorIs(t, err, sql.ErrNoRows)
	})
}
