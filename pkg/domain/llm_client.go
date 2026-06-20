package domain

import (
	"context"
)

// LLMAction represents a specific tool call request produced by the LLM.
type LLMAction struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

// LLMResponse is the structured schema returned by the LLM client.
type LLMResponse struct {
	Reasoning string      `json:"reasoning"`
	Actions   []LLMAction `json:"actions"`
}

// LLMClient defines the interface for communicating with an external AI provider.
type LLMClient interface {
	// Complete generates a completion for the given system/user prompt.
	Complete(ctx context.Context, prompt string) (*LLMResponse, error)
}
