package services

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CriticalGitignoreRules defines the minimum set of ignore patterns required
// to prevent build artifact explosion, dependency indexing, and repository pollution.
var CriticalGitignoreRules = []string{
	".noctifab/",
	"target/",
	"node_modules/",
	"dist/",
	"build/",
	"bin/",
	"__pycache__/",
	"*.py[cod]",
	".venv/",
	"venv/",
	".bundle/",
	"*.class",
	"*.o",
	"*.so",
	"*.dylib",
	"*.log",
}

// DefaultGitignoreContent returns standard gitignore template for new or empty repositories.
func DefaultGitignoreContent() string {
	var sb strings.Builder
	sb.WriteString("# Build and binary artifacts\n")
	sb.WriteString("target/\n")
	sb.WriteString("dist/\n")
	sb.WriteString("build/\n")
	sb.WriteString("bin/\n")
	sb.WriteString("*.class\n")
	sb.WriteString("*.o\n")
	sb.WriteString("*.a\n")
	sb.WriteString("*.so\n")
	sb.WriteString("*.dylib\n")
	sb.WriteString("*.dll\n")
	sb.WriteString("*.exe\n\n")

	sb.WriteString("# Dependencies and package managers\n")
	sb.WriteString("node_modules/\n")
	sb.WriteString("__pycache__/\n")
	sb.WriteString("*.py[cod]\n")
	sb.WriteString(".venv/\n")
	sb.WriteString("venv/\n")
	sb.WriteString(".bundle/\n")
	sb.WriteString(".gradle/\n\n")

	sb.WriteString("# Testing, coverage and logs\n")
	sb.WriteString(".coverage\n")
	sb.WriteString(".pytest_cache/\n")
	sb.WriteString("*.log\n\n")

	sb.WriteString("# Noctifab metadata\n")
	sb.WriteString(".noctifab/\n")
	return sb.String()
}

// EnsureProjectGitignore verifies that a project has a .gitignore file containing
// essential build artifact and dependency ignore rules. If missing, it creates one;
// if present, it non-destructively appends missing critical rules.
func EnsureProjectGitignore(projectPath string) error {
	if projectPath == "" {
		projectPath = "."
	}
	gitignorePath := filepath.Join(projectPath, ".gitignore")

	data, err := os.ReadFile(gitignorePath)
	if os.IsNotExist(err) {
		if writeErr := os.WriteFile(gitignorePath, []byte(DefaultGitignoreContent()), 0o644); writeErr != nil {
			return fmt.Errorf("failed to create default .gitignore: %w", writeErr)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to read .gitignore: %w", err)
	}

	existingRules := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cleanRule := strings.TrimPrefix(line, "/")
		existingRules[cleanRule] = true
		existingRules[strings.TrimSuffix(cleanRule, "/")] = true
	}

	var missing []string
	for _, rule := range CriticalGitignoreRules {
		cleanRule := strings.TrimPrefix(rule, "/")
		base := strings.TrimSuffix(cleanRule, "/")
		if !existingRules[cleanRule] && !existingRules[base] {
			missing = append(missing, rule)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	var sb strings.Builder
	content := string(data)
	sb.WriteString(content)
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("\n# Added by Noctifab pre-flight guardrails\n")
	for _, rule := range missing {
		sb.WriteString(rule)
		sb.WriteString("\n")
	}

	if writeErr := os.WriteFile(gitignorePath, []byte(sb.String()), 0o644); writeErr != nil {
		return fmt.Errorf("failed to append guardrails to .gitignore: %w", writeErr)
	}

	return nil
}
