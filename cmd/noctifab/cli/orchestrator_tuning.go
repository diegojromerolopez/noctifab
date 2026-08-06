package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

// effectiveConcurrency clamps the task concurrency to 1 when git worktrees
// are disabled: without per-task worktrees every task shares the same working
// directory, and concurrent `git reset --hard`/`git clean -fd`/checkout
// sequences would corrupt each other's workspaces (only individual git
// commands are serialized, not whole checkout+write+commit sequences).
func effectiveConcurrency(useWorktrees bool, configured int) int {
	if configured <= 0 {
		configured = 1
	}
	if !useWorktrees && configured > 1 {
		fmt.Fprintf(os.Stderr, "⚠ Warning: vcs.use_worktrees is disabled; clamping task concurrency from %d to 1 to avoid shared-workspace corruption.\n", configured)
		return 1
	}
	return configured
}

// statePruner is the narrow repository capability needed for retention.
type statePruner interface {
	PruneFinishedStates(ctx context.Context, keepLast int) (int, error)
}

// pruneRetainedStates applies the storage retention policy at daemon startup:
// terminal (SUCCESS/FAILED) story states beyond the most recent keepLast are
// deleted so a long-running daemon's database does not grow monotonically.
// keepLast == 0 applies the default of 20; a negative value disables pruning.
// Pruning failures are non-fatal (logged only).
func pruneRetainedStates(ctx context.Context, repo statePruner, keepLast int) {
	if keepLast < 0 {
		return
	}
	if keepLast == 0 {
		keepLast = 20
	}
	pruned, err := repo.PruneFinishedStates(ctx, keepLast)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Warning: failed to prune old story states: %v\n", err)
		return
	}
	if pruned > 0 {
		fmt.Printf("Storage retention: pruned %d finished story state(s), keeping the most recent %d.\n", pruned, keepLast)
	}
}

// storyExecInterval returns the tick frequency of the story execution loop,
// honoring story_exec_interval when configured and defaulting to 2s.
func storyExecInterval(cfg *config.Config) time.Duration {
	if cfg != nil && time.Duration(cfg.StoryExecInterval) > 0 {
		return time.Duration(cfg.StoryExecInterval)
	}
	return 2 * time.Second
}
