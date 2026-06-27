package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/diegojromerolopez/noctifab/pkg/domain"
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
		mock.ExpectQuery("SELECT s.id, s.project_path").WillReturnError(errors.New("db error"))

		_, err = repo.Load(ctx)
		assert.ErrorContains(t, err, "db error")
	})

	t.Run("when query validations fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		stateRows := sqlmock.NewRows([]string{
			"s.id", "s.project_path", "s.version", "s.build_status", "s.input_source", "s.input_path", "s.integration_branch", "s.feature_name", "s.base_branch", "s.project_version", "s.total_tokens_used", "s.total_cost_usd",
			"t.id", "t.title", "t.description", "t.status", "t.change_type", "t.assigned_to", "t.depends_on", "t.target_files", "t.partial_changelog", "t.retries", "t.max_retries", "t.failure_log", "t.created_at", "t.updated_at",
		}).AddRow(
			"state-1", "/src", 2, "PASSING", "jira", "https://jira.com/1", "feature/foo", "Foo", "main", "1.0.0", 100, "0.00300",
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		)

		mock.ExpectQuery("SELECT s.id, s.project_path").WillReturnRows(stateRows)
		mock.ExpectQuery("SELECT c.question").WillReturnRows(sqlmock.NewRows([]string{"question"}))
		mock.ExpectQuery("SELECT a.timestamp").WillReturnRows(sqlmock.NewRows([]string{"timestamp"}))
		mock.ExpectQuery("SELECT wf.path").WillReturnRows(sqlmock.NewRows([]string{"path"}))
		mock.ExpectQuery("SELECT vc.id").WillReturnError(errors.New("validation error"))

		_, err = repo.Load(ctx)
		assert.ErrorContains(t, err, "validation error")
	})

	t.Run("when query active_agents fails, it returns error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		stateRows := sqlmock.NewRows([]string{
			"s.id", "s.project_path", "s.version", "s.build_status", "s.input_source", "s.input_path", "s.integration_branch", "s.feature_name", "s.base_branch", "s.project_version", "s.total_tokens_used", "s.total_cost_usd",
			"t.id", "t.title", "t.description", "t.status", "t.change_type", "t.assigned_to", "t.depends_on", "t.target_files", "t.partial_changelog", "t.retries", "t.max_retries", "t.failure_log", "t.created_at", "t.updated_at",
		}).AddRow(
			"state-1", "/src", 2, "PASSING", "jira", "https://jira.com/1", "feature/foo", "Foo", "main", "1.0.0", 100, "0.00300",
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		)

		mock.ExpectQuery("SELECT s.id, s.project_path").WillReturnRows(stateRows)
		mock.ExpectQuery("SELECT c.question").WillReturnRows(sqlmock.NewRows([]string{"question"}))
		mock.ExpectQuery("SELECT a.timestamp").WillReturnRows(sqlmock.NewRows([]string{"timestamp"}))
		mock.ExpectQuery("SELECT wf.path").WillReturnRows(sqlmock.NewRows([]string{"path"}))
		mock.ExpectQuery("SELECT vc.id").WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectQuery("SELECT aa.id").WillReturnError(errors.New("agents error"))

		_, err = repo.Load(ctx)
		assert.ErrorContains(t, err, "agents error")
	})
}

