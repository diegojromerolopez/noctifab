package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrchestrator_AuditGeneratorFunctionalOutput(t *testing.T) {
	t.Run("when generator writes clean functional code, it returns no violations", func(t *testing.T) {
		tmpDir := t.TempDir()
		cleanPy := filepath.Join(tmpDir, "server.py")
		code := `
def start_server(port: int) -> bool:
    if port <= 0:
        raise ValueError("Invalid port")
    print(f"Server started on {port}")
    return True
`
		if err := os.WriteFile(cleanPy, []byte(code), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		orch := &Orchestrator{}
		violations := orch.auditGeneratorFunctionalOutput(tmpDir, []string{"server.py"})
		if len(violations) != 0 {
			t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
		}
	})

	t.Run("when generator writes hollow stubs, it detects violations and formats rejection message", func(t *testing.T) {
		tmpDir := t.TempDir()
		stubPy := filepath.Join(tmpDir, "commands.py")
		code := `
def handle_ping():
    pass

def handle_get(key):
    ...

def handle_set(key, val):
    raise NotImplementedError("TODO")
`
		if err := os.WriteFile(stubPy, []byte(code), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		orch := &Orchestrator{}
		violations := orch.auditGeneratorFunctionalOutput(tmpDir, []string{"commands.py"})
		if len(violations) < 3 {
			t.Fatalf("expected at least 3 violations, got %d: %+v", len(violations), violations)
		}

		formattedMsg := orch.formatAntiStubViolations(violations)
		if !strings.Contains(formattedMsg, "PRE-TEST FUNCTIONAL AUDIT REJECTION") {
			t.Errorf("expected formatted rejection header in: %s", formattedMsg)
		}
		if !strings.Contains(formattedMsg, "commands.py") {
			t.Errorf("expected file name in formatted rejection message: %s", formattedMsg)
		}
	})
}

func TestOrchestrator_AuditTesterTestOutput(t *testing.T) {
	t.Run("when tester writes rigorous behavioral tests, it returns no violations", func(t *testing.T) {
		tmpDir := t.TempDir()
		testDir := filepath.Join(tmpDir, "tests")
		_ = os.MkdirAll(testDir, 0o755)
		cleanTest := filepath.Join(testDir, "test_server.py")
		code := `
def test_server_startup():
    res = start_server(6379)
    assert res is True
`
		if err := os.WriteFile(cleanTest, []byte(code), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		orch := &Orchestrator{}
		violations := orch.auditTesterTestOutput(tmpDir)
		if len(violations) != 0 {
			t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
		}
	})

	t.Run("when tester writes tautological assertions or shell masks, it detects violations and formats rejection message", func(t *testing.T) {
		tmpDir := t.TempDir()
		testDir := filepath.Join(tmpDir, "tests")
		_ = os.MkdirAll(testDir, 0o755)
		vacuousTest := filepath.Join(testDir, "test_vacuous.py")
		code := `
def test_something():
    assert True

def test_another():
    assert 1 == 1
`
		if err := os.WriteFile(vacuousTest, []byte(code), 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		orch := &Orchestrator{}
		violations := orch.auditTesterTestOutput(tmpDir)
		if len(violations) < 2 {
			t.Fatalf("expected at least 2 violations, got %d: %+v", len(violations), violations)
		}

		formattedMsg := orch.formatTesterAntiStubViolations(violations)
		if !strings.Contains(formattedMsg, "TEST SUITE AUDIT REJECTION") {
			t.Errorf("expected formatted rejection header in: %s", formattedMsg)
		}
		if !strings.Contains(formattedMsg, "test_vacuous.py") {
			t.Errorf("expected file name in formatted rejection message: %s", formattedMsg)
		}
	})
}
