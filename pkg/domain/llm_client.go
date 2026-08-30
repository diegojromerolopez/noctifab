package domain

import (
	"context"
)

// LLMAction represents a specific tool call request produced by the LLM.
type LLMAction struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

// TokenUsage encapsulates authoritative token metrics from an LLM completion.
type TokenUsage struct {
	InputTokens     int64 `json:"input_tokens"`
	OutputTokens    int64 `json:"output_tokens"`
	ReasoningTokens int64 `json:"reasoning_tokens,omitempty"` // Included in OutputTokens
	CachedTokens    int64 `json:"cached_tokens,omitempty"`    // Included in InputTokens
	TotalTokens     int64 `json:"total_tokens"`
}

// LLMResponse is the structured schema returned by the LLM client.
type LLMResponse struct {
	Reasoning string      `json:"reasoning"`
	Actions   []LLMAction `json:"actions"`
	Usage     TokenUsage  `json:"usage"`
}

// LLMClient defines the interface for communicating with an external AI provider.
type LLMClient interface {
	// Complete generates a completion for the given system/user prompt.
	Complete(ctx context.Context, prompt string) (*LLMResponse, error)
}

// uncompactableTailKey is the context key carrying the byte length of the
// non-compactable tail at the end of a prompt (the machine-readable output
// contract appended by the prompts renderer). Prompt compaction must never
// rewrite that block, so the LLM client compacts only the bytes before it.
type uncompactableTailKey struct{}

// WithUncompactableTail marks the last tailLen bytes of the prompt sent with
// ctx as non-compactable (the output contract block).
func WithUncompactableTail(ctx context.Context, tailLen int) context.Context {
	return context.WithValue(ctx, uncompactableTailKey{}, tailLen)
}

// UncompactableTailLen returns the non-compactable tail length recorded in
// ctx, or 0 when none was set.
func UncompactableTailLen(ctx context.Context) int {
	if n, ok := ctx.Value(uncompactableTailKey{}).(int); ok && n > 0 {
		return n
	}
	return 0
}
