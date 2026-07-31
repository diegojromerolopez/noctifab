package llm

import "time"

type PerplexityClient struct {
	*baseOpenAIClient
}

func NewPerplexityClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &PerplexityClient{
		baseOpenAIClient: newBaseOpenAIClient("perplexity", "https://api.perplexity.ai", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "perplexity",
		BaseURL:        "https://api.perplexity.ai",
		EnvKeys:        []string{"PERPLEXITY_API_KEY"},
		ParseModelFunc: parsePerplexityModel,
		Protocol:       "openai",
		NewClientFunc:  NewPerplexityClient,
	})
}

var parsePerplexityModel = NewModelParser(ParserConfig{
	RequiredPrefix: "sonar",
	DefaultVersion: 1.0,
	Tiers: []KeywordTier{
		{Keywords: []string{"deep-research"}, Score: 50, TierName: "deep-research"},
		{Keywords: []string{"reasoning-pro"}, Score: 40, TierName: "reasoning-pro"},
		{Keywords: []string{"reasoning"}, Score: 30, TierName: "reasoning"},
		{Keywords: []string{"pro"}, Score: 20, TierName: "pro"},
	},
})
