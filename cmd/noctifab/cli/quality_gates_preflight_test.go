package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

func TestDetectProjectLanguages(t *testing.T) {
	tmp := t.TempDir()

	// Initially empty
	langs := DetectProjectLanguages(tmp)
	if len(langs) != 0 {
		t.Errorf("expected 0 languages for empty dir, got %v", langs)
	}

	// Add Go and Python files
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "requirements.txt"), []byte("pytest"), 0644); err != nil {
		t.Fatal(err)
	}

	langs = DetectProjectLanguages(tmp)
	if !langs["go"] {
		t.Errorf("expected 'go' to be detected")
	}
	if !langs["python"] {
		t.Errorf("expected 'python' to be detected")
	}
	if langs["rust"] {
		t.Errorf("did not expect 'rust' to be detected")
	}
}

func TestIsTrivialCommand(t *testing.T) {
	trivial := []string{
		"",
		"true",
		"@true",
		":",
		"@:",
		"exit 0",
		"@exit 0",
		"echo ok",
		"@echo ok",
		"echo pass",
		"@echo passed",
		"echo skipped",
		"@echo skipped",
		"printf 'done\\n'",
		"@true; exit 0",
	}

	for _, cmd := range trivial {
		if !IsTrivialCommand(cmd) {
			t.Errorf("expected %q to be identified as trivial", cmd)
		}
	}

	nonTrivial := []string{
		"go test -v ./...",
		"pytest -v tests/",
		"python3 -m unittest discover -s tests",
		"cargo test",
		"npm test",
		"make test-unit",
		"clang -Wall -Werror main.c -o main",
	}

	for _, cmd := range nonTrivial {
		if IsTrivialCommand(cmd) {
			t.Errorf("expected %q to NOT be identified as trivial", cmd)
		}
	}
}

func TestVerifyQualityAndReleaseGates_LanguageMismatch(t *testing.T) {
	tmp := t.TempDir()
	// Python project
	if err := os.WriteFile(filepath.Join(tmp, "requirements.txt"), []byte("django\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "app.py"), []byte("print('hello')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Sandbox: config.SandboxConfig{
			TestCommand: "go test -v ./...",
		},
	}

	err := VerifyQualityAndReleaseGates(cfg, tmp)
	if err == nil {
		t.Fatal("expected language mismatch error for Go test_command on Python project, got nil")
	}
	if !containsSubstring(err.Error(), "uses go toolchain") && !containsSubstring(err.Error(), "no go code") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestVerifyQualityAndReleaseGates_TrivialCommand(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Sandbox: config.SandboxConfig{
			TestCommand: "exit 0",
		},
	}

	err := VerifyQualityAndReleaseGates(cfg, tmp)
	if err == nil {
		t.Fatal("expected trivial command error, got nil")
	}
	if !containsSubstring(err.Error(), "is trivial") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestVerifyQualityAndReleaseGates_MakefileTrivialTarget(t *testing.T) {
	tmp := t.TempDir()
	makefile := `all: build

test:
	@echo skipped
`
	if err := os.WriteFile(filepath.Join(tmp, "Makefile"), []byte(makefile), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	err := VerifyQualityAndReleaseGates(cfg, tmp)
	if err == nil {
		t.Fatal("expected error for trivial Makefile test target, got nil")
	}
	if !containsSubstring(err.Error(), "is trivial or empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestVerifyQualityAndReleaseGates_MakefileMissingTestTarget(t *testing.T) {
	tmp := t.TempDir()
	makefile := `all: build

build:
	@echo building
`
	if err := os.WriteFile(filepath.Join(tmp, "Makefile"), []byte(makefile), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	err := VerifyQualityAndReleaseGates(cfg, tmp)
	if err == nil {
		t.Fatal("expected error for Makefile missing test target, got nil")
	}
	if !containsSubstring(err.Error(), "missing required quality gate target") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestVerifyQualityAndReleaseGates_ValidProject(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module myapp\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Sandbox: config.SandboxConfig{
			TestCommand: "go test -v ./...",
		},
	}

	err := VerifyQualityAndReleaseGates(cfg, tmp)
	if err != nil {
		t.Fatalf("expected valid project to pass pre-flight quality gate check, got error: %v", err)
	}
}

func TestVerifyQualityAndReleaseGates_MakefileMultiTarget(t *testing.T) {
	tmp := t.TempDir()
	makefile := `all: build

test check: build
	@echo "running test suite"
	./bin/test_runner
`
	if err := os.WriteFile(filepath.Join(tmp, "Makefile"), []byte(makefile), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	err := VerifyQualityAndReleaseGates(cfg, tmp)
	if err != nil {
		t.Fatalf("expected multi-target rule 'test check:' to pass, got error: %v", err)
	}
}

func TestVerifyQualityAndReleaseGates_MakefileDelegationTarget(t *testing.T) {
	tmp := t.TempDir()
	makefile := `all: build

test: test-unit test-python

test-unit:
	./bin/unit_tests

test-python:
	python3 -m unittest
`
	if err := os.WriteFile(filepath.Join(tmp, "Makefile"), []byte(makefile), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	err := VerifyQualityAndReleaseGates(cfg, tmp)
	if err != nil {
		t.Fatalf("expected delegated test target to pass, got error: %v", err)
	}
}

func TestVerifyQualityAndReleaseGates_MakefileMissingTargetFallback(t *testing.T) {
	tmp := t.TempDir()
	makefile := `all: build
build:
	@echo building
`
	if err := os.WriteFile(filepath.Join(tmp, "Makefile"), []byte(makefile), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Sandbox: config.SandboxConfig{
			TestCommand: "pytest -v",
		},
	}
	if err := os.WriteFile(filepath.Join(tmp, "requirements.txt"), []byte("pytest\n"), 0644); err != nil {
		t.Fatal(err)
	}

	err := VerifyQualityAndReleaseGates(cfg, tmp)
	if err != nil {
		t.Fatalf("expected fallback to valid TestCommand when Makefile lacks test target, got error: %v", err)
	}
}

func containsSubstring(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
