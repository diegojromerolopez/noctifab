package config

import (
	"reflect"
	"testing"
)

func TestRoleCapabilities(t *testing.T) {
	want := []RoleCapability{
		{Name: "auditor", Capability: "implemented"},
		{Name: "generator", Capability: "implemented"},
		{Name: "orchestrator", Capability: "deterministic-controller"},
		{Name: "planner", Capability: "implemented"},
		{Name: "product_manager", Capability: "implemented"},
		{Name: "qa", Capability: "experimental-disabled"},
		{Name: "tester", Capability: "implemented"},
		{Name: "unblocker", Capability: "implemented"},
	}
	if got := RoleCapabilities(DefaultConfig()); !reflect.DeepEqual(got, want) {
		t.Fatalf("RoleCapabilities() = %#v, want %#v", got, want)
	}
	cfg := DefaultConfig()
	cfg.Agents.QA.Enabled = true
	if got := RoleCapabilities(cfg)[5].Capability; got != "experimental-enabled" {
		t.Fatalf("enabled QA capability = %q", got)
	}
}
