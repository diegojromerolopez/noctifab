package services

import (
	"strings"
	"testing"
)

func TestFormatStructuredErrorFeedback_GoCompileError(t *testing.T) {
	rawLog := `
# github.com/diegojromerolopez/noctifab/pkg/services
pkg/services/foo.go:42:15: undefined: BarCalculator
pkg/services/foo.go:43:2: syntax error: unexpected newline
FAIL	github.com/diegojromerolopez/noctifab/pkg/services [build failed]
`

	formatted := FormatStructuredErrorFeedback(rawLog)
	if !strings.Contains(formatted, "STRUCTURED DIAGNOSTIC FEEDBACK") {
		t.Errorf("expected feedback header in output")
	}
	if !strings.Contains(formatted, "pkg/services/foo.go (Line 42, Col 15)") {
		t.Errorf("expected extracted line location in feedback, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "undefined: BarCalculator") {
		t.Errorf("expected error message in feedback")
	}
}

func TestFormatStructuredErrorFeedback_CSharpRoslynError(t *testing.T) {
	rawLog := `
Board.cs(20,50): error CS0206: A property or indexer may not be passed as an out or ref parameter
Build FAILED.
`
	formatted := FormatStructuredErrorFeedback(rawLog)
	if !strings.Contains(formatted, "Board.cs (Line 20, Col 50)") {
		t.Errorf("expected C# file and line extracted, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "CS0206") {
		t.Errorf("expected CS0206 code in feedback, got:\n%s", formatted)
	}
}

func TestFormatStructuredErrorFeedback_RustCargoError(t *testing.T) {
	rawLog := `
error[E0425]: cannot find value 'x' in this scope
  --> src/main.rs:12:5
`
	formatted := FormatStructuredErrorFeedback(rawLog)
	if !strings.Contains(formatted, "src/main.rs (Line 12, Col 5)") {
		t.Errorf("expected Rust file and line extracted, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "Rust Compiler Error") {
		t.Errorf("expected Rust Compiler Error category, got:\n%s", formatted)
	}
}

func TestFormatStructuredErrorFeedback_OCamlDuneError(t *testing.T) {
	rawLog := `
File "lib/engine.ml", line 44, characters 12-25:
Error: Syntax error
`
	formatted := FormatStructuredErrorFeedback(rawLog)
	if !strings.Contains(formatted, "lib/engine.ml (Line 44, Col 12)") {
		t.Errorf("expected OCaml file and line extracted, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "OCaml Compiler Error") {
		t.Errorf("expected OCaml Compiler Error category, got:\n%s", formatted)
	}
}

func TestFormatStructuredErrorFeedback_JavaCompilerError(t *testing.T) {
	rawLog := `
src/main/java/App.java:23: error: cannot find symbol
`
	formatted := FormatStructuredErrorFeedback(rawLog)
	if !strings.Contains(formatted, "src/main/java/App.java (Line 23)") {
		t.Errorf("expected Java file and line extracted, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "Java Compiler Error") {
		t.Errorf("expected Java Compiler Error category, got:\n%s", formatted)
	}
}

func TestFormatStructuredErrorFeedback_MakeMissingTarget(t *testing.T) {
	rawLog := `
make: *** No rule to make target 'build'. Stop.
`
	formatted := FormatStructuredErrorFeedback(rawLog)
	if !strings.Contains(formatted, "Makefile") {
		t.Errorf("expected Makefile extracted for missing target, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "Missing Makefile target 'build'") {
		t.Errorf("expected missing target message, got:\n%s", formatted)
	}
}

func TestFormatStructuredErrorFeedback_CargoDependency(t *testing.T) {
	rawLog := `
error: failed to select a version for the requirement 'inkwell = "^0.5.1"'
`
	formatted := FormatStructuredErrorFeedback(rawLog)
	if !strings.Contains(formatted, "Cargo.toml") {
		t.Errorf("expected Cargo.toml extracted for dependency error, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "inkwell = \"^0.5.1\"") {
		t.Errorf("expected requirement in message, got:\n%s", formatted)
	}
}

func TestFormatStructuredErrorFeedback_PytestError(t *testing.T) {
	rawLog := `
FAILED tests/test_calc.py::test_add - AssertionError: assert 4 == 5
`

	formatted := FormatStructuredErrorFeedback(rawLog)
	if !strings.Contains(formatted, "tests/test_calc.py") {
		t.Errorf("expected pytest failure file extracted")
	}
	if !strings.Contains(formatted, "Failed test 'test_add'") {
		t.Errorf("expected pytest failure message extracted")
	}
}

func TestFormatStructuredErrorFeedback_EmptyOrBlank(t *testing.T) {
	if got := FormatStructuredErrorFeedback(""); got != "No error output captured." {
		t.Errorf("expected default feedback for empty input, got: %s", got)
	}
	if got := FormatStructuredErrorFeedback("   \n\t  "); got != "No error output captured." {
		t.Errorf("expected default feedback for whitespace input, got: %s", got)
	}
}
