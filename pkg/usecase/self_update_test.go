package usecase

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePatch_Allowed(t *testing.T) {
	m := &SelfUpdateManager{}
	if err := m.validatePatch(Patch{Path: "pkg/usecase/x.go"}); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if err := m.validatePatch(Patch{Path: "cmd/noctifab/main.go"}); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestValidatePatch_GoMod(t *testing.T) {
	m := &SelfUpdateManager{}
	if err := m.validatePatch(Patch{Path: "go.mod"}); err == nil {
		t.Error("expected error for go.mod")
	}
}

func TestValidatePatch_GoSum(t *testing.T) {
	m := &SelfUpdateManager{}
	if err := m.validatePatch(Patch{Path: "go.sum"}); err == nil {
		t.Error("expected error for go.sum")
	}
}

func TestValidatePatch_OutsidePrefix(t *testing.T) {
	m := &SelfUpdateManager{}
	if err := m.validatePatch(Patch{Path: "tests/x.go"}); err == nil {
		t.Error("expected error for tests/x.go")
	}
}

func TestValidatePatch_Docs(t *testing.T) {
	m := &SelfUpdateManager{}
	if err := m.validatePatch(Patch{Path: "docs/x.md"}); err == nil {
		t.Error("expected error for docs/x.md")
	}
}

func setupTestModule(t *testing.T, dir string, withTest bool) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test-self-update\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "cmd/noctifab"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cmd/noctifab/main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(dir, "pkg/example"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg/example/hello.go"), []byte("package example\n\nfunc Hello() string { return \"hello\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if withTest {
		if err := os.WriteFile(filepath.Join(dir, "pkg/example/hello_test.go"), []byte("package example\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) {\n\tif Hello() != \"hello\" {\n\t\tt.Fatal(\"expected hello\")\n\t}\n}\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBuildAndTest_Success(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestModule(t, tmpDir, true)

	m := &SelfUpdateManager{
		RepoPath: tmpDir,
		GoCmd:    "go",
	}

	patch := Patch{
		Path:    "pkg/example/hello.go",
		Content: "package example\n\nfunc Hello() string {\n\treturn \"hello\"\n}\n",
	}

	err := m.BuildAndTest(context.Background(), []Patch{patch})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestBuildAndTest_EmptyPatches(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestModule(t, tmpDir, true)

	m := &SelfUpdateManager{
		RepoPath: tmpDir,
		GoCmd:    "go",
	}

	err := m.BuildAndTest(context.Background(), []Patch{})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestBuildAndTest_BuildFailure(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestModule(t, tmpDir, false)

	m := &SelfUpdateManager{
		RepoPath: tmpDir,
		GoCmd:    "go",
	}

	patch := Patch{
		Path:    "cmd/noctifab/main.go",
		Content: "package main\n\nfunc main() {\n",
	}

	err := m.BuildAndTest(context.Background(), []Patch{patch})
	if err == nil {
		t.Fatal("expected error for build failure")
	}
}

func TestBuildAndTest_TestFailure(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestModule(t, tmpDir, true)

	m := &SelfUpdateManager{
		RepoPath: tmpDir,
		GoCmd:    "go",
	}

	patch := Patch{
		Path:    "pkg/example/hello.go",
		Content: "package example\n\nfunc Hello() string { return \"wrong\" }\n",
	}

	err := m.BuildAndTest(context.Background(), []Patch{patch})
	if err == nil {
		t.Fatal("expected error for test failure")
	}
}

func TestBuildAndTest_InvalidPatchRejected(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestModule(t, tmpDir, false)

	m := &SelfUpdateManager{
		RepoPath: tmpDir,
		GoCmd:    "go",
	}

	patch := Patch{
		Path:    "go.mod",
		Content: "module hacked\n",
	}

	err := m.BuildAndTest(context.Background(), []Patch{patch})
	if err == nil {
		t.Fatal("expected error for invalid patch")
	}
}
