package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// StructureViolationType categorizes the specific project layout defect.
type StructureViolationType string

const (
	ViolationShadowing        StructureViolationType = "module_shadowing"
	ViolationLayoutConvention StructureViolationType = "layout_convention"
	ViolationOrphanZombie     StructureViolationType = "orphan_zombie_file"
)

// StructureViolation encapsulates a detected packaging or architectural layout defect.
type StructureViolation struct {
	Type        StructureViolationType `json:"type"`
	File        string                 `json:"file,omitempty"`
	Dir         string                 `json:"dir,omitempty"`
	Language    string                 `json:"language"`
	Description string                 `json:"description"`
	Remedy      string                 `json:"remedy"`
}

func (v StructureViolation) String() string {
	target := v.File
	if target == "" {
		target = v.Dir
	}
	return fmt.Sprintf("[%s] %s (%s): %s | Remedy: %s", v.Type, target, v.Language, v.Description, v.Remedy)
}

// ProjectStructureReport holds the aggregated findings from the structure analyzer.
type ProjectStructureReport struct {
	Valid      bool                 `json:"valid"`
	Violations []StructureViolation `json:"violations"`
}

// Summary returns a human-readable diagnosis of all detected structure issues.
func (r *ProjectStructureReport) Summary() string {
	if r.Valid || len(r.Violations) == 0 {
		return "Project structure and layout comply with standard community guidelines."
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d project structure / layout defect(s):\n", len(r.Violations))
	for idx, v := range r.Violations {
		fmt.Fprintf(&sb, "%d. %s\n", idx+1, v.String())
	}
	return sb.String()
}

// ProjectStructureAnalyzer performs deterministic verification of language-agnostic
// packaging conventions, module shadowing collisions, and orphan zombie files.
type ProjectStructureAnalyzer struct {
	workspaceRoot string
	excludedDirs  map[string]struct{}
}

// NewProjectStructureAnalyzer creates a new structure analyzer.
func NewProjectStructureAnalyzer(workspaceRoot string) *ProjectStructureAnalyzer {
	return &ProjectStructureAnalyzer{
		workspaceRoot: workspaceRoot,
		excludedDirs: map[string]struct{}{
			".git":          {},
			".noctifab":     {},
			".venv":         {},
			"venv":          {},
			"__pycache__":   {},
			"node_modules":  {},
			"target":        {},
			"dist":          {},
			"build":         {},
			".mypy_cache":   {},
			".ruff_cache":   {},
			".pytest_cache": {},
			"vendor":        {},
		},
	}
}

// Analyze runs the complete suite of A, B, and C structural checks against projectPath.
func (a *ProjectStructureAnalyzer) Analyze(projectPath string) (*ProjectStructureReport, error) {
	absPath := projectPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(a.workspaceRoot, projectPath)
	}

	var violations []StructureViolation
	violations = append(violations, a.CheckModuleShadowing(absPath)...)
	violations = append(violations, a.CheckLayoutConventions(absPath)...)
	violations = append(violations, a.CheckOrphanZombieFiles(absPath)...)

	return &ProjectStructureReport{
		Valid:      len(violations) == 0,
		Violations: violations,
	}, nil
}

