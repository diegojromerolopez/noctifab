package llm

import "context"

// ProviderClient defines the interface for specific LLM provider API communication.
type ProviderClient interface {
	// Call executes the text generation request to the provider.
	Call(ctx context.Context, model, apiKey, prompt, customURL string) ([]byte, error)
	// GetAvailableModels retrieves the list of model identifiers currently supported by the provider's API.
	GetAvailableModels(ctx context.Context, apiKey string) ([]string, error)
}
