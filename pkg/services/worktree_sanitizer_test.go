package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeWorkspace(t *testing.T) {
	tempDir := t.TempDir()

	// Setup normal files
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("mkdir src failed: %v", err)
	}
	mainGo := filepath.Join(srcDir, "main.go")
	if err := os.WriteFile(mainGo, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("write main.go failed: %v", err)
	}

	// Setup ephemeral cache dirs
	pycache := filepath.Join(srcDir, "__pycache__")
	if err := os.MkdirAll(pycache, 0755); err != nil {
		t.Fatalf("mkdir __pycache__ failed: %v", err)
	}
	pyc := filepath.Join(pycache, "module.cpython-310.pyc")
	if err := os.WriteFile(pyc, []byte("bytecode"), 0644); err != nil {
		t.Fatalf("write pyc failed: %v", err)
	}

	pytestCache := filepath.Join(tempDir, ".pytest_cache")
	if err := os.MkdirAll(pytestCache, 0755); err != nil {
		t.Fatalf("mkdir .pytest_cache failed: %v", err)
	}

	// Setup ephemeral cache files
	dsStore := filepath.Join(tempDir, ".DS_Store")
	if err := os.WriteFile(dsStore, []byte("macos garbage"), 0644); err != nil {
		t.Fatalf("write dsStore failed: %v", err)
	}
	tmpFile := filepath.Join(srcDir, "temp.tmp")
	if err := os.WriteFile(tmpFile, []byte("temp"), 0644); err != nil {
		t.Fatalf("write tmpFile failed: %v", err)
	}

	// Run SanitizeWorkspace
	if err := SanitizeWorkspace(tempDir); err != nil {
		t.Fatalf("SanitizeWorkspace failed: %v", err)
	}

	// Verify normal files remain
	if _, err := os.Stat(mainGo); err != nil {
		t.Errorf("expected %s to exist, err: %v", mainGo, err)
	}

	// Verify ephemeral items are deleted
	if _, err := os.Stat(pycache); !os.IsNotExist(err) {
		t.Errorf("expected __pycache__ to be deleted, err: %v", err)
	}
	if _, err := os.Stat(pytestCache); !os.IsNotExist(err) {
		t.Errorf("expected .pytest_cache to be deleted, err: %v", err)
	}
	if _, err := os.Stat(dsStore); !os.IsNotExist(err) {
		t.Errorf("expected .DS_Store to be deleted, err: %v", err)
	}
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Errorf("expected temp.tmp to be deleted, err: %v", err)
	}
}

func TestSanitizeWorkspace_WithCustomSkipFolders(t *testing.T) {
	tempDir := t.TempDir()

	customDir := filepath.Join(tempDir, "custom_artifacts")
	if err := os.MkdirAll(customDir, 0755); err != nil {
		t.Fatalf("mkdir custom_artifacts failed: %v", err)
	}
	// An ephemeral file inside custom_artifacts that would otherwise be deleted
	nestedTmp := filepath.Join(customDir, "data.tmp")
	if err := os.WriteFile(nestedTmp, []byte("preserve me"), 0644); err != nil {
		t.Fatalf("write nestedTmp failed: %v", err)
	}

	// Normal temp file outside
	rootTmp := filepath.Join(tempDir, "root.tmp")
	if err := os.WriteFile(rootTmp, []byte("delete me"), 0644); err != nil {
		t.Fatalf("write rootTmp failed: %v", err)
	}

	if err := SanitizeWorkspace(tempDir, "custom_artifacts"); err != nil {
		t.Fatalf("SanitizeWorkspace failed: %v", err)
	}

	// The custom skip folder and its contents should be preserved untouched
	if _, err := os.Stat(nestedTmp); err != nil {
		t.Errorf("expected %s to be preserved by custom skip folder, err: %v", nestedTmp, err)
	}
	// The outside temp file should be deleted
	if _, err := os.Stat(rootTmp); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted, err: %v", rootTmp, err)
	}
}

func TestFilterRelevantFiles(t *testing.T) {
	tempDir := t.TempDir()

	textPath := filepath.Join(tempDir, "code.py")
	if err := os.WriteFile(textPath, []byte("print('hello')\n"), 0644); err != nil {
		t.Fatalf("write code.py failed: %v", err)
	}

	binPath := filepath.Join(tempDir, "image.png")
	if err := os.WriteFile(binPath, []byte("\x89PNG\r\n\x1a\n\x00\x00"), 0644); err != nil {
		t.Fatalf("write image.png failed: %v", err)
	}

	emptyPath := filepath.Join(tempDir, "empty.txt")
	if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
		t.Fatalf("write empty.txt failed: %v", err)
	}

	hugePath := filepath.Join(tempDir, "huge.txt")
	hugeData := make([]byte, MaxContextFileSize+10)
	for i := range hugeData {
		hugeData[i] = 'a'
	}
	if err := os.WriteFile(hugePath, hugeData, 0644); err != nil {
		t.Fatalf("write huge.txt failed: %v", err)
	}

	files := []string{"code.py", "image.png", "empty.txt", "huge.txt", "nonexistent.py"}
	filtered := FilterRelevantFiles(tempDir, files, nil)

	if len(filtered) != 1 || filtered[0] != "code.py" {
		t.Fatalf("expected only ['code.py'], got: %v", filtered)
	}
}

func TestIsContextRelevantFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"src/app.py", true},
		{"pkg/services/server.go", true},
		{"assets/logo.png", false},
		{"docs/manual.pdf", false},
		{".git/config", false},
		{".noctifab/state.json", false},
		{"node_modules/lodash/index.js", false},
		{"app.pyc", false},
		{".DS_Store", false},
	}

	for _, tt := range tests {
		got := IsContextRelevantFile(tt.path, nil)
		if got != tt.expected {
			t.Errorf("IsContextRelevantFile(%q) = %v; want %v", tt.path, got, tt.expected)
		}
	}
}
