package config

import (
	"strings"
)

// HypothesesResolution defines the strategy used to resolve multi-hypothesis results.
type HypothesesResolution string

const (
	ResolutionCorrect HypothesesResolution = "correct"
	ResolutionMixture HypothesesResolution = "mixture"
)

// HypothesesConfig configures multi-hypothesis parallel worktree prototyping.
type HypothesesConfig struct {
	Enabled    bool                 `yaml:"enabled"`
	Agents     int                  `yaml:"agents,omitempty"`
	Resolution HypothesesResolution `yaml:"resolution,omitempty"`
}

// IsEnabled returns true if multi-hypothesis prototyping is explicitly enabled or agents > 1.
func (h HypothesesConfig) IsEnabled() bool {
	return h.Enabled || h.Agents > 1
}

// GetAgents returns the number of parallel hypothesis agents (default: 2).
func (h HypothesesConfig) GetAgents() int {
	if h.Agents <= 0 {
		return 2
	}
	return h.Agents
}

// GetResolution returns the resolution strategy ("correct" or "mixture", default: "correct").
func (h HypothesesConfig) GetResolution() HypothesesResolution {
	clean := strings.ToLower(strings.TrimSpace(string(h.Resolution)))
	if clean == "mixture" || clean == "mix" || clean == "synthesize" {
		return ResolutionMixture
	}
	return ResolutionCorrect
}
