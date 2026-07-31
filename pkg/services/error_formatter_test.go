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
