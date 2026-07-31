package llm

import "time"

type OpenRouterClient struct {
	*baseOpenAIClient
}

func NewOpenRouterClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &OpenRouterClient{
		baseOpenAIClient: newBaseOpenAIClient("openrouter", "https://openrouter.ai/api/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "openrouter",
		BaseURL:        "https://openrouter.ai/api/v1",
		EnvKeys:        []string{"OPENROUTER_API_KEY"},
		ParseModelFunc: parseOpenAIModel,
		Protocol:       "openai",
		NewClientFunc:  NewOpenRouterClient,
	})
}
