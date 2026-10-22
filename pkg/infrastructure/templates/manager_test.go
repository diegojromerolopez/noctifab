package templates

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

func TestManager_GetBuiltin(t *testing.T) {
	mgr := NewManager("")

	// Canonical name
	content, source, path, err := mgr.Get("python")
	if err != nil {
		t.Fatalf("unexpected error getting python template: %v", err)
	}
	if source != SourceBuiltin {
		t.Errorf("expected SourceBuiltin, got %s", source)
	}
	if path != "" {
		t.Errorf("expected empty path for builtin, got %q", path)
	}
	if content == "" {
		t.Error("expected non-empty template content")
	}

	// Alias resolution
	contentJS, sourceJS, _, errJS := mgr.Get("javascript")
	if errJS != nil {
		t.Fatalf("unexpected error resolving javascript alias: %v", errJS)
	}
	if sourceJS != SourceBuiltin {
		t.Errorf("expected SourceBuiltin for alias, got %s", sourceJS)
	}
	contentTS, _, _, _ := mgr.Get("typescript")
	if contentJS != contentTS {
		t.Errorf("expected javascript alias to match typescript content")
	}

	// Unknown template
	_, _, _, errUnknown := mgr.Get("unknown_lang_xyz")
	if errUnknown == nil {
		t.Error("expected error for non-existent template, got nil")
	}
}

func TestManager_GlobalAndWorkspacePrecedence(t *testing.T) {
	wsDir := t.TempDir()
	globalDir := t.TempDir()

	mgr := NewManager(wsDir)
	mgr.SetGlobalDir(globalDir)

	// Baseline: python is builtin
	_, source, _, err := mgr.Get("python")
	if err != nil || source != SourceBuiltin {
		t.Fatalf("expected builtin python, got source=%s, err=%v", source, err)
	}

	// Add global python template: ~/.noctifab/templates/python.yaml
	globalCustomContent := "sandbox:\n  test_command: custom-global-test\n"
	if err := os.WriteFile(filepath.Join(globalDir, "python.yaml"), []byte(globalCustomContent), 0644); err != nil {
		t.Fatalf("failed to write global template: %v", err)
	}

	contentGlobal, sourceGlobal, pathGlobal, errGlobal := mgr.Get("python")
	if errGlobal != nil {
		t.Fatalf("unexpected error getting global template: %v", errGlobal)
	}
	if sourceGlobal != SourceGlobal {
		t.Errorf("expected SourceGlobal, got %s", sourceGlobal)
	}
	if contentGlobal != globalCustomContent {
		t.Errorf("expected global custom content, got %q", contentGlobal)
	}
	if pathGlobal != filepath.Join(globalDir, "python.yaml") {
		t.Errorf("unexpected path %q", pathGlobal)
	}

	// Add workspace python template: <workspace>/.noctifab/templates/python.yaml
	wsTemplatesDir := filepath.Join(wsDir, ".noctifab", "templates")
	if err := os.MkdirAll(wsTemplatesDir, 0755); err != nil {
		t.Fatalf("failed to create ws templates dir: %v", err)
	}
	wsCustomContent := "sandbox:\n  test_command: custom-workspace-test\n"
	if err := os.WriteFile(filepath.Join(wsTemplatesDir, "python.yaml"), []byte(wsCustomContent), 0644); err != nil {
		t.Fatalf("failed to write ws template: %v", err)
	}

	// Workspace must override global
	contentWS, sourceWS, pathWS, errWS := mgr.Get("python")
	if errWS != nil {
		t.Fatalf("unexpected error getting ws template: %v", errWS)
	}
	if sourceWS != SourceWorkspace {
		t.Errorf("expected SourceWorkspace, got %s", sourceWS)
	}
	if contentWS != wsCustomContent {
		t.Errorf("expected workspace custom content, got %q", contentWS)
	}
	if pathWS != filepath.Join(wsTemplatesDir, "python.yaml") {
		t.Errorf("unexpected path %q", pathWS)
	}
}

func TestManager_List(t *testing.T) {
	wsDir := t.TempDir()
	globalDir := t.TempDir()

	mgr := NewManager(wsDir)
	mgr.SetGlobalDir(globalDir)

	list, err := mgr.List()
	if err != nil {
		t.Fatalf("unexpected error listing templates: %v", err)
	}
	if len(list) < len(BuiltinTemplates) {
		t.Errorf("expected at least %d templates, got %d", len(BuiltinTemplates), len(list))
	}

	// Add a unique user template in global
	customGlobal := "sandbox:\n  test_command: my-lang-test\n"
	if err := os.WriteFile(filepath.Join(globalDir, "customlang.yaml"), []byte(customGlobal), 0644); err != nil {
		t.Fatalf("failed to write global template: %v", err)
	}

	listAfter, errAfter := mgr.List()
	if errAfter != nil {
		t.Fatalf("unexpected error listing templates after addition: %v", errAfter)
	}

	foundCustom := false
	for _, info := range listAfter {
		if info.Name == "customlang" {
			foundCustom = true
			if info.Source != SourceGlobal {
				t.Errorf("expected SourceGlobal for customlang, got %s", info.Source)
			}
		}
	}
	if !foundCustom {
		t.Error("expected customlang to appear in template list")
	}
}

func TestManager_Apply(t *testing.T) {
	mgr := NewManager("")

	cfg := config.DefaultConfig()

	// Apply python template
	if err := mgr.Apply(cfg, "python"); err != nil {
		t.Fatalf("unexpected error applying python template: %v", err)
	}

	if cfg.Sandbox.TestCommand != "pytest" {
		t.Errorf("expected TestCommand 'pytest', got %q", cfg.Sandbox.TestCommand)
	}
	if cfg.Sandbox.FormatterCommand != "ruff format ." {
		t.Errorf("expected FormatterCommand 'ruff format .', got %q", cfg.Sandbox.FormatterCommand)
	}

	hasPytest := false
	for _, cmd := range cfg.Sandbox.AllowedCommands {
		if cmd == "pytest" {
			hasPytest = true
			break
		}
	}
	if !hasPytest {
		t.Errorf("expected 'pytest' in AllowedCommands, got %v", cfg.Sandbox.AllowedCommands)
	}
}

func TestManager_Export(t *testing.T) {
	destDir := t.TempDir()
	mgr := NewManager("")

	// Export rust template
	outPath, err := mgr.Export("rust", destDir, false)
	if err != nil {
		t.Fatalf("unexpected error exporting rust template: %v", err)
	}
	if filepath.Base(outPath) != "rust.yaml" {
		t.Errorf("expected rust.yaml, got %s", outPath)
	}

	// Export again without overwrite should fail
	_, errSecond := mgr.Export("rust", destDir, false)
	if errSecond == nil {
		t.Error("expected error exporting when file already exists without overwrite")
	}

	// Export again with overwrite should succeed
	_, errOverwrite := mgr.Export("rust", destDir, true)
	if errOverwrite != nil {
		t.Errorf("expected success with overwrite=true, got %v", errOverwrite)
	}
}
