package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// WriteFilesTool implements write_files to atomically write or replace multiple files in a single turn.
type WriteFilesTool struct {
	// SyntaxChecker is the optional post-write syntax validation hook.
	// When nil a NoopSyntaxChecker is used (no external binary dependency).
	SyntaxChecker SyntaxChecker
}

// Name returns the unique tool identifier "write_files".
func (t *WriteFilesTool) Name() string { return "write_files" }

// Description returns the LLM-facing documentation for write_files.
func (t *WriteFilesTool) Description() string {
	return "write_files atomically creates or replaces multiple files in the workspace in a single turn. Arguments: files (map of string file paths to string contents or list of objects with path and content)."
}

type fileEntry struct {
	path    string
	content string
}

func parseWriteFilesArgs(args map[string]any) ([]fileEntry, error) {
	if args == nil {
		return nil, errors.New("missing arguments")
	}

	var entries []fileEntry

	if filesMap, ok := args["files"].(map[string]any); ok {
		for p, cVal := range filesMap {
			c, ok := cVal.(string)
			if !ok {
				return nil, fmt.Errorf("invalid content for file %q: must be string", p)
			}
			entries = append(entries, fileEntry{path: p, content: c})
		}
	} else if filesMapStr, ok := args["files"].(map[string]string); ok {
		for p, c := range filesMapStr {
			entries = append(entries, fileEntry{path: p, content: c})
		}
	} else if filesList, ok := args["files"].([]any); ok {
		for _, item := range filesList {
			if m, ok := item.(map[string]any); ok {
				p, _ := m["path"].(string)
				c, _ := m["content"].(string)
				if p == "" {
					return nil, errors.New("file item missing 'path'")
				}
				entries = append(entries, fileEntry{path: p, content: c})
			} else {
				return nil, errors.New("invalid file item in files list")
			}
		}
	} else if filesListMap, ok := args["files"].([]map[string]string); ok {
		for _, m := range filesListMap {
			p := m["path"]
			c := m["content"]
			if p == "" {
				return nil, errors.New("file item missing 'path'")
			}
			entries = append(entries, fileEntry{path: p, content: c})
		}
	} else {
		for p, cVal := range args {
			if p == "reasoning" || p == "tool" {
				continue
			}
			if c, ok := cVal.(string); ok {
				entries = append(entries, fileEntry{path: p, content: c})
			}
		}
	}

	if len(entries) == 0 {
		return nil, errors.New("no files provided to write_files")
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].path < entries[j].path
	})

	return entries, nil
}

// Execute performs writing of all given files within the sandbox boundary.
func (t *WriteFilesTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	entries, err := parseWriteFilesArgs(args)
	if err != nil {
		return "", err
	}

	var writtenPaths []string
	for _, entry := range entries {
		fullPath, err := resolveSandboxPath(state.ProjectPath, entry.path)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return "", fmt.Errorf("failed creating parent directory for %s: %w", entry.path, err)
		}
		content := normalizeMakefileTabs(entry.path, entry.content)
		perm := determineFilePerm(entry.path)
		if err := os.WriteFile(fullPath, []byte(content), perm); err != nil {
			return "", fmt.Errorf("failed writing file %s: %w", entry.path, err)
		}
		if perm == 0755 {
			_ = os.Chmod(fullPath, 0755)
		}
		if err := syntaxCheckerOrNoop(t.SyntaxChecker).Check(ctx, fullPath); err != nil {
			return "", fmt.Errorf("syntax check failed on %s: %w", entry.path, err)
		}
		writtenPaths = append(writtenPaths, entry.path)
	}

	return fmt.Sprintf("Successfully wrote %d files: %s", len(writtenPaths), strings.Join(writtenPaths, ", ")), nil
}
