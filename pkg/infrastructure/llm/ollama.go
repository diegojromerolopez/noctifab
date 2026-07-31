package llm

import "time"

type OllamaClient struct {
	*baseOpenAIClient
}

func NewOllamaClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &OllamaClient{
		baseOpenAIClient: newBaseOpenAIClient("ollama", "https://ollama.com/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "ollama",
		BaseURL:        "https://ollama.com/v1",
		EnvKeys:        []string{"OLLAMA_API_KEY"},
		ParseModelFunc: parseOllamaModel,
		Protocol:       "openai",
		NewClientFunc:  NewOllamaClient,
	})
}

var parseOllamaModel = NewModelParser(ParserConfig{
	DefaultVersion: 1.0,
	VersionRegexp:  `([0-9]+\.[0-9]+)`,
	SizeWeights:    StandardSizeWeights,
})
