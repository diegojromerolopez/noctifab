package services

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var makeTargetRegex = regexp.MustCompile(`^([a-zA-Z0-9_\-\.]+)\s*:`)

// MakefileInspector checks for target existence in Makefiles and provides fallback commands.
type MakefileInspector struct{}

// NewMakefileInspector creates a new MakefileInspector.
func NewMakefileInspector() *MakefileInspector {
	return &MakefileInspector{}
}

// HasTarget checks if a given target rule exists in Makefile within projectPath.
func (m *MakefileInspector) HasTarget(projectPath, target string) bool {
	makefilePath := filepath.Join(projectPath, "Makefile")
	if _, err := os.Stat(makefilePath); err != nil {
		// Also check GNUMakefile or makefile
		makefilePath = filepath.Join(projectPath, "makefile")
		if _, err := os.Stat(makefilePath); err != nil {
			makefilePath = filepath.Join(projectPath, "GNUMakefile")
			if _, err := os.Stat(makefilePath); err != nil {
				return false
			}
		}
	}

	targets, err := m.ParseTargets(makefilePath)
	if err != nil {
		return false
	}

	targetLower := strings.ToLower(strings.TrimSpace(target))
	for _, t := range targets {
		if strings.ToLower(t) == targetLower {
			return true
		}
	}
	return false
}

// ParseTargets extracts all target names from a Makefile.
func (m *MakefileInspector) ParseTargets(makefilePath string) ([]string, error) {
	file, err := os.Open(makefilePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var targets []string
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		// Ignore comment lines and indented recipes
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, " ") {
			continue
		}

		matches := makeTargetRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			targetName := strings.TrimSpace(matches[1])
			if !seen[targetName] && targetName != ".PHONY" && !strings.HasPrefix(targetName, ".") {
				seen[targetName] = true
				targets = append(targets, targetName)
			}
		}
	}

	return targets, scanner.Err()
}

// ResolveSafeMakeCommand inspects a command string like "make test" or "make build".
// If the target is missing in Makefile, it returns a safe fallback command for the project ecosystem
// or an empty string if the step should be gracefully skipped.
func (m *MakefileInspector) ResolveSafeMakeCommand(projectPath, command string) (string, error) {
	cmdTrim := strings.TrimSpace(command)
	if !strings.HasPrefix(cmdTrim, "make ") && cmdTrim != "make" {
		return command, nil
	}

	parts := strings.Fields(cmdTrim)
	target := "all"
	for _, p := range parts[1:] {
		if !strings.HasPrefix(p, "-") && !strings.Contains(p, "=") {
			target = p
			break
		}
	}

	if m.HasTarget(projectPath, target) {
		return command, nil
	}

	// Target is missing: determine fallback based on target and project ecosystem
	fmt.Printf("⚠️  [Makefile Preflight] Target %q not found in Makefile at %s; finding native fallback...\n", target, projectPath)

	switch target {
	case "test", "test-all", "check":
		fallback := detectNativeTestFallback(projectPath)
		if fallback != "" {
			fmt.Printf("ℹ [Makefile Preflight] Falling back from 'make %s' to %q\n", target, fallback)
			return fallback, nil
		}
		return "", fmt.Errorf("make target %q missing and no native test fallback detected", target)

	case "build", "compile", "all":
		fallback := detectNativeBuildFallback(projectPath)
		if fallback != "" {
			fmt.Printf("ℹ [Makefile Preflight] Falling back from 'make %s' to %q\n", target, fallback)
			return fallback, nil
		}
		// Graceful bypass for build if no fallback
		return "", nil

	case "format", "fmt":
		fallback := detectNativeFormatFallback(projectPath)
		return fallback, nil

	case "lint":
		// Linters are optional quality gates per AGENTS.md §13; bypass if recipe is missing
		fmt.Printf("ℹ [Makefile Preflight] Optional quality gate 'make lint' recipe missing; gracefully skipping.\n")
		return "", nil

	default:
		return command, nil
	}
}

func detectNativeTestFallback(projectPath string) string {
	if _, err := os.Stat(filepath.Join(projectPath, "Cargo.toml")); err == nil {
		return "cargo test"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
		return "go test -v ./..."
	}
	if _, err := os.Stat(filepath.Join(projectPath, "package.json")); err == nil {
		return "npm test"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "pytest.ini")); err == nil {
		return "pytest"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "pyproject.toml")); err == nil {
		return "pytest"
	}
	// Check for C# (.csproj)
	matches, _ := filepath.Glob(filepath.Join(projectPath, "*.csproj"))
	if len(matches) > 0 {
		return "dotnet test"
	}
	matches, _ = filepath.Glob(filepath.Join(projectPath, "*", "*.csproj"))
	if len(matches) > 0 {
		return "dotnet test"
	}
	return ""
}

func detectNativeBuildFallback(projectPath string) string {
	if _, err := os.Stat(filepath.Join(projectPath, "Cargo.toml")); err == nil {
		return "cargo check"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
		return "go build ./..."
	}
	matches, _ := filepath.Glob(filepath.Join(projectPath, "*.csproj"))
	if len(matches) > 0 {
		return "dotnet build"
	}
	matches, _ = filepath.Glob(filepath.Join(projectPath, "*", "*.csproj"))
	if len(matches) > 0 {
		return "dotnet build"
	}
	return ""
}

func detectNativeFormatFallback(projectPath string) string {
	if _, err := os.Stat(filepath.Join(projectPath, "Cargo.toml")); err == nil {
		return "cargo fmt"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
		return "go fmt ./..."
	}
	return ""
}
