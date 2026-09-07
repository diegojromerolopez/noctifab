package services

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestTaskDiagnosticCache_RunTestsCaching(t *testing.T) {
	cache := NewTaskDiagnosticCache(true)

	// Initial check should be cache miss
	_, _, ok := cache.TryGetCachedResult("run_tests")
	if ok {
		t.Fatalf("expected cache miss on fresh cache, got hit")
	}

	// Record test run
	cache.OnToolExecuted("run_tests", nil, "PASS: 5 tests passed", nil)

	// Next check without mutations should be a cache hit
	out, err, ok := cache.TryGetCachedResult("run_tests")
	if !ok {
		t.Fatalf("expected cache hit after run_tests, got miss")
	}
	if err != nil {
		t.Fatalf("expected nil error on cached result, got: %v", err)
	}
	if out == "" {
		t.Fatalf("expected non-empty cached output")
	}

	// Mutate workspace via edit_file
	cache.OnToolExecuted("edit_file", map[string]any{"path": "file.go"}, "File updated", nil)

	// Check after mutation should be a cache miss
	_, _, ok = cache.TryGetCachedResult("run_tests")
	if ok {
		t.Fatalf("expected cache miss after edit_file mutation, got hit")
	}
}

func TestTaskDiagnosticCache_RunLinterCaching(t *testing.T) {
	cache := NewTaskDiagnosticCache(true)

	cache.OnToolExecuted("run_linter", nil, "No lint errors found", nil)

	out, err, ok := cache.TryGetCachedResult("run_linter")
	if !ok {
		t.Fatalf("expected cache hit for run_linter, got miss")
	}
	if err != nil || out == "" {
		t.Fatalf("unexpected cached result: out=%q, err=%v", out, err)
	}

	// Delete file mutation
	cache.OnToolExecuted("delete_file", map[string]any{"path": "file.go"}, "File removed", nil)

	_, _, ok = cache.TryGetCachedResult("run_linter")
	if ok {
		t.Fatalf("expected cache miss after delete_file, got hit")
	}

	// Test error failure caching
	cache.OnToolExecuted("run_linter", nil, "linter failed", errors.New("exit code 1"))
	out, err, ok = cache.TryGetCachedResult("run_linter")
	if !ok || err == nil {
		t.Fatalf("expected cached linter error, got out=%q err=%v ok=%v", out, err, ok)
	}
}

func TestTaskDiagnosticCache_InspectionCaching(t *testing.T) {
	cache := NewTaskDiagnosticCache(true)
	args := map[string]any{"path": "src"}

	// Initial check miss
	_, _, ok := cache.TryGetCachedInspection("list_directory", args)
	if ok {
		t.Fatalf("expected cache miss on fresh cache, got hit")
	}

	// Record list_directory result
	cache.OnToolExecuted("list_directory", args, "src/main.c\nsrc/quote.c", nil)

	// Next check should be hit
	out, err, ok := cache.TryGetCachedInspection("list_directory", args)
	if !ok {
		t.Fatalf("expected cache hit for list_directory, got miss")
	}
	if err != nil || out == "" {
		t.Fatalf("unexpected cached result: out=%q err=%v", out, err)
	}

	// Mutate workspace via write_file
	cache.OnToolExecuted("write_file", map[string]any{"path": "src/new.c"}, "file written", nil)

	// Check after mutation should be a cache miss
	_, _, ok = cache.TryGetCachedInspection("list_directory", args)
	if ok {
		t.Fatalf("expected cache miss after write_file mutation, got hit")
	}
}

func TestTaskDiagnosticCache_Disabled(t *testing.T) {
	cache := NewTaskDiagnosticCache(false)
	args := map[string]any{"path": "src"}

	cache.OnToolExecuted("list_directory", args, "src/main.c", nil)
	_, _, ok := cache.TryGetCachedInspection("list_directory", args)
	if ok {
		t.Fatalf("expected cache miss when cache is disabled, got hit")
	}
}

