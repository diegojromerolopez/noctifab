package services

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strings"
)

// FacadeViolation records a method or property call in a test on a facade/class
// that does not exist in the source code definition.
type FacadeViolation struct {
	ClassName      string
	MissingMethod  string
	CallerFile     string
	DeclaredClass  string
	SourceFile     string
	AvailableNames []string
}

func (v FacadeViolation) Error() string {
	avail := strings.Join(v.AvailableNames, ", ")
	if len(avail) > 80 {
		avail = avail[:80] + "..."
	}
	return fmt.Sprintf("facade integrity violation in %s: class '%s' (from %s) is missing method '%s'; available methods: [%s]",
		v.CallerFile, v.ClassName, v.SourceFile, v.MissingMethod, avail)
}

// FacadeIntegrityValidator checks that methods invoked on core domain classes/structs
// in tests actually exist on those classes in the source implementation.
type FacadeIntegrityValidator struct{}

// NewFacadeIntegrityValidator constructs a new FacadeIntegrityValidator.
func NewFacadeIntegrityValidator() *FacadeIntegrityValidator {
	return &FacadeIntegrityValidator{}
}

// ValidateFacades inspects source and test code across Python and Go projects
// and returns any detected missing methods on invoked classes.
func (v *FacadeIntegrityValidator) ValidateFacades(sourceFiles, testFiles map[string]string) []FacadeViolation {
	var violations []FacadeViolation

	// Python facade extraction
	pyClasses := v.extractPythonClasses(sourceFiles)
	if len(pyClasses) > 0 {
		violations = append(violations, v.validatePythonTestCalls(pyClasses, testFiles)...)
	}

	// Go facade extraction
	goStructs := v.extractGoStructs(sourceFiles)
	if len(goStructs) > 0 {
		violations = append(violations, v.validateGoTestCalls(goStructs, testFiles)...)
	}

	return violations
}

type classInfo struct {
	name       string
	sourceFile string
	methods    map[string]bool
}

var (
	pyClassRegex  = regexp.MustCompile(`(?m)^class\s+([A-Za-z0-9_]+)(?:\([^)]*\))?:`)
	pyMethodRegex = regexp.MustCompile(`(?m)^\s+def\s+([A-Za-z0-9_]+)\s*\(`)
	pyAttrRegex   = regexp.MustCompile(`(?m)^\s+self\.([A-Za-z0-9_]+)\s*=`)
)

func (v *FacadeIntegrityValidator) extractPythonClasses(sourceFiles map[string]string) map[string]classInfo {
	classes := make(map[string]classInfo)

	for path, content := range sourceFiles {
		if !strings.HasSuffix(path, ".py") {
			continue
		}

		lines := strings.Split(content, "\n")
		var currentClass *classInfo

		for _, line := range lines {
			if matches := pyClassRegex.FindStringSubmatch(line); len(matches) > 1 {
				cName := matches[1]
				info := classInfo{
					name:       cName,
					sourceFile: path,
					methods:    make(map[string]bool),
				}
				classes[cName] = info
				currentClass = &info
				continue
			}

			if currentClass != nil {
				// Dedent to 0 whitespace means class scope ended
				if len(line) > 0 && line[0] != ' ' && line[0] != '\t' && line[0] != '#' && !strings.HasPrefix(line, "class ") {
					currentClass = nil
					continue
				}

				if mMatches := pyMethodRegex.FindStringSubmatch(line); len(mMatches) > 1 {
					currentClass.methods[mMatches[1]] = true
					classes[currentClass.name] = *currentClass
				}
				if aMatches := pyAttrRegex.FindStringSubmatch(line); len(aMatches) > 1 {
					currentClass.methods[aMatches[1]] = true
					classes[currentClass.name] = *currentClass
				}
			}
		}
	}

	return classes
}

var (
	// Detect variable = ClassName(...)
	pyInstRegex = regexp.MustCompile(`([A-Za-z0-9_]+)\s*=\s*([A-Za-z0-9_]+)\s*\(`)
	// Detect instance.method_name(...)
	pyCallRegex = regexp.MustCompile(`([A-Za-z0-9_]+)\.([A-Za-z0-9_]+)\s*\(`)
)

