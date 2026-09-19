package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

const (
	// grepMaxFileSize skips files larger than 1MB.
	grepMaxFileSize = 1 << 20
	// grepMaxMatches caps the number of matched lines returned.
	grepMaxMatches = 200
	// grepMaxLineLength caps the length of each matched line.
	grepMaxLineLength = 500
	// grepTruncationMarker is appended when the match cap is hit.
	grepTruncationMarker = "[grep truncated at 200 matches]"
	// grepBinarySniffLen is how many leading bytes are checked for NUL
	// bytes to detect binary files.
	grepBinarySniffLen = 512
)

// FindFilesTool implements find_files.
type FindFilesTool struct {
	ExcludePaths []string
}

func (t *FindFilesTool) Name() string { return "find_files" }
func (t *FindFilesTool) Description() string {
	return "find_files finds files matching a pattern. Arguments: pattern (string)."
}
func (t *FindFilesTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	pattern, ok := args["pattern"].(string)
	if !ok || strings.TrimSpace(pattern) == "" {
		pattern = "*"
	}

	var matched []string
	err := filepath.WalkDir(state.ProjectPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(state.ProjectPath, path)
		if err != nil {
			return nil
		}
		if rel == "." || rel == ".." {
			return nil
		}
		// Ignore hidden/excluded paths using isPathExcluded
		if isPathExcluded(rel, t.ExcludePaths) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		matchedName, _ := filepath.Match(pattern, d.Name())
		matchedRel, _ := filepath.Match(pattern, rel)
		if matchedName || matchedRel {
			matched = append(matched, rel)
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	return strings.Join(matched, "\n"), nil
}

// isBinaryContent reports whether content looks binary by checking the first
// 512 bytes for a NUL byte.
func isBinaryContent(content []byte) bool {
	sniff := content
	if len(sniff) > grepBinarySniffLen {
		sniff = sniff[:grepBinarySniffLen]
	}
	return bytes.IndexByte(sniff, 0) != -1
}

// GrepSearchTool implements grep_search. To bound memory and output size it
// skips files larger than 1MB, skips binary files (NUL byte in the first 512
// bytes), caps results at 200 matched lines, and caps each matched line at
// 500 characters.
type GrepSearchTool struct {
	ExcludePaths []string
}

func (t *GrepSearchTool) Name() string { return "grep_search" }
func (t *GrepSearchTool) Description() string {
	return "grep_search performs regex query searches over file contents. Arguments: query (string), path (optional, string)."
}
func (t *GrepSearchTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return "", errors.New("missing or invalid 'query' argument")
	}
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}

	fullPath, err := resolveSandboxPath(state.ProjectPath, path)
	if err != nil {
		return "", err
	}

	re, err := regexp.Compile(query)
	if err != nil {
		return "", fmt.Errorf("invalid regex query: %w", err)
	}

	var matchedLines []string
	truncated := false
	err = filepath.WalkDir(fullPath, func(fPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if truncated {
			return filepath.SkipAll
		}
		rel, _ := filepath.Rel(state.ProjectPath, fPath)
		if d.IsDir() {
			if isPathExcluded(rel, t.ExcludePaths) {
				return filepath.SkipDir
			}
			return nil
		}
		if isPathExcluded(rel, t.ExcludePaths) {
			return nil
		}

		if info, infoErr := d.Info(); infoErr == nil && info.Size() > grepMaxFileSize {
			return nil
		}

		contentBytes, err := os.ReadFile(fPath)
		if err != nil {
			return nil
		}
		if isBinaryContent(contentBytes) {
			return nil
		}
		relPath, _ := filepath.Rel(state.ProjectPath, fPath)

		lines := strings.Split(string(contentBytes), "\n")
		for idx, line := range lines {
			if !re.MatchString(line) {
				continue
			}
			if len(matchedLines) >= grepMaxMatches {
				matchedLines = append(matchedLines, grepTruncationMarker)
				truncated = true
				return filepath.SkipAll
			}
			if len(line) > grepMaxLineLength {
				line = line[:grepMaxLineLength]
			}
			matchedLines = append(matchedLines, fmt.Sprintf("%s:%d: %s", relPath, idx+1, line))
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	return strings.Join(matchedLines, "\n"), nil
}
