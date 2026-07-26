package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

func TestContextSlicer_FullMode(t *testing.T) {
	slicer := NewContextSlicer(config.ContextConfig{Mode: "full"})
	content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	result := slicer.SliceFileContext("main.go", content, "")

	if !strings.Contains(result, "File main.go:") {
		t.Errorf("expected header 'File main.go:', got: %s", result)
	}
	if !strings.Contains(result, content) {
		t.Errorf("expected full content preserved, got: %s", result)
	}
}

func TestContextSlicer_DiffWindowMode(t *testing.T) {
	slicer := NewContextSlicer(config.ContextConfig{Mode: "diff_window", DiffWindowLines: 2})

	t.Run("with explicit diff content", func(t *testing.T) {
		diff := "+ func Add(a, b int) int {\n+   return a + b\n+ }"
		result := slicer.SliceFileContext("math.go", "raw content", diff)

		if !strings.Contains(result, "Diff Window") {
			t.Errorf("expected Diff Window header, got: %s", result)
		}
		if !strings.Contains(result, diff) {
			t.Errorf("expected diff content in result, got: %s", result)
		}
	})

	t.Run("with long raw content", func(t *testing.T) {
		var lines []string
		for i := 1; i <= 20; i++ {
			lines = append(lines, "line "+string(rune('0'+i)))
		}
		raw := strings.Join(lines, "\n")
		result := slicer.SliceFileContext("long.txt", raw, "")

		if !strings.Contains(result, "omitted") {
			t.Errorf("expected omitted lines indicator, got: %s", result)
		}
	})
}

func TestContextSlicer_TreeSitterMode(t *testing.T) {
	slicer := NewContextSlicer(config.ContextConfig{Mode: "tree_sitter"})

	t.Run("ruby code symbol extraction", func(t *testing.T) {
		var lines []string
		lines = append(lines, "require 'spec_helper'", "module Calculator", "  class Engine")
		for i := 0; i < 30; i++ {
			lines = append(lines, "    # comment line "+string(rune('a'+i%26)))
		}
		lines = append(lines, "    def add(a, b)", "      a + b", "    end", "  end", "end")

		rubyCode := strings.Join(lines, "\n")
		result := slicer.SliceFileContext("lib/calculator/engine.rb", rubyCode, "")

		if !strings.Contains(result, "Tree-Sitter AST Symbol Map") {
			t.Errorf("expected Tree-Sitter symbol header, got: %s", result)
		}
		if !strings.Contains(result, "class Engine") || !strings.Contains(result, "def add(a, b)") {
			t.Errorf("expected class and def symbols extracted, got: %s", result)
		}
	})

	t.Run("go code symbol extraction", func(t *testing.T) {
		var lines []string
		lines = append(lines, "package math", "import \"fmt\"", "type Calculator struct{}")
		for i := 0; i < 30; i++ {
			lines = append(lines, "  // filler line "+string(rune('a'+i%26)))
		}
		lines = append(lines, "func (c *Calculator) Add(a, b int) int { return a + b }")

		goCode := strings.Join(lines, "\n")
		result := slicer.SliceFileContext("math.go", goCode, "")

		if !strings.Contains(result, "Tree-Sitter AST Symbol Map") {
			t.Errorf("expected Tree-Sitter symbol header, got: %s", result)
		}
		if !strings.Contains(result, "type Calculator struct{}") || !strings.Contains(result, "func (c *Calculator) Add") {
			t.Errorf("expected Go symbols extracted, got: %s", result)
		}
	})
}

func TestContextSlicer_SliceFileFromDisk(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "slicer-disk-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	filePath := filepath.Join(tmpDir, "test.go")
	content := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	slicer := NewContextSlicer(config.ContextConfig{Mode: "full"})
	result, err := slicer.SliceFileFromDisk(tmpDir, "test.go")
	if err != nil {
		t.Fatalf("unexpected error reading from disk: %v", err)
	}

	if !strings.Contains(result, "func main() {}") {
		t.Errorf("expected content read from disk, got: %s", result)
	}
}
