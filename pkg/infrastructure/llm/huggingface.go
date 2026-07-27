package llm

import "time"

type HuggingFaceClient struct {
	*baseOpenAIClient
}

func NewHuggingFaceClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &HuggingFaceClient{
		baseOpenAIClient: newBaseOpenAIClient("huggingface", "https://api-inference.huggingface.co/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "huggingface",
		BaseURL:        "https://api-inference.huggingface.co/v1",
		EnvKeys:        []string{"HUGGINGFACE_API_KEY", "HF_TOKEN"},
		ParseModelFunc: parseHuggingFaceModel,
		Protocol:       "openai",
		NewClientFunc:  NewHuggingFaceClient,
	})
}

var parseHuggingFaceModel = parseOllamaModel
