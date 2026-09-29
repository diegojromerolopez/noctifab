package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockPreWarmRunner struct {
	mu       sync.Mutex
	commands []string
}

func (m *mockPreWarmRunner) RunCommand(ctx context.Context, dir, command, input string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands = append(m.commands, command)
	return "downloaded ok", nil
}

func TestDetectManifests(t *testing.T) {
	tmpDir := t.TempDir()

	// Initially empty
	if detected := DetectManifests(tmpDir); len(detected) != 0 {
		t.Fatalf("expected 0 manifests, got %v", detected)
	}

	// Create go.mod and package.json
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte("{}"), 0644)

	detected := DetectManifests(tmpDir)
	if len(detected) != 2 {
		t.Fatalf("expected 2 manifests, got %d: %v", len(detected), detected)
	}
}

func TestGetPreWarmCommand(t *testing.T) {
	tmpDir := t.TempDir()

	mgr, cmd := GetPreWarmCommand("go.mod", tmpDir)
	if mgr != "go" || cmd != "go mod download" {
		t.Errorf("unexpected go.mod command: %s %s", mgr, cmd)
	}

	mgr, cmd = GetPreWarmCommand("Cargo.toml", tmpDir)
	if mgr != "cargo" || cmd != "cargo fetch" {
		t.Errorf("unexpected Cargo.toml command: %s %s", mgr, cmd)
	}

	mgr, cmd = GetPreWarmCommand("package.json", tmpDir)
	if mgr != "npm" || !strings.Contains(cmd, "npm install") {
		t.Errorf("unexpected package.json command: %s %s", mgr, cmd)
	}

	mgr, cmd = GetPreWarmCommand("requirements.txt", tmpDir)
	if mgr != "pip" || !strings.Contains(cmd, "pip download") {
		t.Errorf("unexpected requirements.txt command: %s %s", mgr, cmd)
	}

	mgr, cmd = GetPreWarmCommand("pyproject.toml", tmpDir)
	if mgr != "pip" || !strings.Contains(cmd, "pip install --dry-run") {
		t.Errorf("unexpected pyproject.toml command: %s %s", mgr, cmd)
	}
}

func TestPreWarmDependencies_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "Cargo.toml"), []byte("[package]\nname=\"test\""), 0644)

	dm := NewDependencyManager([]string{"go", "cargo"})
	runner := &mockPreWarmRunner{}

	results, err := dm.PreWarmDependencies(context.Background(), tmpDir, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, res := range results {
		if res.Error != "" {
			t.Errorf("unexpected error in result for %s: %s", res.Manifest, res.Error)
		}
	}
}

func TestStartBackgroundPreWarm(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)

	dm := NewDependencyManager([]string{"go"})
	runner := &mockPreWarmRunner{}

	ch := dm.StartBackgroundPreWarm(context.Background(), tmpDir, runner)
	select {
	case res := <-ch:
		if len(res) != 1 {
			t.Fatalf("expected 1 result from background channel, got %d", len(res))
		}
		if res[0].Manifest != "go.mod" {
			t.Fatalf("expected manifest go.mod, got %s", res[0].Manifest)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("background pre-warm timed out")
	}
}
