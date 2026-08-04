package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// savingCountRepo wraps mockRepo and counts Save invocations.
type savingCountRepo struct {
	mockRepo
	saves int
}

func (r *savingCountRepo) Save(ctx context.Context, s *domain.State) error {
	r.saves++
	return r.mockRepo.Save(ctx, s)
}

func TestSyncWorkspaceFiles_SkipsSaveWhenUnchanged(t *testing.T) {
	newSyncOrchestrator := func(t *testing.T) (*Orchestrator, *savingCountRepo, *domain.State) {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
		state := &domain.State{ID: "sync-state", ProjectPath: dir}
		repo := &savingCountRepo{mockRepo: mockRepo{state: state}}
		orch := &Orchestrator{repo: repo, git: NewGitClient(dir)}
		return orch, repo, state
	}

	t.Run("when the file index changes, it saves the state", func(t *testing.T) {
		orch, repo, state := newSyncOrchestrator(t)
		if err := orch.syncWorkspaceFiles(context.Background(), state); err != nil {
			t.Fatalf("sync failed: %v", err)
		}
		if repo.saves != 1 {
			t.Errorf("expected 1 save after initial index build, got %d", repo.saves)
		}
		if len(state.Files) == 0 {
			t.Error("expected state.Files to be populated")
		}
	})

	t.Run("when the file index is unchanged, it skips the save", func(t *testing.T) {
		orch, repo, state := newSyncOrchestrator(t)
		if err := orch.syncWorkspaceFiles(context.Background(), state); err != nil {
			t.Fatalf("first sync failed: %v", err)
		}
		savesAfterFirst := repo.saves

		// Re-sync with an unchanged workspace: index equal -> no save.
		current, _ := repo.Load(context.Background())
		if err := orch.syncWorkspaceFiles(context.Background(), current); err != nil {
			t.Fatalf("second sync failed: %v", err)
		}
		if repo.saves != savesAfterFirst {
			t.Errorf("expected no additional save for unchanged index, got %d extra", repo.saves-savesAfterFirst)
		}
	})

	t.Run("when a file is added, it saves the updated index", func(t *testing.T) {
		orch, repo, state := newSyncOrchestrator(t)
		if err := orch.syncWorkspaceFiles(context.Background(), state); err != nil {
			t.Fatalf("first sync failed: %v", err)
		}
		savesAfterFirst := repo.saves

		if err := os.WriteFile(filepath.Join(state.ProjectPath, "extra.go"), []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
		current, _ := repo.Load(context.Background())
		if err := orch.syncWorkspaceFiles(context.Background(), current); err != nil {
			t.Fatalf("second sync failed: %v", err)
		}
		if repo.saves != savesAfterFirst+1 {
			t.Errorf("expected exactly one additional save after adding a file, got %d extra", repo.saves-savesAfterFirst)
		}
	})
}

func TestFileIndexesEqual(t *testing.T) {
	t.Run("when both indexes are empty, they are equal", func(t *testing.T) {
		if !fileIndexesEqual(nil, nil) {
			t.Error("expected nil indexes to be equal")
		}
	})

	t.Run("when lengths differ, they are not equal", func(t *testing.T) {
		if fileIndexesEqual([]domain.FileInfo{{Path: "a"}}, nil) {
			t.Error("expected indexes of different lengths to differ")
		}
	})

	t.Run("when a size differs, they are not equal", func(t *testing.T) {
		a := []domain.FileInfo{{Path: "a", Size: 1}}
		b := []domain.FileInfo{{Path: "a", Size: 2}}
		if fileIndexesEqual(a, b) {
			t.Error("expected differing sizes to be detected")
		}
	})
}
