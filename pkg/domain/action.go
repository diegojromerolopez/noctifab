package domain

import (
	"encoding/json"
	"time"
)

// Action records execution history and outcomes of tools run by agents.
type Action struct {
	ID        string         `json:"id,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args"`
	Reasoning string         `json:"reasoning"`
	Result    string         `json:"result"`
	Success   bool           `json:"success"`
}

// UnmarshalJSON implements custom JSON unmarshaling for Action to support LLM field aliases (cmd, name, command).
func (a *Action) UnmarshalJSON(data []byte) error {
	type Alias Action
	aux := &struct {
		*Alias
		Cmd     string `json:"cmd"`
		Name    string `json:"name"`
		Command string `json:"command"`
	}{
		Alias: (*Alias)(a),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if a.Tool == "" {
		if aux.Cmd != "" {
			a.Tool = aux.Cmd
		} else if aux.Name != "" {
			a.Tool = aux.Name
		} else if aux.Command != "" {
			a.Tool = aux.Command
		}
	}
	return nil
}
