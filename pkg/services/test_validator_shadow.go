package services

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ShadowTestCollision represents a detected duplicate or shadowed test file collision.
type ShadowTestCollision struct {
	ShadowPath string // The conflicting or legacy root-level test file path
	Target     string // The modular or conflicting counterpart path
	Reason     string // Reason for the collision
}

// DetectShadowTestFiles scans the project test tree for shadow or duplicate test file collisions.
// A collision occurs when:
//  1. A test file resides directly at the root of the test directory (e.g., tests/test_foo.py)
//     while modular subdirectories (tests/unit, tests/integration, tests/e2e) exist.
//  2. Multiple test files share the exact same base name across different directory depths.
func DetectShadowTestFiles(projectPath string) []ShadowTestCollision {
	if projectPath == "" {
		return nil
	}

	testFiles := DiscoverTestFiles(projectPath)
	if len(testFiles) <= 1 {
		return nil
	}

	var collisions []ShadowTestCollision
	byBaseName := make(map[string][]string)

	hasModularDir := false
	var rootTestFiles []string

	for _, tf := range testFiles {
		slashPath := filepath.ToSlash(tf)
		parts := strings.Split(slashPath, "/")

		// Check if it's within a test directory
		if len(parts) >= 2 && (parts[0] == "tests" || parts[0] == "test") {
			baseName := filepath.Base(slashPath)
			byBaseName[baseName] = append(byBaseName[baseName], slashPath)

			if len(parts) == 2 {
				// Directly in tests/ (e.g. tests/test_foo.py)
				rootTestFiles = append(rootTestFiles, slashPath)
			} else if len(parts) >= 3 {
				sub := parts[1]
				if sub == "unit" || sub == "integration" || sub == "e2e" {
					hasModularDir = true
				}
			}
		}
	}

	// 1. Duplicate filename collision across depths
	reportedShadows := make(map[string]bool)
	for baseName, paths := range byBaseName {
		if len(paths) > 1 {
			for i := 0; i < len(paths); i++ {
				for j := i + 1; j < len(paths); j++ {
					p1, p2 := paths[i], paths[j]
					shallower, deeper := p1, p2
					if strings.Count(p2, "/") < strings.Count(p1, "/") {
						shallower, deeper = p2, p1
					}
					key := shallower + "::" + deeper
					if !reportedShadows[key] {
						reportedShadows[key] = true
						collisions = append(collisions, ShadowTestCollision{
							ShadowPath: shallower,
							Target:     deeper,
							Reason: fmt.Sprintf(
								"duplicate test filename %q at different depths (%s shadows %s)",
								baseName, shallower, deeper,
							),
						})
					}
				}
			}
		}
	}

	// 2. Root test file violation when modular partitions exist
	if hasModularDir {
		for _, rootFile := range rootTestFiles {
			alreadyReported := false
			for _, c := range collisions {
				if c.ShadowPath == rootFile {
					alreadyReported = true
					break
				}
			}
			if !alreadyReported {
				collisions = append(collisions, ShadowTestCollision{
					ShadowPath: rootFile,
					Target:     "tests/unit or tests/integration",
					Reason: fmt.Sprintf(
						"root-level test file %s violates test partitioning (tests must reside in tests/unit, tests/integration, or tests/e2e)",
						rootFile,
					),
				})
			}
		}
	}

	return collisions
}

// FormatShadowTestCollisions formats collision diagnostics for test runner error output.
func FormatShadowTestCollisions(collisions []ShadowTestCollision) string {
	if len(collisions) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Shadow test file collision detected (%d violation(s)):\n", len(collisions))
	for _, c := range collisions {
		fmt.Fprintf(&sb, "- %s: %s\n", c.ShadowPath, c.Reason)
	}
	sb.WriteString("\nRemediation Mandate:\n")
	sb.WriteString("Tests MUST strictly reside in 'tests/unit', 'tests/integration', or 'tests/e2e' subfolders.\n")
	sb.WriteString("Remove redundant or conflicting legacy test files using the 'delete_file' tool.\n")
	return sb.String()
}
