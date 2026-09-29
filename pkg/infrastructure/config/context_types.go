package config

import (
	"strings"
)

// ContextConfig defines configuration for context pruning and packing.
type ContextConfig struct {
	Mode              string   `yaml:"mode"`
	TreeSitter        bool     `yaml:"tree_sitter"`
	DiffWindowLines   int      `yaml:"diff_window_lines"`
	WindowSize        int      `yaml:"window_size"`
	CavemanCompaction bool     `yaml:"caveman_compaction"`
	Compaction        string   `yaml:"compaction"`   // Options: "none" (default), "simple_english", "caveman"
	SkipFolders       []string `yaml:"skip_folders"` // Custom directories/folders to skip from context
}

func (c ContextConfig) GetCompactionMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Compaction))
	if mode != "" {
		return mode
	}
	if c.CavemanCompaction {
		return "caveman"
	}
	return "none"
}

func (c ContextConfig) GetWindowLines() int {
	if c.WindowSize > 0 {
		return c.WindowSize
	}
	if c.DiffWindowLines > 0 {
		return c.DiffWindowLines
	}
	return 15
}

func (c ContextConfig) GetMode() ContextMode {
	if c.TreeSitter {
		return ContextModeTreeSitter
	}
	switch ContextMode(strings.ToLower(strings.TrimSpace(c.Mode))) {
	case ContextModeDiffWindow:
		return ContextModeDiffWindow
	case ContextModeTreeSitter:
		return ContextModeTreeSitter
	default:
		return ContextModeFull
	}
}

// GetExcludedPaths returns the merged, deduplicated slice of all excluded paths and custom skip folders.
func (c *Config) GetExcludedPaths() []string {
	if c == nil {
		return nil
	}
	seen := make(map[string]bool)
	var result []string
	add := func(p string) {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			result = append(result, trimmed)
		}
	}

	for _, p := range c.Sandbox.ExcludePaths {
		add(p)
	}
	for _, p := range c.Sandbox.SkipFolders {
		add(p)
	}
	for _, p := range c.Context.SkipFolders {
		add(p)
	}
	for _, p := range c.SkipFolders {
		add(p)
	}
	return result
}
