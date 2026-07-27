package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDuration_YAML(t *testing.T) {
	t.Run("Unmarshal success", func(t *testing.T) {
		var d Duration
		node := &yaml.Node{Kind: yaml.ScalarNode, Value: "5m"}
		err := d.UnmarshalYAML(node)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if time.Duration(d) != 5*time.Minute {
			t.Errorf("expected 5m, got %v", time.Duration(d))
		}
	})

	t.Run("Unmarshal invalid format", func(t *testing.T) {
		var d Duration
		node := &yaml.Node{Kind: yaml.ScalarNode, Value: "invalid"}
		err := d.UnmarshalYAML(node)
		if err == nil {
			t.Error("expected error unmarshaling invalid duration")
		}
	})

	t.Run("Unmarshal invalid type", func(t *testing.T) {
		var d Duration
		node := &yaml.Node{Kind: yaml.SequenceNode}
		err := d.UnmarshalYAML(node)
		if err == nil {
			t.Error("expected error unmarshaling non-scalar Node")
		}
	})

	t.Run("Marshal", func(t *testing.T) {
		d := Duration(2 * time.Hour)
		val, err := d.MarshalYAML()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "2h0m0s" {
			t.Errorf("expected 2h0m0s, got %v", val)
		}
	})
}

func TestWriteDefaultConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "noctifab-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	path := filepath.Join(tmpDir, "subdir", "config.yaml")
	err = WriteDefaultConfig(path)
	if err != nil {
		t.Fatalf("failed to write default config: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file was not created: %v", err)
	}

	// Try reading and unmarshaling
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written config: %v", err)
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		t.Fatalf("failed to unmarshal written config: %v", err)
	}

	if cfg.ConfigVersion != "1.0" {
		t.Errorf("expected config_version 1.0, got %s", cfg.ConfigVersion)
	}

	// Test write failure due to dir creation error
	invalidPath := filepath.Join("/nonexistent-dir-12345/foo/bar", "config.yaml")
	err = WriteDefaultConfig(invalidPath)
	if err == nil {
		t.Error("expected error writing to invalid path")
	}

	// Test write failure due to file write error
	dirPath := filepath.Join(tmpDir, "some-dir")
	if err := os.Mkdir(dirPath, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	err = WriteDefaultConfig(dirPath)
	if err == nil {
		t.Error("expected error writing to directory path")
	}
}

func TestMetricsConfig_IsEnabled(t *testing.T) {
	t.Run("default nil is enabled", func(t *testing.T) {
		mc := MetricsConfig{Enabled: nil}
		if !mc.IsEnabled() {
			t.Errorf("expected default (nil) MetricsConfig to be enabled")
		}
	})

	t.Run("explicitly enabled", func(t *testing.T) {
		val := true
		mc := MetricsConfig{Enabled: &val}
		if !mc.IsEnabled() {
			t.Errorf("expected MetricsConfig to be enabled")
		}
	})

	t.Run("explicitly disabled", func(t *testing.T) {
		val := false
		mc := MetricsConfig{Enabled: &val}
		if mc.IsEnabled() {
			t.Errorf("expected MetricsConfig to be disabled")
		}
	})
}

func TestContextConfig_GetMode(t *testing.T) {
	t.Run("default empty is full", func(t *testing.T) {
		cc := ContextConfig{}
		if cc.GetMode() != ContextModeFull {
			t.Errorf("expected ContextModeFull, got %v", cc.GetMode())
		}
	})

	t.Run("diff_window mode", func(t *testing.T) {
		cc := ContextConfig{Mode: "diff_window"}
		if cc.GetMode() != ContextModeDiffWindow {
			t.Errorf("expected ContextModeDiffWindow, got %v", cc.GetMode())
		}
	})

	t.Run("tree_sitter mode", func(t *testing.T) {
		cc := ContextConfig{Mode: "tree_sitter"}
		if cc.GetMode() != ContextModeTreeSitter {
			t.Errorf("expected ContextModeTreeSitter, got %v", cc.GetMode())
		}
	})

	t.Run("invalid mode falls back to full", func(t *testing.T) {
		cc := ContextConfig{Mode: "unknown_mode"}
		if cc.GetMode() != ContextModeFull {
			t.Errorf("expected fallback to ContextModeFull, got %v", cc.GetMode())
		}
	})
}

func TestWorkspaceCacheConfig_IsEnabled(t *testing.T) {
	t.Run("default nil is enabled", func(t *testing.T) {
		wc := WorkspaceCacheConfig{Enabled: nil}
		if !wc.IsEnabled() {
			t.Errorf("expected default nil WorkspaceCacheConfig to be enabled")
		}
	})

	t.Run("explicitly enabled", func(t *testing.T) {
		val := true
		wc := WorkspaceCacheConfig{Enabled: &val}
		if !wc.IsEnabled() {
			t.Errorf("expected WorkspaceCacheConfig to be enabled")
		}
	})

	t.Run("explicitly disabled", func(t *testing.T) {
		val := false
		wc := WorkspaceCacheConfig{Enabled: &val}
		if wc.IsEnabled() {
			t.Errorf("expected WorkspaceCacheConfig to be disabled")
		}
	})
}
