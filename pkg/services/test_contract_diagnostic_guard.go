package services

import (
	"fmt"
	"strings"
)


// ContractDiagnosticViolation reports a test assertion that lacks diagnostic context
// (e.g. bare assertEqual without operation input data or contract rule).
type ContractDiagnosticViolation struct {
	FilePath   string
	LineNumber int
	Expression string
	Reason     string
}

func (v ContractDiagnosticViolation) Error() string {
	return fmt.Sprintf("contract diagnostic violation at %s:%d: %s (%s)",
		v.FilePath, v.LineNumber, v.Expression, v.Reason)
}

// TestContractDiagnosticGuard enforces that authored tests provide rich diagnostic
// context (inputs, operation contracts, expected vs actual) upon failure.
type TestContractDiagnosticGuard struct{}

// NewTestContractDiagnosticGuard constructs a new TestContractDiagnosticGuard.
func NewTestContractDiagnosticGuard() *TestContractDiagnosticGuard {
	return &TestContractDiagnosticGuard{}
}



// ValidateDiagnosticRichness inspects test code and flags assertions that lack
// descriptive message payloads.
func (g *TestContractDiagnosticGuard) ValidateDiagnosticRichness(filePath, content string) []ContractDiagnosticViolation {
	var violations []ContractDiagnosticViolation

	if strings.HasSuffix(filePath, ".py") {
		violations = append(violations, g.validatePythonDiagnostics(filePath, content)...)
	}

	return violations
}

func (g *TestContractDiagnosticGuard) validatePythonDiagnostics(filePath, content string) []ContractDiagnosticViolation {
	var violations []ContractDiagnosticViolation
	lines := strings.Split(content, "\n")

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "self.assert") {
			if strings.Contains(trimmed, "self.dispatch(") || strings.Contains(trimmed, "dispatch(") {
				if !strings.Contains(trimmed, "msg=") && !hasThirdDiagnosticArg(trimmed) {
					violations = append(violations, ContractDiagnosticViolation{
						FilePath:   filePath,
						LineNumber: idx + 1,
						Expression: trimmed,
						Reason:     "operation dispatch assertion lacks descriptive msg= parameter with input data and contract context",
					})
				}
			}
		}
	}

	return violations
}

// hasThirdDiagnosticArg checks if an assertion call has at least 2 top-level commas (3 arguments).
func hasThirdDiagnosticArg(expr string) bool {
	openParen := strings.Index(expr, "(")
	if openParen == -1 {
		return false
	}
	closeParen := strings.LastIndex(expr, ")")
	if closeParen <= openParen {
		return false
	}
	inner := expr[openParen+1 : closeParen]

	parenDepth := 0
	bracketDepth := 0
	inSingleQuote := false
	inDoubleQuote := false
	topLevelCommas := 0

	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		if ch == '\\' && (inSingleQuote || inDoubleQuote) {
			i++ // skip escaped char
			continue
		}
		if ch == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			continue
		}
		if ch == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			continue
		}
		if inSingleQuote || inDoubleQuote {
			continue
		}
		switch ch {
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[', '{':
			bracketDepth++
		case ']', '}':
			bracketDepth--
		case ',':
			if parenDepth == 0 && bracketDepth == 0 {
				topLevelCommas++
			}
		}
	}

	return topLevelCommas >= 2
}


// FormatRichContractMessage generates a standard, high-visibility diagnostic message string
// for inclusion in test assertions.
func (g *TestContractDiagnosticGuard) FormatRichContractMessage(operation, inputArgs, contractRule string) string {
	return fmt.Sprintf("\n[CONTRACT VIOLATION] Operation: %s\nInput Args: %s\nContract Rule: %s",
		operation, inputArgs, contractRule)
}
