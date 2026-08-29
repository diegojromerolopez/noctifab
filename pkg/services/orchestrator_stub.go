package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ensureTargetStubFilesExist creates minimal compilation stub files for any target source file
// specified in task.TargetFiles that does not yet exist on disk. This prevents Turn 1 build/import
// failures when running in tester_first mode.
func (o *Orchestrator) ensureTargetStubFilesExist(projectPath string, task *domain.Task) {
	if task == nil || len(task.TargetFiles) == 0 {
		return
	}

	for _, relPath := range task.TargetFiles {
		cleanRel := filepath.Clean(relPath)
		if cleanRel == "." || cleanRel == "" {
			continue
		}

		fullPath, err := resolveSandboxPath(projectPath, cleanRel)
		if err != nil {
			continue
		}

		// Skip if file already exists on disk
		if _, statErr := os.Stat(fullPath); statErr == nil {
			continue
		}

		// Ensure parent directories exist
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ [Orchestrator] Failed to create parent directory %s for stub file: %v\n", dir, err)
			continue
		}

		// Generate language-appropriate minimal stub content
		stubContent := generateStubContent(cleanRel)
		if err := os.WriteFile(fullPath, []byte(stubContent), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "⚠ [Orchestrator] Failed to write pre-seeded stub file %s: %v\n", cleanRel, err)
		} else {
			fmt.Printf("ℹ [Orchestrator] Pre-seeded minimal compilation stub for %s (tester_first mode)\n", cleanRel)
		}
	}
}

// generateStubContent returns a minimal boilerplate stub string based on the file extension.
func generateStubContent(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	baseName := filepath.Base(filePath)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)

	switch ext {
	case ".py", ".sh", ".yaml", ".yml", ".toml", ".ini", ".conf", ".cfg":
		return fmt.Sprintf("# Stub implementation for %s\n", baseName)
	case "":
		if strings.ToLower(baseName) == "makefile" || strings.ToLower(baseName) == "dockerfile" {
			return fmt.Sprintf("# Stub implementation for %s\n", baseName)
		}
		return fmt.Sprintf("// Stub implementation for %s\n", baseName)
	case ".go":
		pkg := nameWithoutExt
		if pkg == "main" || pkg == "app" {
			pkg = "main"
		} else if strings.Contains(filePath, "/") {
			dir := filepath.Base(filepath.Dir(filePath))
			if dir != "." && dir != "/" {
				pkg = dir
			}
		}
		return fmt.Sprintf("package %s\n\n// Stub implementation for %s\n", pkg, baseName)
	case ".h", ".hpp":
		guard := strings.ToUpper(strings.ReplaceAll(nameWithoutExt, "-", "_")) + "_STUB_H"
		return fmt.Sprintf("/* Stub header for %s */\n#ifndef %s\n#define %s\n\n#endif // %s\n", baseName, guard, guard, guard)
	case ".c", ".cpp":
		header := nameWithoutExt + ".h"
		return fmt.Sprintf("/* Stub implementation for %s */\n#include \"%s\"\n", baseName, header)
	case ".ts", ".js", ".jsx", ".tsx":
		return fmt.Sprintf("// Stub implementation for %s\nexport {};\n", baseName)
	case ".rs":
		return fmt.Sprintf("// Stub implementation for %s\n", baseName)
	default:
		return fmt.Sprintf("// Stub implementation for %s\n", baseName)
	}
}
