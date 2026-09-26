package services

import (
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SyntaxViolation represents a syntax or parse error detected in a file before tests are run.
type SyntaxViolation struct {
	FilePath string
	Line     int
	Message  string
}

// SyntaxValidator provides fast, in-process and deterministic verification across
// source code, catching syntax errors, empty stub placeholders, and undeclared imports.
type SyntaxValidator struct {
	diffValidator   *DiffMutationValidator
	manifestGuard   *ManifestIntegrityGuard
	facadeValidator *FacadeIntegrityValidator
	diagnosticGuard *TestContractDiagnosticGuard
}

// NewSyntaxValidator creates a new SyntaxValidator instance.
func NewSyntaxValidator() *SyntaxValidator {
	return &SyntaxValidator{
		diffValidator:   NewDiffMutationValidator(),
		manifestGuard:   NewManifestIntegrityGuard(),
		facadeValidator: NewFacadeIntegrityValidator(),
		diagnosticGuard: NewTestContractDiagnosticGuard(),
	}
}

// Check implements the SyntaxChecker interface for immediate in-tool validation.
func (v *SyntaxValidator) Check(ctx context.Context, fullPath string) error {
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return v.validateDirectory(ctx, fullPath)
	}
	violation, err := v.ValidateFile(ctx, fullPath)
	if err != nil {
		return err
	}
	if violation != nil {
		return fmt.Errorf("validation failed on %s: %s", violation.FilePath, violation.Message)
	}
	return nil
}

func (v *SyntaxValidator) validateDirectory(ctx context.Context, dir string) error {
	return filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, rErr := filepath.Rel(dir, p)
		if rErr == nil && IsPathExcluded(rel, nil) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		violation, vErr := v.ValidateFile(ctx, p)
		if vErr != nil {
			return vErr
		}
		if violation != nil {
			return fmt.Errorf("validation failed on %s: %s", violation.FilePath, violation.Message)
		}
		return nil
	})
}

// ValidateFile performs deterministic validation on a single file based on its extension.
func (v *SyntaxValidator) ValidateFile(ctx context.Context, fullPath string) (*SyntaxViolation, error) {
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File was deleted or does not exist
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, nil
	}

	ext := strings.ToLower(filepath.Ext(fullPath))
	switch ext {
	case ".go":
		return v.validateGo(fullPath)
	case ".json":
		return v.validateJSON(fullPath)
	case ".py":
		return v.validatePython(ctx, fullPath)
	default:
		return nil, nil
	}
}

// ValidateFiles validates a slice of file paths (either absolute or relative to baseDir).
func (v *SyntaxValidator) ValidateFiles(ctx context.Context, baseDir string, files []string) ([]SyntaxViolation, error) {
	var violations []SyntaxViolation
	for _, rel := range files {
		fullPath := rel
		if !filepath.IsAbs(fullPath) && baseDir != "" {
			fullPath = filepath.Join(baseDir, fullPath)
		}
		violation, err := v.ValidateFile(ctx, fullPath)
		if err != nil {
			return nil, err
		}
		if violation != nil {
			violations = append(violations, *violation)
		}
	}
	return violations, nil
}

func (v *SyntaxValidator) validateGo(path string) (*SyntaxViolation, error) {
	content, rErr := os.ReadFile(path)
	if rErr != nil {
		return nil, rErr
	}

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, path, content, parser.AllErrors)
	if err != nil {
		return &SyntaxViolation{
			FilePath: path,
			Message:  fmt.Sprintf("Go syntax error: %v", err),
		}, nil
	}

	// Anti-stub check on non-test files
	if !isTestPath(path) && v.diffValidator != nil {
		if stubErr := v.diffValidator.ValidateASTBody(path, string(content)); stubErr != nil {
			return &SyntaxViolation{
				FilePath: path,
				Message:  stubErr.Error(),
			}, nil
		}
	}

	// Manifest import check if go.mod exists in project hierarchy
	if v.manifestGuard != nil {
		if root := findProjectRoot(path); root != "" {
			goModPath := filepath.Join(root, "go.mod")
			if goModBytes, mErr := os.ReadFile(goModPath); mErr == nil {
				modName, declaredDeps, pErr := ParseGoMod(string(goModBytes))
				if pErr == nil {
					var declaredKeys []string
					for k := range declaredDeps {
						declaredKeys = append(declaredKeys, k)
					}
					if impErr := v.manifestGuard.ValidateImports(path, string(content), declaredKeys, modName); impErr != nil {
						return &SyntaxViolation{
							FilePath: path,
							Message:  impErr.Error(),
						}, nil
					}
				}
			}
		}
	}

	return nil, nil
}

