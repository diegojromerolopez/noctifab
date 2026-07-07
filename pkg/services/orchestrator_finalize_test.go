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
}
