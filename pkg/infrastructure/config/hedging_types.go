package config

import (
	"strings"
	"time"
)

// HedgingConfig controls speculative fallback hedging across model providers.
type HedgingConfig struct {
	Enabled    *bool    `yaml:"enabled,omitempty"`
	Delay      Duration `yaml:"delay,omitempty"`
	HeavyDelay Duration `yaml:"heavy_delay,omitempty"`
}

// IsEnabled returns whether speculative hedging is active (default: true).
func (h HedgingConfig) IsEnabled() bool {
	if h.Enabled == nil {
		return true
	}
	return *h.Enabled
}

// GetDelay returns the base speculative hedge delay (default: 25s).
func (h HedgingConfig) GetDelay() time.Duration {
	if h.Delay > 0 {
		return time.Duration(h.Delay)
	}
	return 25 * time.Second
}

// GetHeavyDelay returns the minimum hedge delay for heavy batch roles (default: 90s).
func (h HedgingConfig) GetHeavyDelay() time.Duration {
	if h.HeavyDelay > 0 {
		return time.Duration(h.HeavyDelay)
	}
	return 90 * time.Second
}

// JSONReminderCapConfig configures a single cap threshold for format reminder prompts.
type JSONReminderCapConfig struct {
	Cap int `yaml:"cap,omitempty"`
}

// JSONReminderConfig configures context caps for JSON envelope reminder prompts.
type JSONReminderConfig struct {
	Task JSONReminderCapConfig `yaml:"task,omitempty"`
	Body JSONReminderCapConfig `yaml:"body,omitempty"`
}

// GetTaskCap returns the maximum original task context retained on format reminders (default: 1500).
func (j JSONReminderConfig) GetTaskCap() int {
	if j.Task.Cap > 0 {
		return j.Task.Cap
	}
	return 1500
}

// GetBodyCap returns the maximum rejected response tail retained on format reminders (default: 12000).
func (j JSONReminderConfig) GetBodyCap() int {
	if j.Body.Cap > 0 {
		return j.Body.Cap
	}
	return 12000
}

// IsSmartAudit returns whether the agent role uses smart deterministic validation bypass (default: true).
func (a AgentRoleConfig) IsSmartAudit() bool {
	mode := strings.ToLower(strings.TrimSpace(a.AuditMode))
	if mode == "exhaustive" || mode == "all" || mode == "force" {
		return false
	}
	return true
}
