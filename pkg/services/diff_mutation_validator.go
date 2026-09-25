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

var (
	commentLineRE = regexp.MustCompile(`(?m)^\s*(?://|#|/\*|\*).*$`)
	whitespaceRE  = regexp.MustCompile(`\s+`)
)

// DiffMutationValidator deterministically prevents vacuous edits (diffs that only change
// comments or whitespace) and verifies that newly declared functions contain substantive bodies.
type DiffMutationValidator struct{}

// NewDiffMutationValidator initializes a new DiffMutationValidator.
func NewDiffMutationValidator() *DiffMutationValidator {
	return &DiffMutationValidator{}
}

// ValidateNonVacuousDiff checks that newContent contains functional semantic modifications
// relative to oldContent, rejecting edits that solely modify whitespace or comments.
func (v *DiffMutationValidator) ValidateNonVacuousDiff(oldContent, newContent string) error {
	if oldContent == newContent {
		return fmt.Errorf("zero mutation: content is completely identical")
	}

	stripTrivia := func(s string) string {
		noComments := commentLineRE.ReplaceAllString(s, "")
		normalized := whitespaceRE.ReplaceAllString(noComments, " ")
		return strings.TrimSpace(normalized)
	}

	strippedOld := stripTrivia(oldContent)
	strippedNew := stripTrivia(newContent)

	if strippedOld == strippedNew {
		return fmt.Errorf("vacuous edit rejected: modifications only alter whitespace or comments without functional code mutations")
	}

	return nil
}

// ValidateASTBody verifies that newly declared functions in filePath contain non-trivial logic.
func (v *DiffMutationValidator) ValidateASTBody(filePath string, content string) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return v.validateGoAST(filePath, content)
	case ".py":
		return v.validatePythonAST(filePath, content)
	}
	return nil
}

func (v *DiffMutationValidator) validateGoAST(filePath, content string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, content, 0)
	if err != nil {
		return fmt.Errorf("AST parse failed for %s: %w", filePath, err)
	}

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		if len(fn.Body.List) == 0 {
			return fmt.Errorf("anti-stub AST violation in %s: function %s() has an empty body", filePath, fn.Name.Name)
		}

		if len(fn.Body.List) == 1 {
			// Check if single statement is return nil/false/panic
			if ret, isRet := fn.Body.List[0].(*ast.ReturnStmt); isRet {
				if len(ret.Results) == 1 {
					if ident, isIdent := ret.Results[0].(*ast.Ident); isIdent {
						if ident.Name == "nil" || ident.Name == "false" {
							return fmt.Errorf("anti-stub AST violation in %s: function %s() merely returns %s", filePath, fn.Name.Name, ident.Name)
						}
					}
				}
			}
			if exprStmt, isExpr := fn.Body.List[0].(*ast.ExprStmt); isExpr {
				if call, isCall := exprStmt.X.(*ast.CallExpr); isCall {
					if ident, isIdent := call.Fun.(*ast.Ident); isIdent && ident.Name == "panic" {
						return fmt.Errorf("anti-stub AST violation in %s: function %s() merely panics as a placeholder", filePath, fn.Name.Name)
					}
				}
			}
		}
	}
	return nil
}

var (
	pyEmptyFuncRE = regexp.MustCompile(`(?m)^\s*(?:async\s+)?def\s+([a-zA-Z0-9_]+)\s*\([^)]*\)\s*:\s*(?:pass|\.\.\.)\s*$`)
)

func (v *DiffMutationValidator) validatePythonAST(filePath, content string) error {
	if matches := pyEmptyFuncRE.FindStringSubmatch(content); len(matches) > 1 {
		return fmt.Errorf("anti-stub AST violation in %s: python function %s() contains only pass or ellipsis placeholder", filePath, matches[1])
	}
	return nil
}
