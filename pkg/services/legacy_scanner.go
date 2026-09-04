package services

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	// MinLegacyFileLineCount is the minimum non-comment, non-blank lines a single file
	// must have to be considered candidate legacy code rather than a stub or empty file.
	MinLegacyFileLineCount = 5

	// MinLegacyTotalLineCount is the minimum total non-comment, non-blank lines across
	// all non-manifest workspace files required to classify a repository as legacy.
	// Below this threshold, the workspace is classified as Greenfield.
	MinLegacyTotalLineCount = 50
)

// ignoredLegacyFiles contains lowercase filenames of project metadata, documentation,
// containers, VCS ignore rules, manifests, and build tooling configs that do not constitute legacy business logic.
var ignoredLegacyFiles = map[string]bool{
	// Documentation and legal metadata
	"spec.md": true, "readme.md": true, "changelog.md": true,
	"license": true, "license.md": true, "license.txt": true,
	"version": true, "contributing.md": true, "authors": true,
	"code_of_conduct.md": true, "noctifab_evaluation_report.md": true,

	// Docker and container orchestration
	"dockerfile": true, "dockerfile.validation": true,
	"docker-compose.yml": true, "docker-compose.yaml": true,
	"docker-compose.e2e.yml": true, "docker-compose.e2e.yaml": true,
	".dockerignore": true,

	// VCS and project meta
	".gitignore": true, ".gitattributes": true, ".editorconfig": true,

	// Dependency manifests & lockfiles
	"go.mod": true, "go.sum": true, "go.work": true, "go.work.sum": true,
	"cargo.toml": true, "cargo.lock": true,
	"package.json": true, "package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true, "bun.lockb": true,
	"requirements.txt": true, "requirements-dev.txt": true, "pipfile": true, "pipfile.lock": true,
	"pyproject.toml": true, "setup.py": true, "setup.cfg": true, "environment.yml": true,
	"gemfile": true, "gemfile.lock": true, "rakefile": true,
	"pom.xml": true, "build.gradle": true, "build.gradle.kts": true, "settings.gradle": true, "settings.gradle.kts": true,
	"makefile": true, "gnumakefile": true, "cmakelists.txt": true, "cmakecache.txt": true,
	"composer.json": true, "composer.lock": true,
	"mix.exs": true, "mix.lock": true,

	// Linters, test runner configs & test helpers
	".rubocop.yml": true, ".rspec": true, "spec_helper.rb": true, "test_helper.rb": true, "conftest.py": true,
	".golangci.yml": true, ".golangci.yaml": true,
	"pytest.ini": true, "tox.ini": true, ".flake8": true, "mypy.ini": true, "ruff.toml": true, ".ruff.toml": true,
	"tsconfig.json": true, "jsconfig.json": true,
	"eslint.config.js": true, ".eslintrc": true, ".eslintrc.json": true, ".eslintrc.js": true,
	".prettierrc": true, ".prettierrc.json": true,
	".travis.yml": true, ".gitlab-ci.yml": true, "azure-pipelines.yml": true, ".pre-commit-config.yaml": true,
}

// IsIgnoredLegacyFile checks if a file name or path matches known metadata, documentation,
// manifests, or configuration files that should not trigger legacy codebase stabilization.
func IsIgnoredLegacyFile(relPath string) bool {
	cleanRel := filepath.ToSlash(filepath.Clean(relPath))
	parts := strings.Split(cleanRel, "/")
	for _, part := range parts {
		// Ignore hidden directories or dotfiles (.github, .vscode, .idea, etc.)
		if strings.HasPrefix(part, ".") && part != "." && part != ".." {
			return true
		}
		// Ignore common build artifact / dependency directories
		lowerPart := strings.ToLower(part)
		if lowerPart == "target" || lowerPart == "node_modules" || lowerPart == "vendor" ||
			lowerPart == "build" || lowerPart == "dist" || lowerPart == "output" ||
			lowerPart == "__pycache__" || lowerPart == "venv" || lowerPart == ".venv" {
			return true
		}
	}

	baseLower := strings.ToLower(filepath.Base(cleanRel))
	if ignoredLegacyFiles[baseLower] {
		return true
	}

	if strings.HasPrefix(baseLower, "dockerfile") || strings.HasPrefix(baseLower, "docker-compose") {
		return true
	}

	if strings.HasPrefix(baseLower, "readme") || strings.HasPrefix(baseLower, "license") ||
		strings.HasPrefix(baseLower, "changelog") || strings.HasPrefix(baseLower, "contributing") {
		return true
	}

	if strings.HasSuffix(baseLower, ".lock") || strings.HasSuffix(baseLower, ".sum") {
		return true
	}

	return false
}

// CountSignificantLines counts non-empty, non-comment lines of code in a source file.
func CountSignificantLines(filePath string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Skip standard single-line and block comment markers across major languages
		if strings.HasPrefix(line, "//") ||
			strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "/*") ||
			strings.HasPrefix(line, "*") ||
			strings.HasPrefix(line, "*/") ||
			strings.HasPrefix(line, "--") ||
			strings.HasPrefix(line, ";") ||
			strings.HasPrefix(line, "%") ||
			strings.HasPrefix(line, "<!--") {
			continue
		}
		count++
	}
	return count, scanner.Err()
}

// ScanLegacyFiles walks projectPath and returns relative paths of existing legacy source files.
// It excludes build artifacts, project metadata, manifests, lockfiles, and stub files (< 5 lines).
// If the total non-manifest significant lines of code across all candidate files is less than
// MinLegacyTotalLineCount (50 lines), the repository is classified as Greenfield and returns an empty slice.
func ScanLegacyFiles(projectPath string) ([]string, error) {
	exclude := []string{"roadmap", "user-stories", "tasks", "output", "dist", ".git", ".noctifab", "target", "node_modules", "vendor", "build", ".venv", "venv", "__pycache__"}
	files, err := ListWorkspaceSourceFiles(context.Background(), projectPath, exclude)
	if err != nil {
		return nil, err
	}

	type fileCandidate struct {
		relPath string
		lines   int
	}

	var candidates []fileCandidate
	totalLines := 0

	for _, rel := range files {
		if IsIgnoredLegacyFile(rel) {
			continue
		}
		fullPath := filepath.Join(projectPath, rel)
		lines, err := CountSignificantLines(fullPath)
		if err != nil || lines < MinLegacyFileLineCount {
			continue
		}
		candidates = append(candidates, fileCandidate{
			relPath: rel,
			lines:   lines,
		})
		totalLines += lines
	}

	// Greenfield classification: if total significant lines of candidate legacy code is below threshold,
	// treat as Greenfield repository without legacy stabilization burden.
	if totalLines < MinLegacyTotalLineCount {
		return nil, nil
	}

	result := make([]string, 0, len(candidates))
	for _, c := range candidates {
		result = append(result, c.relPath)
	}
	sort.Strings(result)
	return result, nil
}

// IsGreenfieldWorkspace evaluates whether the workspace at projectPath should be treated as
// greenfield (no legacy codebase). It returns isGreenfield, the detected legacy files (if any), and any error.
func IsGreenfieldWorkspace(projectPath string) (bool, []string, error) {
	legacyFiles, err := ScanLegacyFiles(projectPath)
	if err != nil {
		return false, nil, err
	}
	return len(legacyFiles) == 0, legacyFiles, nil
}
