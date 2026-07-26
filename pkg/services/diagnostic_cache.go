package services

import (
	"fmt"
)

// TaskDiagnosticCache manages in-memory caching for diagnostic tools (run_tests and run_linter)
// during an agent's execution loop. The cache is automatically invalidated whenever a file-mutating
// tool action is executed.
type TaskDiagnosticCache struct {
	isDirty          bool
	lastTestOutput   string
	lastTestErr      error
	hasTestResult    bool
	lastLinterOutput string
	lastLinterErr    error
	hasLinterResult  bool
}

// NewTaskDiagnosticCache constructs a new TaskDiagnosticCache instance.
func NewTaskDiagnosticCache() *TaskDiagnosticCache {
	return &TaskDiagnosticCache{
		isDirty: true,
	}
}

// OnToolExecuted records tool execution results and invalidates the cache when mutating actions occur.
func (c *TaskDiagnosticCache) OnToolExecuted(toolName string, output string, err error) {
	switch toolName {
	case "write_file", "edit_file", "multi_replace_file_content", "delete_file":
		if err == nil {
			c.isDirty = true
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
	}
}

// TryGetCachedResult checks if a valid cached result exists for run_tests or run_linter.
func (c *TaskDiagnosticCache) TryGetCachedResult(toolName string) (string, error, bool) {
	if c.isDirty {
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
