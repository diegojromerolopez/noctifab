package services

import (
	"fmt"
	"strings"
)

// UndeclaredImportViolation reports an imported package that is missing from project manifests.
type UndeclaredImportViolation struct {
	FilePath string
	Import   string
	Line     int
}

func (v UndeclaredImportViolation) Error() string {
	return fmt.Sprintf("undeclared import violation in %s:%d: package '%s' is neither part of the standard library nor declared in project manifests; add it to project manifest or use standard library",
		v.FilePath, v.Line, v.Import)
}

// UndeclaredImportGuard verifies that external imports correspond to standard library
// modules, local modules, or dependencies declared in project manifests.
type UndeclaredImportGuard struct {
	goModule     string
	localModules map[string]bool
}

// NewUndeclaredImportGuard creates a new UndeclaredImportGuard.
func NewUndeclaredImportGuard(goModule string, localModules map[string]bool) *UndeclaredImportGuard {
	if localModules == nil {
		localModules = make(map[string]bool)
	}
	return &UndeclaredImportGuard{
		goModule:     goModule,
		localModules: localModules,
	}
}

// ValidateFileImports checks that external imports in code content are declared in project manifests.
func (g *UndeclaredImportGuard) ValidateFileImports(filePath, content string, declaredManifestDeps []string) []UndeclaredImportViolation {
	declaredSet := make(map[string]struct{}, len(declaredManifestDeps))
	for _, d := range declaredManifestDeps {
		clean := strings.ToLower(strings.TrimSpace(d))
		// Strip version constraints: e.g. "pytest>=7.0" -> "pytest"
		for _, sep := range []string{">=", "<=", "==", "!=", "~=", "^", ">", "<"} {
			if idx := strings.Index(clean, sep); idx != -1 {
				clean = clean[:idx]
			}
		}
		clean = normalizeImportPkg(strings.TrimSpace(clean))
		if clean != "" {
			declaredSet[clean] = struct{}{}
		}
	}

	occurrences := ExtractImportsFromFile(filePath, content, g.goModule, g.localModules)
	var violations []UndeclaredImportViolation

	for _, occ := range occurrences {
		pkgNorm := normalizeImportPkg(occ.Package)
		if _, ok := declaredSet[pkgNorm]; !ok {
			// Check without normalization if scoped or path
			if _, rawOk := declaredSet[strings.ToLower(occ.Package)]; !rawOk {
				violations = append(violations, UndeclaredImportViolation{
					FilePath: filePath,
					Import:   occ.Package,
					Line:     occ.Line,
				})
			}
		}
	}

	return violations
}

func normalizeImportPkg(pkg string) string {
	pkg = strings.ToLower(strings.TrimSpace(pkg))
	return strings.ReplaceAll(pkg, "-", "_")
}
