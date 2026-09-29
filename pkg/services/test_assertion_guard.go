package services

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
)

// TestAssertionGuard enforces test quality invariants: ensuring that all tests contain real
// assertion statements and preventing agents from "fixing" failing tests by deleting assertions.
type TestAssertionGuard struct{}

// NewTestAssertionGuard creates an instance of TestAssertionGuard.
func NewTestAssertionGuard() *TestAssertionGuard {
	return &TestAssertionGuard{}
}

// CountAssertions returns the number of assertions found in the provided test file content.
func (g *TestAssertionGuard) CountAssertions(filePath, content string) (int, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return g.countGoAssertions(filePath, content)
	case ".py":
		return g.countPythonAssertions(content), nil
	default:
		return 0, nil
	}
}

// ValidateAssertionMonotonicity verifies that the number of assertions in newContent is
// greater than or equal to oldContent, preventing the agent from weakening existing test suites.
func (g *TestAssertionGuard) ValidateAssertionMonotonicity(filePath, oldContent, newContent string) error {
	oldCount, err := g.CountAssertions(filePath, oldContent)
	if err != nil {
		return err
	}
	newCount, err := g.CountAssertions(filePath, newContent)
	if err != nil {
		return err
	}

	if newCount < oldCount {
		return fmt.Errorf("assertion weakening detected in %s: original test had %d assertions, but updated test has only %d", filePath, oldCount, newCount)
	}

	return nil
}

// ValidateAssertionDensity ensures that all test functions defined in content contain at least one assertion.
func (g *TestAssertionGuard) ValidateAssertionDensity(filePath, content string) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".go" {
		return g.validateGoAssertionDensity(filePath, content)
	}
	return nil
}

func (g *TestAssertionGuard) countGoAssertions(filePath, content string) (int, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, content, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to parse Go test file %s: %w", filePath, err)
	}

	count := 0
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isGoAssertionCall(call) {
			count++
		}
		return true
	})

	return count, nil
}

func (g *TestAssertionGuard) validateGoAssertionDensity(filePath, content string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, content, 0)
	if err != nil {
		return fmt.Errorf("failed to parse Go test file %s: %w", filePath, err)
	}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}

		fnAssertions := 0
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if isGoAssertionCall(call) {
				fnAssertions++
			}
			return true
		})

		if fnAssertions == 0 {
			return fmt.Errorf("vacuous test detected in %s: function %s() contains 0 assertions", filePath, fn.Name.Name)
		}
	}

	return nil
}

func isGoAssertionCall(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		// match assert.*, require.*, t.Error*, t.Fatal*, t.Fail*
		name := fun.Sel.Name
		if strings.HasPrefix(name, "Error") || strings.HasPrefix(name, "Fatal") || strings.HasPrefix(name, "Fail") {
			return true
		}
		if ident, isIdent := fun.X.(*ast.Ident); isIdent {
			if ident.Name == "assert" || ident.Name == "require" {
				return true
			}
		}
	}
	return false
}

var (
	pyAssertRE = regexp.MustCompile(`(?m)^\s*(?:assert\b|self\.assert[a-zA-Z0-9_]*\s*\(|pytest\.raises\s*\()`)
)

func (g *TestAssertionGuard) countPythonAssertions(content string) int {
	matches := pyAssertRE.FindAllStringIndex(content, -1)
	return len(matches)
}
