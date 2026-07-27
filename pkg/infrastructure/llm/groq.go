package llm

import "time"

type GroqClient struct {
	*baseOpenAIClient
}

func NewGroqClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &GroqClient{
		baseOpenAIClient: newBaseOpenAIClient("groq", "https://api.groq.com/openai/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "groq",
		BaseURL:        "https://api.groq.com/openai/v1",
		EnvKeys:        []string{"GROQ_API_KEY"},
		ParseModelFunc: parseOpenAIModel,
		Protocol:       "openai",
		NewClientFunc:  NewGroqClient,
	})
}
