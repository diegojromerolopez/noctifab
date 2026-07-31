package llm

import "time"

type MistralClient struct {
	*baseOpenAIClient
}

func NewMistralClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &MistralClient{
		baseOpenAIClient: newBaseOpenAIClient("mistral", "https://api.mistral.ai/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "mistral",
		BaseURL:        "https://api.mistral.ai/v1",
		EnvKeys:        []string{"MISTRAL_API_KEY"},
		ParseModelFunc: parseMistralModel,
		Protocol:       "openai",
		NewClientFunc:  NewMistralClient,
	})
}

var parseMistralModel = NewModelParser(ParserConfig{
	DefaultVersion: 1.0,
	Tiers: []KeywordTier{
		{Keywords: []string{"large", "codestral"}, Score: 40, TierName: "large"},
		{Keywords: []string{"medium"}, Score: 30, TierName: "medium"},
		{Keywords: []string{"small"}, Score: 20, TierName: "small"},
	},
})
