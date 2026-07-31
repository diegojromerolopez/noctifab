package llm

import "time"

type SambaNovaClient struct {
	*baseOpenAIClient
}

func NewSambaNovaClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &SambaNovaClient{
		baseOpenAIClient: newBaseOpenAIClient("sambanova", "https://api.sambanova.ai/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "sambanova",
		BaseURL:        "https://api.sambanova.ai/v1",
		EnvKeys:        []string{"SAMBANOVA_API_KEY"},
		ParseModelFunc: parseHuggingFaceModel,
		Protocol:       "openai",
		NewClientFunc:  NewSambaNovaClient,
	})
}
