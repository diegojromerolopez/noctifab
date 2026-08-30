package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
)

func TestMultiLoopStoryOutcomes(t *testing.T) {
	t.Run("when all stories succeed in loop 1, it completes successfully", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Runtime.Loops = 2

		storyFiles := []string{"roadmap/user-stories/US-001.md", "roadmap/user-stories/US-002.md"}
		storyOutcomes := make(map[string]error)
		for _, sf := range storyFiles {
			storyOutcomes[sf] = errors.New("pending")
		}

		executedPasses := 0
		executor := func(ctx context.Context, storyFile string) error {
			return nil // all succeed
		}

		totalLoops := cfg.Runtime.GetLoops()
		assert.Equal(t, 2, totalLoops)

		for loopIdx := 1; loopIdx <= totalLoops; loopIdx++ {
			executedPasses++
			for _, sf := range storyFiles {
				storyErr := executor(context.Background(), sf)
				storyOutcomes[sf] = storyErr
			}
			allSucceeded := true
			for _, sf := range storyFiles {
				if storyOutcomes[sf] != nil {
					allSucceeded = false
					break
				}
			}
			if allSucceeded {
				break
			}
		}

		assert.Equal(t, 1, executedPasses, "should stop after loop 1 when all stories succeed")
		for _, sf := range storyFiles {
			assert.NoError(t, storyOutcomes[sf])
		}
	})

	t.Run("when a story fails in loop 1 and succeeds in loop 2, overall execution succeeds", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Runtime.Loop.Count = 3

		storyFiles := []string{"roadmap/user-stories/US-001.md", "roadmap/user-stories/US-002.md"}
		storyOutcomes := make(map[string]error)
		for _, sf := range storyFiles {
			storyOutcomes[sf] = errors.New("pending")
		}

		attempts := make(map[string]int)
		executor := func(ctx context.Context, storyFile string) error {
			attempts[storyFile]++
			if storyFile == "roadmap/user-stories/US-001.md" && attempts[storyFile] == 1 {
				return errors.New("task 1 failed: characterization test assertion failed")
			}
			return nil
		}

		totalLoops := cfg.Runtime.GetLoops()
		assert.Equal(t, 3, totalLoops)

		executedPasses := 0
		for loopIdx := 1; loopIdx <= totalLoops; loopIdx++ {
			executedPasses++
			for _, sf := range storyFiles {
				storyErr := executor(context.Background(), sf)
				storyOutcomes[sf] = storyErr
			}
			allSucceeded := true
			for _, sf := range storyFiles {
				if storyOutcomes[sf] != nil {
					allSucceeded = false
					break
				}
			}
			if allSucceeded {
				break
			}
		}

		assert.Equal(t, 2, executedPasses, "should succeed on loop 2 after remediating story 1")
		assert.NoError(t, storyOutcomes["roadmap/user-stories/US-001.md"])
		assert.NoError(t, storyOutcomes["roadmap/user-stories/US-002.md"])
		assert.Equal(t, 2, attempts["roadmap/user-stories/US-001.md"])
		assert.Equal(t, 2, attempts["roadmap/user-stories/US-002.md"])
	})

	t.Run("when a story fails all loops, failure is aggregated and reported", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Runtime.Loops = 2

		storyFiles := []string{"roadmap/user-stories/US-001.md", "roadmap/user-stories/US-002.md"}
		storyOutcomes := make(map[string]error)
		for _, sf := range storyFiles {
			storyOutcomes[sf] = errors.New("pending")
		}

		executor := func(ctx context.Context, storyFile string) error {
			if storyFile == "roadmap/user-stories/US-002.md" {
				return errors.New("persistent build failure")
			}
			return nil
		}

		totalLoops := cfg.Runtime.GetLoops()
		for loopIdx := 1; loopIdx <= totalLoops; loopIdx++ {
			for _, sf := range storyFiles {
				storyErr := executor(context.Background(), sf)
				storyOutcomes[sf] = storyErr
			}
			allSucceeded := true
			for _, sf := range storyFiles {
				if storyOutcomes[sf] != nil {
					allSucceeded = false
					break
				}
			}
			if allSucceeded {
				break
			}
		}

		assert.NoError(t, storyOutcomes["roadmap/user-stories/US-001.md"])
		assert.Error(t, storyOutcomes["roadmap/user-stories/US-002.md"])
		assert.Contains(t, storyOutcomes["roadmap/user-stories/US-002.md"].Error(), "persistent build failure")
	})
}

