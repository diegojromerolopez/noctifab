package config

import (
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a wrapper around time.Duration to support YAML unmarshaling of duration strings.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

func (d Duration) MarshalYAML() (interface{}, error) {
	return time.Duration(d).String(), nil
}

// LoopConfig specifies iteration loop control parameters.
type LoopConfig struct {
	Count int `yaml:"count,omitempty"`
}

type RuntimeConfig struct {
	SpecSource             string     `yaml:"spec_source"`
	MaxActions             int        `yaml:"max_actions"`
	MaxDuration            Duration   `yaml:"max_duration"`
	MaxSilentStallDuration Duration   `yaml:"max_silent_stall_duration,omitempty"`
	MaxTokensPerStory      int64      `yaml:"max_tokens_per_story,omitempty"`
	MaxTokensPerTask       int64      `yaml:"max_tokens_per_task,omitempty"`
	MaxTokens              int64      `yaml:"max_tokens,omitempty"`
	Loops                  int        `yaml:"loops,omitempty"`
	Loop                   LoopConfig `yaml:"loop,omitempty"`
}

// GetLoops returns the effective number of iteration loops (defaults to 1).
func (r RuntimeConfig) GetLoops() int {
	if r.Loops > 1 && r.Loop.Count <= 1 {
		return r.Loops
	}
	if r.Loop.Count > 1 && r.Loops <= 1 {
		return r.Loop.Count
	}
	if r.Loop.Count > 0 {
		return r.Loop.Count
	}
	if r.Loops > 0 {
		return r.Loops
	}
	return 1
}

type NotificationsConfig struct {
	Desktop    bool   `yaml:"desktop"`
	WebhookURL string `yaml:"webhook_url"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}
