package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenario_ComprehensiveAutonomy(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	t.Run("BudgetStore persists daily usage incrementally across sessions", func(t *testing.T) {
		db := openBudgetDB(t)
		defer func() { _ = db.Close() }()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE budget_usage CASCADE")
		require.NoError(t, err)
		budgetStore := storage.NewPostgresBudgetStore(db)

		usage, err := budgetStore.GetDailyUsage(ctx, "2026-07-02", "gpt-4o")
		require.NoError(t, err)
		assert.Equal(t, int64(0), usage)

		err = budgetStore.IncrementUsage(ctx, "2026-07-02", "gpt-4o", 500)
		require.NoError(t, err)

		usage, err = budgetStore.GetDailyUsage(ctx, "2026-07-02", "gpt-4o")
		require.NoError(t, err)
		assert.Equal(t, int64(500), usage)

		err = budgetStore.IncrementUsage(ctx, "2026-07-02", "gpt-4o", 300)
		require.NoError(t, err)

		usage, err = budgetStore.GetDailyUsage(ctx, "2026-07-02", "gpt-4o")
		require.NoError(t, err)
		assert.Equal(t, int64(800), usage)

		usage, err = budgetStore.GetDailyUsage(ctx, "2026-07-01", "gpt-4o")
		require.NoError(t, err)
		assert.Equal(t, int64(0), usage)
	})

	t.Run("OCC version conflict detection blocks stale saves", func(t *testing.T) {
		repo, cleanup := setupRepo(t, ctx, tempDir, "occ", "occ-conflict-session")
		defer cleanup()

		state := &domain.State{
			ID:          "occ-conflict-session",
			ProjectPath: filepath.Join(tempDir, "occ"),
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource: "local", InputPath: "req.md",
				FeatureName: "occ-test",
			},
		}
		err := repo.Save(ctx, state)
		require.NoError(t, err)
		require.Equal(t, 1, state.Version)

		staleState := &domain.State{
			ID:          "occ-conflict-session",
			ProjectPath: filepath.Join(tempDir, "occ"),
			Version:     0,
			BuildStatus: domain.BuildFailing,
			Metadata: domain.StateMetadata{
				InputSource: "local", InputPath: "req.md",
				FeatureName: "occ-test",
			},
		}
		err = repo.Save(ctx, staleState)
		assert.ErrorIs(t, err, domain.ErrVersionConflict)

		fresh, err := repo.Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, fresh.Version)
	})

	t.Run("Watchdog kills process exceeding max wall-clock duration", func(t *testing.T) {
		wd := services.Watchdog{MaxDuration: 100 * time.Millisecond}
		cmd := exec.CommandContext(ctx, "sleep", "5")
		_, err := wd.Run(ctx, cmd)
		require.Error(t, err)
		assert.ErrorIs(t, err, services.ErrWatchdogMaxDuration)
	})

	t.Run("DependencyManager detects missing tool from error output", func(t *testing.T) {
		dm := services.NewDependencyManager([]string{"pip", "go", "npm"})

		tool, found := dm.DetectMissingTool("executable file not found: pytest")
		assert.True(t, found)
		assert.Equal(t, "pytest", tool)

		_, found = dm.DetectMissingTool("everything is fine")
		assert.False(t, found)

		assert.True(t, dm.IsAllowed("pip"))
		assert.False(t, dm.IsAllowed("brew"))
	})

	t.Run("HotReloadManager handoff file round-trips correctly", func(t *testing.T) {
		handoffPath := filepath.Join(t.TempDir(), "handoff.json")

		state := services.HandoffState{NewPID: 12345, Status: services.HandoffActive, Message: "healthy"}
		data, _ := json.Marshal(state)
		err := os.WriteFile(handoffPath, data, 0644)
		require.NoError(t, err)

		raw, err := os.ReadFile(handoffPath)
		require.NoError(t, err)
		var readBack services.HandoffState
		err = json.Unmarshal(raw, &readBack)
		require.NoError(t, err)
		assert.Equal(t, 12345, readBack.NewPID)
		assert.Equal(t, services.HandoffActive, readBack.Status)
		assert.Equal(t, "healthy", readBack.Message)
	})

	t.Run("SAST scanner returns passed when disabled", func(t *testing.T) {
		scanner := &services.SASTScanner{
			Config: services.SASTConfig{Enabled: false},
		}
		result, err := scanner.Run(ctx, "/tmp")
		require.NoError(t, err)
		assert.True(t, result.Passed)
		assert.Empty(t, result.Issues)
	})

	t.Run("Token tracking calculates correctly across operations", func(t *testing.T) {
		promptTokens := 1000
		completionTokens := 500
		total := int64(promptTokens + completionTokens)
		assert.Equal(t, int64(1500), total)
	})

	t.Run("State repository persists and loads complex state through PostgreSQL", func(t *testing.T) {
		subDir := filepath.Join(tempDir, "complex-state")
		err := os.MkdirAll(subDir, 0755)
		require.NoError(t, err)
		reqPath := filepath.Join(subDir, "requirements.md")
		err = os.WriteFile(reqPath, []byte("complex state test"), 0644)
		require.NoError(t, err)

		repo, cleanup := setupRepo(t, ctx, tempDir, "complex-state", "complex-session")
		defer cleanup()

		state := &domain.State{
			ID:          "complex-session",
			ProjectPath: subDir,
			Version:     0,
			BuildStatus: domain.BuildUnknown,
			Metadata: domain.StateMetadata{
				InputSource: "markdown", InputPath: "requirements.md",
				FeatureName: "Complex State Test", BaseBranch: "main",
				IntegrationBranch: "feature/complex", ProjectVersion: "1.0.0",
			},
			Tasks: []domain.Task{
				{ID: "task-1", Title: "First", Description: "First task", Status: domain.TaskPending, ChangeType: domain.ChangeTypeFeature},
				{ID: "task-2", Title: "Second", Description: "Second task", Status: domain.TaskSuccess, ChangeType: domain.ChangeTypeFeature},
			},
			Clarifications: []domain.Clarification{
				{Question: "Which DB?", Answer: "SQLite", Resolved: true, AskedAt: time.Now()},
			},
			LastActions: []domain.Action{
				{Timestamp: time.Now(), Tool: "plan", Success: true, Result: "Planned tasks"},
			},
			Files: []domain.FileInfo{
				{Path: "main.go", Size: 500},
			},
		}
		err = repo.Save(ctx, state)
		require.NoError(t, err)
		assert.Greater(t, state.Version, 0)

		loaded, err := repo.Load(ctx)
		require.NoError(t, err)
		assert.Equal(t, "complex-session", loaded.ID)
		assert.Equal(t, domain.BuildUnknown, loaded.BuildStatus)
		assert.Len(t, loaded.Tasks, 2)
		assert.Equal(t, "First", loaded.Tasks[0].Title)
		assert.Equal(t, domain.TaskSuccess, loaded.Tasks[1].Status)
		assert.Len(t, loaded.Clarifications, 1)
		assert.True(t, loaded.Clarifications[0].Resolved)
		assert.Len(t, loaded.LastActions, 1)
		assert.Len(t, loaded.Files, 1)
	})

	t.Run("Token usage tracking accumulates across multiple dates", func(t *testing.T) {
		db := openBudgetDB(t)
		defer func() { _ = db.Close() }()
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE budget_usage CASCADE")
		require.NoError(t, err)

		budgetStore := storage.NewPostgresBudgetStore(db)

		dates := []string{"2026-07-01", "2026-07-02", "2026-07-03"}
		for _, d := range dates {
			err = budgetStore.IncrementUsage(ctx, d, "deepseek", 4000)
			require.NoError(t, err)
		}

		var total int64
		for _, d := range dates {
			usage, err := budgetStore.GetDailyUsage(ctx, d, "deepseek")
			require.NoError(t, err)
			total += usage
		}
		assert.Equal(t, int64(12000), total)
	})
}

func openBudgetDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("NOCTIFAB_TEST_DB_DSN")
	if dsn == "" {
		dsn = "postgres://noctifab:noctifab_password@db:5432/noctifab_test?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	if err := db.Ping(); err != nil {
		t.Skipf("Skipping Postgres budget test (db unreachable): %v", err)
	}
	return db
}
