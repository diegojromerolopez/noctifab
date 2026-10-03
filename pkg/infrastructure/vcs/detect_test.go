package vcs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGitRemoteSlug(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "when ssh url with git suffix, it extracts owner and repo",
			input:    "git@github.com:diegojromerolopez/noctifab.git",
			expected: "diegojromerolopez/noctifab",
		},
		{
			name:     "when https url with git suffix, it extracts owner and repo",
			input:    "https://github.com/diegojromerolopez/noctifab.git",
			expected: "diegojromerolopez/noctifab",
		},
		{
			name:     "when https url without git suffix, it extracts owner and repo",
			input:    "https://github.com/diegojromerolopez/noctifab",
			expected: "diegojromerolopez/noctifab",
		},
		{
			name:     "when https url with trailing slash, it extracts owner and repo",
			input:    "https://github.com/diegojromerolopez/noctifab/",
			expected: "diegojromerolopez/noctifab",
		},
		{
			name:     "when gitlab nested subgroup, it extracts full subgroup path",
			input:    "https://gitlab.com/group/subgroup/project.git",
			expected: "group/subgroup/project",
		},
		{
			name:     "when ssh protocol url, it extracts owner and repo",
			input:    "ssh://git@github.com/myorg/myproject.git",
			expected: "myorg/myproject",
		},
		{
			name:     "when git protocol url, it extracts owner and repo",
			input:    "git://github.com/myorg/myproject.git",
			expected: "myorg/myproject",
		},
		{
			name:     "when input is empty, it returns empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "when input has no repo path, it returns empty string",
			input:    "https://github.com",
			expected: "",
		},
		{
			name:     "when input is single token, it returns empty string",
			input:    "just-a-name",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseGitRemoteSlug(tt.input)
			if got != tt.expected {
				t.Errorf("ParseGitRemoteSlug(%q) = %q, expected %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestDetectGitRepository(t *testing.T) {
	t.Run("when not a git directory, it returns empty string", func(t *testing.T) {
		tmpDir := t.TempDir()
		got := DetectGitRepository(tmpDir)
		if got != "" {
			t.Errorf("expected empty string for non-git dir, got %q", got)
		}
	})

	t.Run("when git directory has config file with origin, it extracts slug", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitDir := filepath.Join(tmpDir, ".git")
		if err := os.MkdirAll(gitDir, 0755); err != nil {
			t.Fatal(err)
		}
		configContent := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
[remote "origin"]
	url = git@github.com:myorg/awesome-repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
`
		if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(configContent), 0644); err != nil {
			t.Fatal(err)
		}

		got := DetectGitRepository(tmpDir)
		if got != "myorg/awesome-repo" {
			t.Errorf("expected 'myorg/awesome-repo', got %q", got)
		}
	})
}
