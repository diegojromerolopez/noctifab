package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ToolManifestMap associates build and test tool binaries with their expected configuration manifests.
var ToolManifestMap = map[string][]string{
	"cargo":     {"Cargo.toml"},
	"go":        {"go.mod"},
	"npm":       {"package.json"},
	"pnpm":      {"package.json"},
	"yarn":      {"package.json"},
	"npx":       {"package.json"},
	"node":      {"package.json"},
	"bun":       {"package.json"},
	"pytest":    {"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt"},
	"python":    {"pyproject.toml", "setup.py", "setup.cfg"},
	"python3":   {"pyproject.toml", "setup.py", "setup.cfg"},
	"dotnet":    {"*.csproj", "*.sln"},
	"rebar3":    {"rebar.config"},
	"dune":      {"dune-project", "dune"},
	"bundle":    {"Gemfile"},
	"rspec":     {"Gemfile"},
	"rake":      {"Gemfile", "Rakefile"},
	"mvn":       {"pom.xml"},
	"gradle":    {"build.gradle", "build.gradle.kts", "settings.gradle"},
	"./gradlew": {"build.gradle", "build.gradle.kts", "settings.gradle"},
	"gradlew":   {"build.gradle", "build.gradle.kts", "settings.gradle"},
	"make":      {"Makefile", "makefile", "GNUMakefile"},
}

// ignoredSubdirectoryNames contains directory names that should never be treated as package roots.
var ignoredSubdirectoryNames = map[string]bool{
	".git":         true,
	".noctifab":    true,
	"node_modules": true,
	"vendor":       true,
	".venv":        true,
	"venv":         true,
	"target":       true,
	"bin":          true,
	"obj":          true,
	"dist":         true,
	"build":        true,
	".idea":        true,
	".vscode":      true,
}

// ResolveSubdirectoryManifest checks whether targetDir already contains a manifest for binary.
// If it does not, it scans immediate subdirectories of targetDir to locate a subdirectory that contains
// the required manifest, returning that subdirectory if found.
func ResolveSubdirectoryManifest(targetDir, binary, pkg string) string {
	if pkg != "" {
		return targetDir
	}

	manifestPatterns, ok := ToolManifestMap[strings.ToLower(strings.TrimSpace(binary))]
	if !ok || len(manifestPatterns) == 0 {
		return targetDir
	}

	// 1. If targetDir already contains a matching manifest, use targetDir directly
	if hasMatchingManifest(targetDir, manifestPatterns) {
		return targetDir
	}

	// 2. Scan immediate subdirectories
	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return targetDir
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || ignoredSubdirectoryNames[name] {
			continue
		}

		subDir := filepath.Join(targetDir, name)
		if hasMatchingManifest(subDir, manifestPatterns) {
			fmt.Printf("ℹ [Sandbox Manifest Resolver] Discovered manifest in subdirectory %q for tool %q; setting working directory to %s\n",
				name, binary, subDir)
			return subDir
		}
	}

	return targetDir
}

func hasMatchingManifest(dir string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(pattern, "*") {
			matches, err := filepath.Glob(filepath.Join(dir, pattern))
			if err == nil && len(matches) > 0 {
				return true
			}
		} else {
			if _, err := os.Stat(filepath.Join(dir, pattern)); err == nil {
				return true
			}
		}
	}
	return false
}

// listCandidateSubdirectories returns immediate non-hidden, non-ignored subdirectories in projectPath.
func listCandidateSubdirectories(projectPath string) []string {
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return nil
	}
	var res []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || ignoredSubdirectoryNames[name] {
			continue
		}
		res = append(res, filepath.Join(projectPath, name))
	}
	return res
}

