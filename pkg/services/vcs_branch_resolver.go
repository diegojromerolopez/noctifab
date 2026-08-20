package services

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
)

// GitRunner represents an object capable of executing git commands.
type GitRunner interface {
	Run(ctx context.Context, isWrite bool, args ...string) (string, error)
}

func isNilGitRunner(git GitRunner) bool {
	if git == nil {
		return true
	}
	val := reflect.ValueOf(git)
	if val.Kind() == reflect.Ptr || val.Kind() == reflect.Interface {
		return val.IsNil()
	}
	return false
}

// BranchResolution holds the resolved base branch and integration branch for story execution.
type BranchResolution struct {
	BaseBranch        string
	IntegrationBranch string
	IsExistingBranch  bool
}

// ResolveBranches resolves the base and integration branch for a story execution based on configuration and repo state.
//
// Rules:
//  1. Base branch: if configuredBase is specified and not "auto"/empty, it is used.
//     If "auto" or empty, Noctifab checks if "master" branch exists (local/remote); if so, "master" is chosen, else "main".
//  2. Current branch check: if the currently checked-out git branch is different from "main" and "master",
//     Noctifab automatically uses that current branch as the integration branch.
//  3. Otherwise (on main/master): if configuredBranchName is set, Noctifab uses that branch name (e.g. "noctifab/implementation").
//     Otherwise, it defaults to "<prefix>feature-<storyName>".
func ResolveBranches(ctx context.Context, git GitRunner, configuredBase string, configuredBranchName string, branchPrefix string, storyFeatureName string) BranchResolution {
	res := BranchResolution{}

	// 1. Resolve base branch (master if exists, otherwise main)
	res.BaseBranch = ResolveBaseBranch(ctx, git, configuredBase)

	// 2. Check currently checked-out branch
	if !isNilGitRunner(git) {
		if cur, err := git.Run(ctx, false, "branch", "--show-current"); err == nil {
			curBranch := strings.TrimSpace(cur)
			if curBranch == "" || curBranch == "HEAD" {
				if rev, err := git.Run(ctx, false, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
					curBranch = strings.TrimSpace(rev)
				}
			}
			if curBranch != "" && curBranch != "HEAD" && curBranch != "main" && curBranch != "master" {
				res.IntegrationBranch = curBranch
				res.IsExistingBranch = true
				return res
			}
		}
	}

	// 3. On main/master: determine integration branch
	if configuredBranchName != "" {
		res.IntegrationBranch = configuredBranchName
	} else {
		prefix := branchPrefix
		if prefix == "" {
			prefix = "noctifab/"
		}
		feat := strings.TrimSuffix(filepath.Base(storyFeatureName), filepath.Ext(storyFeatureName))
		res.IntegrationBranch = prefix + "feature-" + feat
	}

	return res
}

// ResolveBaseBranch determines the target base branch for VCS operations.
// If configuredBase is specified (not empty and not "auto"), it is returned verbatim.
// If configuredBase is empty or "auto", it checks whether "master" branch exists locally or in remote.
// Returns "master" if found, otherwise "main".
func ResolveBaseBranch(ctx context.Context, git GitRunner, configuredBase string) string {
	clean := strings.TrimSpace(strings.ToLower(configuredBase))
	if clean != "" && clean != "auto" {
		return configuredBase
	}

	if !isNilGitRunner(git) {
		if _, err := git.Run(ctx, false, "rev-parse", "--verify", "master"); err == nil {
			return "master"
		}
		if _, err := git.Run(ctx, false, "show-ref", "--verify", "--quiet", "refs/heads/master"); err == nil {
			return "master"
		}
		if _, err := git.Run(ctx, false, "show-ref", "--verify", "--quiet", "refs/remotes/origin/master"); err == nil {
			return "master"
		}
	}

	return "main"
}
