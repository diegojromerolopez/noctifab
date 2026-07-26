package services

import (
	"errors"
	"testing"
)

func TestTaskDiagnosticCache_RunTestsCaching(t *testing.T) {
	cache := NewTaskDiagnosticCache()

	// Initial check should be cache miss
	_, _, ok := cache.TryGetCachedResult("run_tests")
	if ok {
		t.Fatalf("expected cache miss on fresh cache, got hit")
	}

	// Record test run
	cache.OnToolExecuted("run_tests", "PASS: 5 tests passed", nil)

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
	cache.OnToolExecuted("edit_file", "File updated", nil)

	// Check after mutation should be a cache miss
	_, _, ok = cache.TryGetCachedResult("run_tests")
	if ok {
		t.Fatalf("expected cache miss after edit_file mutation, got hit")
	}
}

func TestTaskDiagnosticCache_RunLinterCaching(t *testing.T) {
	cache := NewTaskDiagnosticCache()

	cache.OnToolExecuted("run_linter", "No lint errors found", nil)

	out, err, ok := cache.TryGetCachedResult("run_linter")
	if !ok {
		t.Fatalf("expected cache hit for run_linter, got miss")
	}
	if err != nil || out == "" {
		t.Fatalf("unexpected cached result: out=%q, err=%v", out, err)
	}

	// Delete file mutation
	cache.OnToolExecuted("delete_file", "File removed", nil)

	_, _, ok = cache.TryGetCachedResult("run_linter")
	if ok {
		t.Fatalf("expected cache miss after delete_file, got hit")
	}
}

func TestTaskDiagnosticCache_ErrorResultCaching(t *testing.T) {
	cache := NewTaskDiagnosticCache()

	testErr := errors.New("command failed with exit code 1")
	cache.OnToolExecuted("run_tests", "FAIL: 1 test failed", testErr)

	out, err, ok := cache.TryGetCachedResult("run_tests")
	if !ok {
		t.Fatalf("expected cache hit for failing test run, got miss")
	}
	if err != testErr {
		t.Fatalf("expected cached error %v, got %v", testErr, err)
	}
	if out == "" {
		t.Fatalf("expected non-empty output for failed run")
	}
}
