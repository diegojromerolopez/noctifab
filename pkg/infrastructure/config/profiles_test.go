package config

import (
	"testing"
)

func TestGetProfile(t *testing.T) {
	t.Run("valid profile retrieval", func(t *testing.T) {
		preset, err := GetProfile("ollama-qwen")
		if err != nil {
			t.Fatalf("expected profile to exist, got err: %v", err)
		}
		if preset.Name != "ollama-qwen" {
			t.Errorf("expected name ollama-qwen, got %s", preset.Name)
		}
		if preset.ConfigYAML == "" {
			t.Errorf("expected non-empty config yaml")
		}
	})

	t.Run("case insensitive retrieval", func(t *testing.T) {
		preset, err := GetProfile("OLLAMA-DEEPSEEK")
		if err != nil {
			t.Fatalf("expected case-insensitive match, got err: %v", err)
		}
		if preset.Name != "ollama-deepseek" {
			t.Errorf("expected name ollama-deepseek, got %s", preset.Name)
		}
	})

	t.Run("unknown profile returns descriptive error", func(t *testing.T) {
		_, err := GetProfile("nonexistent-profile-xyz")
		if err == nil {
			t.Fatalf("expected error for unknown profile")
		}
	})

	t.Run("list profiles returns all presets", func(t *testing.T) {
		list := ListProfiles()
		if len(list) < 4 {
			t.Errorf("expected at least 4 profiles, got %d", len(list))
		}
	})

	t.Run("all available profiles decode cleanly without unknown fields", func(t *testing.T) {
		for name := range AvailableProfiles {
			cfg := DefaultConfig()
			err := ApplyProfile(cfg, name)
			if err != nil {
				t.Errorf("profile %q failed to apply: %v", name, err)
			}
			if cfg.ConfigVersion != "2.0" {
				t.Errorf("expected config_version 2.0 for profile %q, got %q", name, cfg.ConfigVersion)
			}
			if cfg.LLM.Provider == "" {
				t.Errorf("expected non-empty LLM provider for profile %q", name)
			}
		}
	})

	t.Run("apply unknown profile returns error", func(t *testing.T) {
		cfg := DefaultConfig()
		err := ApplyProfile(cfg, "nonexistent-profile-xyz")
		if err == nil {
			t.Fatal("expected error applying unknown profile")
		}
	})
}
