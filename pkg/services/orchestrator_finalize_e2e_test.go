package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectE2EContainerFiles(t *testing.T) {
	tempDir := t.TempDir()

	composeContent := "version: '3.8'\nservices:\n  test-runner-e2e:\n    image: test\n"
	err := os.WriteFile(filepath.Join(tempDir, "docker-compose.e2e.yml"), []byte(composeContent), 0644)
	require.NoError(t, err)

	dockerfileContent := "FROM alpine\nCMD [\"echo\", \"done\"]\n"
	err = os.WriteFile(filepath.Join(tempDir, "Dockerfile"), []byte(dockerfileContent), 0644)
	require.NoError(t, err)

	matchedFiles, contextStr := collectE2EContainerFiles(tempDir)
	assert.Contains(t, matchedFiles, "docker-compose.e2e.yml")
	assert.Contains(t, matchedFiles, "Dockerfile")
	assert.NotContains(t, matchedFiles, "Makefile")

	assert.Contains(t, contextStr, "test-runner-e2e")
	assert.Contains(t, contextStr, "FROM alpine")
}

func TestQueueStoryRemediationTask_E2EContext(t *testing.T) {
	tempDir := t.TempDir()

	composeContent := "version: '3.8'\nservices:\n  test-runner-e2e:\n    image: python:3.11\n"
	err := os.WriteFile(filepath.Join(tempDir, "docker-compose.e2e.yml"), []byte(composeContent), 0644)
	require.NoError(t, err)

	mockRepo := &mockStateRepo{
		state: &domain.State{
			ProjectPath: tempDir,
			Metadata: domain.StateMetadata{
				InputPath:   "roadmap/user-stories/US-001.md",
				FeatureName: "US-001",
			},
			Tasks: []domain.Task{
				{ID: "US-001-TASK-001", StoryID: "US-001", Status: domain.TaskSuccess},
			},
		},
	}
	orch := &Orchestrator{
		repo: mockRepo,
	}

	qaResult := &StoryQAResult{
		Passed:          false,
		Summary:         "Containerized E2E test suite failed with exit code 1",
		MissingFeatures: []string{"E2E container build or test execution failed"},
	}

	ok := orch.queueStoryRemediationTask(context.Background(), mockRepo.state, qaResult)
	assert.True(t, ok)
	require.Equal(t, 2, len(mockRepo.state.Tasks))

	queuedTask := mockRepo.state.Tasks[1]
	assert.Equal(t, "qa-remediation-us-001-1", queuedTask.ID)
	assert.Contains(t, queuedTask.TargetFiles, "docker-compose.e2e.yml")
	assert.Contains(t, queuedTask.Description, "run_e2e_tests")
	assert.Contains(t, queuedTask.Description, "CONTAINER & E2E HARNESS CONFIGURATION FILES")
	assert.Contains(t, queuedTask.Description, "test-runner-e2e")
}

func TestQueueStoryRemediationTask_SovereignRescueEscalation(t *testing.T) {
	repoDir, _, cleanup := setupTestGitRepo(t)
	defer cleanup()

	mockRepo := &mockStateRepo{
		state: &domain.State{
			ProjectPath: repoDir,
			Metadata: domain.StateMetadata{
				InputPath:   "roadmap/user-stories/US-001.md",
				FeatureName: "US-001",
			},
			Tasks: []domain.Task{
				{ID: "US-001-TASK-001", StoryID: "US-001", Status: domain.TaskSuccess},
				{ID: "qa-remediation-us-001-1", StoryID: "US-001", Status: domain.TaskSuccess},
				{ID: "qa-remediation-us-001-2", StoryID: "US-001", Status: domain.TaskSuccess},
			},
		},
	}

	orch := &Orchestrator{
		repo: mockRepo,
		git:  NewGitClient(repoDir),
		// FallbackAgent nil will make RunFallbackAgent return false, handled gracefully
	}

	qaResult := &StoryQAResult{
		Passed:          false,
		Summary:         "Persistent E2E failure after multiple remediations",
		MissingFeatures: []string{"E2E test suite still failing"},
	}

	// Should detect remediationCount = 2 and escalate to Sovereign Rescue
	ok := orch.queueStoryRemediationTask(context.Background(), mockRepo.state, qaResult)
	// Since no fallback agent is configured, it attempts Sovereign Rescue and returns false
	assert.False(t, ok)
	// No 3rd qa-remediation task should be queued
	assert.Equal(t, 3, len(mockRepo.state.Tasks))
}
