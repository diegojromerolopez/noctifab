package config

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestHedgingConfig_Defaults(t *testing.T) {
	h := HedgingConfig{}
	if !h.IsEnabled() {
		t.Errorf("expected default IsEnabled() == true, got false")
	}
	if h.GetDelay() != 25*time.Second {
		t.Errorf("expected default GetDelay() == 25s, got %v", h.GetDelay())
	}
	if h.GetHeavyDelay() != 90*time.Second {
		t.Errorf("expected default GetHeavyDelay() == 90s, got %v", h.GetHeavyDelay())
	}
}

func TestHedgingConfig_Custom(t *testing.T) {
	f := false
	h := HedgingConfig{
		Enabled:    &f,
		Delay:      Duration(10 * time.Second),
		HeavyDelay: Duration(45 * time.Second),
	}
	if h.IsEnabled() {
		t.Errorf("expected IsEnabled() == false, got true")
	}
	if h.GetDelay() != 10*time.Second {
		t.Errorf("expected GetDelay() == 10s, got %v", h.GetDelay())
	}
	if h.GetHeavyDelay() != 45*time.Second {
		t.Errorf("expected GetHeavyDelay() == 45s, got %v", h.GetHeavyDelay())
	}
}

func TestJSONReminderConfig_DefaultsAndCustom(t *testing.T) {
	j := JSONReminderConfig{}
	if j.GetTaskCap() != 1500 {
		t.Errorf("expected default GetTaskCap() == 1500, got %d", j.GetTaskCap())
	}
	if j.GetBodyCap() != 12000 {
		t.Errorf("expected default GetBodyCap() == 12000, got %d", j.GetBodyCap())
	}

	jCustom := JSONReminderConfig{
		Task: JSONReminderCapConfig{Cap: 500},
		Body: JSONReminderCapConfig{Cap: 3000},
	}
	if jCustom.GetTaskCap() != 500 {
		t.Errorf("expected custom GetTaskCap() == 500, got %d", jCustom.GetTaskCap())
	}
	if jCustom.GetBodyCap() != 3000 {
		t.Errorf("expected custom GetBodyCap() == 3000, got %d", jCustom.GetBodyCap())
	}
}

func TestAgentRoleConfig_IsSmartAudit(t *testing.T) {
	tests := []struct {
		mode     string
		expected bool
	}{
		{"", true},
		{"smart", true},
		{"adaptive", true},
		{"exhaustive", false},
		{"all", false},
		{"force", false},
		{"  EXHAUSTIVE  ", false},
	}
	for _, tc := range tests {
		a := AgentRoleConfig{AuditMode: tc.mode}
		if got := a.IsSmartAudit(); got != tc.expected {
			t.Errorf("IsSmartAudit() for mode %q: got %v, expected %v", tc.mode, got, tc.expected)
		}
	}
}

func TestYAML_ParsingNewConfigFields(t *testing.T) {
	yamlData := `
agents:
  product_manager:
    audit_mode: "exhaustive"
llm:
  hedging:
    enabled: false
    delay: 15s
    heavy_delay: 60s
  json_reminder:
    task:
      cap: 2000
    body:
      cap: 8000
`
	var cfg Config
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("failed to unmarshal yaml: %v", err)
	}

	if cfg.Agents.ProductManager.AuditMode != "exhaustive" {
		t.Errorf("expected audit_mode == exhaustive, got %q", cfg.Agents.ProductManager.AuditMode)
	}
	if cfg.Agents.ProductManager.IsSmartAudit() {
		t.Errorf("expected IsSmartAudit == false for exhaustive mode")
	}
	if cfg.LLM.Hedging.IsEnabled() {
		t.Errorf("expected hedging.enabled == false")
	}
	if cfg.LLM.Hedging.GetDelay() != 15*time.Second {
		t.Errorf("expected hedging delay == 15s, got %v", cfg.LLM.Hedging.GetDelay())
	}
	if cfg.LLM.Hedging.GetHeavyDelay() != 60*time.Second {
		t.Errorf("expected hedging heavy_delay == 60s, got %v", cfg.LLM.Hedging.GetHeavyDelay())
	}
	if cfg.LLM.JSONReminder.GetTaskCap() != 2000 {
		t.Errorf("expected json_reminder.task.cap == 2000, got %d", cfg.LLM.JSONReminder.GetTaskCap())
	}
	if cfg.LLM.JSONReminder.GetBodyCap() != 8000 {
		t.Errorf("expected json_reminder.body.cap == 8000, got %d", cfg.LLM.JSONReminder.GetBodyCap())
	}
}
