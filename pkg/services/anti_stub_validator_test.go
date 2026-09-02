package services_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAntiStubValidator_PythonStubs(t *testing.T) {
	v := services.NewAntiStubValidator()

	t.Run("when python has raise NotImplementedError, it detects violation", func(t *testing.T) {
		code := `
def get_user(user_id: int):
    raise NotImplementedError("TODO implement")
`
		violations := v.ValidateContent("src/user.py", code)
		assert.Len(t, violations, 1)
		assert.Equal(t, "python_not_implemented_stub", violations[0].Rule)
	})

	t.Run("when python has empty def with pass, it detects violation", func(t *testing.T) {
		code := `
def process_stream():
    pass
`
		violations := v.ValidateContent("src/stream.py", code)
		assert.Len(t, violations, 1)
		assert.Equal(t, "python_empty_stub_function", violations[0].Rule)
	})

	t.Run("when python has Protocol method with ellipsis, it is allowed", func(t *testing.T) {
		code := `
class Clock(Protocol):
    def now(self) -> float: ...
`
		violations := v.ValidateContent("src/clock.py", code)
		assert.Empty(t, violations)
	})

	t.Run("when python has real implementation with pass in except block, it is allowed", func(t *testing.T) {
		code := `
def read_file(p):
    try:
        with open(p) as f:
            return f.read()
    except FileNotFoundError:
        pass
    return None
`
		violations := v.ValidateContent("src/reader.py", code)
		assert.Empty(t, violations)
	})
}

func TestAntiStubValidator_ShellMasks(t *testing.T) {
	v := services.NewAntiStubValidator()

	t.Run("when shell script masks errors with || true, it detects violation", func(t *testing.T) {
		script := `#!/bin/bash
redis-cli -p 6379 PING || true
`
		violations := v.ValidateContent("tests/e2e/run_tests.sh", script)
		assert.Len(t, violations, 1)
		assert.Equal(t, "shell_error_suppression_mask", violations[0].Rule)
	})

	t.Run("when shell script masks with || exit 0, it detects violation", func(t *testing.T) {
		script := `#!/bin/bash
pytest tests/ || exit 0
`
		violations := v.ValidateContent("tests/run.sh", script)
		assert.Len(t, violations, 1)
		assert.Equal(t, "shell_error_exit_mask", violations[0].Rule)
	})
}

func TestAntiStubValidator_OtherLanguages(t *testing.T) {
	v := services.NewAntiStubValidator()

	t.Run("when Go has panic stub, it detects violation", func(t *testing.T) {
		code := `
package server

func Start() {
	panic("not implemented")
}
`
		violations := v.ValidateContent("server.go", code)
		assert.Len(t, violations, 1)
		assert.Equal(t, "go_panic_stub", violations[0].Rule)
	})

	t.Run("when Rust has todo macro, it detects violation", func(t *testing.T) {
		code := `
fn handle_req() {
    todo!("implement this");
}
`
		violations := v.ValidateContent("src/main.rs", code)
		assert.Len(t, violations, 1)
		assert.Equal(t, "rust_todo_stub", violations[0].Rule)
	})

	t.Run("when JS has throw not implemented, it detects violation", func(t *testing.T) {
		code := `
function execute() {
    throw new Error("not implemented");
}
`
		violations := v.ValidateContent("src/index.ts", code)
		assert.Len(t, violations, 1)
		assert.Equal(t, "javascript_not_implemented_stub", violations[0].Rule)
	})
}

func TestAntiStubValidator_TautologicalTests(t *testing.T) {
	v := services.NewAntiStubValidator()

	t.Run("when test has assert True, it detects violation", func(t *testing.T) {
		code := `
def test_something():
    assert True
`
		violations := v.ValidateContent("tests/unit/test_demo.py", code)
		assert.Len(t, violations, 1)
		assert.Equal(t, "tautological_test_assertion", violations[0].Rule)
	})

	t.Run("when production file has assert True, it is not flagged as test tautology", func(t *testing.T) {
		code := `
def validate_flag(f):
    assert True if f else False
`
		violations := v.ValidateContent("src/demo.py", code)
		assert.Empty(t, violations)
	})
}

func TestAntiStubValidator_ValidateWorkspace(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	require.NoError(t, os.MkdirAll(srcDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "valid.py"), []byte("def hello():\n    return 'world'\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "stub.py"), []byte("def pending():\n    raise NotImplementedError()\n"), 0644))

	v := services.NewAntiStubValidator()
	violations, err := v.ValidateWorkspace(tmpDir, nil)
	require.NoError(t, err)
	assert.Len(t, violations, 1)
	assert.Contains(t, violations[0].Path, "stub.py")

	// Target files filtering
	targetViolations, err := v.ValidateWorkspace(tmpDir, []string{"src/valid.py"})
	require.NoError(t, err)
	assert.Empty(t, targetViolations)
}

func TestAntiStubValidator_MakefileStubs(t *testing.T) {
	v := services.NewAntiStubValidator()

	t.Run("when Makefile test target only echoes message, it detects violation", func(t *testing.T) {
		makefile := `
.PHONY: test
test:
	@echo "Running unit tests..."
`
		violations := v.ValidateContent("Makefile", makefile)
		assert.Len(t, violations, 1)
		assert.Equal(t, "stub_makefile_target", violations[0].Rule)
	})

	t.Run("when Makefile test target runs real test command, it is allowed", func(t *testing.T) {
		makefile := `
.PHONY: test
test:
	@echo "Running tests..."
	npm run test
`
		violations := v.ValidateContent("Makefile", makefile)
		assert.Empty(t, violations)
	})
}
