package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ResolveReportPath resolves and validates an execution report path configuration.
func ResolveReportPath(projectPath, configured string) (string, bool, error) {
	return ResolveReportPathWithTime(projectPath, configured, time.Now().UTC())
}

// ResolveReportPathWithTime resolves execution report path using a provided timestamp for deterministic testing.
func ResolveReportPathWithTime(projectPath, configured string, now time.Time) (string, bool, error) {
	trimmed := strings.TrimSpace(configured)
	if trimmed == "" {
		return "", false, nil
	}

	if strings.ContainsRune(trimmed, '\x00') {
		return "", false, errors.New("report path contains NUL byte")
	}

	if projectPath == "" {
		return "", false, errors.New("project path cannot be empty")
	}

	cleanProjectPath, err := filepath.Abs(filepath.Clean(projectPath))
	if err != nil {
		return "", false, fmt.Errorf("invalid project path: %w", err)
	}

	folderName := filepath.Base(cleanProjectPath)
	if envProj := strings.TrimSpace(os.Getenv("PROJECT")); envProj != "" {
		folderName = envProj
	} else if envProjName := strings.TrimSpace(os.Getenv("PROJECT_NAME")); envProjName != "" {
		folderName = envProjName
	} else if folderName == "" || folderName == "." || folderName == "/" || folderName == "src_mount" {
		folderName = "project"
	}

	var rawTarget string
	if filepath.IsAbs(trimmed) {
		rawTarget = filepath.Clean(trimmed)
	} else {
		rawTarget = filepath.Clean(filepath.Join(cleanProjectPath, trimmed))
	}

	dir := filepath.Dir(rawTarget)
	baseName := filepath.Base(rawTarget)

	timestamp := now.UTC().Format("20060102_150405")

	var formattedBase string
	if baseName == "report.md" || baseName == "EXECUTION_REPORT.md" || baseName == "execution_report.md" {
		formattedBase = fmt.Sprintf("%s_%s.md", timestamp, folderName)
	} else {
		ext := filepath.Ext(baseName)
		stem := strings.TrimSuffix(baseName, ext)
		if stem == "" {
			stem = "report"
		}
		if ext == "" {
			ext = ".md"
		}
		formattedBase = fmt.Sprintf("%s_%s_%s%s", timestamp, folderName, stem, ext)
	}

	resolvedPath := filepath.Join(dir, formattedBase)

	// Lexical workspace boundary checks
	reportsDir := filepath.Join(cleanProjectPath, ".noctifab", "reports")

	inWorkspace := resolvedPath == cleanProjectPath || strings.HasPrefix(resolvedPath, cleanProjectPath+string(filepath.Separator))
	if inWorkspace {
		inReportsDir := resolvedPath == reportsDir || strings.HasPrefix(resolvedPath, reportsDir+string(filepath.Separator))
		if !inReportsDir {
			return "", false, fmt.Errorf("in-workspace execution report path must be inside %s", reportsDir)
		}
	}

	// Check forbidden subpaths / filenames
	pathParts := strings.Split(filepath.ToSlash(resolvedPath), "/")
	for _, part := range pathParts {
		if part == ".git" || part == "secrets.yaml" {
			return "", false, fmt.Errorf("report path cannot target protected system location (%s)", part)
		}
	}

	return resolvedPath, true, nil
}
