package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestEnsureTargetStubFilesExist(t *testing.T) {
	tempDir := t.TempDir()

	task := &domain.Task{
		ID: "T-STUB-01",
		TargetFiles: []string{
			"src/etag.c",
			"src/etag.h",
			"app/calculator.py",
			"pkg/util/helper.go",
		},
	}

	orch := &Orchestrator{}
	orch.ensureTargetStubFilesExist(tempDir, task)

	for _, relFile := range task.TargetFiles {
		fullPath := filepath.Join(tempDir, relFile)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("expected stub file %s to exist, but it was not created", relFile)
		}
	}

	// Verify idempotency: calling again on existing files does not overwrite
	contentBefore, _ := os.ReadFile(filepath.Join(tempDir, "src/etag.c"))
	orch.ensureTargetStubFilesExist(tempDir, task)
	contentAfter, _ := os.ReadFile(filepath.Join(tempDir, "src/etag.c"))

	if string(contentBefore) != string(contentAfter) {
		t.Errorf("expected ensureTargetStubFilesExist to be idempotent")
	}
}

func TestGenerateStubContent(t *testing.T) {
	tests := []struct {
		file     string
		contains string
	}{
		{"app/main.py", "# Stub implementation for main.py"},
		{"pkg/service/foo.go", "package service"},
		{"src/header.h", "#ifndef HEADER_STUB_H"},
		{"src/main.c", "#include \"main.h\""},
		{"src/app.ts", "export {};"},
		{"src/store.erl", "-module(store)."},
		{"src/catalog.ml", "(* Stub implementation for catalog.ml *)"},
		{"src/chess.cs", "public class chess"},
		{"src/main/java/com/example/App.java", "package com.example;"},
		{"go.mod", "module app"},
		{"Cargo.toml", "[package]"},
		{"package.json", "\"name\": \"app\""},
		{"Gemfile", "https://rubygems.org"},
		{"rebar.config", "{erl_opts, [debug_info]}."},
		{"dune-project", "(lang dune 3.0)"},
		{"pyproject.toml", "[project]"},
		{"CMakeLists.txt", "cmake_minimum_required"},
	}

	for _, tt := range tests {
		got := generateStubContent(tt.file)
		if !strings.Contains(got, tt.contains) {
			t.Errorf("expected stub content for %s to contain %q, got:\n%s", tt.file, tt.contains, got)
		}
	}
}
