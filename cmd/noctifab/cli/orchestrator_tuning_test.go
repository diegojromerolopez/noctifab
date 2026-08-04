package cli

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
)

func TestEffectiveConcurrency(t *testing.T) {
	t.Run("when worktrees are enabled it keeps the configured concurrency", func(t *testing.T) {
		assert.Equal(t, 3, effectiveConcurrency(true, 3))
	})

	t.Run("when worktrees are disabled it clamps concurrency to 1", func(t *testing.T) {
		assert.Equal(t, 1, effectiveConcurrency(false, 3))
	})

	t.Run("when worktrees are disabled and concurrency is already 1 it stays 1", func(t *testing.T) {
		assert.Equal(t, 1, effectiveConcurrency(false, 1))
	})

	t.Run("when concurrency is zero or negative it defaults to 1", func(t *testing.T) {
		assert.Equal(t, 1, effectiveConcurrency(true, 0))
		assert.Equal(t, 1, effectiveConcurrency(true, -2))
	})
}

type fakePruner struct {
	calledWith int
	called     bool
	pruned     int
	err        error
}

func (f *fakePruner) PruneFinishedStates(_ context.Context, keepLast int) (int, error) {
	f.called = true
	f.calledWith = keepLast
	return f.pruned, f.err
}

func TestPruneRetainedStates(t *testing.T) {
	t.Run("when keepLast is positive it prunes with that bound", func(t *testing.T) {
		p := &fakePruner{pruned: 3}
		pruneRetainedStates(context.Background(), p, 5)
		assert.True(t, p.called)
		assert.Equal(t, 5, p.calledWith)
	})

	t.Run("when keepLast is zero it applies the default of 20", func(t *testing.T) {
		p := &fakePruner{}
		pruneRetainedStates(context.Background(), p, 0)
		assert.True(t, p.called)
		assert.Equal(t, 20, p.calledWith)
	})

	t.Run("when keepLast is negative pruning is disabled", func(t *testing.T) {
		p := &fakePruner{}
		pruneRetainedStates(context.Background(), p, -1)
		assert.False(t, p.called)
	})

	t.Run("when pruning fails it does not panic and is non-fatal", func(t *testing.T) {
		p := &fakePruner{err: errors.New("db locked")}
		assert.NotPanics(t, func() { pruneRetainedStates(context.Background(), p, 5) })
	})
}

func TestStoryExecInterval(t *testing.T) {
	t.Run("when story_exec_interval is unset it defaults to 2 seconds", func(t *testing.T) {
		assert.Equal(t, 2*time.Second, storyExecInterval(&config.Config{}))
	})

	t.Run("when story_exec_interval is configured it is honored", func(t *testing.T) {
		cfg := &config.Config{StoryExecInterval: config.Duration(10 * time.Second)}
		assert.Equal(t, 10*time.Second, storyExecInterval(cfg))
	})

	t.Run("when the config is nil it defaults to 2 seconds", func(t *testing.T) {
		assert.Equal(t, 2*time.Second, storyExecInterval(nil))
	})

	t.Run("when the default config is loaded it is 2 seconds", func(t *testing.T) {
		assert.Equal(t, 2*time.Second, storyExecInterval(config.DefaultConfig()))
	})
}
