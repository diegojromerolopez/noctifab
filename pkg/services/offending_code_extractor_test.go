package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractOffendingCodeSnippets_LinePrecision(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	code := `using System;

namespace DotChess {
    public class Board {
        private int turn = 0;
        public int CurrentTurn => turn;
        public int Winner { get; set; }

        public void MakeMove() {
            int result = 42;
            UpdateState(ref this.CurrentTurn, out this.Winner);
        }

        private void UpdateState(ref int turn, out int winner) {
            winner = 0;
        }
    }
}`
	boardFile := filepath.Join(srcDir, "Board.cs")
	if err := os.WriteFile(boardFile, []byte(code), 0644); err != nil {
		t.Fatalf("failed to write Board.cs: %v", err)
	}

	diagnostics := []DiagnosticError{
		{
			FilePath:   "Board.cs", // Note: basename only, must be resolved by findSourceFile
			LineNumber: "11",
			Column:     "30",
			Message:    "CS0206: A property or indexer may not be passed as an out or ref parameter",
			Category:   "C# Compiler Error",
		},
	}

	snippets := ExtractOffendingCodeSnippets(tempDir, diagnostics, 3)
	if !strings.Contains(snippets, "OFFENDING CODE SNIPPET(S) FROM DISK") {
		t.Fatalf("expected header in snippets, got:\n%s", snippets)
	}
	if !strings.Contains(snippets, "File: src/Board.cs (around line 11)") {
		t.Errorf("expected relative path to Board.cs with line 11, got:\n%s", snippets)
	}
	if !strings.Contains(snippets, ">>   11 |             UpdateState(ref this.CurrentTurn, out this.Winner);   <-- OFFENDING CODE") {
		t.Errorf("expected visual pointer '>>' on line 11, got:\n%s", snippets)
	}
	if !strings.Contains(snippets, "```csharp") {
		t.Errorf("expected csharp code fence, got:\n%s", snippets)
	}
}

func TestExtractOffendingCodeSnippets_MakefileTarget(t *testing.T) {
	tempDir := t.TempDir()
	makeContent := `all: test

test:
	go test ./...

clean:
	rm -rf bin/
`
	if err := os.WriteFile(filepath.Join(tempDir, "Makefile"), []byte(makeContent), 0644); err != nil {
		t.Fatalf("failed to write Makefile: %v", err)
	}

	diagnostics := []DiagnosticError{
		{
			FilePath:   "Makefile",
			LineNumber: "1",
			Message:    "Missing Makefile target 'build'",
			Category:   "Build Manifest Error",
		},
	}

	snippets := ExtractOffendingCodeSnippets(tempDir, diagnostics, 5)
	if !strings.Contains(snippets, "File: Makefile") {
		t.Errorf("expected Makefile in snippets, got:\n%s", snippets)
	}
	if !strings.Contains(snippets, "all: test") {
		t.Errorf("expected Makefile contents in snippets, got:\n%s", snippets)
	}
	if !strings.Contains(snippets, "```makefile") {
		t.Errorf("expected makefile code fence, got:\n%s", snippets)
	}
}

func TestExtractOffendingCodeSnippets_EmptyAndMissing(t *testing.T) {
	if got := ExtractOffendingCodeSnippets("", nil, 5); got != "" {
		t.Errorf("expected empty string for empty input, got: %s", got)
	}

	tempDir := t.TempDir()
	diagnostics := []DiagnosticError{
		{
			FilePath:   "nonexistent.rs",
			LineNumber: "10",
			Message:    "cannot find symbol",
			Category:   "Rust Compiler Error",
		},
	}
	if got := ExtractOffendingCodeSnippets(tempDir, diagnostics, 5); got != "" {
		t.Errorf("expected empty string for nonexistent file, got: %s", got)
	}
}
