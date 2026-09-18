package services

import (
	"path/filepath"
	"strings"
)

// isMakefile reports whether path points to a Makefile or Makefile include (.mk).
func isMakefile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "makefile" || strings.HasSuffix(base, ".mk")
}

// normalizeMakefileTabs ensures recipe lines in Makefiles start with a tab (\t).
// LLMs frequently output Makefile recipe lines indented with spaces (e.g. 4 spaces),
// causing GNU Make to fail with "Makefile:x: *** missing separator. Stop.".
func normalizeMakefileTabs(path string, content string) string {
	if !isMakefile(path) {
		return content
	}

	lines := strings.Split(content, "\n")
	inRule := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// A target definition line starts at column 0 (no leading spaces/tabs) and contains a colon ':'
		// excluding variable assignment ':='
		hasLeadingSpace := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
		if !hasLeadingSpace && isTargetDefinition(line) {
			inRule = true
			continue
		}

		if inRule && hasLeadingSpace {
			// Convert leading spaces/tabs into a single standard Makefile tab prefix
			lines[i] = "\t" + strings.TrimLeft(line, " \t")
		} else if !hasLeadingSpace {
			// Top-level item (variable assignment, directive, etc.) terminates the recipe block unless it's a new target
			inRule = isTargetDefinition(line)
		}
	}

	return strings.Join(lines, "\n")
}

// isTargetDefinition reports whether a top-level line defines a Makefile rule target.
func isTargetDefinition(line string) bool {
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return false
	}
	// Check for variable assignment ':='
	if len(line) > idx+1 && line[idx+1] == '=' {
		return false
	}
	// Ensure the text before ':' looks like a target (no spaces or valid target name)
	left := strings.TrimSpace(line[:idx])
	if left == "" {
		return false
	}
	// Special directives like 'include', 'vpath', 'export' are not targets
	lowerLeft := strings.ToLower(left)
	if strings.HasPrefix(lowerLeft, "include") || strings.HasPrefix(lowerLeft, "vpath") || strings.HasPrefix(lowerLeft, "export") {
		return false
	}
	return true
}

// StandardizeMakefile ensures that a Makefile contains standard GNU Make targets:
// .PHONY: all build test clean run
// If targets like 'all', 'build', 'test', 'clean', or 'run' are missing, it supplements them
// with appropriate aliases and defaults, and normalizes tabs.
func StandardizeMakefile(content string) string {
	content = normalizeMakefileTabs("Makefile", content)
	lines := strings.Split(content, "\n")
	targets := make(map[string]bool)
	hasPhony := false
	var existingPhonies []string

	for _, line := range lines {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ".PHONY:") {
			hasPhony = true
			fields := strings.Fields(strings.TrimPrefix(trimmed, ".PHONY:"))
			existingPhonies = append(existingPhonies, fields...)
			continue
		}
		if isTargetDefinition(line) {
			idx := strings.Index(line, ":")
			targetName := strings.TrimSpace(line[:idx])
			for _, t := range strings.Fields(targetName) {
				targets[t] = true
			}
		}
	}

	var appends []string
	if !targets["all"] && targets["build"] {
		appends = append(appends, "all: build")
		targets["all"] = true
	} else if targets["all"] && !targets["build"] {
		appends = append(appends, "build: all")
		targets["build"] = true
	} else if !targets["all"] && !targets["build"] {
		if targets["test"] {
			appends = append(appends, "all: build\n\nbuild: test")
		} else {
			appends = append(appends, "all: build\n\nbuild:\n\t@echo \"Build complete.\"")
		}
		targets["all"] = true
		targets["build"] = true
	}

	if !targets["test"] {
		appends = append(appends, "test:\n\t@echo \"No test target defined.\"")
		targets["test"] = true
	}

	if !targets["clean"] {
		appends = append(appends, "clean:\n\t@rm -rf build dist *.pyc __pycache__ .pytest_cache 2>/dev/null || true")
		targets["clean"] = true
	}

	if !targets["run"] {
		appends = append(appends, "run:\n\t@echo \"No run target specified.\"")
		targets["run"] = true
	}

	standardPhonies := []string{"all", "build", "test", "clean", "run"}
	phonySet := make(map[string]bool)
	for _, p := range existingPhonies {
		phonySet[p] = true
	}
	var newPhonies []string
	for _, p := range standardPhonies {
		if !phonySet[p] {
			newPhonies = append(newPhonies, p)
			phonySet[p] = true
		}
	}

	var result strings.Builder
	if !hasPhony {
		result.WriteString(".PHONY: " + strings.Join(standardPhonies, " ") + "\n\n")
	} else if len(newPhonies) > 0 {
		result.WriteString(".PHONY: " + strings.Join(newPhonies, " ") + "\n")
	}

	result.WriteString(content)
	if len(appends) > 0 {
		if !strings.HasSuffix(content, "\n") {
			result.WriteString("\n")
		}
		result.WriteString("\n# Standardized Makefile targets added by Noctifab\n")
		result.WriteString(strings.Join(appends, "\n\n"))
		result.WriteString("\n")
	}

	return normalizeMakefileTabs("Makefile", result.String())
}

