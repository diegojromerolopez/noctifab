package services

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SymbolDef represents a declared struct, class, interface, or function.
type SymbolDef struct {
	Name string
	Kind string
	Line int
}

// FileDependency represents a file's outgoing imports and declared symbols.
type FileDependency struct {
	Path       string
	Imports    []string
	ImportedBy []string
	Symbols    []SymbolDef
}

// ImportGraph holds the in-memory dependency graph of workspace files.
type ImportGraph struct {
	Files     map[string]*FileDependency
	SymbolMap map[string]string // Symbol Name -> File Path
}

// NewImportGraph initializes an empty ImportGraph.
func NewImportGraph() *ImportGraph {
	return &ImportGraph{
		Files:     make(map[string]*FileDependency),
		SymbolMap: make(map[string]string),
	}
}

var (
	pyClassFuncRE = regexp.MustCompile(`^\s*(?:async\s+)?(?:def|class)\s+([a-zA-Z0-9_]+)`)
	rustSymRE     = regexp.MustCompile(`^\s*(?:pub\s+)?(?:struct|enum|trait|type|fn|async fn)\s+([a-zA-Z0-9_]+)`)
	jsSymRE       = regexp.MustCompile(`^\s*(?:export\s+)?(?:class|interface|type|function|async function)\s+([a-zA-Z0-9_]+)`)
)

// ParseFileDependencies extracts imports and top-level symbols from a source file.
func ParseFileDependencies(relPath string, content string, allFiles map[string]bool) *FileDependency {
	dep := &FileDependency{
		Path:    relPath,
		Imports: []string{},
		Symbols: []SymbolDef{},
	}

	ext := strings.ToLower(filepath.Ext(relPath))
	dir := filepath.Dir(relPath)

	switch ext {
	case ".go":
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, relPath, content, 0)
		if err == nil {
			for _, imp := range node.Imports {
				impPath := strings.Trim(imp.Path.Value, "\"")
				// Check local file matching package basename
				parts := strings.Split(impPath, "/")
				pkgName := parts[len(parts)-1]
				for f := range allFiles {
					if strings.HasSuffix(filepath.Dir(f), pkgName) && strings.HasSuffix(f, ".go") {
						dep.Imports = append(dep.Imports, f)
					}
				}
			}
			ast.Inspect(node, func(n ast.Node) bool {
				switch decl := n.(type) {
				case *ast.TypeSpec:
					kind := "type"
					if _, ok := decl.Type.(*ast.StructType); ok {
						kind = "struct"
					} else if _, ok := decl.Type.(*ast.InterfaceType); ok {
						kind = "interface"
					}
					pos := fset.Position(decl.Pos())
					dep.Symbols = append(dep.Symbols, SymbolDef{Name: decl.Name.Name, Kind: kind, Line: pos.Line})
				case *ast.FuncDecl:
					pos := fset.Position(decl.Pos())
					dep.Symbols = append(dep.Symbols, SymbolDef{Name: decl.Name.Name, Kind: "func", Line: pos.Line})
				}
				return true
			})
		}

	case ".py":
		scanner := bufio.NewScanner(strings.NewReader(content))
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				continue
			}

			// Imports: from X import Y or import X
			if strings.HasPrefix(trimmed, "from ") || strings.HasPrefix(trimmed, "import ") {
				var modName string
				if strings.HasPrefix(trimmed, "from ") {
					parts := strings.Fields(trimmed)
					if len(parts) >= 2 {
						modName = parts[1]
					}
				} else {
					parts := strings.Fields(strings.TrimPrefix(trimmed, "import "))
					if len(parts) >= 1 {
						modName = strings.TrimSuffix(parts[0], ",")
					}
				}
				modName = strings.TrimPrefix(modName, ".")
				modPath1 := filepath.Join(dir, modName+".py")
				modPath2 := filepath.Join(modName + ".py")
				modPath3 := filepath.Join("src", modName+".py")
				modInit := filepath.Join(modName, "__init__.py")

				for _, cand := range []string{modPath1, modPath2, modPath3, modInit} {
					candNorm := filepath.ToSlash(cand)
					if allFiles[candNorm] {
						dep.Imports = append(dep.Imports, candNorm)
						break
					}
				}
			}

			if m := pyClassFuncRE.FindStringSubmatch(trimmed); len(m) > 1 {
				kind := "def"
				if strings.HasPrefix(trimmed, "class") {
					kind = "class"
				}
				dep.Symbols = append(dep.Symbols, SymbolDef{Name: m[1], Kind: kind, Line: lineNo})
			}
		}

	case ".rs":
		scanner := bufio.NewScanner(strings.NewReader(content))
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}

			if strings.HasPrefix(trimmed, "use crate::") || strings.HasPrefix(trimmed, "mod ") {
				var modName string
				if strings.HasPrefix(trimmed, "mod ") {
					modName = strings.TrimSuffix(strings.TrimPrefix(trimmed, "mod "), ";")
				} else {
					sub := strings.TrimPrefix(trimmed, "use crate::")
					modName = strings.Split(sub, "::")[0]
				}
				modName = strings.TrimSpace(modName)
				cand1 := filepath.Join("src", modName+".rs")
				cand2 := filepath.Join("src", modName, "mod.rs")
				for _, cand := range []string{cand1, cand2} {
					candNorm := filepath.ToSlash(cand)
					if allFiles[candNorm] {
						dep.Imports = append(dep.Imports, candNorm)
						break
					}
				}
			}

			if m := rustSymRE.FindStringSubmatch(trimmed); len(m) > 1 {
				dep.Symbols = append(dep.Symbols, SymbolDef{Name: m[1], Kind: "rust_sym", Line: lineNo})
			}
		}

	case ".ts", ".js", ".tsx", ".jsx":
		scanner := bufio.NewScanner(strings.NewReader(content))
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}

			if (strings.HasPrefix(trimmed, "import ") || strings.Contains(trimmed, "require(")) && strings.Contains(trimmed, "./") {
				re := regexp.MustCompile(`['"](\.[^'"]+)['"]`)
				if matches := re.FindStringSubmatch(trimmed); len(matches) > 1 {
					relTarget := matches[1]
					resolved := filepath.Join(dir, relTarget)
					for _, extTry := range []string{"", ".ts", ".js", ".tsx", ".jsx", "/index.ts", "/index.js"} {
						candNorm := filepath.ToSlash(filepath.Clean(resolved + extTry))
						if allFiles[candNorm] {
							dep.Imports = append(dep.Imports, candNorm)
							break
						}
					}
				}
			}

			if m := jsSymRE.FindStringSubmatch(trimmed); len(m) > 1 {
				dep.Symbols = append(dep.Symbols, SymbolDef{Name: m[1], Kind: "js_sym", Line: lineNo})
			}
		}
	}

	return dep
}