type mockStateRepo struct {
	state *domain.State
	err   error
}

func (m *mockStateRepo) Load(ctx context.Context) (*domain.State, error) {
	return m.state, m.err
}

func (m *mockStateRepo) LoadByID(ctx context.Context, id string) (*domain.State, error) {
	return m.state, m.err
}

func (m *mockStateRepo) LoadAll(ctx context.Context) ([]*domain.State, error) {
	if m.state != nil {
		return []*domain.State{m.state}, m.err
	}
	return nil, m.err
}

func (m *mockStateRepo) LoadAllSummaries(ctx context.Context) ([]domain.StateSummary, error) {
	return nil, nil
}

func (m *mockStateRepo) Save(ctx context.Context, s *domain.State) error {
	m.state = s
	return nil
}

func (m *mockStateRepo) PruneFinishedStates(ctx context.Context, keepLast int) (int, error) {
	return 0, nil
}

func TestIsStoryCompletedSuccessfully(t *testing.T) {
	t.Run("when repo is nil, it returns false", func(t *testing.T) {
		assert.False(t, isStoryCompletedSuccessfully(context.Background(), nil, "US-001.md", "story-0001", "US-001"))
	})

	t.Run("when repo returns error, it returns false", func(t *testing.T) {
		repo := &mockStateRepo{err: errors.New("db error")}
		assert.False(t, isStoryCompletedSuccessfully(context.Background(), repo, "US-001.md", "story-0001", "US-001"))
	})

	t.Run("when story status in state is StoryRunning, it returns false", func(t *testing.T) {
		repo := &mockStateRepo{
			state: &domain.State{
				Stories: []domain.Story{
					{ID: "US-001", Status: domain.StoryRunning},
				},
			},
		}
		assert.False(t, isStoryCompletedSuccessfully(context.Background(), repo, "US-001.md", "story-0001", "US-001"))
	})

	t.Run("when active story tasks have a failed task, it returns false", func(t *testing.T) {
		repo := &mockStateRepo{
			state: &domain.State{
				Metadata: domain.StateMetadata{
					FeatureName: "US-001",
					InputPath:   "US-001.md",
				},
				StoryStatus: domain.StorySuccess,
				Stories: []domain.Story{
					{ID: "US-001", Status: domain.StorySuccess},
				},
				Tasks: []domain.Task{
					{ID: "t-1", Status: domain.TaskSuccess},
					{ID: "t-2", Status: domain.TaskFailed},
				},
			},
		}
		assert.False(t, isStoryCompletedSuccessfully(context.Background(), repo, "US-001.md", "story-0001", "US-001"))
	})

	t.Run("when story status is StorySuccess and all tasks are TaskSuccess, it returns true", func(t *testing.T) {
		repo := &mockStateRepo{
			state: &domain.State{
				Metadata: domain.StateMetadata{
					FeatureName: "US-001",
					InputPath:   "US-001.md",
				},
				StoryStatus: domain.StorySuccess,
				Stories: []domain.Story{
					{ID: "US-001", Status: domain.StorySuccess},
				},
				Tasks: []domain.Task{
					{ID: "t-1", Status: domain.TaskSuccess},
					{ID: "t-2", Status: domain.TaskSuccess},
				},
			},
		}
		assert.True(t, isStoryCompletedSuccessfully(context.Background(), repo, "US-001.md", "story-0001", "US-001"))
	})
}

func TestComputeFailureSignature(t *testing.T) {
	outcomes := map[string]error{
		"roadmap/user-stories/US-001.md": nil,
		"roadmap/user-stories/US-002.md": errors.New("compiler error"),
		"roadmap/user-stories/US-003.md": errors.New("test timeout"),
	}

	sig1 := computeFailureSignature(outcomes)
	sig2 := computeFailureSignature(outcomes)
	assert.Equal(t, sig1, sig2, "failure signature must be deterministic")
	assert.Contains(t, sig1, "US-002.md:compiler error")
	assert.Contains(t, sig1, "US-003.md:test timeout")
}

func TestStartCmd_LoopsFlag(t *testing.T) {
	flag := startCmd.Flags().Lookup("loops")
	assert.NotNil(t, flag, "startCmd should have a --loops flag")
	assert.Equal(t, "0", flag.DefValue)

	rFlag := resumeCmd.Flags().Lookup("loops")
	assert.NotNil(t, rFlag, "resumeCmd should have a --loops flag")
	assert.Equal(t, "0", flag.DefValue)
}

