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
