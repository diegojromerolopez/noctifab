package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

type cachedReadResult struct {
	output     string
	checksum   string
	err        error
	isFromSeed bool
}

// computeSHA256 returns the hex-encoded SHA-256 checksum of a byte slice.
func computeSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// TaskDiagnosticCache manages in-memory caching for workspace filesystem inspection tools
// (list_directory, read_file, find_files, grep_search) and diagnostic tools (run_tests and run_linter).
// The cache is automatically invalidated whenever a file-mutating tool action is executed,
// and file content identity is cryptographically verified via SHA-256 checksums.
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
		isDirty:         false,
		inspectionCache: make(map[string]cachedReadResult),
	}
}

var fileContextRegex = regexp.MustCompile("(?s)(?:File|Project Manifest \\()([^\\):\\s]+)(?:\\)|:).*?```(?:\\w+)?\n(.*?)\n```")

// SeedFileContent seeds pre-read file content into the inspection cache with its SHA-256 checksum.
func (c *TaskDiagnosticCache) SeedFileContent(path string, content string) {
	if c == nil || !c.enabled || path == "" {
		return
	}
	key := buildArgsKey("read_file", map[string]any{"path": path})
	c.inspectionCache[key] = cachedReadResult{
		output:     content,
		checksum:   computeSHA256([]byte(content)),
		err:        nil,
		isFromSeed: true,
	}
	c.isDirty = false
}

// SeedContexts parses pre-loaded file contexts and seeds them into the inspection cache.
func (c *TaskDiagnosticCache) SeedContexts(contexts ...[]string) {
	if c == nil || !c.enabled {
		return
	}
	for _, ctxList := range contexts {
		for _, ctxStr := range ctxList {
			matches := fileContextRegex.FindAllStringSubmatch(ctxStr, -1)
			for _, m := range matches {
				if len(m) >= 3 {
					path := strings.TrimSpace(m[1])
					content := m[2]
					if path != "" && content != "" {
						c.SeedFileContent(path, content)
					}
				}
			}
		}
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
		// Invalidate cache regardless of success/failure: even a failed mutation
		// attempt means the agent is actively trying to change the workspace.
		// Stale cached diagnostic results (run_linter, run_tests) no longer
		// reflect the state the agent is reasoning about after any mutation attempt.
		c.isDirty = true
		c.inspectionCache = make(map[string]cachedReadResult)
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
			c.inspectionCache[key] = cachedReadResult{
				output:   output,
				checksum: computeSHA256([]byte(output)),
				err:      err,
			}
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

// TryGetCachedInspection checks if a valid cached inspection result exists for read-only tools,
// validating file integrity against disk SHA-256 checksums before serving.
func (c *TaskDiagnosticCache) TryGetCachedInspection(toolName string, args map[string]any) (string, error, bool) {
	if c == nil || !c.enabled || c.isDirty {
		return "", nil, false
	}
	key := buildArgsKey(toolName, args)
	res, found := c.inspectionCache[key]
	if !found {
		return "", nil, false
	}
	if toolName == "read_file" {
		path, _ := args["path"].(string)
		if path != "" {
			if diskContent, err := os.ReadFile(path); err == nil {
				diskChecksum := computeSHA256(diskContent)
				if diskChecksum != res.checksum {
					// File on disk has changed: invalidate cached entry to fetch fresh content
					delete(c.inspectionCache, key)
					return "", nil, false
				}
			}
		}
		if res.isFromSeed {
			return fmt.Sprintf("[Cached - SHA256 Verified Unmodified] File %q is already present in your initial prompt context above.", path), nil, true
		}
	}
	return fmt.Sprintf("[Cached Result - Workspace Unmodified]\n%s", res.output), res.err, true
}

// IsFileDependentTool returns true if the tool's execution depends on workspace files.
func IsFileDependentTool(tool string) bool {
	switch tool {
	case "read_file", "find_files", "list_directory", "grep_search", "run_tests", "run_linter":
		return true
	default:
		return false
	}
}

// IsMutatingTool returns true if the tool modifies workspace files.
func IsMutatingTool(tool string) bool {
	switch tool {
	case "write_file", "write_files", "edit_file", "multi_replace_file_content", "apply_patch", "delete_file":
		return true
	default:
		return false
	}
}