func TestPostgresRepository_Load(t *testing.T) {
	ctx := context.Background()

	t.Run("when state exists, it constructs model using SQL joins", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}
		now := time.Now()

		stateRows := sqlmock.NewRows([]string{
			"s.id", "s.project_path", "s.version", "s.build_status", "s.input_source", "s.input_path", "s.integration_branch", "s.feature_name", "s.base_branch", "s.project_version", "s.total_tokens_used", "s.total_cost_usd",
			"t.id", "t.title", "t.description", "t.status", "t.change_type", "t.assigned_to", "t.depends_on", "t.target_files", "t.partial_changelog", "t.retries", "t.max_retries", "t.failure_log", "t.created_at", "t.updated_at",
		}).AddRow(
			"state-1", "/src", 2, "PASSING", "jira", "https://jira.com/1", "feature/foo", "Foo", "main", "1.0.0", 100, "0.00300",
			"task-1", "Task Title", "Desc", "PENDING", "FIX", "agent-1", `["task-0"]`, `["foo.go"]`, `["Changelog"]`, 0, 3, "Test Failure Log", now, now,
		)

		mock.ExpectQuery("SELECT s.id, s.project_path").WillReturnRows(stateRows)

		mock.ExpectQuery("SELECT c.question").
			WithArgs("state-1").
			WillReturnRows(sqlmock.NewRows([]string{"question", "answer", "resolved", "asked_at"}).
				AddRow("Auth Key?", "secret", 1, now))

		mock.ExpectQuery("SELECT a.timestamp").
			WithArgs("state-1").
			WillReturnRows(sqlmock.NewRows([]string{"timestamp", "tool", "args", "reasoning", "result", "success"}).
				AddRow(now, "grep_search", []byte(`{"pattern":"foo"}`), "Search for pattern", "found", 1))

		mock.ExpectQuery("SELECT wf.path").
			WithArgs("state-1").
			WillReturnRows(sqlmock.NewRows([]string{"path", "size", "last_modified"}).
				AddRow("foo.go", 1024, now))

		mock.ExpectQuery("SELECT vc.id").
			WithArgs("state-1").
			WillReturnRows(sqlmock.NewRows([]string{"id", "type", "expression", "description", "passed", "error_log"}).
				AddRow("crit-1", "COMMAND", "go test", "desc", 1, ""))

		mock.ExpectQuery("SELECT aa.id").
			WithArgs("state-1").
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "role", "status", "task_id", "started_at", "completed_at", "tokens_used", "last_error"}).
				AddRow("agent-1", "Gen", "GENERATOR", "WORKING", "task-1", now, now, 100, ""))

		loaded, err := repo.Load(ctx)
		assert.NoError(t, err)
		require.NotNil(t, loaded)
		assert.Equal(t, "state-1", loaded.ID)
		assert.Equal(t, "/src", loaded.ProjectPath)
		assert.Equal(t, 2, loaded.Version)
		assert.Equal(t, domain.BuildPassing, loaded.BuildStatus)
		assert.Len(t, loaded.Tasks, 1)
		assert.Equal(t, "task-1", loaded.Tasks[0].ID)
		assert.Equal(t, []string{"task-0"}, loaded.Tasks[0].DependsOn)
		assert.Equal(t, []string{"foo.go"}, loaded.Tasks[0].TargetFiles)
		assert.Equal(t, []string{"Changelog"}, loaded.Tasks[0].PartialChangelog)

		assert.Len(t, loaded.Clarifications, 1)
		assert.Equal(t, "Auth Key?", loaded.Clarifications[0].Question)
		assert.Equal(t, "secret", loaded.Clarifications[0].Answer)
		assert.True(t, loaded.Clarifications[0].Resolved)

		assert.Len(t, loaded.LastActions, 1)
		assert.Equal(t, "grep_search", loaded.LastActions[0].Tool)
		assert.Equal(t, "foo", loaded.LastActions[0].Args["pattern"])

		assert.Len(t, loaded.Files, 1)
		assert.Equal(t, "foo.go", loaded.Files[0].Path)

		assert.Len(t, loaded.ValidationCriteria, 1)
		assert.Equal(t, "crit-1", loaded.ValidationCriteria[0].ID)
		assert.Equal(t, domain.ValidationCommand, loaded.ValidationCriteria[0].Type)

		assert.Len(t, loaded.ActiveAgents, 1)
		assert.Equal(t, "agent-1", loaded.ActiveAgents[0].ID)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("when state does not exist, it returns sql.ErrNoRows", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		repo := &PostgresRepository{db: db}

		mock.ExpectQuery("SELECT s.id, s.project_path").WillReturnError(sql.ErrNoRows)

		loaded, err := repo.Load(ctx)
		assert.ErrorIs(t, err, sql.ErrNoRows)
		assert.Nil(t, loaded)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}
