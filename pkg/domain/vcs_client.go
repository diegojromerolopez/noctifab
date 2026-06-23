package domain

import "context"

// VCSClient defines remote pull/merge request automation interfaces
type VCSClient interface {
	// CreatePullRequest submits a remote PR and returns the PR identifier/URL
	CreatePullRequest(ctx context.Context, title, body, headBranch, baseBranch string) (string, error)

	// MergePullRequest automatically merges the approved pull request
	MergePullRequest(ctx context.Context, prID string) error
}