func TestTaskDiagnosticCache_FailedEditInvalidatesCache(t *testing.T) {
	cache := NewTaskDiagnosticCache(true)

	// Record a successful linter run — cache is now warm.
	cache.OnToolExecuted("run_linter", nil, "No lint errors found", nil)
	_, _, ok := cache.TryGetCachedResult("run_linter")
	if !ok {
		t.Fatalf("expected cache hit after run_linter, got miss")
	}

	// A FAILED edit_file (target_content not found) must also dirty the cache.
	cache.OnToolExecuted("edit_file", map[string]any{"path": "app/main.py"}, "edit_file failed: target_content not found", errors.New("target_content not found"))

	_, _, ok = cache.TryGetCachedResult("run_linter")
	if ok {
		t.Fatalf("expected cache miss after failed edit_file, got stale hit — this was the root cause of the linter lock-in bug")
	}
}

func TestTaskDiagnosticCache_FailedWriteInvalidatesCache(t *testing.T) {
	cache := NewTaskDiagnosticCache(true)

	// Record a successful test run.
	cache.OnToolExecuted("run_tests", nil, "PASS: 10 tests passed", nil)
	_, _, ok := cache.TryGetCachedResult("run_tests")
	if !ok {
		t.Fatalf("expected cache hit after run_tests, got miss")
	}

	// A FAILED write_file must also dirty the cache.
	cache.OnToolExecuted("write_file", map[string]any{"path": "app/foo.py"}, "", errors.New("permission denied"))

	_, _, ok = cache.TryGetCachedResult("run_tests")
	if ok {
		t.Fatalf("expected cache miss after failed write_file, got stale hit")
	}
}

func TestTaskDiagnosticCache_SHA256DiskIntegrityVerification(t *testing.T) {
	cache := NewTaskDiagnosticCache(true)
	tmpDir := t.TempDir()
	specFile := tmpDir + "/SPEC.md"

	// Write initial SPEC.md on disk
	initialContent := "# Project Specification\nSection 1: Overview"
	if err := os.WriteFile(specFile, []byte(initialContent), 0600); err != nil {
		t.Fatalf("failed to write spec file: %v", err)
	}

	// Seed the cache with the prompt context content
	cache.SeedFileContent(specFile, initialContent)

	// 1. Check with unmodified file: should hit cache with SHA-256 verified notice
	out, err, ok := cache.TryGetCachedInspection("read_file", map[string]any{"path": specFile})
	if !ok || err != nil {
		t.Fatalf("expected cache hit for unmodified seeded file, got ok=%v err=%v", ok, err)
	}
	if !strings.Contains(out, "[Cached - SHA256 Verified Unmodified]") {
		t.Fatalf("expected SHA-256 verified cached notice, got: %q", out)
	}

	// 2. Modify file on disk directly
	modifiedContent := "# Project Specification\nSection 1: Overview\nSection 2: Architecture"
	if err := os.WriteFile(specFile, []byte(modifiedContent), 0600); err != nil {
		t.Fatalf("failed to write modified spec file: %v", err)
	}

	// 3. Check again: SHA-256 mismatch must invalidate cache entry and return miss
	_, _, ok = cache.TryGetCachedInspection("read_file", map[string]any{"path": specFile})
	if ok {
		t.Fatalf("expected cache miss after disk modification changed SHA-256 checksum, got hit")
	}
}

func TestIsFileDependentToolAndIsMutatingTool(t *testing.T) {
	fileDependent := []string{"read_file", "find_files", "list_directory", "grep_search", "run_tests", "run_linter"}
	for _, tool := range fileDependent {
		if !IsFileDependentTool(tool) {
			t.Errorf("expected %s to be file dependent", tool)
		}
	}

	if IsFileDependentTool("noop") || IsFileDependentTool("request_test_fix") {
		t.Errorf("noop and request_test_fix should not be file dependent")
	}

	mutating := []string{"write_file", "edit_file", "multi_replace_file_content", "apply_patch", "delete_file"}
	for _, tool := range mutating {
		if !IsMutatingTool(tool) {
			t.Errorf("expected %s to be mutating tool", tool)
		}
	}

	if IsMutatingTool("read_file") || IsMutatingTool("run_tests") {
		t.Errorf("read_file and run_tests should not be mutating tools")
	}
}
