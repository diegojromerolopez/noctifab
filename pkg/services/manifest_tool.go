package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ValidateWorkspaceManifests performs deterministic manifest syntax and undeclared dependency checks.
func ValidateWorkspaceManifests(projectPath string, targetFiles []string) (*ManifestValidationResult, error) {
	res := &ManifestValidationResult{
		Valid:        true,
		DeclaredDeps: make(map[string]string),
		Violations:   []UndeclaredDepViolation{},
		SyntaxErrors: []string{},
	}

	// 1. Detect Manifest
	var manifestPath string
	var manifestType string

	candidates := []struct {
		mType string
		file  string
	}{
		{"cargo", "Cargo.toml"},
		{"pyproject", "pyproject.toml"},
		{"requirements", "requirements.txt"},
		{"gomod", "go.mod"},
		{"npm", "package.json"},
	}

	for _, c := range candidates {
		full := filepath.Join(projectPath, c.file)
		if _, err := os.Stat(full); err == nil {
			manifestPath = full
			manifestType = c.mType
			break
		}
	}

	if manifestPath == "" {
		res.Diagnostic = "No project manifest detected (checked Cargo.toml, pyproject.toml, requirements.txt, go.mod, package.json)."
		return res, nil
	}

	res.ManifestType = manifestType
	res.ManifestPath = filepath.Base(manifestPath)

	contentBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		res.Valid = false
		res.SyntaxErrors = append(res.SyntaxErrors, fmt.Sprintf("Failed to read manifest %s: %v", res.ManifestPath, err))
		return res, nil
	}
	content := string(contentBytes)

	var goModule string
	switch manifestType {
	case "cargo":
		deps, parseErr := ParseCargoToml(content)
		if parseErr != nil {
			res.Valid = false
			res.SyntaxErrors = append(res.SyntaxErrors, parseErr.Error())
		}
		res.DeclaredDeps = deps

	case "pyproject":
		deps, parseErr := ParsePyprojectToml(content)
		if parseErr != nil {
			res.Valid = false
			res.SyntaxErrors = append(res.SyntaxErrors, parseErr.Error())
		}
		// Also merge requirements.txt if present
		reqPath := filepath.Join(projectPath, "requirements.txt")
		if reqBytes, err := os.ReadFile(reqPath); err == nil {
			if reqDeps, err := ParseRequirementsTxt(string(reqBytes)); err == nil {
				for k, v := range reqDeps {
					deps[k] = v
				}
			}
		}
		res.DeclaredDeps = deps

	case "requirements":
		deps, parseErr := ParseRequirementsTxt(content)
		if parseErr != nil {
			res.Valid = false
			res.SyntaxErrors = append(res.SyntaxErrors, parseErr.Error())
		}
		res.DeclaredDeps = deps

	case "gomod":
		mod, deps, parseErr := ParseGoMod(content)
		if parseErr != nil {
			res.Valid = false
			res.SyntaxErrors = append(res.SyntaxErrors, parseErr.Error())
		}
		goModule = mod
		res.DeclaredDeps = deps

	case "npm":
		deps, parseErr := ParsePackageJSON(content)
		if parseErr != nil {
			res.Valid = false
			res.SyntaxErrors = append(res.SyntaxErrors, parseErr.Error())
		}
		res.DeclaredDeps = deps
	}

	// 2. Discover local workspace files
	localModules := DiscoverLocalModules(projectPath)

	// Determine files to scan
	var filesToScan []string
	if len(targetFiles) > 0 {
		filesToScan = targetFiles
	} else {
		_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				if info != nil && info.IsDir() {
					name := info.Name()
					if name == ".git" || name == ".noctifab" || name == "node_modules" || name == "target" || name == ".venv" || name == "__pycache__" {
						return filepath.SkipDir
					}
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".py" || ext == ".rs" || ext == ".go" || ext == ".ts" || ext == ".js" {
				rel, rErr := filepath.Rel(projectPath, path)
				if rErr == nil {
					filesToScan = append(filesToScan, rel)
				}
			}
			return nil
		})
	}

	res.ScannedFiles = filesToScan

	// 3. Scan imports in source files and verify declaration
	for _, relFile := range filesToScan {
		fullPath := filepath.Join(projectPath, relFile)
		srcBytes, rErr := os.ReadFile(fullPath)
		if rErr != nil {
			continue
		}

		occurrences := ExtractImportsFromFile(relFile, string(srcBytes), goModule, localModules)
		for _, occ := range occurrences {
			normPkg := strings.ToLower(occ.Package)
			normPkg = strings.ReplaceAll(normPkg, "-", "_")

			declared := false
			if _, ok := res.DeclaredDeps[normPkg]; ok {
				declared = true
			} else if _, ok := res.DeclaredDeps[occ.Package]; ok {
				declared = true
			}

			if !declared {
				violation := UndeclaredDepViolation{
					File:       relFile,
					Line:       occ.Line,
					Package:    occ.Package,
					Manifest:   res.ManifestPath,
					Diagnostic: fmt.Sprintf("Package/crate %q is imported in %s:%d, but is missing from %s", occ.Package, relFile, occ.Line, res.ManifestPath),
				}
				switch manifestType {
				case "cargo":
					violation.SuggestedFix = fmt.Sprintf("Add `%s = \"...\"` to [dependencies] in Cargo.toml", occ.Package)
				case "pyproject":
					violation.SuggestedFix = fmt.Sprintf("Add `%q` to dependencies in pyproject.toml or requirements.txt", occ.Package)
				case "requirements":
					violation.SuggestedFix = fmt.Sprintf("Add `%s` to requirements.txt", occ.Package)
				case "gomod":
					violation.SuggestedFix = fmt.Sprintf("Run `go get %s` to record dependency in go.mod", occ.Package)
				case "npm":
					violation.SuggestedFix = fmt.Sprintf("Add `%q: \"*\"` to dependencies in package.json", occ.Package)
				}
				res.Violations = append(res.Violations, violation)
				res.Valid = false
			}
		}
	}

	return res, nil
}

