package services

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ValidateTaskCohesion enforces that no DAG task is defined purely for interfaces,
// isolated typedefs/micro-structs, or abstractions without including its corresponding
// concrete implementation file and co-located tests, and prevents monolithic mega-tasks.
func ValidateTaskCohesion(tasks []domain.Task) error {
	for _, task := range tasks {
		if isInterfaceOnlyTask(task) {
			return fmt.Errorf("task cohesion violation in task %q (%s): interface definitions must be paired with implementation files in target_files", task.ID, task.Title)
		}
		if isMicroTaskViolation(task) {
			return fmt.Errorf("task granularity violation in task %q (%s): task is a micro-task (< 4 CU) and must be merged into an adjacent implementation task", task.ID, task.Title)
		}
		if isMega, reason := isMegaTaskViolation(task); isMega {
			return fmt.Errorf("task granularity violation in task %q (%s): task is a mega-task (%s); must be decomposed into smaller, modular sibling tasks", task.ID, task.Title, reason)
		}
	}
	return nil
}

func isInterfaceOnlyTask(task domain.Task) bool {
	if len(task.TargetFiles) == 0 {
		return false
	}

	interfaceOnly := true
	hasInterfaceFile := false

	for _, file := range task.TargetFiles {
		base := strings.ToLower(filepath.Base(file))
		if isInterfaceFilename(base) {
			hasInterfaceFile = true
		} else {
			interfaceOnly = false
		}
	}

	if hasInterfaceFile && interfaceOnly {
		// If task title or description explicitly indicates interface-only without implementation
		titleLower := strings.ToLower(task.Title)
		if strings.Contains(titleLower, "interface") || strings.Contains(titleLower, "stub") || strings.Contains(titleLower, "definition") {
			return true
		}
		return true
	}

	return false
}

func isInterfaceFilename(base string) bool {
	return strings.HasSuffix(base, "_interface.go") ||
		strings.HasSuffix(base, "_interfaces.go") ||
		base == "interface.go" ||
		base == "interfaces.go" ||
		strings.HasSuffix(base, "_stub.go")
}

func isMicroTaskViolation(task domain.Task) bool {
	titleLower := strings.ToLower(task.Title)

	isTypeDefTitle := strings.HasPrefix(titleLower, "define struct") ||
		strings.HasPrefix(titleLower, "create struct") ||
		strings.HasPrefix(titleLower, "define typedef") ||
		strings.HasPrefix(titleLower, "create type definition") ||
		strings.HasPrefix(titleLower, "define interface")

	if isTypeDefTitle && len(task.Description) < 100 && len(task.TargetFiles) <= 1 {
		return true
	}
	return false
}

// isMegaTaskViolation checks if a task is overly monolithic (> 6 production files or enumerating > 8 command/operation keywords).
func isMegaTaskViolation(task domain.Task) (bool, string) {
	// 1. Check non-test target files count (> 6 production files is too broad for a single task)
	nonTestCount := 0
	for _, f := range task.TargetFiles {
		clean := strings.ToLower(filepath.ToSlash(f))
		base := filepath.Base(clean)
		if isCohesionTestFile(base) || isCohesionDocOrConfig(base) {
			continue
		}
		nonTestCount++
	}
	if nonTestCount > 6 {
		return true, fmt.Sprintf("task targets %d production files (maximum 6 allowed per task)", nonTestCount)
	}

	// 2. Check excessive command/operation enumeration in description or title (> 8 command keywords)
	cmdCount := countEnumeratedCommands(task.Title + " " + task.Description)
	if cmdCount > 8 {
		return true, fmt.Sprintf("task enumerates %d distinct commands/operations (maximum 8 allowed per task)", cmdCount)
	}

	return false, ""
}

func isCohesionTestFile(base string) bool {
	return strings.HasPrefix(base, "test_") ||
		strings.HasSuffix(base, "_test.go") ||
		strings.HasSuffix(base, "_test.py") ||
		strings.HasSuffix(base, "_test.rs") ||
		strings.HasSuffix(base, "_test.ts") ||
		strings.HasSuffix(base, "_test.js") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".spec.js")
}

func isCohesionDocOrConfig(base string) bool {
	return base == "makefile" ||
		base == "gnumakefile" ||
		base == "pyproject.toml" ||
		base == "package.json" ||
		base == "cargo.toml" ||
		base == "go.mod" ||
		base == "go.sum" ||
		base == ".gitignore" ||
		base == "readme.md" ||
		strings.HasSuffix(base, ".md")
}

var nonCommandWords = map[string]bool{
	"CRITICAL": true, "MANDATE": true, "TASK": true, "DAG": true,
	"JSON": true, "YAML": true, "XML": true, "HTML": true,
	"HTTP": true, "TCP": true, "UDP": true, "IP": true,
	"REST": true, "RESP": true, "RESP2": true, "RESP3": true,
	"API": true, "CLI": true, "E2E": true, "QA": true,
	"LLM": true, "CU": true, "DOD": true, "AOF": true,
	"RDB": true, "TTL": true, "UTC": true, "ID": true,
	"URL": true, "URI": true, "UUID": true, "OK": true,
	"FAIL": true, "TRUE": true, "FALSE": true, "NONE": true,
	"SOLID": true, "DDD": true, "AST": true, "VCS": true,
}

var commandWordRegex = regexp.MustCompile(`\b[A-Z][A-Z0-9_]{1,14}\b`)

func countEnumeratedCommands(text string) int {
	matches := commandWordRegex.FindAllString(text, -1)
	if len(matches) == 0 {
		return 0
	}
	seen := make(map[string]bool)
	for _, m := range matches {
		if !nonCommandWords[m] {
			seen[m] = true
		}
	}
	return len(seen)
}