// CheckModuleShadowing (A) detects collisions where a flat source file <name>.<ext>
// exists alongside a package directory <name>/ in the same folder.
func (a *ProjectStructureAnalyzer) CheckModuleShadowing(projectPath string) []StructureViolation {
	var violations []StructureViolation
	sourceExts := map[string]string{
		".py":  "python",
		".ts":  "typescript",
		".tsx": "typescript",
		".js":  "javascript",
		".jsx": "javascript",
		".rs":  "rust",
		".go":  "go",
	}

	_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		base := info.Name()
		if _, skip := a.excludedDirs[base]; skip {
			return filepath.SkipDir
		}

		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return nil
		}

		dirs := make(map[string]struct{})
		for _, e := range entries {
			if e.IsDir() {
				if _, skip := a.excludedDirs[e.Name()]; !skip {
					dirs[e.Name()] = struct{}{}
				}
			}
		}

		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			lang, isSource := sourceExts[ext]
			if !isSource {
				continue
			}

			stem := strings.TrimSuffix(e.Name(), ext)
			if _, exists := dirs[stem]; exists {
				collidingDir := filepath.Join(path, stem)
				relFile, _ := filepath.Rel(projectPath, filepath.Join(path, e.Name()))
				relDir, _ := filepath.Rel(projectPath, collidingDir)

				var desc, remedy string
				switch lang {
				case "python":
					desc = fmt.Sprintf("Python package shadowing collision: '%s' and directory '%s/' both exist. The directory shadows the flat module on import, causing split-brain interfaces.", relFile, relDir)
					remedy = fmt.Sprintf("Remove obsolete legacy file '%s' if modularized into '%s/', or rename one of them to eliminate collision.", relFile, relDir)
				case "typescript", "javascript":
					desc = fmt.Sprintf("JavaScript/TypeScript module shadowing collision: '%s' collides with directory '%s/'.", relFile, relDir)
					remedy = fmt.Sprintf("Consolidate '%s' into '%s/index%s' or eliminate duplicate module definition.", relFile, relDir, ext)
				case "rust":
					desc = fmt.Sprintf("Rust module shadowing collision: '%s' and directory '%s/' share the same module name.", relFile, relDir)
					remedy = fmt.Sprintf("Follow 2018 edition module conventions: keep either '%s' with submodule files or '%s/mod.rs', not both.", relFile, relDir)
				default:
					desc = fmt.Sprintf("Module shadowing collision: file '%s' and directory '%s/' conflict in the same package path.", relFile, relDir)
					remedy = fmt.Sprintf("Remove or rename conflicting flat file '%s'.", relFile)
				}

				violations = append(violations, StructureViolation{
					Type:        ViolationShadowing,
					File:        relFile,
					Dir:         relDir,
					Language:    lang,
					Description: desc,
					Remedy:      remedy,
				})
			}
		}

		return nil
	})

	return violations
}

// CheckLayoutConventions (B) enforces community standards across supported ecosystems.
func (a *ProjectStructureAnalyzer) CheckLayoutConventions(projectPath string) []StructureViolation {
	var violations []StructureViolation

	// 1. Python Layout Conventions (PEP 518/621, src-layout, package markers)
	violations = append(violations, a.checkPythonLayout(projectPath)...)

	// 2. TypeScript / Node Layout Conventions
	violations = append(violations, a.checkTypeScriptLayout(projectPath)...)

	// 3. Rust Layout Conventions (Cargo standards)
	violations = append(violations, a.checkRustLayout(projectPath)...)

	return violations
}

func (a *ProjectStructureAnalyzer) checkPythonLayout(projectPath string) []StructureViolation {
	var violations []StructureViolation
	hasPyFiles := false
	_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if _, skip := a.excludedDirs[info.Name()]; skip {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), ".py") {
			hasPyFiles = true
			return filepath.SkipAll
		}
		return nil
	})

	if !hasPyFiles {
		return nil
	}

	// Check for package markers (__init__.py) in subdirectories containing Python source code
	_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		base := info.Name()
		if _, skip := a.excludedDirs[base]; skip {
			return filepath.SkipDir
		}
		if path == projectPath {
			return nil
		}

		rel, _ := filepath.Rel(projectPath, path)
		if strings.HasPrefix(rel, "tests") || strings.HasPrefix(rel, "docs") {
			return nil
		}

		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			return nil
		}

		hasModulePy := false
		hasInit := false
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".py") {
				if e.Name() == "__init__.py" {
					hasInit = true
				} else {
					hasModulePy = true
				}
			}
		}

		if hasModulePy && !hasInit {
			violations = append(violations, StructureViolation{
				Type:        ViolationLayoutConvention,
				Dir:         rel,
				Language:    "python",
				Description: fmt.Sprintf("Python package directory '%s' is missing mandatory '__init__.py' marker.", rel),
				Remedy:      fmt.Sprintf("Create an empty '%s/__init__.py' file to establish explicit package boundaries.", rel),
			})
		}

		return nil
	})

	return violations
}

func (a *ProjectStructureAnalyzer) checkTypeScriptLayout(projectPath string) []StructureViolation {
	var violations []StructureViolation
	hasTSFiles := false
	_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if _, skip := a.excludedDirs[info.Name()]; skip {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), ".ts") || strings.HasSuffix(info.Name(), ".tsx") {
			hasTSFiles = true
			return filepath.SkipAll
		}
		return nil
	})

	if !hasTSFiles {
		return nil
	}

	// Community rule: TypeScript projects require package.json or tsconfig.json
	hasPkgJSON := fileExists(filepath.Join(projectPath, "package.json"))
	hasTSConfig := fileExists(filepath.Join(projectPath, "tsconfig.json"))
	if !hasPkgJSON && !hasTSConfig {
		violations = append(violations, StructureViolation{
			Type:        ViolationLayoutConvention,
			Dir:         ".",
			Language:    "typescript",
			Description: "TypeScript project is missing both 'package.json' and 'tsconfig.json'.",
			Remedy:      "Create 'package.json' and 'tsconfig.json' specifying module resolution and build configuration.",
		})
	}

	return violations
}

