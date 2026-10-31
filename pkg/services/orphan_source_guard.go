package services

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// OrphanSourceViolation records a non-test source file that exists in the workspace
// but is omitted from build manifests or compilation targets.
type OrphanSourceViolation struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

var wildcardRegex = regexp.MustCompile(`(?i)\$\(wildcard\s+[^)]+\)|\b\*\.(?:c|cpp|cc|go|rs|java)\b`)

// DetectOrphanSourceFiles scans projectPath to verify that all primary source files
// in source directories (e.g., src/, lib/) are referenced by the build configuration.
func DetectOrphanSourceFiles(projectPath string) ([]OrphanSourceViolation, error) {
	var violations []OrphanSourceViolation
	if projectPath == "" {
		return violations, nil
	}

	makefilePath := filepath.Join(projectPath, "Makefile")
	if _, err := os.Stat(makefilePath); err != nil {
		makefilePath = filepath.Join(projectPath, "makefile")
		if _, err := os.Stat(makefilePath); err != nil {
			makefilePath = filepath.Join(projectPath, "GNUMakefile")
			if _, err := os.Stat(makefilePath); err != nil {
				// No Makefile present
				return violations, nil
			}
		}
	}

	makeBytes, err := os.ReadFile(makefilePath)
	if err != nil {
		return violations, err
	}
	makefileContent := string(makeBytes)

	// If the Makefile uses wildcards (e.g. $(wildcard src/*.c) or *.c),
	// all source files matching the pattern are considered referenced.
	if wildcardRegex.MatchString(makefileContent) {
		return violations, nil
	}

	// Scan source files under projectPath
	err = filepath.Walk(projectPath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, rErr := filepath.Rel(projectPath, path)
		if rErr != nil {
			return nil
		}
		if info.IsDir() {
			if IsPathExcluded(rel, nil) {
				return filepath.SkipDir
			}
			return nil
		}
		if IsPathExcluded(rel, nil) {
			return nil
		}

		cleanRel := filepath.ToSlash(rel)
		// Only inspect files in source directories like src/ or lib/
		if !strings.HasPrefix(cleanRel, "src/") && !strings.HasPrefix(cleanRel, "lib/") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".c" && ext != ".cpp" && ext != ".cc" && ext != ".h" && ext != ".hpp" {
			return nil
		}

		baseName := filepath.Base(path)
		lowerBase := strings.ToLower(baseName)
		if strings.HasPrefix(lowerBase, "test_") || strings.HasSuffix(lowerBase, "_test.c") ||
			strings.HasSuffix(lowerBase, "_test.cpp") || strings.Contains(cleanRel, "test") {
			return nil
		}

		// Check if the file, its base name, or its object target appears in the Makefile
		stem := strings.TrimSuffix(baseName, ext)
		objName := stem + ".o"

		if !strings.Contains(makefileContent, cleanRel) &&
			!strings.Contains(makefileContent, baseName) &&
			!strings.Contains(makefileContent, objName) {
			violations = append(violations, OrphanSourceViolation{
				Path: cleanRel,
				Reason: fmt.Sprintf(
					"Orphan source file: %s is not referenced, linked, or compiled in %s",
					cleanRel, filepath.Base(makefilePath),
				),
			})
		}

		return nil
	})

	return violations, err
}

// FormatOrphanSourceViolations formats the list of unreferenced files for test failure reporting.
func FormatOrphanSourceViolations(violations []OrphanSourceViolation) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Orphan source file validation failed with %d unreferenced file(s):\n", len(violations))
	for _, v := range violations {
		fmt.Fprintf(&sb, "- %s: %s\n", v.Path, v.Reason)
	}
	return sb.String()
}
