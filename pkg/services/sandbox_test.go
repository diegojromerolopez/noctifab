package services

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

func TestHostSandbox_EvictTool(t *testing.T) {
	s := NewHostSandbox([]string{"sh", "pytest"}, "", 0, nil)
	if s.IsToolEvicted("pytest") {
		t.Error("expected pytest to not be evicted initially")
	}

	s.EvictTool("pytest")
	if !s.IsToolEvicted("pytest") {
		t.Error("expected pytest to be evicted after EvictTool")
	}

	tools := s.GetEvictedTools()
	if len(tools) != 1 || tools[0] != "pytest" {
		t.Errorf("expected ['pytest'], got %v", tools)
	}

	// Running an evicted tool should fail-fast / degrade
	ctx := context.Background()
	_, err := s.RunCommand(ctx, t.TempDir(), "pytest", "")
	if err == nil {
		t.Error("expected error when running evicted tool")
	}
}

func TestBuildCacheVolumeArgs(t *testing.T) {
	args := BuildCacheVolumeArgs()
	if len(args) == 0 {
		t.Skip("skipping build cache test: home directory not available")
	}
	hasGoMod := false
	for i, arg := range args {
		if arg == "-v" && i+1 < len(args) && (filepath.Ext(args[i+1]) == "" || filepath.Base(args[i+1]) != "") {
			hasGoMod = true
			break
		}
	}
	if !hasGoMod {
		t.Error("expected volume flag args in BuildCacheVolumeArgs")
	}
}

func TestDetectDefaultTestCommand(t *testing.T) {
	// 1. Makefile with test target
	tmpMake := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpMake, "Makefile"), []byte("all:\n\ntest:\n\t@echo ok\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectDefaultTestCommand(tmpMake); got != "make test" {
		t.Errorf("expected 'make test', got %q", got)
	}

	// 2. Cargo.toml
	tmpRust := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpRust, "Cargo.toml"), []byte("[package]"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectDefaultTestCommand(tmpRust); got != "cargo test" {
		t.Errorf("expected 'cargo test', got %q", got)
	}

	// 3. package.json
	tmpJS := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpJS, "package.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectDefaultTestCommand(tmpJS); got != "npm test" {
		t.Errorf("expected 'npm test', got %q", got)
	}

	// 4. Python
	tmpPy := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpPy, "requirements.txt"), []byte("django\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectDefaultTestCommand(tmpPy); got != "python3 -m unittest discover -s tests" {
		t.Errorf("expected 'python3 -m unittest discover -s tests', got %q", got)
	}

	// 5. Go
	tmpGo := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpGo, "go.mod"), []byte("module test"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectDefaultTestCommand(tmpGo); got != "go test -v ./..." {
		t.Errorf("expected 'go test -v ./...', got %q", got)
	}
}

func TestDetectDefaultFormatterCommand(t *testing.T) {
	// 1. Makefile with format target
	tmpMake := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpMake, "Makefile"), []byte("all:\n\nformat:\n\t@echo fmt\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectDefaultFormatterCommand(tmpMake); got != "make format" {
		t.Errorf("expected 'make format', got %q", got)
	}

	// 2. Go
	tmpGo := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpGo, "go.mod"), []byte("module test"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectDefaultFormatterCommand(tmpGo); got != "go fmt ./..." {
		t.Errorf("expected 'go fmt ./...', got %q", got)
	}

	// 3. Cargo.toml
	tmpRust := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpRust, "Cargo.toml"), []byte("[package]"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectDefaultFormatterCommand(tmpRust); got != "cargo fmt" {
		t.Errorf("expected 'cargo fmt', got %q", got)
	}

	// 4. package.json
	tmpJS := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpJS, "package.json"), []byte(`{"scripts":{"format":"prettier -w ."}}`), 0644); err != nil {
		t.Fatal(err)
	}
	if got := DetectDefaultFormatterCommand(tmpJS); got != "npm run format" {
		t.Errorf("expected 'npm run format', got %q", got)
	}
}

func TestDetectAutoFixImportsCommand(t *testing.T) {
	t.Run("when python project has src and tests directories it targets them with ruff", func(t *testing.T) {
		tmpDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(tmpDir, "pyproject.toml"), []byte("[project]"), 0644)
		_ = os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
		_ = os.MkdirAll(filepath.Join(tmpDir, "tests"), 0755)

		got := DetectAutoFixImportsCommand(tmpDir)
		if got != "ruff check --select F401 --fix src tests" {
			t.Errorf("expected 'ruff check --select F401 --fix src tests', got %q", got)
		}
	})

	t.Run("when python project has no standard subdirectories it falls back to dot", func(t *testing.T) {
		tmpDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(tmpDir, "requirements.txt"), []byte("requests\n"), 0644)

		got := DetectAutoFixImportsCommand(tmpDir)
		if got != "ruff check --select F401 --fix ." {
			t.Errorf("expected 'ruff check --select F401 --fix .', got %q", got)
		}
	})

	t.Run("when go project it returns goimports", func(t *testing.T) {
		tmpDir := t.TempDir()
		_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)

		got := DetectAutoFixImportsCommand(tmpDir)
		if got != "goimports -w ." {
			t.Errorf("expected 'goimports -w .', got %q", got)
		}
	})

	t.Run("when empty project path provided it returns empty string", func(t *testing.T) {
		if got := DetectAutoFixImportsCommand(""); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})
}
