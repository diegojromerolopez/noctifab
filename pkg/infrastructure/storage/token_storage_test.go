package storage_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
)

func TestSQLiteRepositoryTokenPersistence(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test_tokens.db")

	repo, err := storage.NewSQLiteRepository(ctx, dbPath)
	if err != nil {
		t.Fatalf("failed to create sqlite repo: %v", err)
	}
	defer func() { _ = repo.Close() }()

	now := time.Now().UTC()
	st := &domain.State{
		ID:          "state-1",
		ProjectPath: "/tmp/test",
		Version:     0,
		BuildStatus: domain.BuildPassing,
		StoryStatus: domain.StoryRunning,
		Metadata: domain.StateMetadata{
			TotalInputTokens:  15000,
			TotalOutputTokens: 2500,
			TotalTokensUsed:   17500,
		},
		Stories: []domain.Story{
			{
				ID:           "US-001",
				StateID:      "state-1",
				Title:        "Test Story",
				FilePath:     "stories/US-001.md",
				Status:       domain.StorySuccess,
				InputTokens:  10000,
				OutputTokens: 1500,
				TokensUsed:   11500,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
		Tasks: []domain.Task{
			{
				ID:           "US-001-T1",
				Title:        "Task 1",
				Description:  "Desc 1",
				Status:       domain.TaskSuccess,
				ChangeType:   domain.ChangeTypeFeature,
				StoryID:      "US-001",
				InputTokens:  5000,
				OutputTokens: 800,
				TokensUsed:   5800,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
		ActiveAgents: []domain.Agent{
			{
				ID:           "agt-1",
				Name:         "Alpha",
				Role:         domain.AgentRoleGenerator,
				Status:       domain.AgentCompleted,
				InputTokens:  5000,
				OutputTokens: 800,
				TokensUsed:   5800,
			},
		},
	}

	if err := repo.Save(ctx, st); err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	loaded, err := repo.LoadByID(ctx, "state-1")
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}

	if loaded.Metadata.TotalInputTokens != 15000 || loaded.Metadata.TotalOutputTokens != 2500 || loaded.Metadata.TotalTokensUsed != 17500 {
		t.Errorf("metadata tokens mismatch: %+v", loaded.Metadata)
	}

	if len(loaded.Stories) != 1 || loaded.Stories[0].InputTokens != 10000 || loaded.Stories[0].OutputTokens != 1500 || loaded.Stories[0].TokensUsed != 11500 {
		t.Errorf("story tokens mismatch: %+v", loaded.Stories)
	}

	if len(loaded.Tasks) != 1 || loaded.Tasks[0].InputTokens != 5000 || loaded.Tasks[0].OutputTokens != 800 || loaded.Tasks[0].TokensUsed != 5800 {
		t.Errorf("task tokens mismatch: %+v", loaded.Tasks)
	}

	if len(loaded.ActiveAgents) != 1 || loaded.ActiveAgents[0].InputTokens != 5000 || loaded.ActiveAgents[0].OutputTokens != 800 || loaded.ActiveAgents[0].TokensUsed != 5800 {
		t.Errorf("agent tokens mismatch: %+v", loaded.ActiveAgents)
	}
}
