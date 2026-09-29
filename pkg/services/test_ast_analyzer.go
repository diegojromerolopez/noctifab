package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// TestASTViolation represents a structural test quality issue detected via AST analysis.
type TestASTViolation struct {
	FilePath string `json:"path"`
	Line     int    `json:"line"`
	TestName string `json:"test_name"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
}

// TestASTAnalyzer performs language-native AST analysis on test files to verify test quality.
type TestASTAnalyzer struct{}

// NewTestASTAnalyzer constructs a new TestASTAnalyzer.
func NewTestASTAnalyzer() *TestASTAnalyzer {
	return &TestASTAnalyzer{}
}

// AnalyzeWorkspace scans all test files in projectPath and returns AST violations.
func (a *TestASTAnalyzer) AnalyzeWorkspace(ctx context.Context, projectPath string) ([]TestASTViolation, error) {
	testFiles := DiscoverTestFiles(projectPath)
	var violations []TestASTViolation

	for _, tf := range testFiles {
		fullPath := tf
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(projectPath, fullPath)
		}
		fileViolations, err := a.AnalyzeFile(ctx, fullPath)
		if err != nil {
			return nil, err
		}
		violations = append(violations, fileViolations...)
	}
	return violations, nil
}

// AnalyzeFile inspects a single test file based on extension.
func (a *TestASTAnalyzer) AnalyzeFile(ctx context.Context, fullPath string) ([]TestASTViolation, error) {
	ext := strings.ToLower(filepath.Ext(fullPath))
	switch ext {
	case ".go":
		if strings.HasSuffix(fullPath, "_test.go") {
			return a.analyzeGoTest(fullPath)
		}
	case ".py":
		base := filepath.Base(fullPath)
		if strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") {
			return a.analyzePythonTest(ctx, fullPath)
		}
	}
	return nil, nil
}

// analyzeGoTest parses and validates a Go test file using go/parser and go/ast.
func (a *TestASTAnalyzer) analyzeGoTest(fullPath string) ([]TestASTViolation, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, fullPath, nil, parser.ParseComments)
	if err != nil {
		return nil, nil // Syntax errors handled by SyntaxValidator
	}

	var violations []TestASTViolation
	seenSignatures := make(map[string]string) // signature hash -> first test name

	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Body == nil {
			continue
		}
		testName := fn.Name.Name
		if !strings.HasPrefix(testName, "Test") && !strings.HasPrefix(testName, "Benchmark") {
			continue
		}
		pos := fset.Position(fn.Pos())

		// 1. Not empty check
		if len(fn.Body.List) == 0 {
			violations = append(violations, TestASTViolation{
				FilePath: fullPath,
				Line:     pos.Line,
				TestName: testName,
				Rule:     "ast_empty_test",
				Message:  fmt.Sprintf("Test %s has an empty body", testName),
			})
			continue
		}

		// 2. Main assertion & Tautology & Mock checks
		hasAssertion := false
		hasMockSetup := false
		hasMockVerification := false

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if n == nil {
				return true
			}
			// Check for tautological binary expressions (e.g. x == x, 1 == 1)
			if bin, isBin := n.(*ast.BinaryExpr); isBin {
				if bin.Op == token.EQL || bin.Op == token.NEQ {
					leftStr := exprToString(bin.X)
					rightStr := exprToString(bin.Y)
					if leftStr != "" && leftStr == rightStr {
						bPos := fset.Position(bin.Pos())
						violations = append(violations, TestASTViolation{
							FilePath: fullPath,
							Line:     bPos.Line,
							TestName: testName,
							Rule:     "ast_tautological_comparison",
							Message:  fmt.Sprintf("Tautological comparison (%s %s %s) in %s", leftStr, bin.Op, rightStr, testName),
						})
					}
				}
			}

			// Check call expressions
			if call, isCall := n.(*ast.CallExpr); isCall {
				callName := exprToString(call.Fun)

				// Detect mock setup & verification
				if strings.Contains(callName, "On") || strings.Contains(callName, "Called") {
					hasMockSetup = true
					if len(call.Args) == 0 {
						cPos := fset.Position(call.Pos())
						violations = append(violations, TestASTViolation{
							FilePath: fullPath,
							Line:     cPos.Line,
							TestName: testName,
							Rule:     "ast_ill_defined_mock",
							Message:  fmt.Sprintf("Mock call in %s has no arguments/expectations defined", testName),
						})
					}
				}
				if strings.Contains(callName, "AssertExpectations") || strings.Contains(callName, "Finish") || strings.Contains(callName, "AssertNumberOfCalls") {
					hasMockVerification = true
				}

				// Detect assertions
				if isAssertionCall(callName) {
					hasAssertion = true
					// Check for tautological assertion args
					if isTautologicalAssert(callName, call.Args) {
						cPos := fset.Position(call.Pos())
						violations = append(violations, TestASTViolation{
							FilePath: fullPath,
							Line:     cPos.Line,
							TestName: testName,
							Rule:     "ast_tautological_assertion",
							Message:  fmt.Sprintf("Tautological assertion %s in %s", callName, testName),
						})
					}
				}
			}
			return true
		})

		// 3. At least one main assertion
		if !hasAssertion {
			violations = append(violations, TestASTViolation{
				FilePath: fullPath,
				Line:     pos.Line,
				TestName: testName,
				Rule:     "ast_missing_assertion",
				Message:  fmt.Sprintf("Test %s contains no assertions (t.Error/Fatal, assert.*, require.*)", testName),
			})
		}

		// 4. Mock calls checked via assertions
		if hasMockSetup && !hasMockVerification && !hasAssertion {
			violations = append(violations, TestASTViolation{
				FilePath: fullPath,
				Line:     pos.Line,
				TestName: testName,
				Rule:     "ast_unverified_mock",
				Message:  fmt.Sprintf("Test %s configures mocks but does not assert expectations", testName),
			})
		}

		// 5. Carbon-copy duplicate detection via structural AST signature
		sig := computeStructuralSignature(fn.Body)
		if firstTest, exists := seenSignatures[sig]; exists {
			violations = append(violations, TestASTViolation{
				FilePath: fullPath,
				Line:     pos.Line,
				TestName: testName,
				Rule:     "ast_carbon_copy_duplicate",
				Message:  fmt.Sprintf("Test %s is a structural carbon copy duplicate of %s", testName, firstTest),
			})
		} else {
			seenSignatures[sig] = testName
		}
	}

	return violations, nil
}

func isAssertionCall(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "assert") ||
		strings.Contains(lower, "require") ||
		strings.HasSuffix(lower, ".error") ||
		strings.HasSuffix(lower, ".errorf") ||
		strings.HasSuffix(lower, ".fatal") ||
		strings.HasSuffix(lower, ".fatalf") ||
		strings.HasSuffix(lower, ".fail") ||
		strings.HasSuffix(lower, ".failnow") ||
		strings.Contains(lower, "should") ||
		strings.Contains(lower, "expect")
}

func isTautologicalAssert(callName string, args []ast.Expr) bool {
	lower := strings.ToLower(callName)
	// assert.True(t, true) or assert.False(t, false)
	if strings.Contains(lower, "true") || strings.Contains(lower, "false") {
		for _, arg := range args {
			str := exprToString(arg)
			if str == "true" || str == "false" {
				return true
			}
		}
	}
	// assert.Equal(t, a, a)
	if strings.Contains(lower, "equal") && len(args) >= 2 {
		idx1 := 0
		if len(args) >= 3 {
			idx1 = 1 // skip testing.T parameter
		}
		idx2 := idx1 + 1
		if idx2 < len(args) {
			s1 := exprToString(args[idx1])
			s2 := exprToString(args[idx2])
			if s1 != "" && s1 == s2 {
				return true
			}
		}
	}
	return false
}

func exprToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.BasicLit:
		return e.Value
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	case *ast.BinaryExpr:
		return exprToString(e.X) + " " + e.Op.String() + " " + exprToString(e.Y)
	default:
		return fmt.Sprintf("%T", expr)
	}
}

// computeStructuralSignature hashes the sequence of AST node types to detect identical structural duplicates.
func computeStructuralSignature(body *ast.BlockStmt) string {
	var sb strings.Builder
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		// Write node type and omit exact identifier names
		switch n.(type) {
		case *ast.Ident:
			sb.WriteString("I;")
		case *ast.BasicLit:
			sb.WriteString("L;")
		case *ast.CallExpr:
			sb.WriteString("C;")
		case *ast.IfStmt:
			sb.WriteString("IF;")
		case *ast.ForStmt, *ast.RangeStmt:
			sb.WriteString("LOOP;")
		case *ast.AssignStmt:
			sb.WriteString("ASN;")
		case *ast.ReturnStmt:
			sb.WriteString("RET;")
		default:
			fmt.Fprintf(&sb, "%T;", n)
		}
		return true
	})
	hash := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(hash[:16])
}

// analyzePythonTest analyzes a python test using Python's ast module if python3 is available.
func (a *TestASTAnalyzer) analyzePythonTest(ctx context.Context, fullPath string) ([]TestASTViolation, error) {
	pyPath, err := exec.LookPath("python3")
	if err != nil {
		return nil, nil // Skip if python3 is not available
	}

	pyScript := `
import ast, sys, hashlib

path = sys.argv[1]
try:
    with open(path, "r", encoding="utf-8") as f:
        tree = ast.parse(f.read(), filename=path)
except Exception:
    sys.exit(0)

seen_sigs = {}
for node in ast.walk(tree):
    if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
        if not node.name.startswith("test_"):
            continue
        # 1. Not empty
        real_stmts = [s for s in node.body if not (isinstance(s, ast.Expr) and isinstance(s.value, ast.Constant) and isinstance(s.value.value, str))]
        if not real_stmts or (len(real_stmts) == 1 and isinstance(real_stmts[0], ast.Pass)):
            print(f"{node.lineno}|{node.name}|ast_empty_test|Test {node.name} has an empty body")
            continue

        has_assert = False
        for child in ast.walk(node):
            if isinstance(child, ast.Assert):
                has_assert = True
                # Tautological assert True / 1 == 1
                if isinstance(child.test, ast.Constant) and child.test.value is True:
                    print(f"{child.lineno}|{node.name}|ast_tautological_assertion|Tautological assert True in {node.name}")
                elif isinstance(child.test, ast.Compare) and len(child.test.ops) == 1 and isinstance(child.test.ops[0], ast.Eq):
                    left = ast.dump(child.test.left)
                    right = ast.dump(child.test.comparators[0])
                    if left == right:
                        print(f"{child.lineno}|{node.name}|ast_tautological_comparison|Tautological comparison in {node.name}")
            elif isinstance(child, ast.Call):
                func_str = ast.dump(child.func)
                if "assert" in func_str.lower():
                    has_assert = True

        if not has_assert:
            print(f"{node.lineno}|{node.name}|ast_missing_assertion|Test {node.name} contains no assertions")

        # Carbon copy duplicate detection
        sig = hashlib.sha256(ast.dump(node).encode("utf-8")).hexdigest()[:16]
        if sig in seen_sigs:
            print(f"{node.lineno}|{node.name}|ast_carbon_copy_duplicate|Test {node.name} is a duplicate of {seen_sigs[sig]}")
        else:
            seen_sigs[sig] = node.name
`
	cmd := exec.CommandContext(ctx, pyPath, "-c", pyScript, fullPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, nil
	}

	var violations []TestASTViolation
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) == 4 {
			var lineNum int
			_, _ = fmt.Sscanf(parts[0], "%d", &lineNum)
			violations = append(violations, TestASTViolation{
				FilePath: fullPath,
				Line:     lineNum,
				TestName: parts[1],
				Rule:     parts[2],
				Message:  parts[3],
			})
		}
	}
	return violations, nil
}

// CountDiscoveredTests returns the total count of test functions discovered via AST.
func CountDiscoveredTests(projectPath string) int {
	testFiles := DiscoverTestFiles(projectPath)
	total := 0
	fset := token.NewFileSet()

	for _, tf := range testFiles {
		fullPath := tf
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(projectPath, fullPath)
		}
		if strings.HasSuffix(fullPath, "_test.go") {
			node, err := parser.ParseFile(fset, fullPath, nil, 0)
			if err == nil {
				for _, decl := range node.Decls {
					if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name != nil {
						if strings.HasPrefix(fn.Name.Name, "Test") {
							total++
						}
					}
				}
			}
		} else if strings.HasSuffix(fullPath, ".py") {
			content, err := os.ReadFile(fullPath)
			if err == nil {
				re := regexp.MustCompile(`(?m)^\s*def\s+(test_[a-zA-Z0-9_]+)\s*\(`)
				matches := re.FindAllString(string(content), -1)
				total += len(matches)
			}
		}
	}
	return total
}
