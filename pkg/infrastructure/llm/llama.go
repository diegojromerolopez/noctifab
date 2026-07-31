package llm

import "time"

type LlamaClient struct {
	*baseOpenAIClient
}

func NewLlamaClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &LlamaClient{
		baseOpenAIClient: newBaseOpenAIClient("llama", "https://api.together.xyz/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	llamaSpec := &ProviderSpec{
		Name:           "llama",
		BaseURL:        "https://api.together.xyz/v1",
		EnvKeys:        []string{"LLAMA_API_KEY", "META_API_KEY"},
		ParseModelFunc: parseLlamaModel,
		Protocol:       "openai",
		NewClientFunc:  NewLlamaClient,
	}
	RegisterProvider(llamaSpec)
	RegisterProvider(&ProviderSpec{
		Name:           "meta",
		BaseURL:        "https://api.together.xyz/v1",
		EnvKeys:        []string{"LLAMA_API_KEY", "META_API_KEY"},
		ParseModelFunc: parseLlamaModel,
		Protocol:       "openai",
		NewClientFunc:  NewLlamaClient,
	})
}

var parseLlamaModel = NewModelParser(ParserConfig{
	RequiredPrefix: "llama",
	DefaultVersion: 3.1,
	VersionRegexp:  `([0-9]+\.[0-9]+)`,
	SizeWeights:    StandardSizeWeights,
})
