package services

import (
	"fmt"
	"sort"
	"strings"
)

type cachedReadResult struct {
	output string
	err    error
}

// TaskDiagnosticCache manages in-memory caching for workspace filesystem inspection tools
// (list_directory, read_file, find_files, grep_search) and diagnostic tools (run_tests and run_linter).
// The cache is automatically invalidated whenever a file-mutating tool action is executed.
type TaskDiagnosticCache struct {
	enabled          bool
	isDirty          bool
	lastTestOutput   string
	lastTestErr      error
	hasTestResult    bool
	lastLinterOutput string
	lastLinterErr    error
	hasLinterResult  bool
	inspectionCache  map[string]cachedReadResult
}

// NewTaskDiagnosticCache constructs a new TaskDiagnosticCache instance.
func NewTaskDiagnosticCache(enabled bool) *TaskDiagnosticCache {
	return &TaskDiagnosticCache{
		enabled:         enabled,
		isDirty:         true,
		inspectionCache: make(map[string]cachedReadResult),
	}
}

func buildArgsKey(toolName string, args map[string]any) string {
	if len(args) == 0 {
		return toolName
	}
	var keys []string
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, args[k]))
	}
	return fmt.Sprintf("%s:%s", toolName, strings.Join(parts, "&"))
}

// OnToolExecuted records tool execution results and invalidates the cache when mutating actions occur.
func (c *TaskDiagnosticCache) OnToolExecuted(toolName string, args map[string]any, output string, err error) {
	if c == nil || !c.enabled {
		return
	}

	switch toolName {
	case "write_file", "edit_file", "multi_replace_file_content", "delete_file":
		if err == nil {
			c.isDirty = true
			c.inspectionCache = make(map[string]cachedReadResult)
		}
	case "run_tests":
		c.lastTestOutput = output
		c.lastTestErr = err
		c.hasTestResult = true
		c.isDirty = false
	case "run_linter":
		c.lastLinterOutput = output
		c.lastLinterErr = err
		c.hasLinterResult = true
		c.isDirty = false
	case "list_directory", "read_file", "find_files", "grep_search":
		if err == nil {
			key := buildArgsKey(toolName, args)
			c.inspectionCache[key] = cachedReadResult{output: output, err: err}
			c.isDirty = false
		}
	}
}

// TryGetCachedResult checks if a valid cached result exists for run_tests or run_linter.
func (c *TaskDiagnosticCache) TryGetCachedResult(toolName string) (string, error, bool) {
	if c == nil || !c.enabled || c.isDirty {
		return "", nil, false
	}
	if toolName == "run_tests" && c.hasTestResult {
		return fmt.Sprintf("[Cached Result - Workspace Unmodified]\n%s", c.lastTestOutput), c.lastTestErr, true
	}
	if toolName == "run_linter" && c.hasLinterResult {
		return fmt.Sprintf("[Cached Result - Workspace Unmodified]\n%s", c.lastLinterOutput), c.lastLinterErr, true
	}
	return "", nil, false
}

// TryGetCachedInspection checks if a valid cached inspection result exists for read-only tools.
func (c *TaskDiagnosticCache) TryGetCachedInspection(toolName string, args map[string]any) (string, error, bool) {
	if c == nil || !c.enabled || c.isDirty {
		return "", nil, false
	}
	key := buildArgsKey(toolName, args)
	res, found := c.inspectionCache[key]
	if !found {
		return "", nil, false
	}
	return fmt.Sprintf("[Cached Result - Workspace Unmodified]\n%s", res.output), res.err, true
}
