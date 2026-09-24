package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

func TestParseFileDependencies_Python(t *testing.T) {
	content := `
import os
import sys
from client import RedisClient
from utils import helper

class RespParser:
    def __init__(self):
        pass

    def parse_chunk(self, data):
        return True
`
	allFiles := map[string]bool{
		"client.py": true,
		"utils.py":  true,
		"parser.py": true,
	}

	dep := ParseFileDependencies("parser.py", content, allFiles)
	if len(dep.Imports) != 2 {
		t.Errorf("expected 2 local imports, got %d: %+v", len(dep.Imports), dep.Imports)
	}
	if len(dep.Symbols) != 3 {
		t.Errorf("expected 3 symbols (1 class + 2 defs), got %d: %+v", len(dep.Symbols), dep.Symbols)
	}
}

func TestParseFileDependencies_Go(t *testing.T) {
	content := `package engine

import (
	"context"
	"fmt"
	"github.com/foo/bar/pkg/services"
)

type WorkerEngine struct {
	ID string
}

type Runner interface {
	Run(ctx context.Context) error
}

func StartEngine() *WorkerEngine {
	return &WorkerEngine{ID: "1"}
}
`
	allFiles := map[string]bool{
		"pkg/services/registry.go": true,
		"pkg/engine/worker.go":     true,
	}

	dep := ParseFileDependencies("pkg/engine/worker.go", content, allFiles)
	if len(dep.Symbols) < 3 {
		t.Errorf("expected at least 3 symbols (WorkerEngine, Runner, StartEngine), got %d", len(dep.Symbols))
	}
}

func TestParseFileDependencies_Rust(t *testing.T) {
	content := `
use crate::protocol::RESP;
use crate::storage::MemoryStore;

pub struct Server {
    port: u16,
}

pub fn run_server() {
}
`
	allFiles := map[string]bool{
		"src/protocol.rs": true,
		"src/storage.rs":  true,
		"src/main.rs":     true,
	}

	dep := ParseFileDependencies("src/main.rs", content, allFiles)
	if len(dep.Imports) != 2 {
		t.Errorf("expected 2 crate imports, got %d: %+v", len(dep.Imports), dep.Imports)
	}
	if len(dep.Symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(dep.Symbols))
	}
}

func TestImportGraphWalker_TraverseContext(t *testing.T) {
	tempDir := t.TempDir()

	// Setup file structure:
	// src/types.py -> declares RespType
	// src/parser.py -> imports types, declares parse_resp
	// tests/test_parser.py -> imports parser
	srcDir := filepath.Join(tempDir, "src")
	testsDir := filepath.Join(tempDir, "tests")
	_ = os.MkdirAll(srcDir, 0755)
	_ = os.MkdirAll(testsDir, 0755)

	_ = os.WriteFile(filepath.Join(srcDir, "types.py"), []byte("class RespType:\n    pass\n"), 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "parser.py"), []byte("from types import RespType\nclass Parser:\n    pass\n"), 0644)
	_ = os.WriteFile(filepath.Join(testsDir, "test_parser.py"), []byte("from parser import Parser\ndef test_parse():\n    pass\n"), 0644)

	workspaceFiles := []string{
		"src/types.py",
		"src/parser.py",
		"tests/test_parser.py",
	}

	walker := NewImportGraphWalker()
	slicer := NewContextSlicer(config.ContextConfig{})

	// Task targets src/parser.py
	gathered := walker.GatherContext(tempDir, []string{"src/parser.py"}, "Implement parser", "Add RESP parsing", workspaceFiles, slicer)
	if len(gathered) < 2 {
		t.Fatalf("expected at least 2 files gathered (parser + dependency or test), got %d", len(gathered))
	}

	joined := strings.Join(gathered, "\n")
	if !strings.Contains(joined, "parser.py") {
		t.Errorf("expected parser.py in context")
	}
	if !strings.Contains(joined, "types.py") && !strings.Contains(joined, "test_parser.py") {
		t.Errorf("expected neighbor files in context: %s", joined)
	}
}

func TestImportGraphWalker_SeedFromSymbolInDescription(t *testing.T) {
	tempDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tempDir, "models.py"), []byte("class UserSession:\n    pass\n"), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "main.py"), []byte("import models\n"), 0644)

	workspaceFiles := []string{"models.py", "main.py"}
	walker := NewImportGraphWalker()

	// Task targets empty, but mentions UserSession in description
	gathered := walker.GatherContext(tempDir, nil, "Authenticate user", "Validate UserSession token", workspaceFiles, nil)
	joined := strings.Join(gathered, "\n")
	if !strings.Contains(joined, "models.py") {
		t.Errorf("expected models.py in gathered context, got %s", joined)
	}
}
