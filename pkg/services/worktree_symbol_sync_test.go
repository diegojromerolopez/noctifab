package services_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractUnresolvedSymbol(t *testing.T) {
	csOut := "src/Engine/Perft.cs(12,44): error CS0103: The name 'MoveGenerator' does not exist in the current context"
	assert.Equal(t, "MoveGenerator", services.ExtractUnresolvedSymbol(csOut))

	goOut := "main.go:15:2: undefined: CalculateStats"
	assert.Equal(t, "CalculateStats", services.ExtractUnresolvedSymbol(goOut))

	rustOut := "error[E0425]: cannot find function `accumulate_sums` in this scope"
	assert.Equal(t, "accumulate_sums", services.ExtractUnresolvedSymbol(rustOut))

	pyOut := "NameError: name 'UserSession' is not defined"
	assert.Equal(t, "UserSession", services.ExtractUnresolvedSymbol(pyOut))
}

func TestAutoLinkSymbolDependencies(t *testing.T) {
	tasks := []domain.Task{
		{
			ID:          "TASK-001",
			Title:       "Create MoveGenerator engine",
			TargetFiles: []string{"src/Dotchess/Engine/MoveGenerator.cs"},
		},
		{
			ID:          "TASK-002",
			Title:       "Implement Perft calculation",
			Description: "Call MoveGenerator to generate legal moves and calculate perft nodes",
			TargetFiles: []string{"src/Dotchess/Engine/Perft.cs"},
		},
	}

	services.AutoLinkSymbolDependencies(tasks)
	assert.Contains(t, tasks[1].DependsOn, "TASK-001")
}

func TestSynchronizeMissingSymbolFromSiblings(t *testing.T) {
	tmpRoot := t.TempDir()
	worktreesDir := filepath.Join(tmpRoot, ".noctifab", "worktrees")
	wt1 := filepath.Join(worktreesDir, "task-1")
	wt2 := filepath.Join(worktreesDir, "task-2")

	err := os.MkdirAll(filepath.Join(wt1, "src"), 0755)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(wt2, "src"), 0755)
	require.NoError(t, err)

	// In wt1, MoveGenerator.cs exists
	err = os.WriteFile(filepath.Join(wt1, "src", "MoveGenerator.cs"), []byte("public class MoveGenerator {}"), 0644)
	require.NoError(t, err)

	// In wt2, MoveGenerator.cs is missing
	synced := services.SynchronizeMissingSymbolFromSiblings(context.Background(), wt2, tmpRoot, "MoveGenerator")
	assert.True(t, synced)

	// Verify wt2 now has MoveGenerator.cs
	content, err := os.ReadFile(filepath.Join(wt2, "src", "MoveGenerator.cs"))
	require.NoError(t, err)
	assert.Equal(t, "public class MoveGenerator {}", string(content))
}
