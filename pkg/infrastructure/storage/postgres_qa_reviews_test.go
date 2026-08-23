package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresRepositorySaveQAReviews(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &PostgresRepository{db: db}
	state := qaReviewState("postgres-qa")
	state.StoryContracts = state.StoryContracts[:1]
	state.ReviewPhases = state.ReviewPhases[:1]
	state.QAScenarios = state.QAScenarios[:1]

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT version FROM state WHERE id = \$1 FOR UPDATE`).WithArgs(state.ID).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`INSERT INTO state`).WillReturnResult(sqlmock.NewResult(1, 1))
	for _, table := range []string{"tasks", "clarifications", "actions", "workspace_files", "validation_criteria", "active_agents",
		"qa_findings", "qa_scenarios", "review_phases", "story_contracts"} {
		mock.ExpectExec(`DELETE FROM ` + table + ` WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
	}
	contractsJSON, err := json.Marshal(state.StoryContracts[0].PublicContracts)
	require.NoError(t, err)
	mock.ExpectExec(`INSERT INTO story_contracts`).WithArgs(state.ID, state.StoryContracts[0].StoryID,
		state.StoryContracts[0].SourcePath, state.StoryContracts[0].SourceSHA256, contractsJSON).WillReturnResult(sqlmock.NewResult(1, 1))
	phase := state.ReviewPhases[0]
	manifestJSON, err := json.Marshal(phase.ArtifactManifest)
	require.NoError(t, err)
	mock.ExpectExec(`INSERT INTO review_phases`).WithArgs(phase.ID, state.ID, phase.StoryID, phase.TaskID, phase.Role,
		phase.ArtifactID, manifestJSON, phase.Attempt, string(phase.Status), phase.TerminalReason, phase.StartedAt, phase.DeadlineAt,
		phase.CompletedAt, phase.TokensUsed).WillReturnResult(sqlmock.NewResult(1, 1))
	stepsJSON, err := json.Marshal(state.QAScenarios[0].Steps)
	require.NoError(t, err)
	scenario := state.QAScenarios[0]
	mock.ExpectExec(`INSERT INTO qa_scenarios`).WithArgs(scenario.ID, state.ID, scenario.ReviewPhaseID,
		scenario.PublicContractID, scenario.Name, scenario.Fingerprint, stepsJSON, string(scenario.Status), scenario.Evidence).
		WillReturnResult(sqlmock.NewResult(1, 1))
	finding := state.QAFindings[0]
	mock.ExpectExec(`INSERT INTO qa_findings`).WithArgs(finding.ID, state.ID, finding.ReviewPhaseID, finding.TaskID,
		finding.ArtifactID, finding.ScenarioFingerprint, finding.PublicContractID, finding.Severity, finding.Expected,
		finding.Actual, finding.Evidence, finding.Disposition).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`DELETE FROM stories WHERE state_id = \$1`).WithArgs(state.ID).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	err = repo.Save(context.Background(), state)
	assert.NoError(t, err)
	assert.Equal(t, 1, state.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}
