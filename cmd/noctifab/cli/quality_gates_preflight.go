package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

var (
	trivialCmdRegex = regexp.MustCompile(`^(?:@?\s*(?:true|exit\s+0|:|echo\s+['"]?(?:ok|pass|passed|skipped|done)['"]?|printf\s+['"]?[^'"\n]*['"]?)\s*;?\s*)+$`)
)

// DetectProjectLanguages inspects files and manifests in projectDir to detect all active languages.
func DetectProjectLanguages(projectDir string) map[string]bool {
	langs := make(map[string]bool)
	if projectDir == "" {
		projectDir = "."
	}

	// Manifest-level checks
	if _, err := os.Stat(filepath.Join(projectDir, "go.mod")); err == nil {
		langs["go"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "go.sum")); err == nil {
		langs["go"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "Cargo.toml")); err == nil {
		langs["rust"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err == nil {
		langs["javascript"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "requirements.txt")); err == nil {
		langs["python"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "setup.py")); err == nil {
		langs["python"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "pyproject.toml")); err == nil {
		langs["python"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "Pipfile")); err == nil {
		langs["python"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "pytest.ini")); err == nil {
		langs["python"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "pom.xml")); err == nil {
		langs["java"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "build.gradle")); err == nil {
		langs["java"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "Gemfile")); err == nil {
		langs["ruby"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "Makefile")); err == nil {
		langs["c"] = true
	}
	if _, err := os.Stat(filepath.Join(projectDir, "CMakeLists.txt")); err == nil {
		langs["c"] = true
	}

	// Shallow file walk (max depth 3, capped at 1000 scanned files for performance)
	fileCount := 0
	_ = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fileCount > 1000 {
			return filepath.SkipDir
		}
		fileCount++

		rel, rErr := filepath.Rel(projectDir, path)
		if rErr != nil {
			return nil
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == ".noctifab" || base == "node_modules" ||
				base == "venv" || base == ".venv" || base == "build" || base == "dist" ||
				base == "reports" || base == "target" || base == "vendor" || base == ".cache" {
				return filepath.SkipDir
			}
			if strings.Count(rel, string(filepath.Separator)) > 3 {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".go":
			langs["go"] = true
		case ".py":
			langs["python"] = true
		case ".rs":
			langs["rust"] = true
		case ".js", ".ts", ".jsx", ".tsx", ".mjs":
			langs["javascript"] = true
		case ".c", ".h", ".cpp", ".hpp", ".cc", ".s", ".asm":
			langs["c"] = true
		case ".java":
			langs["java"] = true
		case ".rb":
			langs["ruby"] = true
		}
		return nil
	})

	return langs
}

// IsTrivialCommand returns true if the command does nothing of substance (e.g. exit 0, true, echo pass).
func IsTrivialCommand(cmdStr string) bool {
	// Strip comments first
	var nonComments []string
	for _, line := range strings.Split(cmdStr, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		nonComments = append(nonComments, trimmed)
	}
	content := strings.Join(nonComments, " ")
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return true
	}
	// Check simple literals
	switch trimmed {
	case "true", "@true", ":", "@:", "exit 0", "@exit 0", "exit 0;", "@exit 0;":
		return true
	}
	return trivialCmdRegex.MatchString(trimmed)
}

// ParseMakefileTargets extracts all declared target names and their multi-line recipes from a Makefile.
// Supports multi-target rules (e.g. `test check:`), ignores variable assignments and .PHONY headers,
// and strictly associates tab-indented recipes.
func ParseMakefileTargets(content string) map[string]string {
	targets := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(content))
	var currentTargets []string
	var currentRecipe strings.Builder

	flush := func() {
		if len(currentTargets) > 0 {
			recipe := currentRecipe.String()
			for _, tgt := range currentTargets {
				if existing, ok := targets[tgt]; ok && existing != "" {
					targets[tgt] = existing + "\n" + recipe
				} else {
					targets[tgt] = recipe
				}
			}
			currentTargets = nil
			currentRecipe.Reset()
		}
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Real Makefile recipes are tab-indented
		if strings.HasPrefix(line, "\t") {
			if len(currentTargets) > 0 {
				currentRecipe.WriteString(strings.TrimSpace(line))
				currentRecipe.WriteString("\n")
			}
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Skip variable assignments (=, :=, +=, ?=, !=)
		if strings.Contains(line, ":=") || strings.Contains(line, "+=") ||
			strings.Contains(line, "?=") || strings.Contains(line, "!=") {
			flush()
			continue
		}
		if eqIdx := strings.Index(line, "="); eqIdx != -1 {
			colonIdx := strings.Index(line, ":")
			if colonIdx == -1 || eqIdx < colonIdx {
				flush()
				continue
			}
		}

		// Match target lines (e.g. `test:`, `test-unit test-all: build`, `all: bin/app`)
		if colonIdx := strings.Index(line, ":"); colonIdx != -1 {
			leftSide := strings.TrimSpace(line[:colonIdx])
			rightSide := strings.TrimSpace(line[colonIdx+1:])

			// Skip special targets starting with '.' like '.PHONY', '.DEFAULT', '.SILENT'
			if strings.HasPrefix(leftSide, ".") {
				flush()
				continue
			}

			// Extract all target names before the colon
			targetNames := strings.Fields(leftSide)
			if len(targetNames) > 0 {
				flush()
				currentTargets = targetNames
				// If target line itself has prerequisites (e.g. `test: test-unit test-python`),
				// store prerequisite references as part of recipe metadata
				if rightSide != "" {
					currentRecipe.WriteString("# prereqs: ")
					currentRecipe.WriteString(rightSide)
					currentRecipe.WriteString("\n")
				}
			}
		} else {
			// Non-recipe, non-target line (e.g. conditional ifeq / endif)
			flush()
		}
	}
	flush()
	return targets
}

