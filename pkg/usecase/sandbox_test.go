package usecase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectProjectLanguage_Go(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectProjectLanguage(tmp); got != "go" {
		t.Errorf("expected 'go', got %q", got)
	}
}

func TestDetectProjectLanguage_Rust(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "Cargo.toml"), []byte("[package]\nname = \"test\""), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectProjectLanguage(tmp); got != "rust" {
		t.Errorf("expected 'rust', got %q", got)
	}
}

func TestDetectProjectLanguage_JavaScript(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectProjectLanguage(tmp); got != "javascript" {
		t.Errorf("expected 'javascript', got %q", got)
	}
}

func TestDetectProjectLanguage_PythonRequirements(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "requirements.txt"), []byte("pytest\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectProjectLanguage(tmp); got != "python" {
		t.Errorf("expected 'python', got %q", got)
	}
}

func TestDetectProjectLanguage_PythonSetup(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "setup.py"), []byte("from setuptools import setup\nsetup()"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectProjectLanguage(tmp); got != "python" {
		t.Errorf("expected 'python', got %q", got)
	}
}

func TestDetectProjectLanguage_Java(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "pom.xml"), []byte("<project></project>"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectProjectLanguage(tmp); got != "java" {
		t.Errorf("expected 'java', got %q", got)
	}
}

func TestDetectProjectLanguage_JavaGradle(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "build.gradle"), []byte("apply plugin: 'java'"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectProjectLanguage(tmp); got != "java" {
		t.Errorf("expected 'java', got %q", got)
	}
}

func TestDetectProjectLanguage_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	if got := DetectProjectLanguage(tmp); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestDetectProjectLanguage_Precedence(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "Cargo.toml"), []byte("[package]"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectProjectLanguage(tmp); got != "go" {
		t.Errorf("expected 'go' (precedence), got %q", got)
	}
}

func TestHostSandbox_DepMgrNil_SkipsInstall(t *testing.T) {
	s := NewHostSandbox([]string{"sh", "echo"}, "", 0, nil)
	ctx := context.Background()
	tmp := t.TempDir()
	out, err := s.RunCommand(ctx, tmp, "echo hello", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Error("expected output from echo command")
	}
}

func TestHostSandbox_DepMgrSet_WithAllowedCommand(t *testing.T) {
	dm := NewDependencyManager([]string{"pip", "go", "brew", "curl"})
	s := NewHostSandbox([]string{"echo"}, "", 0, dm)
	ctx := context.Background()
	out, err := s.RunCommand(ctx, t.TempDir(), "echo depmgr works", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == "" {
		t.Error("expected output from echo command")
	}
}
