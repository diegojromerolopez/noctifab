package services

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var defaultExcludedDirs = map[string]bool{
	".git":          true,
	".noctifab":     true,
	"target":        true,
	"node_modules":  true,
	"__pycache__":   true,
	".venv":         true,
	"venv":          true,
	"dist":          true,
	"bin":           true,
	"build":         true,
	".bundle":       true,
	".gradle":       true,
	".cargo":        true,
	".pytest_cache": true,
	".coverage":     true,
	".idea":         true,
	".vscode":       true,
}

var defaultExcludedExts = map[string]bool{
	".pyc":   true,
	".pyo":   true,
	".pyd":   true,
	".class": true,
	".o":     true,
	".a":     true,
	".so":    true,
	".dylib": true,
	".dll":   true,
	".exe":   true,
	".log":   true,
}

// IsPathExcluded evaluates whether a relative path is excluded by system rules or configured patterns.
func IsPathExcluded(relPath string, excludePaths []string) bool {
	cleanRel := filepath.Clean(relPath)
	if cleanRel == "." || cleanRel == "" {
		return false
	}
	slashRel := filepath.ToSlash(cleanRel)
	parts := strings.Split(slashRel, "/")
	for _, p := range parts {
		if defaultExcludedDirs[p] {
			return true
		}
	}

	ext := strings.ToLower(filepath.Ext(slashRel))
	if defaultExcludedExts[ext] {
		return true
	}

	for _, exp := range excludePaths {
		expTrim := strings.TrimSpace(exp)
		if expTrim == "" {
			continue
		}
		cleanExp := filepath.Clean(expTrim)
		slashExp := filepath.ToSlash(cleanExp)

		// Exact match or prefix directory match
		if slashRel == slashExp || strings.HasPrefix(slashRel, slashExp+"/") {
			return true
		}

		// Base name or pattern match
		for _, part := range parts {
			if matched, _ := filepath.Match(cleanExp, part); matched {
				return true
			}
		}

		if matched, _ := filepath.Match(slashExp, slashRel); matched {
			return true
		}
	}
	return false
}

// IsTextFile checks if a file contains readable text (non-binary) by inspecting the initial byte stream.
func IsTextFile(fullPath string) bool {
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() || info.Size() == 0 {
		return false
	}
	// Skip files larger than 1MB
	if info.Size() > 1024*1024 {
		return false
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return false
	}
	defer func() {
		_ = f.Close()
	}()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return false
	}

	// Any null byte in the first 512 bytes signifies binary content
	return bytes.IndexByte(buf[:n], 0) == -1
}

// IsGitIgnored checks if git considers a relative path ignored.
func IsGitIgnored(ctx context.Context, projectPath, relPath string) bool {
	if projectPath == "" || relPath == "" {
		return false
	}
	cmd := exec.CommandContext(ctx, "git", "check-ignore", "-q", relPath)
	cmd.Dir = projectPath
	return cmd.Run() == nil
}

// ListWorkspaceSourceFiles returns relative paths to all tracked, non-ignored, text source files in the project.
func ListWorkspaceSourceFiles(ctx context.Context, projectPath string, excludePaths []string) ([]string, error) {
	if projectPath == "" {
		return nil, nil
	}

	// 1. Try git ls-files (fast, honors .gitignore, global gitignore, and nested ignore rules)
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-c", "-o", "--exclude-standard")
	cmd.Dir = projectPath
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		lines := strings.Split(out.String(), "\n")
		var result []string
		for _, line := range lines {
			rel := strings.TrimSpace(line)
			if rel == "" || IsPathExcluded(rel, excludePaths) {
				continue
			}
			fullPath := filepath.Join(projectPath, rel)
			if IsTextFile(fullPath) {
				result = append(result, rel)
			}
		}
		return result, nil
	}

	// 2. Fallback: walk filesystem respecting excludePaths and text file invariants
	var result []string
	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, wErr error) error {
		if wErr != nil || info == nil {
			return nil
		}
		rel, rErr := filepath.Rel(projectPath, path)
		if rErr != nil || rel == "." {
			return nil
		}

		if info.IsDir() {
			if IsPathExcluded(rel, excludePaths) || IsGitIgnored(ctx, projectPath, rel) {
				return filepath.SkipDir
			}
			return nil
		}

		if IsPathExcluded(rel, excludePaths) || IsGitIgnored(ctx, projectPath, rel) {
			return nil
		}

		if IsTextFile(path) {
			result = append(result, rel)
		}
		return nil
	})

	return result, err
}

// CollectWorkspaceSourceSnapshot builds a capped text summary of all source files for LLM prompt context.
func CollectWorkspaceSourceSnapshot(ctx context.Context, projectPath string, excludePaths []string, maxFiles, maxCharPerFile int) string {
	files, err := ListWorkspaceSourceFiles(ctx, projectPath, excludePaths)
	if err != nil || len(files) == 0 {
		return ""
	}

	if maxFiles <= 0 {
		maxFiles = 50
	}
	if maxCharPerFile <= 0 {
		maxCharPerFile = 3000
	}

	var sb strings.Builder
	count := 0
	for _, rel := range files {
		if count >= maxFiles {
			break
		}
		fullPath := filepath.Join(projectPath, rel)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		fmt.Fprintf(&sb, "--- File: %s ---\n", rel)
		sb.WriteString(capText(string(content), maxCharPerFile))
		sb.WriteString("\n\n")
		count++
	}

	return sb.String()
}
