package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"gopkg.in/yaml.v3"
)

func TestTemplatesListCommand(t *testing.T) {
	var buf bytes.Buffer
	templatesListCmd.SetOut(&buf)

	if err := templatesListCmd.RunE(templatesListCmd, []string{}); err != nil {
		t.Fatalf("templates list failed: %v", err)
	}

	out := buf.String()
	for _, expected := range []string{"python", "rust", "go", "typescript", "c", "ruby", "csharp", "java", "erlang", "ocaml"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected templates list to contain %q, got:\n%s", expected, out)
		}
	}
}

func TestTemplatesShowCommand(t *testing.T) {
	var buf bytes.Buffer
	templatesShowCmd.SetOut(&buf)

	if err := templatesShowCmd.RunE(templatesShowCmd, []string{"python"}); err != nil {
		t.Fatalf("templates show python failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "test_command: pytest") {
		t.Errorf("expected python template to show 'pytest', got:\n%s", out)
	}
	if !strings.Contains(out, "Source:   builtin") {
		t.Errorf("expected Source: builtin, got:\n%s", out)
	}
}

func TestTemplatesInitCommand(t *testing.T) {
	wsDir := t.TempDir()
	t.Setenv("NOCTIFAB_CONFIG", filepath.Join(wsDir, ".noctifab", "config.yaml"))

	var buf bytes.Buffer
	templatesInitCmd.SetOut(&buf)

	// Init specific template: rust
	if err := templatesInitCmd.RunE(templatesInitCmd, []string{"rust"}); err != nil {
		t.Fatalf("templates init rust failed: %v", err)
	}

	rustFile := filepath.Join(wsDir, ".noctifab", "templates", "rust.yaml")
	if _, err := os.Stat(rustFile); os.IsNotExist(err) {
		t.Errorf("expected %s to be created, but it was not found", rustFile)
	}

	// Running again should skip because it already exists
	buf.Reset()
	if err := templatesInitCmd.RunE(templatesInitCmd, []string{"rust"}); err != nil {
		t.Fatalf("second templates init rust failed: %v", err)
	}
	if !strings.Contains(buf.String(), "skipped") {
		t.Errorf("expected skipped message on duplicate init, got: %s", buf.String())
	}
}

func TestEnsureWorkspaceInitializedWithProfileAndTemplate(t *testing.T) {
	wsDir := t.TempDir()

	// Initialize with python template
	createdSpec, err := EnsureWorkspaceInitializedWithProfileAndTemplate(wsDir, "", "python")
	if err != nil {
		t.Fatalf("failed to initialize workspace with template: %v", err)
	}
	if !createdSpec {
		t.Error("expected createdSpec=true on fresh directory")
	}

	// Verify generated config.yaml has python sandbox settings
	cfgPath := filepath.Join(wsDir, ".noctifab", "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read generated config.yaml: %v", err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("failed to unmarshal generated config: %v", err)
	}

	if cfg.Sandbox.TestCommand != "pytest" {
		t.Errorf("expected test_command 'pytest', got %q", cfg.Sandbox.TestCommand)
	}
	if cfg.Sandbox.FormatterCommand != "ruff format ." {
		t.Errorf("expected formatter_command 'ruff format .', got %q", cfg.Sandbox.FormatterCommand)
	}
}
