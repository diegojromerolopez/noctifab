package llm

import (
	"context"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ProviderCallResult contains the raw response body bytes and token usage metadata.
type ProviderCallResult struct {
	Body  []byte
	Usage domain.TokenUsage
}

// ProviderClient defines the interface for specific LLM provider API communication.
type ProviderClient interface {
	// Call executes the text generation request to the provider. maxTokens caps
	// the number of tokens generated (<=0 means provider default); temperature
	// controls sampling (0 means provider default).
	Call(ctx context.Context, model, apiKey, prompt string, maxTokens int, temperature float64) (*ProviderCallResult, error)
	// GetAvailableModels retrieves the list of model identifiers currently supported by the provider's API.
	GetAvailableModels(ctx context.Context, apiKey string) ([]string, error)
}
