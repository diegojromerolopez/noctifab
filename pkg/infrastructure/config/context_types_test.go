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
