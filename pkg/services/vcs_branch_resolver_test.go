package services

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type mockGitRunner struct {
	currentBranch string
	hasMaster     bool
}

func (m *mockGitRunner) Run(ctx context.Context, isWrite bool, args ...string) (string, error) {
	cmd := strings.Join(args, " ")
	if strings.HasPrefix(cmd, "branch --show-current") {
		if m.currentBranch != "" {
			return m.currentBranch, nil
		}
		return "", errors.New("no current branch")
	}
	if strings.HasPrefix(cmd, "rev-parse --abbrev-ref HEAD") {
		if m.currentBranch != "" {
			return m.currentBranch, nil
		}
		return "HEAD", nil
	}
	if strings.Contains(cmd, "master") {
		if m.hasMaster {
			return "refs/heads/master", nil
		}
		return "", errors.New("master branch not found")
	}
	return "", errors.New("unknown command")
}

func TestResolveBaseBranch(t *testing.T) {
	ctx := context.Background()

	// 1. Explicit base branch
	if got := ResolveBaseBranch(ctx, &mockGitRunner{hasMaster: false}, "main"); got != "main" {
		t.Errorf("expected 'main', got %q", got)
	}
	if got := ResolveBaseBranch(ctx, &mockGitRunner{hasMaster: true}, "develop"); got != "develop" {
		t.Errorf("expected 'develop', got %q", got)
	}

	// 2. Auto-detection when master exists
	if got := ResolveBaseBranch(ctx, &mockGitRunner{hasMaster: true}, "auto"); got != "master" {
		t.Errorf("expected 'master', got %q", got)
	}

	// 3. Auto-detection when master does not exist -> main fallback
	if got := ResolveBaseBranch(ctx, &mockGitRunner{hasMaster: false}, "auto"); got != "main" {
		t.Errorf("expected 'main', got %q", got)
	}
	if got := ResolveBaseBranch(ctx, &mockGitRunner{hasMaster: false}, ""); got != "main" {
		t.Errorf("expected 'main', got %q", got)
	}
}

func TestResolveBranches(t *testing.T) {
	ctx := context.Background()

	// Scenario A: Currently on a custom feature branch (e.g. "feature/custom-task")
	gitCustom := &mockGitRunner{currentBranch: "feature/custom-task", hasMaster: true}
	res := ResolveBranches(ctx, gitCustom, "auto", "", "noctifab/", "US-001.md")
	if res.BaseBranch != "master" {
		t.Errorf("expected BaseBranch 'master', got %q", res.BaseBranch)
	}
	if res.IntegrationBranch != "feature/custom-task" {
		t.Errorf("expected IntegrationBranch 'feature/custom-task', got %q", res.IntegrationBranch)
	}
	if !res.IsExistingBranch {
		t.Error("expected IsExistingBranch true when on custom branch")
	}

	// Scenario B: Currently on main, configured branch_name is set to "noctifab/implementation"
	gitMain := &mockGitRunner{currentBranch: "main", hasMaster: false}
	resB := ResolveBranches(ctx, gitMain, "auto", "noctifab/implementation", "noctifab/", "US-001.md")
	if resB.BaseBranch != "main" {
		t.Errorf("expected BaseBranch 'main', got %q", resB.BaseBranch)
	}
	if resB.IntegrationBranch != "noctifab/implementation" {
		t.Errorf("expected IntegrationBranch 'noctifab/implementation', got %q", resB.IntegrationBranch)
	}
	if resB.IsExistingBranch {
		t.Error("expected IsExistingBranch false when on main")
	}

	// Scenario C: Currently on master, no custom branch_name set -> default story feature branch
	gitMaster := &mockGitRunner{currentBranch: "master", hasMaster: true}
	resC := ResolveBranches(ctx, gitMaster, "auto", "", "noctifab/", "US-001.md")
	if resC.BaseBranch != "master" {
		t.Errorf("expected BaseBranch 'master', got %q", resC.BaseBranch)
	}
	if resC.IntegrationBranch != "noctifab/feature-US-001" {
		t.Errorf("expected IntegrationBranch 'noctifab/feature-US-001', got %q", resC.IntegrationBranch)
	}
}
