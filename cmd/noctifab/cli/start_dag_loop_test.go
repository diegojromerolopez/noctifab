package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunStoryIterationLoops_ConcurrentExecution(t *testing.T) {
	t.Run("when story concurrency is enabled, independent stories run concurrently", func(t *testing.T) {
		tempDir := t.TempDir()
		us1 := filepath.Join(tempDir, "US-001.md")
		us2 := filepath.Join(tempDir, "US-002.md")
		us3 := filepath.Join(tempDir, "US-003.md")

		require.NoError(t, os.WriteFile(us1, []byte("# US-001\ndepends_on: []\n"), 0644))
		require.NoError(t, os.WriteFile(us2, []byte("# US-002\ndepends_on: [\"US-001\"]\n"), 0644))
		require.NoError(t, os.WriteFile(us3, []byte("# US-003\ndepends_on: [\"US-001\"]\n"), 0644))

		cfg := config.DefaultConfig()
		cfg.Agents.Orchestrator.Number = 3
		cfg.Runtime.Loops = 1

		var concurrentCount int32
		var maxObservedConcurrent int32
		var mu sync.Mutex
		executed := make(map[string]bool)

		executor := func(ctx context.Context, storyFile string) error {
			current := atomic.AddInt32(&concurrentCount, 1)
			defer atomic.AddInt32(&concurrentCount, -1)

			for {
				old := atomic.LoadInt32(&maxObservedConcurrent)
				if current <= old || atomic.CompareAndSwapInt32(&maxObservedConcurrent, old, current) {
					break
				}
			}

			time.Sleep(30 * time.Millisecond)

			mu.Lock()
			executed[storyFile] = true
			mu.Unlock()
			return nil
		}

		opts := StoryLoopOptions{
			Cfg:          cfg,
			TargetDir:    tempDir,
			StoryFiles:   []string{us1, us2, us3},
			ExecuteStory: executor,
			GitClient:    services.NewGitClient(tempDir),
			TotalLoops:   1,
		}

		outcomes, err := runStoryIterationLoops(context.Background(), opts)
		require.NoError(t, err)
		assert.NoError(t, outcomes[us1])
		assert.NoError(t, outcomes[us2])
		assert.NoError(t, outcomes[us3])

		mu.Lock()
		assert.True(t, executed[us1])
		assert.True(t, executed[us2])
		assert.True(t, executed[us3])
		mu.Unlock()

		// US-002 and US-003 should run concurrently after US-001 completes
		assert.GreaterOrEqual(t, atomic.LoadInt32(&maxObservedConcurrent), int32(2))
	})

	t.Run("when story fails and is retried in loop 2, overall execution passes", func(t *testing.T) {
		tempDir := t.TempDir()
		us1 := filepath.Join(tempDir, "US-001.md")
		require.NoError(t, os.WriteFile(us1, []byte("# US-001\ndepends_on: []\n"), 0644))

		cfg := config.DefaultConfig()
		cfg.Agents.Orchestrator.Number = 2
		cfg.Runtime.Loops = 2

		var attempts int32
		executor := func(ctx context.Context, storyFile string) error {
			att := atomic.AddInt32(&attempts, 1)
			if att == 1 {
				return errors.New("simulated transient failure")
			}
			return nil
		}

		opts := StoryLoopOptions{
			Cfg:          cfg,
			TargetDir:    tempDir,
			StoryFiles:   []string{us1},
			ExecuteStory: executor,
			GitClient:    services.NewGitClient(tempDir),
			TotalLoops:   2,
		}

		outcomes, err := runStoryIterationLoops(context.Background(), opts)
		require.NoError(t, err)
		assert.NoError(t, outcomes[us1])
		assert.Equal(t, int32(2), atomic.LoadInt32(&attempts))
	})
}
