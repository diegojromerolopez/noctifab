package config

import "sort"

// RoleCapability describes one supported orchestration role or controller.
type RoleCapability struct {
	Name       string
	Capability string
}

// RoleCapabilities returns the supported role surface in deterministic order.
func RoleCapabilities(cfg *Config) []RoleCapability {
	qaCapability := "experimental-disabled"
	if cfg != nil && cfg.Agents.QA.Enabled {
		qaCapability = "experimental-enabled"
	}
	capabilities := []RoleCapability{
		{Name: "orchestrator", Capability: "deterministic-controller"},
		{Name: "product_manager", Capability: "implemented"},
		{Name: "planner", Capability: "implemented"},
		{Name: "generator", Capability: "implemented"},
		{Name: "tester", Capability: "implemented"},
		{Name: "unblocker", Capability: "implemented"},
		{Name: "qa", Capability: qaCapability},
	}
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i].Name < capabilities[j].Name })
	return capabilities
}