// DetectSubdirectoryTestCommand scans immediate subdirectories of projectPath for project manifests
// and returns the corresponding test command if found.
func DetectSubdirectoryTestCommand(projectPath string) string {
	for _, subDir := range listCandidateSubdirectories(projectPath) {
		if _, err := os.Stat(filepath.Join(subDir, "Makefile")); err == nil {
			content, rErr := os.ReadFile(filepath.Join(subDir, "Makefile"))
			if rErr == nil && (strings.Contains(string(content), "test:") || strings.Contains(string(content), "test-all:")) {
				return "make test"
			}
		}
		if _, err := os.Stat(filepath.Join(subDir, "Cargo.toml")); err == nil {
			return "cargo test"
		}
		if _, err := os.Stat(filepath.Join(subDir, "package.json")); err == nil {
			return "npm test"
		}
		if _, err := os.Stat(filepath.Join(subDir, "pytest.ini")); err == nil {
			return "pytest"
		}
		if _, err := os.Stat(filepath.Join(subDir, "pyproject.toml")); err == nil {
			return "python3 -m unittest discover -s tests"
		}
		if _, err := os.Stat(filepath.Join(subDir, "requirements.txt")); err == nil {
			return "python3 -m unittest discover -s tests"
		}
		if _, err := os.Stat(filepath.Join(subDir, "setup.py")); err == nil {
			return "python3 -m unittest discover -s tests"
		}
		if _, err := os.Stat(filepath.Join(subDir, "go.mod")); err == nil {
			return "go test -v ./..."
		}
	}
	return ""
}

// DetectSubdirectoryBuildCommand scans immediate subdirectories of projectPath for project manifests
// and returns the corresponding build command if found.
func DetectSubdirectoryBuildCommand(projectPath string) string {
	for _, subDir := range listCandidateSubdirectories(projectPath) {
		if _, err := os.Stat(filepath.Join(subDir, "Makefile")); err == nil {
			content, rErr := os.ReadFile(filepath.Join(subDir, "Makefile"))
			if rErr == nil {
				s := string(content)
				if strings.Contains(s, "build:") || strings.Contains(s, "build :") {
					return "make build"
				}
				if strings.Contains(s, "all:") || strings.Contains(s, "all :") {
					return "make all"
				}
			}
		}
		if _, err := os.Stat(filepath.Join(subDir, "Cargo.toml")); err == nil {
			return "cargo check"
		}
		if _, err := os.Stat(filepath.Join(subDir, "package.json")); err == nil {
			content, rErr := os.ReadFile(filepath.Join(subDir, "package.json"))
			if rErr == nil && strings.Contains(string(content), "\"build\"") {
				return "npm run build"
			}
		}
		if _, err := os.Stat(filepath.Join(subDir, "go.mod")); err == nil {
			return "go build ./..."
		}
	}
	return ""
}

// DetectSubdirectoryFormatterCommand scans immediate subdirectories of projectPath for project manifests
// and returns the corresponding code formatter command if found.
func DetectSubdirectoryFormatterCommand(projectPath string) string {
	for _, subDir := range listCandidateSubdirectories(projectPath) {
		if _, err := os.Stat(filepath.Join(subDir, "Makefile")); err == nil {
			content, rErr := os.ReadFile(filepath.Join(subDir, "Makefile"))
			if rErr == nil {
				s := string(content)
				if strings.Contains(s, "format:") || strings.Contains(s, "format :") {
					return "make format"
				}
				if strings.Contains(s, "fmt:") || strings.Contains(s, "fmt :") {
					return "make fmt"
				}
			}
		}
		if _, err := os.Stat(filepath.Join(subDir, "go.mod")); err == nil {
			return "go fmt ./..."
		}
		if _, err := os.Stat(filepath.Join(subDir, "Cargo.toml")); err == nil {
			return "cargo fmt"
		}
		if _, err := os.Stat(filepath.Join(subDir, "package.json")); err == nil {
			content, rErr := os.ReadFile(filepath.Join(subDir, "package.json"))
			if rErr == nil && strings.Contains(string(content), "\"format\"") {
				return "npm run format"
			}
		}
	}
	return ""
}
