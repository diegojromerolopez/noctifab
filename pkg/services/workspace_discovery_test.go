package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsPathExcluded(t *testing.T) {
	t.Run("internal metadata directories are excluded", func(t *testing.T) {
		if !IsPathExcluded(".git", nil) {
			t.Errorf("expected .git to be excluded")
		}
		if !IsPathExcluded(".git/config", nil) {
			t.Errorf("expected .git/config to be excluded")
		}
		if !IsPathExcluded(".noctifab", nil) {
			t.Errorf("expected .noctifab to be excluded")
		}
		if !IsPathExcluded(".noctifab/data/noctifab.db", nil) {
			t.Errorf("expected .noctifab/data/noctifab.db to be excluded")
		}
	})

	t.Run("default build artifact directories and file extensions are excluded by default", func(t *testing.T) {
		samples := []string{
			"target/debug/mybinary",
			"node_modules/express/index.js",
			"__pycache__/module.cpython-310.pyc",
			"build/libs/app.jar",
			".venv/bin/activate",
			"dist/bundle.js",
			"bin/tool",
			"main.pyc",
			"object.o",
			"libfoo.so",
			"test.log",
		}
		for _, s := range samples {
			if !IsPathExcluded(s, nil) {
				t.Errorf("expected %q to be excluded by default guardrails", s)
			}
		}
		if IsPathExcluded("src/main.rs", nil) {
			t.Errorf("expected src/main.rs NOT to be excluded")
		}
		if IsPathExcluded("pkg/services/server.go", nil) {
			t.Errorf("expected pkg/services/server.go NOT to be excluded")
		}
	})

	t.Run("configured exclude paths and wildcards are excluded", func(t *testing.T) {
		excludes := []string{"target/", "target_container/", "*.tmp", "build"}
		if !IsPathExcluded("target", excludes) {
			t.Errorf("expected target to be excluded")
		}
		if !IsPathExcluded("target/debug/app", excludes) {
			t.Errorf("expected target/debug/app to be excluded")
		}
		if !IsPathExcluded("target_container/release/app", excludes) {
			t.Errorf("expected target_container/release/app to be excluded")
		}
		if !IsPathExcluded("temp.tmp", excludes) {
			t.Errorf("expected temp.tmp to be excluded")
		}
		if !IsPathExcluded("build/output.o", excludes) {
			t.Errorf("expected build/output.o to be excluded")
		}
		if IsPathExcluded("src/main.rs", excludes) {
			t.Errorf("expected src/main.rs NOT to be excluded")
		}
	})

	t.Run("empty and root paths are not excluded", func(t *testing.T) {
		if IsPathExcluded(".", nil) {
			t.Errorf("expected . NOT to be excluded")
		}
		if IsPathExcluded("", nil) {
			t.Errorf("expected empty string NOT to be excluded")
		}
	})
}

func TestIsTextFile(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("regular text file returns true", func(t *testing.T) {
		textPath := filepath.Join(tempDir, "sample.txt")
		if err := os.WriteFile(textPath, []byte("Hello, world!\nLine 2"), 0644); err != nil {
			t.Fatal(err)
		}
		if !IsTextFile(textPath) {
			t.Errorf("expected sample.txt to be recognized as text file")
		}
	})

	t.Run("binary file with null byte returns false", func(t *testing.T) {
		binPath := filepath.Join(tempDir, "sample.bin")
		if err := os.WriteFile(binPath, []byte{0x7f, 0x45, 0x4c, 0x46, 0x00, 0x01, 0x01}, 0644); err != nil {
			t.Fatal(err)
		}
		if IsTextFile(binPath) {
			t.Errorf("expected sample.bin NOT to be recognized as text file")
		}
	})

	t.Run("empty file or non-existent file returns false", func(t *testing.T) {
		emptyPath := filepath.Join(tempDir, "empty.txt")
		if err := os.WriteFile(emptyPath, []byte{}, 0644); err != nil {
			t.Fatal(err)
		}
		if IsTextFile(emptyPath) {
			t.Errorf("expected empty.txt NOT to be recognized as text file")
		}
		if IsTextFile(filepath.Join(tempDir, "non_existent.txt")) {
			t.Errorf("expected non_existent.txt to return false")
		}
	})
}

func TestListWorkspaceSourceFilesAndSnapshot(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	targetDir := filepath.Join(tempDir, "target_container")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}

	mainFile := filepath.Join(srcDir, "main.rs")
	if err := os.WriteFile(mainFile, []byte("fn main() { println!(\"hello\"); }"), 0644); err != nil {
		t.Fatal(err)
	}

	binFile := filepath.Join(targetDir, "main.o")
	if err := os.WriteFile(binFile, []byte{0x00, 0x01, 0x02}, 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	excludes := []string{"target_container"}
	files, err := ListWorkspaceSourceFiles(ctx, tempDir, excludes)
	if err != nil {
		t.Fatalf("unexpected error listing source files: %v", err)
	}

	if len(files) != 1 || filepath.ToSlash(files[0]) != "src/main.rs" {
		t.Errorf("expected ['src/main.rs'], got %v", files)
	}

	snapshot := CollectWorkspaceSourceSnapshot(ctx, tempDir, excludes, 10, 1000)
	if !strings.Contains(snapshot, "src/main.rs") || !strings.Contains(snapshot, "fn main()") {
		t.Errorf("expected snapshot to contain src/main.rs, got:\n%s", snapshot)
	}
}
