package vcs

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ParseGitRemoteSlug parses a Git remote URL and extracts the repository slug (e.g. "owner/repo").
func ParseGitRemoteSlug(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}

	// Strip trailing .git and trailing slash
	trimmed = strings.TrimSuffix(trimmed, "/")
	trimmed = strings.TrimSuffix(trimmed, ".git")

	var pathPart string
	if protoIdx := strings.Index(trimmed, "://"); protoIdx != -1 {
		// URL format: https://host/path or ssh://git@host/path
		urlWithoutProto := trimmed[protoIdx+3:]
		firstSlash := strings.Index(urlWithoutProto, "/")
		if firstSlash != -1 {
			pathPart = urlWithoutProto[firstSlash+1:]
		}
	} else if colonIdx := strings.Index(trimmed, ":"); colonIdx != -1 {
		// SCP-like format: git@github.com:owner/repo
		pathPart = trimmed[colonIdx+1:]
	} else {
		// Fallback: path without protocol or colon
		pathPart = trimmed
	}

	pathPart = strings.TrimPrefix(pathPart, "/")
	pathPart = strings.TrimSuffix(pathPart, "/")

	// Must contain at least one slash (owner/repo)
	parts := strings.Split(pathPart, "/")
	if len(parts) < 2 {
		return ""
	}

	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return ""
		}
	}

	return strings.Join(parts, "/")
}

// DetectGitRepository inspects the given directory for a Git repository remote origin.
// Returns the repository slug (e.g. "owner/repo") if detected, or an empty string.
func DetectGitRepository(repoDir string) string {
	if repoDir == "" {
		repoDir = "."
	}

	gitPath := filepath.Join(repoDir, ".git")
	if fi, err := os.Stat(gitPath); err != nil || (!fi.IsDir() && !fi.Mode().IsRegular()) {
		return ""
	}

	// 1. Try running `git config --get remote.origin.url`
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "config", "--get", "remote.origin.url")
	if out, err := cmd.Output(); err == nil {
		slug := ParseGitRemoteSlug(string(out))
		if slug != "" {
			return slug
		}
	}

	// 2. Fallback: Parse .git/config directly if it's a directory
	configPath := filepath.Join(gitPath, "config")
	if data, err := os.Open(configPath); err == nil {
		defer func() { _ = data.Close() }()
		scanner := bufio.NewScanner(data)
		inOrigin := false
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "[") {
				inOrigin = strings.EqualFold(line, `[remote "origin"]`)
				continue
			}
			if inOrigin && strings.HasPrefix(strings.ToLower(line), "url") {
				eqIdx := strings.Index(line, "=")
				if eqIdx != -1 {
					raw := strings.TrimSpace(line[eqIdx+1:])
					slug := ParseGitRemoteSlug(raw)
					if slug != "" {
						return slug
					}
				}
			}
		}
	}

	return ""
}
