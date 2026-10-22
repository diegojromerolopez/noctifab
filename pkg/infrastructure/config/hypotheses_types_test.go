package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHypothesesConfig_DefaultsAndParsing(t *testing.T) {
	t.Run("defaults when unconfigured", func(t *testing.T) {
		h := HypothesesConfig{}
		if h.IsEnabled() {
			t.Errorf("expected disabled by default")
		}
		if h.GetAgents() != 2 {
			t.Errorf("expected 2 agents default, got %d", h.GetAgents())
		}
		if h.GetResolution() != ResolutionCorrect {
			t.Errorf("expected ResolutionCorrect, got %s", h.GetResolution())
		}
	})

	t.Run("parses yaml with agents and mixture resolution", func(t *testing.T) {
		yamlData := `
enabled: true
agents: 3
resolution: mixture
`
		var h HypothesesConfig
		if err := yaml.Unmarshal([]byte(yamlData), &h); err != nil {
			t.Fatalf("yaml unmarshal error: %v", err)
		}
		if !h.IsEnabled() {
			t.Errorf("expected enabled")
		}
		if h.GetAgents() != 3 {
			t.Errorf("expected 3 agents, got %d", h.GetAgents())
		}
		if h.GetResolution() != ResolutionMixture {
			t.Errorf("expected ResolutionMixture, got %s", h.GetResolution())
		}
	})

	t.Run("agents > 1 implies enabled", func(t *testing.T) {
		h := HypothesesConfig{Agents: 2}
		if !h.IsEnabled() {
			t.Errorf("expected enabled when Agents > 1")
		}
	})
}
