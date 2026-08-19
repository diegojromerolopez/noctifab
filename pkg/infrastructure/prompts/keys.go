// Package prompts provides the per-agent, per-action prompt template system.
//
// Every customizable LLM prompt in noctifab belongs to exactly one agent and
// one action. For each (agent, action) key the effective template is resolved
// in this order (first hit wins):
//
//  1. Explicit path in config (prompts.<agent>.<action>.path).
//  2. Convention file (.noctifab/prompts/<agent>/<action>.tmpl).
//  3. Embedded default (defaults/<agent>/<action>.tmpl, shipped in the binary).
//
// The rendered body is always followed by a non-overridable output-contract
// block (JSON envelope schema + tool list) appended by code, so a custom
// template can never break the machine-readable protocol.
package prompts

import (
	"fmt"
	"sort"
)

// Agent names in the catalog.
const (
	AgentProductManager = "product_manager"
	AgentPlanner        = "planner"
	AgentTester         = "tester"
	AgentGenerator      = "generator"
	AgentQA             = "qa"
	AgentSpec           = "spec"
)

// catalog maps each agent to its customizable actions. It is the single
// source of truth for the (agent, action) key space: 20 keys across 6 agents.
var catalog = map[string][]string{
	AgentProductManager: {"generate", "audit"},
	AgentPlanner:        {"decompose"},
	AgentQA:             {"acceptance"},
	AgentSpec: {
		"pm_draft", "architect_enrich", "tester_enrich",
		"qa_enrich", "consensus_audit", "refine",
	},
	AgentTester: {"write", "fix", "refactor", "write_breadth_first"},
	AgentGenerator: {
		"implement", "refactor", "fix",
		"single_pass", "single_pass_fix",
		"implement_breadth_first", "implement_breadth_first_fix",
	},
}

// Agents returns the agent names in the catalog, sorted alphabetically.
func Agents() []string {
	agents := make([]string, 0, len(catalog))
	for agent := range catalog {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	return agents
}

// Actions returns the actions of the given agent in catalog order, or nil if
// the agent is unknown.
func Actions(agent string) []string {
	actions := catalog[agent]
	out := make([]string, len(actions))
	copy(out, actions)
	return out
}

// IsValidKey reports whether (agent, action) is a known catalog key.
func IsValidKey(agent, action string) bool {
	for _, a := range catalog[agent] {
		if a == action {
			return true
		}
	}
	return false
}

// ValidateKey returns a descriptive error when (agent, action) is not a known
// catalog key.
func ValidateKey(agent, action string) error {
	actions, ok := catalog[agent]
	if !ok {
		return fmt.Errorf("unknown prompt agent %q (valid agents: %v)", agent, Agents())
	}
	for _, a := range actions {
		if a == action {
			return nil
		}
	}
	return fmt.Errorf("unknown prompt action %q for agent %q (valid actions: %v)", action, agent, actions)
}
