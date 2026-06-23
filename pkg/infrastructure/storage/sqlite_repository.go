package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db         *sql.DB
	writeMutex sync.Mutex
}

var _ domain.StateRepository = (*SQLiteRepository)(nil)

// NewSQLiteRepository creates, initializes and runs migrations on a SQLite database.
func NewSQLiteRepository(ctx context.Context, dsn string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Serialized writes by using MaxOpenConns = 1
	db.SetMaxOpenConns(1)

	// Set WAL journal mode and busy timeout via PRAGMA queries
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode = WAL;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set SQLite journal_mode pragma: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000;"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set SQLite busy_timeout pragma: %w", err)
	}

	if err := Migrate(ctx, db, "sqlite"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migration failure: %w", err)
	}

	return &SQLiteRepository{db: db}, nil
}

// Close closes the database connection.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

// Save persists the domain State in a transaction using OCC.
func (r *SQLiteRepository) Save(ctx context.Context, state *domain.State) error {
	r.writeMutex.Lock()
	defer r.writeMutex.Unlock()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 1. Fetch current version to check for conflict
	var currentVersion int
	err = tx.QueryRowContext(ctx, "SELECT version FROM state WHERE id = ?", state.ID).Scan(&currentVersion)
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
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			state.ID, state.ProjectPath, nextVersion, string(state.BuildStatus),
			state.Metadata.InputSource, state.Metadata.InputPath, state.Metadata.IntegrationBranch,
			state.Metadata.FeatureName, state.Metadata.BaseBranch, state.Metadata.ProjectVersion,
			state.Metadata.TotalTokensUsed, state.Metadata.TotalCostUSD,
		)
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE state SET project_path = ?, version = ?, build_status = ?, input_source = ?, input_path = ?, integration_branch = ?, feature_name = ?, base_branch = ?, project_version = ?, total_tokens_used = ?, total_cost_usd = ?
			WHERE id = ?`,
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
	_, err = tx.ExecContext(ctx, "DELETE FROM tasks WHERE state_id = ?", state.ID)
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
			`INSERT INTO tasks (id, state_id, title, description, status, change_type, assigned_to, depends_on, target_files, partial_changelog, retries, max_retries, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			task.ID, state.ID, task.Title, task.Description, string(task.Status), string(task.ChangeType),
			task.AssignedTo, string(dependsOnJSON), string(targetFilesJSON), string(partialChangelogJSON),
			task.Retries, task.MaxRetries, task.CreatedAt, task.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}

	// 5. Save Clarifications
	_, err = tx.ExecContext(ctx, "DELETE FROM clarifications WHERE state_id = ?", state.ID)
	if err != nil {
		return err
	}
	for _, clar := range state.Clarifications {
		clarID := uuid.New().String()
		_, err = tx.ExecContext(ctx,
			`INSERT INTO clarifications (id, state_id, question, answer, resolved, asked_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			clarID, state.ID, clar.Question, clar.Answer, boolToInt(clar.Resolved), clar.AskedAt,
		)
		if err != nil {
			return err
		}
	}

	// 6. Save Actions
	_, err = tx.ExecContext(ctx, "DELETE FROM actions WHERE state_id = ?", state.ID)
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
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			state.ID, act.Timestamp, act.Tool, string(argsJSON), act.Reasoning, act.Result, boolToInt(act.Success),
		)
		if err != nil {
			return err
		}
	}

	// 7. Save Workspace Files
	_, err = tx.ExecContext(ctx, "DELETE FROM workspace_files WHERE state_id = ?", state.ID)
	if err != nil {
		return err
	}
	for _, file := range state.Files {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO workspace_files (path, state_id, size, last_modified)
			VALUES (?, ?, ?, ?)`,
			file.Path, state.ID, file.Size, file.LastModified,
		)
		if err != nil {
			return err
		}
	}

	// 8. Save Validation Criteria
	_, err = tx.ExecContext(ctx, "DELETE FROM validation_criteria WHERE state_id = ?", state.ID)
	if err != nil {
		return err
	}
	for _, crit := range state.ValidationCriteria {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO validation_criteria (id, state_id, type, expression, description, passed, error_log)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			crit.ID, state.ID, string(crit.Type), crit.Expression, crit.Description, boolToInt(crit.Passed), crit.ErrorLog,
		)
		if err != nil {
			return err
		}
	}

	// 9. Save Active Agents
	_, err = tx.ExecContext(ctx, "DELETE FROM active_agents WHERE state_id = ?", state.ID)
	if err != nil {
		return err
	}
	for _, agent := range state.ActiveAgents {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO active_agents (id, state_id, name, role, status, task_id, started_at, completed_at, tokens_used, last_error)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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

// Load retrieves the State domain object from SQLite.
func (r *SQLiteRepository) Load(ctx context.Context) (*domain.State, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, project_path, version, build_status, input_source, input_path, integration_branch, feature_name, base_branch, project_version, total_tokens_used, total_cost_usd
		FROM state LIMIT 1`)

	var state domain.State
	var buildStatusStr string
	err := row.Scan(
		&state.ID, &state.ProjectPath, &state.Version, &buildStatusStr,
		&state.Metadata.InputSource, &state.Metadata.InputPath, &state.Metadata.IntegrationBranch,
		&state.Metadata.FeatureName, &state.Metadata.BaseBranch, &state.Metadata.ProjectVersion,
		&state.Metadata.TotalTokensUsed, &state.Metadata.TotalCostUSD,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, err
	}
	state.BuildStatus = domain.BuildStatus(buildStatusStr)

	// Load Tasks
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, title, description, status, change_type, assigned_to, depends_on, target_files, partial_changelog, retries, max_retries, created_at, updated_at
		FROM tasks WHERE state_id = ?`, state.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	state.Tasks = []domain.Task{}
	for rows.Next() {
		var task domain.Task
		var statusStr, changeTypeStr, dependsOnStr, targetFilesStr, partialChangelogStr string
		err := rows.Scan(
			&task.ID, &task.Title, &task.Description, &statusStr, &changeTypeStr, &task.AssignedTo,
			&dependsOnStr, &targetFilesStr, &partialChangelogStr, &task.Retries, &task.MaxRetries,
			&task.CreatedAt, &task.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		task.Status = domain.TaskStatus(statusStr)
		task.ChangeType = domain.ChangeType(changeTypeStr)

		if err := json.Unmarshal([]byte(dependsOnStr), &task.DependsOn); err != nil {
			return nil, err
		}
		if targetFilesStr != "" {
			if err := json.Unmarshal([]byte(targetFilesStr), &task.TargetFiles); err != nil {
				return nil, err
			}
		}
		if partialChangelogStr != "" {
			if err := json.Unmarshal([]byte(partialChangelogStr), &task.PartialChangelog); err != nil {
				return nil, err
			}
		}
		state.Tasks = append(state.Tasks, task)
	}

	// Load Clarifications
	rowsCl, err := r.db.QueryContext(ctx,
		`SELECT question, answer, resolved, asked_at
		FROM clarifications WHERE state_id = ?`, state.ID)
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

	// Load Actions
	rowsAc, err := r.db.QueryContext(ctx,
		`SELECT timestamp, tool, args, reasoning, result, success
		FROM actions WHERE state_id = ?`, state.ID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rowsAc.Close() }()

	state.LastActions = []domain.Action{}
	for rowsAc.Next() {
		var act domain.Action
		var argsStr string
		var successInt int
		err := rowsAc.Scan(&act.Timestamp, &act.Tool, &argsStr, &act.Reasoning, &act.Result, &successInt)
		if err != nil {
			return nil, err
		}
		act.Success = successInt != 0
		if err := json.Unmarshal([]byte(argsStr), &act.Args); err != nil {
			return nil, err
		}
		state.LastActions = append(state.LastActions, act)
	}

	// Load Files
	rowsFi, err := r.db.QueryContext(ctx,
		`SELECT path, size, last_modified
		FROM workspace_files WHERE state_id = ?`, state.ID)
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

	// Load Validation Criteria
	rowsVc, err := r.db.QueryContext(ctx,
		`SELECT id, type, expression, description, passed, error_log
		FROM validation_criteria WHERE state_id = ?`, state.ID)
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

	// Load Active Agents
	rowsAa, err := r.db.QueryContext(ctx,
		`SELECT id, name, role, status, task_id, started_at, completed_at, tokens_used, last_error
		FROM active_agents WHERE state_id = ?`, state.ID)
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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
