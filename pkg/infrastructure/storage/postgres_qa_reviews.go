package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func (r *PostgresRepository) saveQAReviews(ctx context.Context, tx *sql.Tx, state *domain.State) error {
	for _, table := range []string{"qa_findings", "qa_scenarios", "review_phases", "story_contracts"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE state_id = $1", state.ID); err != nil {
			return err
		}
	}
	for _, contract := range state.StoryContracts {
		payload, err := json.Marshal(contract.PublicContracts)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO story_contracts
			(state_id, story_id, source_path, source_sha256, public_contracts) VALUES ($1, $2, $3, $4, $5)`,
			state.ID, contract.StoryID, contract.SourcePath, contract.SourceSHA256, payload); err != nil {
			return err
		}
	}
	for _, phase := range state.ReviewPhases {
		manifest, err := json.Marshal(phase.ArtifactManifest)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO review_phases
			(id, state_id, story_id, task_id, role, artifact_id, artifact_manifest, attempt, status, terminal_reason, started_at, deadline_at, completed_at, tokens_used, cost_usd)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`, phase.ID, state.ID,
			phase.StoryID, phase.TaskID, phase.Role, phase.ArtifactID, manifest, phase.Attempt, string(phase.Status), phase.TerminalReason,
			phase.StartedAt, phase.DeadlineAt, nullTime(phase.CompletedAt), phase.TokensUsed, phase.CostUSD); err != nil {
			return err
		}
	}
	for _, scenario := range state.QAScenarios {
		steps, err := json.Marshal(scenario.Steps)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO qa_scenarios
			(id, state_id, review_phase_id, public_contract_id, name, fingerprint, steps, status, evidence)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, scenario.ID, state.ID, scenario.ReviewPhaseID,
			scenario.PublicContractID, scenario.Name, scenario.Fingerprint, steps, string(scenario.Status), scenario.Evidence); err != nil {
			return err
		}
	}
	for _, finding := range state.QAFindings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO qa_findings
			(id, state_id, review_phase_id, task_id, artifact_id, scenario_fingerprint, public_contract_id, severity, expected, actual, evidence, disposition)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`, finding.ID, state.ID,
			finding.ReviewPhaseID, finding.TaskID, finding.ArtifactID, finding.ScenarioFingerprint, finding.PublicContractID,
			finding.Severity, finding.Expected, finding.Actual, finding.Evidence, finding.Disposition); err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresRepository) loadQAReviews(ctx context.Context, state *domain.State) error {
	rows, err := r.db.QueryContext(ctx, `SELECT story_id, source_path, source_sha256, public_contracts
		FROM story_contracts WHERE state_id = $1 ORDER BY created_at, story_id`, state.ID)
	if err != nil {
		return err
	}
	state.StoryContracts = []domain.StoryContract{}
	for rows.Next() {
		var contract domain.StoryContract
		var payload []byte
		if err := rows.Scan(&contract.StoryID, &contract.SourcePath, &contract.SourceSHA256, &payload); err != nil {
			_ = rows.Close()
			return err
		}
		if err := json.Unmarshal(payload, &contract.PublicContracts); err != nil {
			_ = rows.Close()
			return err
		}
		state.StoryContracts = append(state.StoryContracts, contract)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return r.loadPostgresReviewResults(ctx, state)
}

func (r *PostgresRepository) loadPostgresReviewResults(ctx context.Context, state *domain.State) error {
	rows, err := r.db.QueryContext(ctx, `SELECT id, story_id, task_id, role, artifact_id, artifact_manifest, attempt, status,
		terminal_reason, started_at, deadline_at, completed_at, tokens_used, cost_usd
		FROM review_phases WHERE state_id = $1 ORDER BY started_at, id`, state.ID)
	if err != nil {
		return err
	}
	state.ReviewPhases = []domain.ReviewPhase{}
	for rows.Next() {
		var phase domain.ReviewPhase
		var completed sql.NullTime
		var manifest []byte
		if err := rows.Scan(&phase.ID, &phase.StoryID, &phase.TaskID, &phase.Role, &phase.ArtifactID, &manifest, &phase.Attempt,
			&phase.Status, &phase.TerminalReason, &phase.StartedAt, &phase.DeadlineAt, &completed, &phase.TokensUsed, &phase.CostUSD); err != nil {
			_ = rows.Close()
			return err
		}
		if err := json.Unmarshal(manifest, &phase.ArtifactManifest); err != nil {
			_ = rows.Close()
			return err
		}
		if completed.Valid {
			phase.CompletedAt = completed.Time
		}
		state.ReviewPhases = append(state.ReviewPhases, phase)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return r.loadPostgresScenariosAndFindings(ctx, state)
}

func (r *PostgresRepository) loadPostgresScenariosAndFindings(ctx context.Context, state *domain.State) error {
	rows, err := r.db.QueryContext(ctx, `SELECT id, review_phase_id, public_contract_id, name, fingerprint, steps, status, evidence
		FROM qa_scenarios WHERE state_id = $1 ORDER BY created_at, id`, state.ID)
	if err != nil {
		return err
	}
	state.QAScenarios = []domain.QAScenario{}
	for rows.Next() {
		var scenario domain.QAScenario
		var steps []byte
		if err := rows.Scan(&scenario.ID, &scenario.ReviewPhaseID, &scenario.PublicContractID, &scenario.Name,
			&scenario.Fingerprint, &steps, &scenario.Status, &scenario.Evidence); err != nil {
			_ = rows.Close()
			return err
		}
		if err := json.Unmarshal(steps, &scenario.Steps); err != nil {
			_ = rows.Close()
			return err
		}
		state.QAScenarios = append(state.QAScenarios, scenario)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = r.db.QueryContext(ctx, `SELECT id, review_phase_id, task_id, artifact_id, scenario_fingerprint,
		public_contract_id, severity, expected, actual, evidence, disposition
		FROM qa_findings WHERE state_id = $1 ORDER BY created_at, id`, state.ID)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	state.QAFindings = []domain.QAFinding{}
	for rows.Next() {
		var finding domain.QAFinding
		if err := rows.Scan(&finding.ID, &finding.ReviewPhaseID, &finding.TaskID, &finding.ArtifactID,
			&finding.ScenarioFingerprint, &finding.PublicContractID, &finding.Severity, &finding.Expected,
			&finding.Actual, &finding.Evidence, &finding.Disposition); err != nil {
			return err
		}
		state.QAFindings = append(state.QAFindings, finding)
	}
	return rows.Err()
}
