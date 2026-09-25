package config

import (
	"testing"
)

func TestAgentRoleConfig_IsPipelined(t *testing.T) {
	t.Run("default is true when Pipelined is nil", func(t *testing.T) {
		role := AgentRoleConfig{}
		if !role.IsPipelined() {
			t.Errorf("expected IsPipelined() to default to true")
		}
	})

	t.Run("returns true when explicitly set to true", func(t *testing.T) {
		tr := true
		role := AgentRoleConfig{Pipelined: &tr}
		if !role.IsPipelined() {
			t.Errorf("expected IsPipelined() to be true")
		}
	})

	t.Run("returns false when explicitly set to false", func(t *testing.T) {
		fa := false
		role := AgentRoleConfig{Pipelined: &fa}
		if role.IsPipelined() {
			t.Errorf("expected IsPipelined() to be false")
		}
	})
}
