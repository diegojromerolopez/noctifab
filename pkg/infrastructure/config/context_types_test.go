package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetExcludedPaths(t *testing.T) {
	cfg := &Config{
		Sandbox: SandboxConfig{
			ExcludePaths: []string{".noctifab", "target"},
			SkipFolders:  []string{"custom_skip", "target"}, // dupe should be deduped
		},
		Context: ContextConfig{
			SkipFolders: []string{"ignored_context_dir"},
		},
		SkipFolders: []string{"root_skip", "custom_skip"}, // dupe
	}

	paths := cfg.GetExcludedPaths()
	expected := []string{".noctifab", "target", "custom_skip", "ignored_context_dir", "root_skip"}

	assert.Equal(t, expected, paths)
}

func TestGetExcludedPaths_Nil(t *testing.T) {
	var cfg *Config
	assert.Nil(t, cfg.GetExcludedPaths())
}

func TestDedupMutatedFiles(t *testing.T) {
	t.Run("defaults to true when unset or nil", func(t *testing.T) {
		var cfg ContextConfig
		assert.True(t, cfg.IsDedupMutatedFilesEnabled())

		var nilCfg *ContextConfig
		assert.True(t, nilCfg.IsDedupMutatedFilesEnabled())
	})

	t.Run("explicitly disabled", func(t *testing.T) {
		disabled := false
		cfg := ContextConfig{
			DedupMutatedFiles: &disabled,
		}
		assert.False(t, cfg.IsDedupMutatedFilesEnabled())
	})

	t.Run("explicitly enabled", func(t *testing.T) {
		enabled := true
		cfg := ContextConfig{
			DedupMutatedFiles: &enabled,
		}
		assert.True(t, cfg.IsDedupMutatedFilesEnabled())
	})
}

func TestContextConfig_DiffWindowAndTreeSitter(t *testing.T) {
	t.Run("configures diff_window mode and custom window lines", func(t *testing.T) {
		cfg := ContextConfig{
			Mode:            "diff_window",
			DiffWindowLines: 25,
		}
		assert.Equal(t, ContextModeDiffWindow, cfg.GetMode())
		assert.Equal(t, 25, cfg.GetWindowLines())
	})

	t.Run("configures tree_sitter mode", func(t *testing.T) {
		cfg := ContextConfig{
			Mode: "tree_sitter",
		}
		assert.Equal(t, ContextModeTreeSitter, cfg.GetMode())
	})
}
