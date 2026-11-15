package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ExtractOffendingCodeSnippets loads offending source code files from disk, isolates the exact
// lines where compiler, linter, or test errors occurred, and formats an instructive contextual snippet
// with line numbers and a visual pointer (">>") marking the offending code.
func ExtractOffendingCodeSnippets(projectPath string, diagnostics []DiagnosticError, contextLines int) string {
	if projectPath == "" || len(diagnostics) == 0 {
		return ""
	}
	if contextLines <= 0 {
		contextLines = 5
	}

	var sb strings.Builder
	sb.WriteString("### 🔍 OFFENDING CODE SNIPPET(S) FROM DISK:\n")

	renderedCount := 0
	seenRegions := make(map[string]bool)

	for _, d := range diagnostics {
		if d.FilePath == "" {
			continue
		}

		fullPath := findSourceFile(projectPath, d.FilePath)
		if fullPath == "" {
			continue
		}

		contentBytes, err := os.ReadFile(fullPath)
		if err != nil || len(contentBytes) == 0 {
			continue
		}

		relPath, err := filepath.Rel(projectPath, fullPath)
		if err != nil {
			relPath = d.FilePath
		}

		lineNum, _ := strconv.Atoi(d.LineNumber)
		regionKey := fmt.Sprintf("%s:%d", relPath, lineNum)
		if seenRegions[regionKey] {
			continue
		}
		seenRegions[regionKey] = true

		renderedSnippet := renderCodeSnippet(relPath, string(contentBytes), lineNum, contextLines, d)
		if renderedSnippet != "" {
			sb.WriteString(renderedSnippet)
			sb.WriteString("\n")
			renderedCount++
			if renderedCount >= 4 {
				// Bound snippet count to prevent context bloating
				break
			}
		}
	}

	if renderedCount == 0 {
		return ""
	}

	return sb.String()
}

func renderCodeSnippet(relPath, content string, targetLine, contextLines int, d DiagnosticError) string {
	rawLines := strings.Split(content, "\n")
	totalLines := len(rawLines)
	if totalLines == 0 {
		return ""
	}

	lang := filePathToLang(relPath)
	var sb strings.Builder

	if targetLine <= 0 {
		// Non-line diagnostic (e.g. Makefile, Cargo.toml root error)
		maxLines := 35
		if totalLines < maxLines {
			maxLines = totalLines
		}
		fmt.Fprintf(&sb, "File: %s\nDiagnostic: [%s] %s\n```%s\n", relPath, d.Category, d.Message, lang)
		for i := 0; i < maxLines; i++ {
			fmt.Fprintf(&sb, "%4d | %s\n", i+1, rawLines[i])
		}
		if totalLines > maxLines {
			sb.WriteString("... [truncated remaining lines]\n")
		}
		sb.WriteString("```\n")
		return sb.String()
	}

	start := targetLine - contextLines
	if start < 1 {
		start = 1
	}
	end := targetLine + contextLines
	if end > totalLines {
		end = totalLines
	}

	fmt.Fprintf(&sb, "File: %s (around line %d)\nDiagnostic: [%s] %s\n```%s\n", relPath, targetLine, d.Category, d.Message, lang)
	for i := start; i <= end; i++ {
		lineContent := rawLines[i-1]
		if i == targetLine {
			fmt.Fprintf(&sb, ">> %4d | %s   <-- OFFENDING CODE\n", i, lineContent)
		} else {
			fmt.Fprintf(&sb, "   %4d | %s\n", i, lineContent)
		}
	}
	sb.WriteString("```\n")
	return sb.String()
}

// findSourceFile searches for a file in projectPath matching filePath, falling back to
// a recursive search by basename if relative path resolution fails.
func findSourceFile(projectPath, filePath string) string {
	candidate := filepath.Join(projectPath, filePath)
	if fileExists(candidate) {
		return candidate
	}

	baseName := filepath.Base(filePath)
	var foundPath string

	_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || foundPath != "" {
			return filepath.SkipDir
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			if name == "bin" || name == "obj" || name == "target" || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() == baseName {
			foundPath = path
			return filepath.SkipDir
		}
		return nil
	})

	return foundPath
}

func filePathToLang(relPath string) string {
	base := strings.ToLower(filepath.Base(relPath))
	if base == "makefile" || base == "gnumakefile" || strings.HasSuffix(base, ".mk") {
		return "makefile"
	}
	if base == "dockerfile" {
		return "dockerfile"
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(relPath), "."))
	switch ext {
	case "go":
		return "go"
	case "cs":
		return "csharp"
	case "rs":
		return "rust"
	case "py":
		return "python"
	case "ml", "mli":
		return "ocaml"
	case "java":
		return "java"
	case "kt":
		return "kotlin"
	case "ts", "tsx":
		return "typescript"
	case "js", "jsx":
		return "javascript"
	case "toml":
		return "toml"
	case "json":
		return "json"
	case "yaml", "yml":
		return "yaml"
	default:
		return ext
	}
}
