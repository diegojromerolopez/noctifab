package llm

import "time"

type CohereClient struct {
	*baseOpenAIClient
}

func NewCohereClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &CohereClient{
		baseOpenAIClient: newBaseOpenAIClient("cohere", "https://api.cohere.com/v2", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "cohere",
		BaseURL:        "https://api.cohere.com/v2",
		EnvKeys:        []string{"COHERE_API_KEY", "CO_API_KEY"},
		ParseModelFunc: parseCohereModel,
		Protocol:       "openai",
		NewClientFunc:  NewCohereClient,
	})
}

var parseCohereModel = NewModelParser(ParserConfig{
	RequiredPrefix: "command",
	DefaultVersion: 1.0,
	Tiers: []KeywordTier{
		{Keywords: []string{"r-plus", "r+"}, Score: 40, TierName: "r-plus"},
		{Keywords: []string{"r"}, Score: 30, TierName: "r"},
		{Keywords: []string{"light"}, Score: 20, TierName: "light"},
	},
})
