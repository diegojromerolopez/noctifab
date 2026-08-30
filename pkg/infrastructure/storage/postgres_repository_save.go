package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/google/uuid"
)

// rewriteRelationGroup performs the full DELETE+INSERT rewrite of a single
// relation group within the save transaction.
func (r *PostgresRepository) rewriteRelationGroup(ctx context.Context, tx *sql.Tx, state *domain.State, group string) error {
	switch group {
	case groupStories:
		return r.saveStories(ctx, tx, state)
	case groupTasks:
		return r.saveTasks(ctx, tx, state)
	case groupClarifications:
		return r.saveClarifications(ctx, tx, state)
	case groupActions:
		return r.saveActions(ctx, tx, state)
	case groupWorkspaceFiles:
		return r.saveWorkspaceFiles(ctx, tx, state)
	case groupValidationCriteria:
		return r.saveValidationCriteria(ctx, tx, state)
	case groupActiveAgents:
		return r.saveActiveAgents(ctx, tx, state)
	case groupQAReviews:
		return r.saveQAReviews(ctx, tx, state)
	default:
		return fmt.Errorf("unknown relation group: %s", group)
	}
}

func (r *PostgresRepository) saveStories(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM stories WHERE state_id = $1", state.ID); err != nil {
		return err
	}
	for _, story := range state.Stories {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO stories (id, state_id, title, file_path, status, started_at, completed_at, input_tokens, output_tokens, tokens_used, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			story.ID, state.ID, story.Title, story.FilePath, string(story.Status),
			nullTimePtr(story.StartedAt), nullTimePtr(story.CompletedAt), story.InputTokens, story.OutputTokens, story.TokensUsed, story.CreatedAt, story.UpdatedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepository) saveTasks(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM tasks WHERE state_id = $1", state.ID); err != nil {
		return err
	}
	seen := make(map[string]bool, len(state.Tasks))
	for _, task := range state.Tasks {
		if seen[task.ID] {
			return fmt.Errorf("duplicate task ID in state: %s", task.ID)
		}
		seen[task.ID] = true
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
			`INSERT INTO tasks (id, state_id, title, description, status, change_type, assigned_to, progress, depends_on, target_files, partial_changelog, retries, max_retries, failure_log, created_at, updated_at, story_id, started_at, completed_at, input_tokens, output_tokens, tokens_used)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)`,
			task.ID, state.ID, task.Title, task.Description, string(task.Status), string(task.ChangeType),
			task.AssignedTo, task.Progress, dependsOnJSON, targetFilesJSON, partialChangelogJSON,
			task.Retries, task.MaxRetries, task.FailureLog, task.CreatedAt, task.UpdatedAt,
			task.StoryID, nullTimePtr(task.StartedAt), nullTimePtr(task.CompletedAt), task.InputTokens, task.OutputTokens, task.TokensUsed,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepository) saveClarifications(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM clarifications WHERE state_id = $1", state.ID); err != nil {
		return err
	}
	for _, clar := range state.Clarifications {
		clarID := uuid.New().String()
		_, err := tx.ExecContext(ctx,
			`INSERT INTO clarifications (id, state_id, question, answer, resolved, asked_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			clarID, state.ID, clar.Question, clar.Answer, boolToInt(clar.Resolved), clar.AskedAt,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepository) saveActions(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM actions WHERE state_id = $1", state.ID); err != nil {
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
	return nil
}

func (r *PostgresRepository) saveWorkspaceFiles(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM workspace_files WHERE state_id = $1", state.ID); err != nil {
		return err
	}
	for _, file := range state.Files {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO workspace_files (path, state_id, size, last_modified)
			VALUES ($1, $2, $3, $4)`,
			file.Path, state.ID, file.Size, file.LastModified,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepository) saveValidationCriteria(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM validation_criteria WHERE state_id = $1", state.ID); err != nil {
		return err
	}
	for _, crit := range state.ValidationCriteria {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO validation_criteria (id, state_id, type, expression, description, passed, error_log)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			crit.ID, state.ID, string(crit.Type), crit.Expression, crit.Description, boolToInt(crit.Passed), crit.ErrorLog,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepository) saveActiveAgents(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM active_agents WHERE state_id = $1", state.ID); err != nil {
		return err
	}
	for _, agent := range state.ActiveAgents {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO active_agents (id, state_id, name, role, status, task_id, started_at, completed_at, input_tokens, output_tokens, tokens_used, last_error)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			agent.ID, state.ID, agent.Name, string(agent.Role), string(agent.Status), agent.TaskID,
			nullTime(agent.StartedAt), nullTime(agent.CompletedAt), agent.InputTokens, agent.OutputTokens, agent.TokensUsed, agent.LastError,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