// ValidateManifestTool implements validate_manifest for agent manifest auditing.
type ValidateManifestTool struct{}

// Name returns the unique tool identifier.
func (t *ValidateManifestTool) Name() string { return "validate_manifest" }

// Description returns LLM documentation for validate_manifest.
func (t *ValidateManifestTool) Description() string {
	return "validate_manifest checks workspace project manifests (Cargo.toml, pyproject.toml, go.mod, package.json, requirements.txt) for syntax errors and ensures all external imports across source files are properly declared."
}

// Execute performs manifest validation and undeclared dependency checking.
func (t *ValidateManifestTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	if state == nil || strings.TrimSpace(state.ProjectPath) == "" {
		return "", fmt.Errorf("invalid state: missing project path")
	}

	var targetFiles []string
	if tfList, ok := args["target_files"].([]any); ok {
		for _, tf := range tfList {
			if s, ok := tf.(string); ok && s != "" {
				targetFiles = append(targetFiles, s)
			}
		}
	} else if tfListStr, ok := args["target_files"].([]string); ok {
		targetFiles = tfListStr
	}

	res, err := ValidateWorkspaceManifests(state.ProjectPath, targetFiles)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	if res.Valid {
		sb.WriteString("Status: VALID\n")
		sb.WriteString("Manifest: ")
		sb.WriteString(res.ManifestPath)
		sb.WriteString(" (")
		sb.WriteString(res.ManifestType)
		sb.WriteString(")\nDeclared Dependencies: ")
		sb.WriteString(strconv.Itoa(len(res.DeclaredDeps)))
		sb.WriteString("\nScanned Source Files: ")
		sb.WriteString(strconv.Itoa(len(res.ScannedFiles)))
		sb.WriteString("\nResult: All imports are properly declared in manifest.\n")
	} else {
		sb.WriteString("Status: INVALID / UNDECLARED_DEPENDENCIES\n")
		sb.WriteString("Manifest: ")
		sb.WriteString(res.ManifestPath)
		sb.WriteString("\n")

		if len(res.SyntaxErrors) > 0 {
			sb.WriteString("Syntax Errors:\n")
			for _, se := range res.SyntaxErrors {
				sb.WriteString("  - ")
				sb.WriteString(se)
				sb.WriteString("\n")
			}
		}

		if len(res.Violations) > 0 {
			sb.WriteString("Undeclared Dependencies (")
			sb.WriteString(strconv.Itoa(len(res.Violations)))
			sb.WriteString(" found):\n")
			for _, v := range res.Violations {
				sb.WriteString("  - ")
				sb.WriteString(v.Diagnostic)
				sb.WriteString("\n    Suggested Fix: ")
				sb.WriteString(v.SuggestedFix)
				sb.WriteString("\n")
			}
		}
	}

	return sb.String(), nil
}
