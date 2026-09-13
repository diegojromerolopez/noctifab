package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockVCSForFinalize struct {
	prTitle      string
	prBody       string
	prHeadBranch string
	prBaseBranch string
	prErr        error
	prCalls      int
}

func (m *mockVCSForFinalize) CreatePullRequest(ctx context.Context, title, body, headBranch, baseBranch string) (string, error) {
	m.prCalls++
	m.prTitle = title
	m.prBody = body
	m.prHeadBranch = headBranch
	m.prBaseBranch = baseBranch
	return "https://github.com/test/repo/pull/1", m.prErr
}

func (m *mockVCSForFinalize) MergePullRequest(ctx context.Context, prID string) error {
	return nil
}

func setupTestGitRepo(t *testing.T) (repoDir string, remoteDir string, cleanup func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "noctifab-finalize-git-*")
	require.NoError(t, err)

	remoteDir = filepath.Join(tempDir, "remote.git")
	repoDir = filepath.Join(tempDir, "repo")

	err = os.MkdirAll(remoteDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(repoDir, 0755)
	require.NoError(t, err)

	runCmd := func(dir string, name string, args ...string) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		// Suppress stderr/stdout unless it fails
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to run %s %v: %v\nOutput: %s", name, args, err, string(out))
		}
	}

	runCmd(remoteDir, "git", "init", "--bare")
	runCmd(repoDir, "git", "init")
	runCmd(repoDir, "git", "config", "user.email", "test@example.com")
	runCmd(repoDir, "git", "config", "user.name", "test")
	runCmd(repoDir, "git", "remote", "add", "origin", remoteDir)

	// Create dummy file for main branch commit
	dummyFile := filepath.Join(repoDir, "README.md")
	err = os.WriteFile(dummyFile, []byte("# Test Repo"), 0644)
	require.NoError(t, err)

	runCmd(repoDir, "git", "add", "README.md")
	runCmd(repoDir, "git", "commit", "-m", "initial commit")
	runCmd(repoDir, "git", "branch", "-M", "main")

	cleanup = func() {
		_ = os.RemoveAll(tempDir)
	}
	return repoDir, remoteDir, cleanup
}