// BuildWorkspaceGraph parses all candidate files and constructs the dependency graph.
func BuildWorkspaceGraph(projectPath string, workspaceFiles []string) *ImportGraph {
	graph := NewImportGraph()
	allFiles := make(map[string]bool)
	for _, f := range workspaceFiles {
		allFiles[filepath.ToSlash(f)] = true
	}

	for _, relPath := range workspaceFiles {
		ext := strings.ToLower(filepath.Ext(relPath))
		if ext != ".go" && ext != ".py" && ext != ".rs" && ext != ".ts" && ext != ".js" {
			continue
		}
		fullPath := filepath.Join(projectPath, relPath)
		contentBytes, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		dep := ParseFileDependencies(relPath, string(contentBytes), allFiles)
		graph.Files[relPath] = dep
		for _, sym := range dep.Symbols {
			graph.SymbolMap[sym.Name] = relPath
		}
	}

	// Build reverse index (ImportedBy / callers)
	for path, dep := range graph.Files {
		for _, imp := range dep.Imports {
			if target, ok := graph.Files[imp]; ok {
				target.ImportedBy = append(target.ImportedBy, path)
			}
		}
	}

	return graph
}

// ImportGraphWalker traverses the dependency graph to gather precise task context.
type ImportGraphWalker struct {
	maxHops  int
	maxFiles int
}

// NewImportGraphWalker creates a graph walker with standard hop limits.
func NewImportGraphWalker() *ImportGraphWalker {
	return &ImportGraphWalker{
		maxHops:  2,
		maxFiles: 8,
	}
}

// GatherContext walks the workspace dependency graph and produces formatted file slices.
func (w *ImportGraphWalker) GatherContext(projectPath string, taskTargetFiles []string, taskTitle string, taskDesc string, workspaceFiles []string, slicer *ContextSlicer) []string {
	if len(workspaceFiles) == 0 {
		return nil
	}

	graph := BuildWorkspaceGraph(projectPath, workspaceFiles)
	visited := make(map[string]bool)
	var orderedFiles []string

	addFile := func(path string) {
		path = filepath.ToSlash(path)
		if path != "" && !visited[path] {
			visited[path] = true
			orderedFiles = append(orderedFiles, path)
		}
	}

	// 1. Seed with task target files
	for _, tf := range taskTargetFiles {
		addFile(tf)
	}

	// 2. Seed with files matching symbols mentioned in title and description
	combinedText := taskTitle + " " + taskDesc
	wordRE := regexp.MustCompile(`\b[a-zA-Z0-9_]{3,}\b`)
	words := wordRE.FindAllString(combinedText, -1)
	for _, word := range words {
		if file, ok := graph.SymbolMap[word]; ok {
			addFile(file)
		}
	}

	// 3. BFS Traversal for dependencies (outgoing imports) and dependents (incoming callers/tests)
	queue := make([]string, len(orderedFiles))
	copy(queue, orderedFiles)
	hops := 0

	for len(queue) > 0 && hops < w.maxHops && len(orderedFiles) < w.maxFiles {
		hops++
		levelSize := len(queue)
		for i := 0; i < levelSize; i++ {
			curr := queue[0]
			queue = queue[1:]

			if dep, ok := graph.Files[curr]; ok {
				// Add direct dependencies (interfaces, types)
				for _, imp := range dep.Imports {
					if !visited[imp] && len(orderedFiles) < w.maxFiles {
						visited[imp] = true
						orderedFiles = append(orderedFiles, imp)
						queue = append(queue, imp)
					}
				}
				// Add direct dependents (tests or callers)
				for _, caller := range dep.ImportedBy {
					if !visited[caller] && len(orderedFiles) < w.maxFiles {
						visited[caller] = true
						orderedFiles = append(orderedFiles, caller)
						queue = append(queue, caller)
					}
				}
			}
		}
	}

	if len(orderedFiles) == 0 {
		return nil
	}

	// Ensure deterministic ordering
	sort.Strings(orderedFiles)

	var gathered []string
	for _, relPath := range orderedFiles {
		fullPath := filepath.Join(projectPath, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil || len(content) == 0 {
			continue
		}

		rawContent := string(content)
		var sliced string
		if slicer != nil {
			sliced = slicer.SliceFileContext(relPath, rawContent, "")
		} else {
			sliced = fmt.Sprintf("File %s:\n```\n%s\n```", relPath, rawContent)
		}
		gathered = append(gathered, sliced)
	}

	return gathered
}
