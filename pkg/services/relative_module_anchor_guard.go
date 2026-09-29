package services

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// RelativeImportViolation reports a relative import that cannot be resolved on the filesystem.
type RelativeImportViolation struct {
	SourceFile   string
	ImportSpec   string
	ResolvedPath string
	Line         int
}

func (v RelativeImportViolation) Error() string {
	return fmt.Sprintf("relative module anchoring violation in %s:%d: relative target '%s' cannot be resolved on disk (expected file/dir at '%s')",
		v.SourceFile, v.Line, v.ImportSpec, v.ResolvedPath)
}

// RelativeModuleAnchorGuard verifies that relative imports resolve to existing files or directories.
type RelativeModuleAnchorGuard struct {
	workspaceRoot string
}

// NewRelativeModuleAnchorGuard creates a new RelativeModuleAnchorGuard rooted at workspaceRoot.
func NewRelativeModuleAnchorGuard(workspaceRoot string) *RelativeModuleAnchorGuard {
	return &RelativeModuleAnchorGuard{
		workspaceRoot: workspaceRoot,
	}
}

var (
	pyRelativeFromRe = regexp.MustCompile(`^\s*from\s+(\.+[a-zA-Z0-9_\.]*)\s+import`)
	jsRelativeRe     = regexp.MustCompile(`(?:import\s+.*?from\s+|require\()\s*['"](\.[^'"]+)['"]`)
)

// ValidateRelativeImports parses relative imports in content and verifies that the referenced
// targets exist on disk relative to sourceFilePath.
func (g *RelativeModuleAnchorGuard) ValidateRelativeImports(sourceFilePath, content string) []RelativeImportViolation {
	absSource := sourceFilePath
	if !filepath.IsAbs(absSource) {
		absSource = filepath.Join(g.workspaceRoot, sourceFilePath)
	}
	sourceDir := filepath.Dir(absSource)

	ext := strings.ToLower(filepath.Ext(sourceFilePath))
	lines := strings.Split(content, "\n")
	var violations []RelativeImportViolation

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch ext {
		case ".py":
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			if m := pyRelativeFromRe.FindStringSubmatch(trimmed); len(m) > 1 {
				importSpec := m[1]
				if err := g.checkPythonRelativeImport(sourceDir, importSpec); err != nil {
					violations = append(violations, RelativeImportViolation{
						SourceFile:   sourceFilePath,
						ImportSpec:   importSpec,
						ResolvedPath: err.Error(),
						Line:         idx + 1,
					})
				}
			}

		case ".js", ".jsx", ".ts", ".tsx", ".mjs":
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if m := jsRelativeRe.FindStringSubmatch(trimmed); len(m) > 1 {
				importSpec := m[1]
				if err := g.checkNodeRelativeImport(sourceDir, importSpec); err != nil {
					violations = append(violations, RelativeImportViolation{
						SourceFile:   sourceFilePath,
						ImportSpec:   importSpec,
						ResolvedPath: err.Error(),
						Line:         idx + 1,
					})
				}
			}
		}
	}

	return violations
}

func (g *RelativeModuleAnchorGuard) checkPythonRelativeImport(sourceDir, spec string) error {
	// Count leading dots
	dotCount := 0
	for _, ch := range spec {
		if ch == '.' {
			dotCount++
		} else {
			break
		}
	}

	remaining := spec[dotCount:]
	targetDir := sourceDir
	// 1 dot = current package/dir; 2 dots = parent dir; 3 dots = parent of parent
	for i := 1; i < dotCount; i++ {
		targetDir = filepath.Dir(targetDir)
	}

	if remaining == "" {
		// e.g. "from . import helper" -> targets current dir
		return nil
	}

	subPath := strings.ReplaceAll(remaining, ".", string(filepath.Separator))
	candidateBase := filepath.Join(targetDir, subPath)

	// Check candidateBase.py
	if fileExists(candidateBase + ".py") {
		return nil
	}
	// Check candidateBase/__init__.py
	if fileExists(filepath.Join(candidateBase, "__init__.py")) {
		return nil
	}
	// Check if directory exists
	if dirExists(candidateBase) {
		return nil
	}

	return fmt.Errorf("%s(.py|/__init__.py)", candidateBase)
}

func (g *RelativeModuleAnchorGuard) checkNodeRelativeImport(sourceDir, spec string) error {
	candidateBase := filepath.Join(sourceDir, spec)

	extensions := []string{"", ".ts", ".tsx", ".js", ".jsx", ".mjs", "/index.ts", "/index.tsx", "/index.js", "/index.jsx"}
	for _, ext := range extensions {
		if fileExists(candidateBase + ext) {
			return nil
		}
	}
	if dirExists(candidateBase) {
		return nil
	}

	return fmt.Errorf("%s[.ts|.js]", candidateBase)
}
