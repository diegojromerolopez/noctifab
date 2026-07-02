package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type PostgresRepository struct {
	db *sql.DB
}

var _ domain.StateRepository = (*PostgresRepository)(nil)

// NewPostgresRepository creates, initializes and runs migrations on a PostgreSQL database.
func NewPostgresRepository(ctx context.Context, dsn string, maxOpen, maxIdle int) (*PostgresRepository, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres database: %w", err)
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)

	if err := Migrate(ctx, db, "postgres"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migration failure: %w", err)
	}

	return &PostgresRepository{db: db}, nil
}

// Close closes the database connection.
func (r *PostgresRepository) Close() error {
	return r.db.Close()
}

// Save persists the domain State in a transaction using SELECT FOR UPDATE row locking and OCC version checking.
func (r *PostgresRepository) Save(ctx context.Context, state *domain.State) error {
	ctx, span := telemetry.Tracer().Start(ctx, "noctifab.postgres_save",
		trace.WithAttributes(
			attribute.String("state.id", state.ID),
			attribute.Int("state.version", state.Version),
			attribute.Int("task_count", len(state.Tasks)),
		))
	defer span.End()
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Fetch current version and lock the row
	var currentVersion int
	err = tx.QueryRowContext(ctx, "SELECT version FROM state WHERE id = $1 FOR UPDATE", state.ID).Scan(&currentVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			currentVersion = 0
		} else {
			return err
		}
	}

	// 2. Perform optimistic concurrency version check
	if state.Version != currentVersion {
		return domain.ErrVersionConflict
	}

	// 3. Increment version and save state updates
	nextVersion := state.Version + 1
	if currentVersion == 0 {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO state (id, project_path, version, build_status, input_source, input_path, integration_branch, feature_name, base_branch, project_version, total_tokens_used, total_cost_usd)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			state.ID, state.ProjectPath, nextVersion, string(state.BuildStatus),
			state.Metadata.InputSource, state.Metadata.InputPath, state.Metadata.IntegrationBranch,
			state.Metadata.FeatureName, state.Metadata.BaseBranch, state.Metadata.ProjectVersion,
			state.Metadata.TotalTokensUsed, state.Metadata.TotalCostUSD,
		)
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE state SET project_path = $1, version = $2, build_status = $3, input_source = $4, input_path = $5, integration_branch = $6, feature_name = $7, base_branch = $8, project_version = $9, total_tokens_used = $10, total_cost_usd = $11
			WHERE id = $12`,
			state.ProjectPath, nextVersion, string(state.BuildStatus),
			state.Metadata.InputSource, state.Metadata.InputPath, state.Metadata.IntegrationBranch,
			state.Metadata.FeatureName, state.Metadata.BaseBranch, state.Metadata.ProjectVersion,
			state.Metadata.TotalTokensUsed, state.Metadata.TotalCostUSD,
			state.ID,
		)
	}
	if err != nil {
		return err
	}

	// 4. Save Tasks
	_, err = tx.ExecContext(ctx, "DELETE FROM tasks WHERE state_id = $1", state.ID)
	if err != nil {
		return err
	}
	for _, task := range state.Tasks {
		dependsOnJSON, err := json.Marshal(task.DependsOn)
		if err != nil {
			return err
		}
		targetFilesJSON, err := json.Marshal(task.TargetFiles)
		if err != nil {
			return err
		}
		partialChangelogJSON, err := json.Marshal(task.PartialChangelog)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO tasks (id, state_id, title, description, status, change_type, assigned_to, depends_on, target_files, partial_changelog, retries, max_retries, failure_log, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
			task.ID, state.ID, task.Title, task.Description, string(task.Status), string(task.ChangeType),
			task.AssignedTo, dependsOnJSON, targetFilesJSON, partialChangelogJSON,
			task.Retries, task.MaxRetries, task.FailureLog, task.CreatedAt, task.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	// 5. Save Clarifications
	_, err = tx.ExecContext(ctx, "DELETE FROM clarifications WHERE state_id = $1", state.ID)
	if err != nil {
		return err
	}
	for _, clar := range state.Clarifications {
		clarID := uuid.New().String()
		_, err = tx.ExecContext(ctx,
			`INSERT INTO clarifications (id, state_id, question, answer, resolved, asked_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			clarID, state.ID, clar.Question, clar.Answer, boolToInt(clar.Resolved), clar.AskedAt,
		)
		if err != nil {
			return err
		}
	}

	// 6. Save Actions
	_, err = tx.ExecContext(ctx, "DELETE FROM actions WHERE state_id = $1", state.ID)
	if err != nil {
		return err
	}
	for _, act := range state.LastActions {
		argsJSON, err := json.Marshal(act.Args)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO actions (state_id, timestamp, tool, args, reasoning, result, success)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			state.ID, act.Timestamp, act.Tool, argsJSON, act.Reasoning, act.Result, boolToInt(act.Success),
		)
		if err != nil {
			return err
		}
	}

	// 7. Save Workspace Files
	_, err = tx.ExecContext(ctx, "DELETE FROM workspace_files WHERE state_id = $1", state.ID)
	if err != nil {
		return err
	}
	for _, file := range state.Files {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO workspace_files (path, state_id, size, last_modified)
			VALUES ($1, $2, $3, $4)`,
			file.Path, state.ID, file.Size, file.LastModified,
		)
		if err != nil {
			return err
		}
	}

	// 8. Save Validation Criteria
	_, err = tx.ExecContext(ctx, "DELETE FROM validation_criteria WHERE state_id = $1", state.ID)
	if err != nil {
		return err
	}
	for _, crit := range state.ValidationCriteria {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO validation_criteria (id, state_id, type, expression, description, passed, error_log)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			crit.ID, state.ID, string(crit.Type), crit.Expression, crit.Description, boolToInt(crit.Passed), crit.ErrorLog,
		)
		if err != nil {
			return err
		}
	}

	// 9. Save Active Agents
	_, err = tx.ExecContext(ctx, "DELETE FROM active_agents WHERE state_id = $1", state.ID)
	if err != nil {
		return err
	}
	for _, agent := range state.ActiveAgents {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO active_agents (id, state_id, name, role, status, task_id, started_at, completed_at, tokens_used, last_error)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			agent.ID, state.ID, agent.Name, string(agent.Role), string(agent.Status), agent.TaskID,
			nullTime(agent.StartedAt), nullTime(agent.CompletedAt), agent.TokensUsed, agent.LastError,
		)
		if err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	state.Version = nextVersion
	return nil
}

// Load retrieves the State domain object from PostgreSQL using explicit joins.
func (r *PostgresRepository) Load(ctx context.Context) (*domain.State, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "noctifab.postgres_load")
	defer span.End()
	rows, err := r.db.QueryContext(ctx,
		`SELECT 
			s.id, s.project_path, s.version, s.build_status, s.input_source, s.input_path, s.integration_branch, s.feature_name, s.base_branch, s.project_version, s.total_tokens_used, s.total_cost_usd,
			t.id, t.title, t.description, t.status, t.change_type, t.assigned_to, t.depends_on, t.target_files, t.partial_changelog, t.retries, t.max_retries, t.failure_log, t.created_at, t.updated_at
		FROM state s
		LEFT JOIN tasks t ON s.id = t.state_id
		LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var state domain.State
	state.Tasks = []domain.Task{}
	var buildStatusStr string
	stateInitialized := false

	for rows.Next() {
		var task domain.Task
		var statusStr, changeTypeStr, dependsOnStr, targetFilesStr, partialChangelogStr sql.NullString
		var titleStr, descStr, assignedToStr sql.NullString
		var retriesNull, maxRetriesNull sql.NullInt64
		var failureLogNull sql.NullString
		var createdAtNull, updatedAtNull sql.NullTime
		var taskID sql.NullString

		err := rows.Scan(
			&state.ID, &state.ProjectPath, &state.Version, &buildStatusStr,
			&state.Metadata.InputSource, &state.Metadata.InputPath, &state.Metadata.IntegrationBranch,
			&state.Metadata.FeatureName, &state.Metadata.BaseBranch, &state.Metadata.ProjectVersion,
			&state.Metadata.TotalTokensUsed, &state.Metadata.TotalCostUSD,
			&taskID, &titleStr, &descStr, &statusStr, &changeTypeStr, &assignedToStr,
			&dependsOnStr, &targetFilesStr, &partialChangelogStr, &retriesNull, &maxRetriesNull,
			&failureLogNull, &createdAtNull, &updatedAtNull,
		)
		if err != nil {
			return nil, err
		}
		if !stateInitialized {
			state.BuildStatus = domain.BuildStatus(buildStatusStr)
			stateInitialized = true
		}

		if taskID.Valid {
			task.ID = taskID.String
			task.Title = titleStr.String
			task.Description = descStr.String
			task.AssignedTo = assignedToStr.String
			task.Status = domain.TaskStatus(statusStr.String)
			task.ChangeType = domain.ChangeType(changeTypeStr.String)
			task.Retries = int(retriesNull.Int64)
			task.MaxRetries = int(maxRetriesNull.Int64)
			if failureLogNull.Valid {
				task.FailureLog = failureLogNull.String
			}
			task.CreatedAt = createdAtNull.Time
			task.UpdatedAt = updatedAtNull.Time

			if dependsOnStr.Valid && dependsOnStr.String != "" {
				if err := json.Unmarshal([]byte(dependsOnStr.String), &task.DependsOn); err != nil {
					return nil, err
				}
			}
			if targetFilesStr.Valid && targetFilesStr.String != "" {
				if err := json.Unmarshal([]byte(targetFilesStr.String), &task.TargetFiles); err != nil {
					return nil, err
				}
			}
			if partialChangelogStr.Valid && partialChangelogStr.String != "" {
				if err := json.Unmarshal([]byte(partialChangelogStr.String), &task.PartialChangelog); err != nil {
					return nil, err
				}
			}
			state.Tasks = append(state.Tasks, task)
		}
	}

	if !stateInitialized {
		return nil, sql.ErrNoRows
	}

	// Load Clarifications using a JOIN
	rowsCl, err := r.db.QueryContext(ctx,
		`SELECT c.question, c.answer, c.resolved, c.asked_at
		FROM state s
		JOIN clarifications c ON s.id = c.state_id
		WHERE s.id = $1`, state.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rowsCl.Close() }()
	state.Clarifications = []domain.Clarification{}
	for rowsCl.Next() {
		var clar domain.Clarification
		var resolvedInt int
		err := rowsCl.Scan(&clar.Question, &clar.Answer, &resolvedInt, &clar.AskedAt)
		if err != nil {
			return nil, err
		}
		clar.Resolved = resolvedInt != 0
		state.Clarifications = append(state.Clarifications, clar)
	}

	// Load Actions using a JOIN
	rowsAc, err := r.db.QueryContext(ctx,
		`SELECT a.timestamp, a.tool, a.args, a.reasoning, a.result, a.success
		FROM state s
		JOIN actions a ON s.id = a.state_id
		WHERE s.id = $1`, state.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rowsAc.Close() }()
	state.LastActions = []domain.Action{}
	for rowsAc.Next() {
		var act domain.Action
		var argsJSON []byte
		var successInt int
		err := rowsAc.Scan(&act.Timestamp, &act.Tool, &argsJSON, &act.Reasoning, &act.Result, &successInt)
		if err != nil {
			return nil, err
		}
		act.Success = successInt != 0
		if len(argsJSON) > 0 {
			if err := json.Unmarshal(argsJSON, &act.Args); err != nil {
				return nil, err
			}
		}
		state.LastActions = append(state.LastActions, act)
	}

	// Load Files using a JOIN
	rowsFi, err := r.db.QueryContext(ctx,
		`SELECT wf.path, wf.size, wf.last_modified
		FROM state s
		JOIN workspace_files wf ON s.id = wf.state_id
		WHERE s.id = $1`, state.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rowsFi.Close() }()
	state.Files = []domain.FileInfo{}
	for rowsFi.Next() {
		var file domain.FileInfo
		err := rowsFi.Scan(&file.Path, &file.Size, &file.LastModified)
		if err != nil {
			return nil, err
		}
		state.Files = append(state.Files, file)
	}

	// Load Validation Criteria using a JOIN
	rowsVc, err := r.db.QueryContext(ctx,
		`SELECT vc.id, vc.type, vc.expression, vc.description, vc.passed, vc.error_log
		FROM state s
		JOIN validation_criteria vc ON s.id = vc.state_id
		WHERE s.id = $1`, state.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rowsVc.Close() }()
	state.ValidationCriteria = []domain.ValidationCriterion{}
	for rowsVc.Next() {
		var crit domain.ValidationCriterion
		var typeStr string
		var passedInt int
		err := rowsVc.Scan(&crit.ID, &typeStr, &crit.Expression, &crit.Description, &passedInt, &crit.ErrorLog)
		if err != nil {
			return nil, err
		}
		crit.Type = domain.ValidationType(typeStr)
		crit.Passed = passedInt != 0
		state.ValidationCriteria = append(state.ValidationCriteria, crit)
	}

	// Load Active Agents using a JOIN
	rowsAa, err := r.db.QueryContext(ctx,
		`SELECT aa.id, aa.name, aa.role, aa.status, aa.task_id, aa.started_at, aa.completed_at, aa.tokens_used, aa.last_error
		FROM state s
		JOIN active_agents aa ON s.id = aa.state_id
		WHERE s.id = $1`, state.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rowsAa.Close() }()
	state.ActiveAgents = []domain.Agent{}
	for rowsAa.Next() {
		var agent domain.Agent
		var roleStr, statusStr string
		var startedAtNull, completedAtNull sql.NullTime
		err := rowsAa.Scan(
			&agent.ID, &agent.Name, &roleStr, &statusStr, &agent.TaskID,
			&startedAtNull, &completedAtNull, &agent.TokensUsed, &agent.LastError,
		)
		if err != nil {
			return nil, err
		}
		agent.Role = domain.AgentRole(roleStr)
		agent.Status = domain.AgentStatus(statusStr)
		if startedAtNull.Valid {
			agent.StartedAt = startedAtNull.Time
		}
		if completedAtNull.Valid {
			agent.CompletedAt = completedAtNull.Time
		}
		state.ActiveAgents = append(state.ActiveAgents, agent)
	}

	return &state, nil
}