func validateCommandToolLanguage(cmdType, cmdStr string, detectedLangs map[string]bool) error {
	trimmed := strings.TrimSpace(cmdStr)
	if trimmed == "" || len(detectedLangs) == 0 {
		return nil
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return nil
	}
	bin := filepath.Base(fields[0])

	toolToLang := map[string]string{
		"go":            "go",
		"golangci-lint": "go",
		"govet":         "go",
		"pytest":        "python",
		"flake8":        "python",
		"black":         "python",
		"ruff":          "python",
		"pylint":        "python",
		"mypy":          "python",
		"cargo":         "rust",
		"rustc":         "rust",
		"clippy":        "rust",
		"npm":           "javascript",
		"npx":           "javascript",
		"yarn":          "javascript",
		"pnpm":          "javascript",
		"jest":          "javascript",
		"mocha":         "javascript",
		"eslint":        "javascript",
		"prettier":      "javascript",
		"mvn":           "java",
		"gradle":        "java",
		"bundle":        "ruby",
		"rake":          "ruby",
		"rspec":         "ruby",
		"rubocop":       "ruby",
	}

	if requiredLang, ok := toolToLang[bin]; ok {
		if !detectedLangs[requiredLang] {
			return fmt.Errorf("pre-flight check failed: configured %s %q uses %s toolchain, but no %s code or manifests exist in project",
				cmdType, cmdStr, requiredLang, requiredLang)
		}
	}

	// Handle `python` / `python3` invocations (e.g. `python3 -m unittest ...`)
	if bin == "python" || bin == "python3" {
		if !detectedLangs["python"] {
			return fmt.Errorf("pre-flight check failed: configured %s %q uses Python toolchain, but no Python code or manifests exist in project",
				cmdType, cmdStr)
		}
	}

	return nil
}

// VerifyQualityAndReleaseGates audits project quality gates, makefile targets, and command configurations.
func VerifyQualityAndReleaseGates(cfg *config.Config, projectDir string) error {
	if projectDir == "" {
		projectDir = "."
	}
	detectedLangs := DetectProjectLanguages(projectDir)

	// 1. Audit Sandbox Configured Commands
	commands := []struct {
		name string
		cmd  string
	}{
		{"test_command", cfg.Sandbox.TestCommand},
		{"linter_command", cfg.Sandbox.LinterCommand},
		{"formatter_command", cfg.Sandbox.FormatterCommand},
	}

	for _, c := range commands {
		if c.cmd == "" {
			continue
		}
		if IsTrivialCommand(c.cmd) {
			return fmt.Errorf("pre-flight check failed: configured %s %q is trivial; quality gate must execute real verification",
				c.name, c.cmd)
		}
		if err := validateCommandToolLanguage(c.name, c.cmd, detectedLangs); err != nil {
			return err
		}
	}

	// 2. Audit Makefile Quality & Release Targets (if Makefile is present)
	makefilePath := filepath.Join(projectDir, "Makefile")
	if data, err := os.ReadFile(makefilePath); err == nil {
		targets := ParseMakefileTargets(string(data))

		// Check that at least one test target is declared
		testTargets := []string{"test", "test-all", "test-unit", "check"}
		foundTestTarget := ""
		for _, tt := range testTargets {
			if _, exists := targets[tt]; exists {
				foundTestTarget = tt
				break
			}
		}

		if foundTestTarget == "" {
			return fmt.Errorf("pre-flight check failed: Makefile found at %s but missing required quality gate target (one of: %s)",
				makefilePath, strings.Join(testTargets, ", "))
		}

		// Ensure found test target is not trivial
		recipe := targets[foundTestTarget]
		if IsTrivialCommand(recipe) {
			// Check if it delegates to sub-targets (e.g. `test: test-unit test-python`)
			hasNonTrivialPrereq := false
			for otherTarget, otherRecipe := range targets {
				if otherTarget != foundTestTarget && strings.Contains(recipe, otherTarget) {
					if !IsTrivialCommand(otherRecipe) {
						hasNonTrivialPrereq = true
						break
					}
				}
			}
			if !hasNonTrivialPrereq {
				return fmt.Errorf("pre-flight check failed: Makefile target %q in %s is trivial or empty; quality gate must execute real verification",
					foundTestTarget, makefilePath)
			}
		}
	}

	var langList []string
	for l := range detectedLangs {
		langList = append(langList, l)
	}
	if len(langList) == 0 {
		langList = append(langList, "unspecified/new")
	}
	fmt.Printf("- Quality & Release Gate targets: verified (languages: %s)\n", strings.Join(langList, ", "))
	return nil
}