func (a *ProjectStructureAnalyzer) checkRustLayout(projectPath string) []StructureViolation {
	var violations []StructureViolation
	hasRSFiles := false
	_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if _, skip := a.excludedDirs[info.Name()]; skip {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), ".rs") {
			hasRSFiles = true
			return filepath.SkipAll
		}
		return nil
	})

	if !hasRSFiles {
		return nil
	}

	// Community rule: Rust projects require Cargo.toml
	if !fileExists(filepath.Join(projectPath, "Cargo.toml")) {
		violations = append(violations, StructureViolation{
			Type:        ViolationLayoutConvention,
			Dir:         ".",
			Language:    "rust",
			Description: "Rust project is missing canonical 'Cargo.toml' manifest.",
			Remedy:      "Initialize 'Cargo.toml' declaring package metadata and dependencies.",
		})
	}

	return violations
}

// CheckOrphanZombieFiles (C) detects non-entrypoint source files that are completely
// unreferenced across the AST / import graph.
func (a *ProjectStructureAnalyzer) CheckOrphanZombieFiles(projectPath string) []StructureViolation {
	var violations []StructureViolation
	allFiles := make(map[string]string) // relPath -> content
	sourceExts := map[string]string{
		".py":  "python",
		".ts":  "typescript",
		".tsx": "typescript",
		".js":  "javascript",
		".rs":  "rust",
		".go":  "go",
	}

	_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if _, ok := sourceExts[ext]; !ok {
			return nil
		}

		for _, part := range strings.Split(path, string(filepath.Separator)) {
			if _, skip := a.excludedDirs[part]; skip {
				return nil
			}
		}

		content, rErr := os.ReadFile(path)
		if rErr == nil {
			rel, _ := filepath.Rel(projectPath, path)
			allFiles[rel] = string(content)
		}
		return nil
	})

	if len(allFiles) < 3 {
		return nil
	}

	for relPath := range allFiles {
		if isEntrypointOrRoot(relPath) {
			continue
		}

		base := filepath.Base(relPath)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		modulePath := strings.TrimSuffix(relPath, filepath.Ext(relPath))
		pythonModuleDot := strings.ReplaceAll(modulePath, string(filepath.Separator), ".")
		pythonShortModule := strings.ReplaceAll(strings.TrimPrefix(modulePath, "src/"), string(filepath.Separator), ".")

		referenced := false
		for otherPath, content := range allFiles {
			if otherPath == relPath {
				continue
			}

			if strings.Contains(content, stem) ||
				strings.Contains(content, modulePath) ||
				strings.Contains(content, pythonModuleDot) ||
				strings.Contains(content, pythonShortModule) {
				referenced = true
				break
			}
		}

		if !referenced {
			ext := strings.ToLower(filepath.Ext(relPath))
			violations = append(violations, StructureViolation{
				Type:        ViolationOrphanZombie,
				File:        relPath,
				Language:    sourceExts[ext],
				Description: fmt.Sprintf("Orphan/zombie file detected: '%s' is completely unreferenced by application code, entrypoints, or test suites.", relPath),
				Remedy:      fmt.Sprintf("Safely prune obsolete file '%s' or add test/import wiring if functionality is active.", relPath),
			})
		}
	}

	sort.Slice(violations, func(i, j int) bool {
		return violations[i].File < violations[j].File
	})

	return violations
}

func isEntrypointOrRoot(relPath string) bool {
	lower := strings.ToLower(relPath)
	base := filepath.Base(lower)

	// Standard entrypoints, tests, configuration scripts, and package markers
	if base == "__init__.py" || base == "__main__.py" || base == "main.py" || base == "app.py" || base == "server.py" ||
		base == "index.ts" || base == "main.ts" || base == "index.js" || base == "main.js" ||
		base == "main.rs" || base == "lib.rs" || base == "mod.rs" || base == "main.go" ||
		base == "setup.py" || base == "conftest.py" {
		return true
	}

	// Tests are verified by test runners, not by imports
	if strings.Contains(lower, "test") || strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") ||
		strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".spec.ts") {
		return true
	}

	return false
}