func TestOrchestrator_FinalizeUserStory(t *testing.T) {
	t.Run("when no tasks were completed successfully, it skips PR creation", func(t *testing.T) {
		vcs := &mockVCSForFinalize{}
		orch := &Orchestrator{
			vcsClient: vcs,
		}

		state := &domain.State{
			Metadata: domain.StateMetadata{
				FeatureName: "US-0001",
			},
			Tasks: []domain.Task{
				{ID: "t1", Status: domain.TaskFailed},
			},
		}

		err := orch.FinalizeUserStory(context.Background(), state)
		assert.NoError(t, err)
		assert.Equal(t, 0, vcs.prCalls)
	})

	t.Run("when tasks were completed successfully, it bumps version, commits, pushes, and creates PR", func(t *testing.T) {
		repoDir, _, cleanup := setupTestGitRepo(t)
		defer cleanup()

		// Write initial version file so BumpVersion works
		err := os.WriteFile(filepath.Join(repoDir, "VERSION"), []byte("1.0.0"), 0644)
		require.NoError(t, err)

		vcs := &mockVCSForFinalize{}
		git := NewGitClient(repoDir)
		orch := &Orchestrator{
			vcsClient: vcs,
			git:       git,
			cfg:       OrchestratorConfig{AutoCreatePR: true},
		}

		state := &domain.State{
			ProjectPath: repoDir,
			Metadata: domain.StateMetadata{
				FeatureName:       "US-0001",
				InputPath:         "roadmap/US-0001.md",
				IntegrationBranch: "noctifab/story-us-0001",
				BaseBranch:        "main",
			},
			Tasks: []domain.Task{
				{ID: "t1", Title: "Fix a bug", Status: domain.TaskSuccess, ChangeType: domain.ChangeTypeFix, PartialChangelog: []string{"Fixed DB bug"}},
			},
		}

		// Run git on current thread to checkout integration branch first
		_, err = git.Run(context.Background(), true, "checkout", "-b", "noctifab/story-us-0001")
		require.NoError(t, err)

		err = orch.FinalizeUserStory(context.Background(), state)
		assert.NoError(t, err)

		// Assert version file bumped
		verBytes, err := os.ReadFile(filepath.Join(repoDir, "VERSION"))
		assert.NoError(t, err)
		assert.Equal(t, "1.0.1", strings.TrimSpace(string(verBytes)))

		// Assert changelog file created/updated
		changelogBytes, err := os.ReadFile(filepath.Join(repoDir, "CHANGELOG.md"))
		assert.NoError(t, err)
		assert.Contains(t, string(changelogBytes), "1.0.1")
		assert.Contains(t, string(changelogBytes), "Fixed DB bug")

		// Assert PR was created
		assert.Equal(t, 1, vcs.prCalls)
		assert.Equal(t, "feat: US-0001", vcs.prTitle)
		assert.Contains(t, vcs.prBody, "Automated Pull Request")
		assert.Equal(t, "noctifab/story-us-0001", vcs.prHeadBranch)
		assert.Equal(t, "main", vcs.prBaseBranch)

		// Verify remote received the push by listing remote branches
		remoteGit := NewGitClient(repoDir)
		branches, err := remoteGit.Run(context.Background(), false, "ls-remote", "--heads", "origin")
		assert.NoError(t, err)
		assert.Contains(t, branches, "refs/heads/noctifab/story-us-0001")
	})

	t.Run("when only a subset of tasks succeeded (some pending or failed), it strictly skips PR creation", func(t *testing.T) {
		vcs := &mockVCSForFinalize{}
		orch := &Orchestrator{
			vcsClient: vcs,
		}

		state := &domain.State{
			Metadata: domain.StateMetadata{
				FeatureName: "US-0002",
			},
			Tasks: []domain.Task{
				{ID: "t1", Status: domain.TaskSuccess},
				{ID: "t2", Status: domain.TaskPending},
			},
		}

		err := orch.FinalizeUserStory(context.Background(), state)
		assert.NoError(t, err)
		assert.Equal(t, 0, vcs.prCalls, "PR creation must be skipped if any task is pending or failed")
	})

	t.Run("when all tasks succeeded but whole-project acceptance audit fails, it skips PR creation", func(t *testing.T) {
		repoDir, _, cleanup := setupTestGitRepo(t)
		defer cleanup()

		err := os.WriteFile(filepath.Join(repoDir, "VERSION"), []byte("1.0.0"), 0644)
		require.NoError(t, err)

		vcs := &mockVCSForFinalize{}
		git := NewGitClient(repoDir)
		mockLLM := &mockAuditorLLM{
			response: &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "submit_acceptance_audit",
						Args: map[string]any{
							"passed":  false,
							"summary": "Missing Redis PING, EXPIRE, and KEYS commands from SPEC.md.",
							"gaps":    []any{"PING command missing", "KEYS command missing"},
						},
					},
				},
			},
		}

		orch := &Orchestrator{
			vcsClient:         vcs,
			git:               git,
			cfg:               OrchestratorConfig{AutoCreatePR: true},
			acceptanceAuditor: NewAcceptanceAuditor(mockLLM, nil),
		}

		state := &domain.State{
			ProjectPath: repoDir,
			Metadata: domain.StateMetadata{
				FeatureName:       "US-0003",
				IntegrationBranch: "noctifab/story-us-0003",
				BaseBranch:        "main",
			},
			Tasks: []domain.Task{
				{ID: "t1", Title: "Partial command dispatch", Status: domain.TaskSuccess},
			},
		}

		// Write a SPEC.md
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "SPEC.md"), []byte("# Redis Spec\nPING, GET, SET, KEYS"), 0644))

		err = orch.FinalizeUserStory(context.Background(), state)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "acceptance audit failed")
		assert.Equal(t, 0, vcs.prCalls, "PR creation must be aborted when acceptance audit fails")
	})

	t.Run("shouldAuditStoryCompleteness scopes remediation count per story", func(t *testing.T) {
		orch := &Orchestrator{
			storyQAAuditor: &StoryQAAuditor{},
		}

		// State has 2 remediation tasks for US-001
		state := &domain.State{
			Metadata: domain.StateMetadata{
				InputPath:   "roadmap/user-stories/US-002.md",
				FeatureName: "US-002",
			},
			Tasks: []domain.Task{
				{ID: "qa-remediation-us-001-1", StoryID: "US-001", Status: domain.TaskSuccess},
				{ID: "qa-remediation-us-001-2", StoryID: "US-001", Status: domain.TaskSuccess},
			},
		}

		// For US-002, remediationCount is 0, so it should be eligible for auditing
		assert.True(t, orch.shouldAuditStoryCompleteness(state), "US-002 must be eligible for audit even if US-001 exhausted its 2 remediations")

		// For US-001, remediationCount is 2, so it should not be eligible for auditing
		state.Metadata.InputPath = "roadmap/user-stories/US-001.md"
		state.Metadata.FeatureName = "US-001"
		assert.False(t, orch.shouldAuditStoryCompleteness(state), "US-001 should not be eligible after 2 remediations")
	})

	t.Run("queueStoryRemediationTask correctly assigns story ID for active story", func(t *testing.T) {
		mockRepo := &mockStateRepo{
			state: &domain.State{
				Metadata: domain.StateMetadata{
					InputPath:   "roadmap/user-stories/US-002.md",
					FeatureName: "US-002",
				},
				Tasks: []domain.Task{
					{ID: "US-001-TASK-001", StoryID: "US-001", Status: domain.TaskSuccess},
					{ID: "US-002-TASK-001", StoryID: "US-002", Status: domain.TaskSuccess},
				},
			},
		}
		orch := &Orchestrator{
			repo: mockRepo,
		}

		qaResult := &StoryQAResult{
			Passed:          false,
			Summary:         "Missing sqlite quote repository",
			MissingFeatures: []string{"sqlite_quote_repository.c is missing"},
		}

		ok := orch.queueStoryRemediationTask(context.Background(), mockRepo.state, qaResult)
		assert.True(t, ok)
		assert.Equal(t, 3, len(mockRepo.state.Tasks))
		queuedTask := mockRepo.state.Tasks[2]
		assert.Equal(t, "qa-remediation-us-002-1", queuedTask.ID)
		assert.Equal(t, "US-002", queuedTask.StoryID)
		assert.Equal(t, domain.TaskPending, queuedTask.Status)
	})

	t.Run("when story remediation task resolves all missing features, finalization succeeds", func(t *testing.T) {
		repoDir, _, cleanup := setupTestGitRepo(t)
		defer cleanup()

		err := os.WriteFile(filepath.Join(repoDir, "VERSION"), []byte("1.0.0"), 0644)
		require.NoError(t, err)

		vcs := &mockVCSForFinalize{}
		git := NewGitClient(repoDir)

		// Mock acceptance auditor that initially passed project audit
		mockLLM := &mockAuditorLLM{
			response: &domain.LLMResponse{
				Actions: []domain.LLMAction{
					{
						Tool: "submit_acceptance_audit",
						Args: map[string]any{
							"passed":  true,
							"summary": "All acceptance criteria verified",
						},
					},
				},
			},
		}

		orch := &Orchestrator{
			vcsClient:         vcs,
			git:               git,
			cfg:               OrchestratorConfig{AutoCreatePR: true},
			acceptanceAuditor: NewAcceptanceAuditor(mockLLM, nil),
		}

		state := &domain.State{
			ProjectPath: repoDir,
			Metadata: domain.StateMetadata{
				FeatureName:       "US-0004",
				InputPath:         "roadmap/user-stories/US-0004.md",
				IntegrationBranch: "noctifab/story-us-0004",
				BaseBranch:        "main",
			},
			Tasks: []domain.Task{
				{ID: "t1", Title: "Base feature", Status: domain.TaskSuccess},
				// Remediation task that successfully fixed the missing features:
				{ID: "qa-remediation-us-0004-1", StoryID: "US-0004", Title: "Remediate missing features", Status: domain.TaskSuccess, ChangeType: domain.ChangeTypeFix, PartialChangelog: []string{"Remediated missing feature"}},
			},
		}

		_, err = git.Run(context.Background(), true, "checkout", "-b", "noctifab/story-us-0004")
		require.NoError(t, err)

		err = orch.FinalizeUserStory(context.Background(), state)
		assert.NoError(t, err)
		assert.Equal(t, 1, vcs.prCalls, "PR must be created when remediation task succeeded and resolved all blockers")
		assert.Equal(t, "feat: US-0004", vcs.prTitle)
	})
}
