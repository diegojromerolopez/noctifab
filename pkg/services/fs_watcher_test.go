package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFSWatcher_DetectsNewUserStory(t *testing.T) {
	tempDir := t.TempDir()
	storiesDir := filepath.Join(tempDir, "roadmap", "user-stories")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))

	var detectedStory string
	onStory := func(storyPath string) {
		detectedStory = storyPath
	}

	watcher := NewFSWatcher(FSWatcherConfig{
		BaseDir:      tempDir,
		Debounce:     10 * time.Millisecond,
		PollInterval: 20 * time.Millisecond,
		OnStory:      onStory,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher.Start(ctx)
	defer watcher.Stop()

	// Initial wait
	time.Sleep(50 * time.Millisecond)

	// Create a new story file
	newStory := filepath.Join(storiesDir, "US-001-test.md")
	require.NoError(t, os.WriteFile(newStory, []byte("# Test Story"), 0644))

	// Allow debounce + poll time
	time.Sleep(150 * time.Millisecond)

	assert.Equal(t, newStory, detectedStory)
}

func TestFSWatcher_StopTerminatesCleanly(t *testing.T) {
	tempDir := t.TempDir()
	watcher := NewFSWatcher(FSWatcherConfig{
		BaseDir:      tempDir,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcher.Start(ctx)
	watcher.Stop()

	// Calling Stop multiple times should be safe
	watcher.Stop()
}
