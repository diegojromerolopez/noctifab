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

// --- stubStateRepository for isolated command tests ---

type stubStateRepository struct {
	state *domain.State
}

func (r *stubStateRepository) Load(_ context.Context) (*domain.State, error) {
	if r.state == nil {
		return &domain.State{ID: "test-state", Version: 0}, nil
	}
	return r.state, nil
}

func (r *stubStateRepository) Save(_ context.Context, s *domain.State) error {
	r.state = s
	return nil
}

// --- StartUserStoryCmd tests ---

func TestStartUserStoryCmd_Execute_HappyPath(t *testing.T) {
	t.Run("when the story file exists, it sends a StoryWorkItem to the channel", func(t *testing.T) {
		dir := t.TempDir()
		storyPath := filepath.Join(dir, "US-0001.md")
		require.NoError(t, os.WriteFile(storyPath, []byte("# User Story\n\nAs a user..."), 0644))

		ch := make(chan services.StoryWorkItem, 1)
		cmd := &services.StartUserStoryCmd{Path: storyPath, StoryCh: ch}

		err := cmd.Execute(context.Background(), &stubStateRepository{})
		require.NoError(t, err)

		item := <-ch
		assert.Equal(t, storyPath, item.Path)
		assert.Contains(t, item.Spec, "User Story")
		assert.Contains(t, item.LogPath, "US-0001.log")
		assert.Contains(t, item.LogPath, ".noctifab/logs/roadmap")
	})
}

func TestStartUserStoryCmd_Execute_MissingFile(t *testing.T) {
	t.Run("when the story file does not exist, it returns an error", func(t *testing.T) {
		ch := make(chan services.StoryWorkItem, 1)
		cmd := &services.StartUserStoryCmd{Path: "/nonexistent/US-0099.md", StoryCh: ch}

		err := cmd.Execute(context.Background(), &stubStateRepository{})
		assert.Error(t, err)
		assert.Len(t, ch, 0)
	})
}

// --- StartDirectoryCmd tests ---

func TestStartDirectoryCmd_Execute_HappyPath(t *testing.T) {
	t.Run("when the directory contains markdown files, it enqueues all in sorted order", func(t *testing.T) {
		dir := t.TempDir()
		stories := []string{"US-0003.md", "US-0001.md", "US-0002.md"}
		for _, name := range stories {
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("# "+name), 0644))
		}

		ch := make(chan services.StoryWorkItem, 10)
		cmd := &services.StartDirectoryCmd{DirPath: dir, StoryCh: ch}

		err := cmd.Execute(context.Background(), &stubStateRepository{})
		require.NoError(t, err)

		close(ch)
		var received []string
		var logPaths []string
		for item := range ch {
			received = append(received, filepath.Base(item.Path))
			logPaths = append(logPaths, filepath.Base(item.LogPath))
		}

		assert.Equal(t, []string{"US-0001.md", "US-0002.md", "US-0003.md"}, received)
		assert.Equal(t, []string{"US-0001.log", "US-0002.log", "US-0003.log"}, logPaths)
	})
}

func TestStartDirectoryCmd_Execute_EmptyDirectory(t *testing.T) {
	t.Run("when the directory has no markdown files, it sends nothing and succeeds", func(t *testing.T) {
		dir := t.TempDir()
		ch := make(chan services.StoryWorkItem, 1)
		cmd := &services.StartDirectoryCmd{DirPath: dir, StoryCh: ch}

		err := cmd.Execute(context.Background(), &stubStateRepository{})
		require.NoError(t, err)
		assert.Len(t, ch, 0)
	})
}

func TestStartDirectoryCmd_Execute_MissingDirectory(t *testing.T) {
	t.Run("when the directory does not exist, it returns an error", func(t *testing.T) {
		ch := make(chan services.StoryWorkItem, 1)
		cmd := &services.StartDirectoryCmd{DirPath: "/nonexistent/roadmap", StoryCh: ch}

		err := cmd.Execute(context.Background(), &stubStateRepository{})
		assert.Error(t, err)
	})
}

// --- NewStateForStory tests ---

func TestNewStateForStory(t *testing.T) {
	t.Run("when creating state for a story, it produces the correct integration branch name", func(t *testing.T) {
		state := services.NewStateForStory("/project", "/project/roadmap/US-0001.md", "main", "noctifab/")
		assert.Equal(t, "noctifab/story-us-0001", state.Metadata.IntegrationBranch)
		assert.Equal(t, "main", state.Metadata.BaseBranch)
		assert.Equal(t, "US-0001.md", state.Metadata.FeatureName)
		assert.Equal(t, "/project", state.ProjectPath)
	})

	t.Run("when no branch prefix is given, it defaults to noctifab/story-<slug>", func(t *testing.T) {
		state := services.NewStateForStory("/project", "/project/roadmap/US-0002.md", "main", "")
		assert.Equal(t, "noctifab/story-us-0002", state.Metadata.IntegrationBranch)
	})
}

// --- MarkStoryInterruptedCmd tests ---

func TestMarkStoryInterruptedCmd_Execute(t *testing.T) {
	t.Run("when in-progress tasks exist, it marks them as INTERRUPTED", func(t *testing.T) {
		repo := &stubStateRepository{state: &domain.State{
			ID:      "s1",
			Version: 0,
			Tasks: []domain.Task{
				{ID: "t1", Status: domain.TaskInProgress},
				{ID: "t2", Status: domain.TaskPending},
			},
		}}

		cmd := &services.MarkStoryInterruptedCmd{}
		err := cmd.Execute(context.Background(), repo)
		require.NoError(t, err)

		assert.Equal(t, domain.TaskInterrupted, repo.state.Tasks[0].Status)
		assert.Equal(t, domain.TaskPending, repo.state.Tasks[1].Status)
	})
}