func (v *FacadeIntegrityValidator) validatePythonTestCalls(classes map[string]classInfo, testFiles map[string]string) []FacadeViolation {
	var violations []FacadeViolation

	for path, content := range testFiles {
		if !strings.HasSuffix(path, ".py") {
			continue
		}

		// Track local instances: varName -> ClassName
		instances := make(map[string]string)

		// Also map lowercase class names directly (e.g. store -> Store)
		for cName := range classes {
			instances[strings.ToLower(cName)] = cName
			instances["self."+strings.ToLower(cName)] = cName
		}

		instMatches := pyInstRegex.FindAllStringSubmatch(content, -1)
		for _, m := range instMatches {
			if len(m) > 2 {
				varName := m[1]
				className := m[2]
				if _, ok := classes[className]; ok {
					instances[varName] = className
				}
			}
		}

		// Now scan calls: receiver.method(...)
		callMatches := pyCallRegex.FindAllStringSubmatch(content, -1)
		for _, m := range callMatches {
			if len(m) > 2 {
				recv := m[1]
				method := m[2]

				// Ignore standard built-ins / helpers
				if strings.HasPrefix(method, "assert") || method == "setUp" || method == "tearDown" {
					continue
				}

				if className, ok := instances[recv]; ok {
					cInfo := classes[className]
					if !cInfo.methods[method] {
						var avail []string
						for k := range cInfo.methods {
							avail = append(avail, k)
						}
						sort.Strings(avail)

						violations = append(violations, FacadeViolation{
							ClassName:      className,
							MissingMethod:  method,
							CallerFile:     path,
							DeclaredClass:  className,
							SourceFile:     cInfo.sourceFile,
							AvailableNames: avail,
						})
					}
				}
			}
		}
	}

	return violations
}

type goStructInfo struct {
	name       string
	sourceFile string
	methods    map[string]bool
}

func (v *FacadeIntegrityValidator) extractGoStructs(sourceFiles map[string]string) map[string]goStructInfo {
	structs := make(map[string]goStructInfo)
	fset := token.NewFileSet()

	for path, content := range sourceFiles {
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			continue
		}

		node, err := parser.ParseFile(fset, path, content, parser.AllErrors)
		if err != nil {
			continue
		}

		for _, decl := range node.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv != nil && len(fn.Recv.List) > 0 {
				recvType := ""
				switch t := fn.Recv.List[0].Type.(type) {
				case *ast.StarExpr:
					if ident, ok := t.X.(*ast.Ident); ok {
						recvType = ident.Name
					}
				case *ast.Ident:
					recvType = t.Name
				}

				if recvType != "" {
					info, exists := structs[recvType]
					if !exists {
						info = goStructInfo{
							name:       recvType,
							sourceFile: path,
							methods:    make(map[string]bool),
						}
					}
					info.methods[fn.Name.Name] = true
					structs[recvType] = info
				}
			}
		}
	}

	return structs
}

func (v *FacadeIntegrityValidator) validateGoTestCalls(structs map[string]goStructInfo, testFiles map[string]string) []FacadeViolation {
	var violations []FacadeViolation
	fset := token.NewFileSet()

	for path, content := range testFiles {
		if !strings.HasSuffix(path, "_test.go") {
			continue
		}

		node, err := parser.ParseFile(fset, path, content, 0)
		if err != nil {
			continue
		}

		// Simple receiver inference: varName -> structName
		vars := make(map[string]string)
		for sName := range structs {
			vars[strings.ToLower(sName)] = sName
			if len(sName) > 0 {
				vars[strings.ToLower(string(sName[0]))] = sName
			}
		}

		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			recvIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}

			if sName, found := vars[recvIdent.Name]; found {
				sInfo := structs[sName]
				method := sel.Sel.Name
				if !sInfo.methods[method] {
					var avail []string
					for m := range sInfo.methods {
						avail = append(avail, m)
					}
					sort.Strings(avail)

					violations = append(violations, FacadeViolation{
						ClassName:      sName,
						MissingMethod:  method,
						CallerFile:     path,
						DeclaredClass:  sName,
						SourceFile:     sInfo.sourceFile,
						AvailableNames: avail,
					})
				}
			}
			return true
		})
	}

	return violations
}