func (v *SyntaxValidator) validateJSON(path string) (*SyntaxViolation, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return nil, nil
	}
	var dummy any
	if err := json.Unmarshal(content, &dummy); err != nil {
		return &SyntaxViolation{
			FilePath: path,
			Message:  fmt.Sprintf("JSON syntax error: %v", err),
		}, nil
	}
	return nil, nil
}

func (v *SyntaxValidator) validatePython(ctx context.Context, path string) (*SyntaxViolation, error) {
	content, rErr := os.ReadFile(path)
	if rErr != nil {
		return nil, rErr
	}

	// Anti-stub check on non-test files
	if !isTestPath(path) && v.diffValidator != nil {
		if stubErr := v.diffValidator.ValidateASTBody(path, string(content)); stubErr != nil {
			return &SyntaxViolation{
				FilePath: path,
				Message:  stubErr.Error(),
			}, nil
		}
	}

	// Manifest import check if pyproject.toml / requirements.txt exists in project hierarchy
	if v.manifestGuard != nil {
		if root := findProjectRoot(path); root != "" {
			var declaredKeys []string
			if pyproj, pErr := os.ReadFile(filepath.Join(root, "pyproject.toml")); pErr == nil {
				if deps, dErr := ParsePyprojectToml(string(pyproj)); dErr == nil {
					for k := range deps {
						declaredKeys = append(declaredKeys, k)
					}
				}
			}
			if reqs, qErr := os.ReadFile(filepath.Join(root, "requirements.txt")); qErr == nil {
				if deps, dErr := ParseRequirementsTxt(string(reqs)); dErr == nil {
					for k := range deps {
						declaredKeys = append(declaredKeys, k)
					}
				}
			}
			if len(declaredKeys) > 0 {
				if impErr := v.manifestGuard.ValidateImports(path, string(content), declaredKeys, ""); impErr != nil {
					return &SyntaxViolation{
						FilePath: path,
						Message:  impErr.Error(),
					}, nil
				}
			}
		}
	}

	// Diagnostic richness check on test files
	if isTestPath(path) && v.diagnosticGuard != nil {
		if diagViolations := v.diagnosticGuard.ValidateDiagnosticRichness(path, string(content)); len(diagViolations) > 0 {
			return &SyntaxViolation{
				FilePath: path,
				Line:     diagViolations[0].LineNumber,
				Message:  diagViolations[0].Reason,
			}, nil
		}
	}

	// Facade integrity check when tests invoke core project classes
	if isTestPath(path) && v.facadeValidator != nil {
		if root := findProjectRoot(path); root != "" {
			srcFiles := loadProjectSourceFiles(filepath.Join(root, "src"))
			tFiles := map[string]string{path: string(content)}
			if fViolations := v.facadeValidator.ValidateFacades(srcFiles, tFiles); len(fViolations) > 0 {
				return &SyntaxViolation{
					FilePath: path,
					Message:  fViolations[0].Error(),
				}, nil
			}
		}
	}

	// Fast Python compilation via python3 -m py_compile if python3 is available
	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, pythonPath, "-m", "py_compile", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return &SyntaxViolation{
			FilePath: path,
			Message:  fmt.Sprintf("Python syntax error: %s", msg),
		}, nil
	}
	return nil, nil
}

func isTestPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	cleanPath := filepath.ToSlash(path)
	return strings.HasSuffix(base, "_test.go") ||
		strings.HasPrefix(base, "test_") ||
		strings.HasSuffix(base, "_test.py") ||
		strings.Contains(cleanPath, "/test/") ||
		strings.Contains(cleanPath, "/tests/")
}

func findProjectRoot(startPath string) string {
	dir := filepath.Dir(startPath)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "requirements.txt")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir || parent == "." || parent == "/" {
			break
		}
		dir = parent
	}
	return ""
}

func loadProjectSourceFiles(srcDir string) map[string]string {
	files := make(map[string]string)
	if _, err := os.Stat(srcDir); err != nil {
		return files
	}
	_ = filepath.WalkDir(srcDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".py") || (strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go")) {
			if content, rErr := os.ReadFile(p); rErr == nil {
				files[p] = string(content)
			}
		}
		return nil
	})
	return files
}

