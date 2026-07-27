package llm

import "time"

type FireworksClient struct {
	*baseOpenAIClient
}

func NewFireworksClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &FireworksClient{
		baseOpenAIClient: newBaseOpenAIClient("fireworks", "https://api.fireworks.ai/inference/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "fireworks",
		BaseURL:        "https://api.fireworks.ai/inference/v1",
		EnvKeys:        []string{"FIREWORKS_API_KEY"},
		ParseModelFunc: parseHuggingFaceModel,
		Protocol:       "openai",
		NewClientFunc:  NewFireworksClient,
	})
}
