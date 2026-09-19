package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareTestEnvironment(t *testing.T) {
	t.Run("when python test files exist in subdirectories it auto-scaffolds __init__.py", func(t *testing.T) {
		tmpDir := t.TempDir()
		unitDir := filepath.Join(tmpDir, "tests", "unit")
		if err := os.MkdirAll(unitDir, 0755); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(unitDir, "test_sample.py"), []byte("def test_one(): pass\n"), 0644)

		if err := PrepareTestEnvironment(tmpDir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		initFile := filepath.Join(unitDir, "__init__.py")
		if _, err := os.Stat(initFile); os.IsNotExist(err) {
			t.Errorf("expected %s to be created, but it does not exist", initFile)
		}
	})

	t.Run("when __init__.py already exists it does not overwrite it", func(t *testing.T) {
		tmpDir := t.TempDir()
		unitDir := filepath.Join(tmpDir, "tests", "unit")
		_ = os.MkdirAll(unitDir, 0755)
		_ = os.WriteFile(filepath.Join(unitDir, "test_sample.py"), []byte("def test_one(): pass\n"), 0644)
		_ = os.WriteFile(filepath.Join(unitDir, "__init__.py"), []byte("# custom init"), 0644)

		_ = PrepareTestEnvironment(tmpDir)

		content, _ := os.ReadFile(filepath.Join(unitDir, "__init__.py"))
		if string(content) != "# custom init" {
			t.Errorf("expected original content to be preserved, got %q", string(content))
		}
	})

	t.Run("when no test directory exists it returns nil without error", func(t *testing.T) {
		tmpDir := t.TempDir()
		if err := PrepareTestEnvironment(tmpDir); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestDiscoverTestFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Go test
	_ = os.MkdirAll(filepath.Join(tmpDir, "pkg"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "pkg", "sample_test.go"), []byte("package pkg"), 0644)

	// Python test
	_ = os.MkdirAll(filepath.Join(tmpDir, "tests", "unit"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "tests", "unit", "test_app.py"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "tests", "unit", "__init__.py"), []byte(""), 0644)

	// Rust test
	_ = os.MkdirAll(filepath.Join(tmpDir, "tests"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "tests", "integration_test.rs"), []byte(""), 0644)

	// TypeScript test
	_ = os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "src", "app.test.ts"), []byte(""), 0644)

	// Ignored folder
	_ = os.MkdirAll(filepath.Join(tmpDir, "node_modules", "lib"), 0755)
	_ = os.WriteFile(filepath.Join(tmpDir, "node_modules", "lib", "foo.test.js"), []byte(""), 0644)

	discovered := DiscoverTestFiles(tmpDir)
	if len(discovered) != 4 {
		t.Fatalf("expected 4 discovered test files, got %d: %v", len(discovered), discovered)
	}
}

func TestEvaluateTestExecution(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("when output contains zero tests ran it reports failure", func(t *testing.T) {
		zero, msg := EvaluateTestExecution(tmpDir, "Ran 0 tests in 0.000s\nOK")
		if !zero {
			t.Errorf("expected zero=true, got false")
		}
		if msg == "" {
			t.Errorf("expected non-empty failure message")
		}
	})

	t.Run("when output contains cargo 0 passed it reports failure", func(t *testing.T) {
		zero, _ := EvaluateTestExecution(tmpDir, "test result: ok. 0 passed; 0 failed")
		if !zero {
			t.Errorf("expected zero=true, got false")
		}
	})

	t.Run("when output contains make nothing to be done it reports failure", func(t *testing.T) {
		zero, msg := EvaluateTestExecution(tmpDir, "make: Nothing to be done for `e2e'.")
		if !zero {
			t.Errorf("expected zero=true for make nothing to be done, got false")
		}
		if msg == "" {
			t.Errorf("expected non-empty failure message")
		}
	})

	t.Run("when output is empty and test files exist it reports failure", func(t *testing.T) {
		projDir := t.TempDir()
		_ = os.MkdirAll(filepath.Join(projDir, "tests"), 0755)
		_ = os.WriteFile(filepath.Join(projDir, "tests", "test_foo.py"), []byte(""), 0644)

		zero, msg := EvaluateTestExecution(projDir, "   ")
		if !zero {
			t.Errorf("expected zero=true on empty output with test files")
		}
		if msg == "" {
			t.Errorf("expected failure message on empty output")
		}
	})

	t.Run("when test discovery discrepancy occurs with multiple unreferenced test files it reports failure", func(t *testing.T) {
		projDir := t.TempDir()
		_ = os.MkdirAll(filepath.Join(projDir, "tests", "unit"), 0755)
		_ = os.WriteFile(filepath.Join(projDir, "tests", "test_discovery.py"), []byte(""), 0644)
		_ = os.WriteFile(filepath.Join(projDir, "tests", "unit", "test_store.py"), []byte(""), 0644)
		_ = os.WriteFile(filepath.Join(projDir, "tests", "unit", "test_resp.py"), []byte(""), 0644)
		_ = os.WriteFile(filepath.Join(projDir, "tests", "unit", "test_server.py"), []byte(""), 0644)

		// Runner output only mentions test_discovery.py with Ran 1 test
		runnerOut := "test_unittest_discovery_finds_test_cases (test_discovery.TestSuiteDiscovery) ... ok\nRan 1 test in 0.001s\nOK\n"

		zero, msg := EvaluateTestExecution(projDir, runnerOut)
		if !zero {
			t.Errorf("expected discrepancy detection to fail, got zero=false")
		}
		if msg == "" || !containsAny(msg, "discrepancy", "unreferenced") {
			t.Errorf("expected discrepancy message, got %q", msg)
		}
	})

	t.Run("when normal tests pass across files it succeeds", func(t *testing.T) {
		projDir := t.TempDir()
		_ = os.MkdirAll(filepath.Join(projDir, "tests"), 0755)
		_ = os.WriteFile(filepath.Join(projDir, "tests", "test_a.py"), []byte(""), 0644)
		_ = os.WriteFile(filepath.Join(projDir, "tests", "test_b.py"), []byte(""), 0644)

		runnerOut := "test_a.py ... ok\ntest_b.py ... ok\nRan 2 tests in 0.005s\nOK\n"

		zero, _ := EvaluateTestExecution(projDir, runnerOut)
		if zero {
			t.Errorf("expected zero=false for valid multi-test run")
		}
	})
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
