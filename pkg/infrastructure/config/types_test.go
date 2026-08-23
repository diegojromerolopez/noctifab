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

	if cfg.ConfigVersion != "2.0" {
		t.Errorf("expected config_version 1.0, got %s", cfg.ConfigVersion)
	}

	// Test write failure due to dir creation error (parent is a regular file)
	regularFilePath := filepath.Join(tmpDir, "regular-file.txt")
	if err := os.WriteFile(regularFilePath, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create regular file: %v", err)
	}
	invalidPath := filepath.Join(regularFilePath, "foo", "bar", "config.yaml")
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

func TestRuntimeConfig_YAML(t *testing.T) {
	yamlData := `
spec_source: "roadmap/user-stories/US-001.md"
max_actions: 150
max_duration: "45m"
max_silent_stall_duration: "20m"
max_tokens_per_story: 2000000
max_tokens_per_task: 500000
`
	var rc RuntimeConfig
	if err := yaml.Unmarshal([]byte(yamlData), &rc); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if rc.SpecSource != "roadmap/user-stories/US-001.md" {
		t.Errorf("expected spec_source 'roadmap/user-stories/US-001.md', got %q", rc.SpecSource)
	}
	if rc.MaxActions != 150 {
		t.Errorf("expected max_actions 150, got %d", rc.MaxActions)
	}
	if time.Duration(rc.MaxDuration) != 45*time.Minute {
		t.Errorf("expected max_duration 45m, got %v", time.Duration(rc.MaxDuration))
	}
	if time.Duration(rc.MaxSilentStallDuration) != 20*time.Minute {
		t.Errorf("expected max_silent_stall_duration 20m, got %v", time.Duration(rc.MaxSilentStallDuration))
	}
	if rc.MaxTokensPerStory != 2000000 {
		t.Errorf("expected max_tokens_per_story 2000000, got %d", rc.MaxTokensPerStory)
	}
	if rc.MaxTokensPerTask != 500000 {
		t.Errorf("expected max_tokens_per_task 500000, got %d", rc.MaxTokensPerTask)
	}
}
