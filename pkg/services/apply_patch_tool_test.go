package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestApplyPatchTool_Execute(t *testing.T) {
	t.Run("when_applying_single_file_patch_it_updates_the_file_correctly", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "main.go")
		initialContent := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
		if err := os.WriteFile(filePath, []byte(initialContent), 0644); err != nil {
			t.Fatalf("failed to write initial file: %v", err)
		}

		state := &domain.State{ProjectPath: tmpDir}
		tool := &ApplyPatchTool{}

		patch := `--- main.go
+++ main.go
@@ -3,3 +3,3 @@
 func main() {
-	println("hello")
+	println("hello world")
 }
`

		res, err := tool.Execute(context.Background(), state, map[string]any{
			"patch": patch,
			"path":  "main.go",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(res, "Patch applied successfully") {
			t.Errorf("expected success message, got: %s", res)
		}

		updated, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("failed to read updated file: %v", err)
		}
		expected := "package main\n\nfunc main() {\n\tprintln(\"hello world\")\n}\n"
		if string(updated) != expected {
			t.Errorf("expected content:\n%s\ngot:\n%s", expected, string(updated))
		}
	})

	t.Run("when_applying_multi_file_patch_it_updates_all_target_files", func(t *testing.T) {
		tmpDir := t.TempDir()
		file1 := filepath.Join(tmpDir, "pkg", "foo.go")
		file2 := filepath.Join(tmpDir, "pkg", "bar.go")
		_ = os.MkdirAll(filepath.Dir(file1), 0755)

		_ = os.WriteFile(file1, []byte("package pkg\n\nconst Version = \"1.0.0\"\n"), 0644)
		_ = os.WriteFile(file2, []byte("package pkg\n\nfunc Bar() string { return \"bar\" }\n"), 0644)

		state := &domain.State{ProjectPath: tmpDir}
		tool := &ApplyPatchTool{}

		patch := `--- a/pkg/foo.go
+++ b/pkg/foo.go
@@ -3,1 +3,1 @@
-const Version = "1.0.0"
+const Version = "1.1.0"
--- a/pkg/bar.go
+++ b/pkg/bar.go
@@ -3,1 +3,1 @@
-func Bar() string { return "bar" }
+func Bar() string { return "bar v2" }
`

		res, err := tool.Execute(context.Background(), state, map[string]any{
			"patch": patch,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(res, "2 file(s)") {
			t.Errorf("expected 2 files updated, got: %s", res)
		}

		c1, _ := os.ReadFile(file1)
		if !strings.Contains(string(c1), "1.1.0") {
			t.Errorf("file1 not updated properly: %s", string(c1))
		}
		c2, _ := os.ReadFile(file2)
		if !strings.Contains(string(c2), "bar v2") {
			t.Errorf("file2 not updated properly: %s", string(c2))
		}
	})

	t.Run("when_creating_a_new_file_via_patch_it_creates_file", func(t *testing.T) {
		tmpDir := t.TempDir()
		state := &domain.State{ProjectPath: tmpDir}
		tool := &ApplyPatchTool{}

		patch := `--- /dev/null
+++ b/new_module.go
@@ -0,0 +1,5 @@
+package main
+
+func NewFunc() {
+	// new file
+}
`

		res, err := tool.Execute(context.Background(), state, map[string]any{
			"patch": patch,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(res, "Patch applied successfully") {
			t.Errorf("got output: %s", res)
		}

		createdPath := filepath.Join(tmpDir, "new_module.go")
		content, err := os.ReadFile(createdPath)
		if err != nil {
			t.Fatalf("failed to read created file: %v", err)
		}
		if !strings.Contains(string(content), "func NewFunc()") {
			t.Errorf("unexpected created content: %s", string(content))
		}
	})

	t.Run("when_deleting_a_file_via_patch_it_removes_file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "obsolete.go")
		_ = os.WriteFile(filePath, []byte("package main\n"), 0644)

		state := &domain.State{ProjectPath: tmpDir}
		tool := &ApplyPatchTool{}

		patch := `--- a/obsolete.go
+++ /dev/null
@@ -1,1 +0,0 @@
-package main
`

		res, err := tool.Execute(context.Background(), state, map[string]any{
			"patch": patch,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(res, "deleted") {
			t.Errorf("expected deletion message, got: %s", res)
		}

		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Errorf("file was not deleted")
		}
	})

	t.Run("when_hunk_offsets_shift_slightly_it_fuzzy_matches_and_patches", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "shifted.txt")
		// Content has extra leading lines shifting the line offset
		initialContent := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\ntarget line\nline 8\n"
		_ = os.WriteFile(filePath, []byte(initialContent), 0644)

		state := &domain.State{ProjectPath: tmpDir}
		tool := &ApplyPatchTool{}

		// Patch specifies OldStart = 2, but target line is actually at line 7
		patch := `--- shifted.txt
+++ shifted.txt
@@ -2,3 +2,3 @@
 line 6
-target line
+updated target line
 line 8
`

		_, err := tool.Execute(context.Background(), state, map[string]any{
			"patch": patch,
			"path":  "shifted.txt",
		})
		if err != nil {
			t.Fatalf("fuzzy match patch failed: %v", err)
		}

		updated, _ := os.ReadFile(filePath)
		if !strings.Contains(string(updated), "updated target line") {
			t.Errorf("fuzzy match failed to update content: %s", string(updated))
		}
	})

	t.Run("when_sandbox_boundary_is_violated_it_returns_sandbox_error", func(t *testing.T) {
		tmpDir := t.TempDir()
		state := &domain.State{ProjectPath: tmpDir}
		tool := &ApplyPatchTool{}

		patch := `--- a/../outside.go
+++ b/../outside.go
@@ -1,1 +1,1 @@
-foo
+bar
`

		_, err := tool.Execute(context.Background(), state, map[string]any{
			"patch": patch,
		})
		if err == nil {
			t.Fatal("expected sandbox violation error, got nil")
		}
		if !strings.Contains(err.Error(), "Sandbox violation") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("when_patch_argument_is_missing_it_returns_error", func(t *testing.T) {
		state := &domain.State{ProjectPath: t.TempDir()}
		tool := &ApplyPatchTool{}

		_, err := tool.Execute(context.Background(), state, map[string]any{})
		if err == nil {
			t.Fatal("expected error on missing patch argument, got nil")
		}
	})
}
