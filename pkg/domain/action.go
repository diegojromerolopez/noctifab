package domain

import "time"

// Action records execution history and outcomes of tools run by agents.
type Action struct {
	Timestamp time.Time      `json:"timestamp"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args"`
	Reasoning string         `json:"reasoning"`
	Result    string         `json:"result"`
	Success   bool           `json:"success"`
}
