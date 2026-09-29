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
	"CRITICAL": true, "MANDATE": true, "TASK": true, "TASKS": true, "DAG": true,
	"JSON": true, "YAML": true, "XML": true, "HTML": true, "CSV": true, "SQL": true,
	"HTTP": true, "HTTPS": true, "TCP": true, "UDP": true, "IP": true, "SSH": true,
	"REST": true, "RESP": true, "RESP2": true, "RESP3": true, "RPC": true, "GRPC": true,
	"API": true, "APIS": true, "CLI": true, "E2E": true, "QA": true, "CI": true, "CD": true,
	"LLM": true, "CU": true, "DOD": true, "AOF": true, "RDB": true, "TTL": true, "UTC": true,
	"ID": true, "IDS": true, "URL": true, "URI": true, "UUID": true, "OK": true, "ERR": true,
	"FAIL": true, "FAILED": true, "TRUE": true, "FALSE": true, "NONE": true, "NIL": true, "NULL": true,
	"SOLID": true, "DDD": true, "AST": true, "VCS": true, "PR": true, "OS": true, "IO": true,
	"ASCII": true, "UTF8": true, "POSIX": true, "FIFO": true, "LRU": true, "LFU": true, "CPU": true, "RAM": true, "EOF": true, "OOM": true,
	// Common English & Markdown heading keywords in uppercase
	"REQUIREMENTS": true, "REQUIREMENT": true, "GOAL": true, "GOALS": true, "SCOPE": true,
	"NOTE": true, "NOTES": true, "IMPORTANT": true, "WARNING": true, "TODO": true, "FIXME": true,
	"ERROR": true, "ERRORS": true, "SUCCESS": true, "PENDING": true, "RUN": true, "RUNS": true,
	"TEST": true, "TESTS": true, "STDOUT": true, "STDERR": true, "EXIT": true, "CODE": true,
	"INPUT": true, "INPUTS": true, "OUTPUT": true, "OUTPUTS": true, "FILES": true, "FILE": true,
	"DEPENDENCIES": true, "DEPENDS": true, "PLAN": true, "STATUS": true, "DESCRIPTION": true, "TITLE": true,
	"STEP": true, "STEPS": true, "CHECK": true, "CHECKS": true, "EXPECTED": true, "ACTUAL": true,
	"RESULT": true, "RESULTS": true, "ASSERT": true, "ASSERTION": true, "ASSERTIONS": true,
	"RETURN": true, "RETURNS": true, "RAISE": true, "RAISES": true, "EXCEPT": true, "CATCH": true,
	"OPTION": true, "OPTIONS": true, "FLAG": true, "FLAGS": true, "VALUE": true, "VALUES": true,
	"KEY": true, "KEYS": true, "TYPE": true, "TYPES": true, "DATA": true, "SYNTAX": true,
	"STORE": true, "SERVER": true, "CLIENT": true, "SOCKET": true, "BUFFER": true, "STREAM": true,
	"MUST": true, "SHOULD": true, "NOT": true, "ONLY": true, "AND": true, "OR": true, "FOR": true,
	"IN": true, "OF": true, "WITH": true, "WITHOUT": true, "BY": true, "ALL": true, "ANY": true,
	// Common protocol argument flags / modifiers (not commands themselves)
	"EX": true, "PX": true, "NX": true, "XX": true, "KEEPTTL": true, "EXAT": true, "PXAT": true,
	"GT": true, "LT": true, "CH": true, "WITHSCORES": true, "LIMIT": true, "WEIGHTS": true,
	"AGGREGATE": true, "REV": true, "BYLEX": true, "BYSCORE": true, "COUNT": true, "MATCH": true,
}

var (
	commandWordRegex    = regexp.MustCompile(`\b[A-Z][A-Z0-9_]{1,14}\b`)
	commandListSeqRegex = regexp.MustCompile(`(?i)(?:implement|commands?|operations?|endpoints?|support|verbs?)[:\s]+([A-Z0-9_,\s\(\)\/]+?)(?:\.|\n|;|$)`)
	commaSeparatedRegex = regexp.MustCompile(`\b[A-Z][A-Z0-9_]{1,14}\b(?:,\s*(?:and\s+)?\b[A-Z][A-Z0-9_]{1,14}\b)+`)
	bulletItemCmdRegex  = regexp.MustCompile(`(?m)^\s*[-*•\d.]+\s*(?:` + "`" + `)?([A-Z][A-Z0-9_]{1,14})(?:` + "`" + `)?(?:\s*[:(]|$)`)
)

func countEnumeratedCommands(titleAndDesc string) int {
	seen := make(map[string]bool)

	// 1. Extract from Title (titles are concise and explicitly name target commands)
	parts := strings.Split(titleAndDesc, "\n")
	title := parts[0]
	for _, m := range commandWordRegex.FindAllString(title, -1) {
		if !nonCommandWords[m] {
			seen[m] = true
		}
	}

	desc := titleAndDesc
	if len(parts) > 1 {
		desc = strings.Join(parts[1:], "\n")
	}

	// 2. Extract from explicit command list phrases in Description (e.g. "Implement A, B, C", "commands: A, B, C")
	for _, match := range commandListSeqRegex.FindAllStringSubmatch(desc, -1) {
		if len(match) > 1 {
			for _, word := range commandWordRegex.FindAllString(match[1], -1) {
				if !nonCommandWords[word] {
					seen[word] = true
				}
			}
		}
	}

	// 3. Extract from comma-separated sequences of uppercase tokens (e.g. "AUTH, BGSAVE, BGREWRITEAOF, CLIENT, COMMAND")
	for _, seq := range commaSeparatedRegex.FindAllString(desc, -1) {
		for _, word := range commandWordRegex.FindAllString(seq, -1) {
			if !nonCommandWords[word] {
				seen[word] = true
			}
		}
	}

	// 4. Extract from bullet items (e.g. "- PING: check connection", "- ECHO: echo argument")
	for _, match := range bulletItemCmdRegex.FindAllStringSubmatch(desc, -1) {
		if len(match) > 1 && !nonCommandWords[match[1]] {
			seen[match[1]] = true
		}
	}

	return len(seen)
}
