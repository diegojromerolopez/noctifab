package storage

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func qaReviewState(id string) *domain.State {
	started := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	return &domain.State{
		ID:          id,
		ProjectPath: "/workspace",
		BuildStatus: domain.BuildPassing,
		StoryContracts: []domain.StoryContract{{
			StoryID: "US-002", SourcePath: "roadmap/US-002.md", SourceSHA256: "source-b",
			PublicContracts: []domain.PublicContract{{
				ID: "cli.invalid", Interface: "CLI ./dist/app", ApplicablePathPrefixes: []string{"cmd/"},
				AllowedExecutables: []string{"./dist/app"}, ExitCodes: []int{2},
				StdoutContains: []string{"usage"}, StderrPrefixes: []string{"ERROR:"},
			}},
		}, {
			StoryID: "US-001", SourcePath: "roadmap/US-001.md", SourceSHA256: "source-a",
			PublicContracts: []domain.PublicContract{{ID: "cli.ok", Interface: "CLI", AllowedExecutables: []string{"./dist/app"}, ExitCodes: []int{0}}},
		}},
		ReviewPhases: []domain.ReviewPhase{{
			ID: "phase-b", StoryID: "US-002", TaskID: "task-2", Role: "qa", ArtifactID: "commit:b", Attempt: 1,
			ArtifactManifest: []domain.ArtifactManifestEntry{{Path: "dist/app", SHA256: "hash-b"}, {Path: "dist/helper", SHA256: "hash-helper"}},
			Status:           domain.ReviewFindings, TerminalReason: "public_contract_failed", StartedAt: started.Add(time.Minute),
			DeadlineAt: started.Add(3 * time.Minute), CompletedAt: started.Add(2 * time.Minute), TokensUsed: 25,
		}, {
			ID: "phase-a", StoryID: "US-001", TaskID: "task-1", Role: "qa", ArtifactID: "commit:a", Attempt: 1,
			ArtifactManifest: []domain.ArtifactManifestEntry{{Path: "dist/app", SHA256: "hash-a"}},
			Status:           domain.ReviewPass, StartedAt: started, DeadlineAt: started.Add(2 * time.Minute), CompletedAt: started.Add(time.Minute),
		}},
		QAScenarios: []domain.QAScenario{{
			ID: "scenario-b", ReviewPhaseID: "phase-b", PublicContractID: "cli.invalid", Name: "empty", Fingerprint: "fingerprint-b",
			Steps: []domain.QAStep{{Command: []string{"./dist/app", "--input", ""}, Stdin: "input", ExpectedExitCode: 2,
				StdoutContains: []string{"usage"}, StderrPrefix: "ERROR:"}}, Status: domain.ReviewFindings, Evidence: "exit 1",
		}, {
			ID: "scenario-a", ReviewPhaseID: "phase-a", PublicContractID: "cli.ok", Name: "normal", Fingerprint: "fingerprint-a",
			Steps: []domain.QAStep{{Command: []string{"./dist/app"}, ExpectedExitCode: 0}}, Status: domain.ReviewPass,
		}},
		QAFindings: []domain.QAFinding{{
			ID: "finding-b", ReviewPhaseID: "phase-b", TaskID: "task-2", ArtifactID: "commit:b",
			ScenarioFingerprint: "fingerprint-b", PublicContractID: "cli.invalid", Severity: "blocking",
			Expected: "exit 2", Actual: "exit 1", Evidence: "exit 1", Disposition: "OPEN",
		}},
	}
}

func TestSQLiteQAReviewPersistence(t *testing.T) {
	ctx := context.Background()

	t.Run("when saving QA records, it round trips every field in deterministic order", func(t *testing.T) {
		repo := newDirtyTestRepo(t)
		state := qaReviewState("qa-round-trip")
		require.NoError(t, repo.Save(ctx, state))

		loaded, err := repo.LoadByID(ctx, state.ID)
		require.NoError(t, err)
		require.Len(t, loaded.StoryContracts, 2)
		assert.Equal(t, []string{"US-001", "US-002"}, []string{loaded.StoryContracts[0].StoryID, loaded.StoryContracts[1].StoryID})
		assert.Equal(t, []string{"phase-a", "phase-b"}, []string{loaded.ReviewPhases[0].ID, loaded.ReviewPhases[1].ID})
		assert.Equal(t, []string{"scenario-a", "scenario-b"}, []string{loaded.QAScenarios[0].ID, loaded.QAScenarios[1].ID})
		assert.Equal(t, state.StoryContracts[0].PublicContracts[0], loaded.StoryContracts[1].PublicContracts[0])
		assert.Equal(t, state.ReviewPhases[0], loaded.ReviewPhases[1])
		assert.Equal(t, state.QAScenarios[0], loaded.QAScenarios[1])
		assert.Equal(t, state.QAFindings, loaded.QAFindings)
	})

	t.Run("when QA uniqueness is violated, the complete state save rolls back", func(t *testing.T) {
		repo := newDirtyTestRepo(t)
		state := qaReviewState("qa-atomic")
		require.NoError(t, repo.Save(ctx, state))
		committedVersion := state.Version
		state.ProjectPath = "/changed"
		state.QAScenarios = append(state.QAScenarios, domain.QAScenario{
			ID: "scenario-duplicate", ReviewPhaseID: "phase-b", PublicContractID: "cli.invalid",
			Name: "duplicate", Fingerprint: "fingerprint-b", Status: domain.ReviewPass,
		})

		require.Error(t, repo.Save(ctx, state))
		assert.Equal(t, committedVersion, state.Version)
		loaded, err := repo.LoadByID(ctx, state.ID)
		require.NoError(t, err)
		assert.Equal(t, "/workspace", loaded.ProjectPath)
		assert.Equal(t, committedVersion, loaded.Version)
		assert.Len(t, loaded.QAScenarios, 2)
		assert.Len(t, loaded.QAFindings, 1)
	})

	t.Run("when a stale QA snapshot is saved, OCC preserves the committed review", func(t *testing.T) {
		repo := newDirtyTestRepo(t)
		state := qaReviewState("qa-occ")
		require.NoError(t, repo.Save(ctx, state))
		stale := qaReviewState("qa-occ")
		stale.QAFindings = nil

		require.ErrorIs(t, repo.Save(ctx, stale), domain.ErrVersionConflict)
		loaded, err := repo.LoadByID(ctx, state.ID)
		require.NoError(t, err)
		assert.Len(t, loaded.QAFindings, 1)
	})
}

func TestQAReviewSchemaUniqueness(t *testing.T) {
	repo := newDirtyTestRepo(t)
	state := qaReviewState("qa-uniqueness")
	state.QAFindings = append(state.QAFindings, domain.QAFinding{
		ID: "finding-duplicate", ReviewPhaseID: "phase-b", TaskID: "task-2", ArtifactID: "commit:b",
		ScenarioFingerprint: "fingerprint-b", PublicContractID: "cli.invalid", Severity: "blocking", Disposition: "OPEN",
	})
	err := repo.Save(context.Background(), state)
	require.Error(t, err)
	var count int
	require.NoError(t, repo.DB().QueryRow("SELECT COUNT(*) FROM state WHERE id = ?", state.ID).Scan(&count))
	assert.Zero(t, count)
}
