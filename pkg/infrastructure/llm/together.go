package llm

import "time"

type TogetherClient struct {
	*baseOpenAIClient
}

func NewTogetherClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &TogetherClient{
		baseOpenAIClient: newBaseOpenAIClient("together", "https://api.together.xyz/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "together",
		BaseURL:        "https://api.together.xyz/v1",
		EnvKeys:        []string{"TOGETHER_API_KEY"},
		ParseModelFunc: parseLlamaModel,
		Protocol:       "openai",
		NewClientFunc:  NewTogetherClient,
	})
}
