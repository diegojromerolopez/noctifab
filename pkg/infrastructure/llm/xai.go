package llm

import "time"

type XAIClient struct {
	*baseOpenAIClient
}

func NewXAIClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &XAIClient{
		baseOpenAIClient: newBaseOpenAIClient("xai", "https://api.x.ai/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	xaiSpec := &ProviderSpec{
		Name:           "xai",
		BaseURL:        "https://api.x.ai/v1",
		EnvKeys:        []string{"XAI_API_KEY", "GROK_API_KEY"},
		ParseModelFunc: parseXAIModel,
		Protocol:       "openai",
		NewClientFunc:  NewXAIClient,
	}
	RegisterProvider(xaiSpec)
	RegisterProvider(&ProviderSpec{
		Name:           "grok",
		BaseURL:        "https://api.x.ai/v1",
		EnvKeys:        []string{"XAI_API_KEY", "GROK_API_KEY"},
		ParseModelFunc: parseXAIModel,
		Protocol:       "openai",
		NewClientFunc:  NewXAIClient,
	})
}

var parseXAIModel = NewModelParser(ParserConfig{
	RequiredPrefix:    "grok",
	DefaultVersion:    2.0,
	VersionRegexp:     `grok-([0-9]+(?:\.[0-9]+)?)`,
	VersionMultiplier: 5,
	Tiers: []KeywordTier{
		{Keywords: []string{"grok-3"}, Score: 60, TierName: "grok-3"},
		{Keywords: []string{"grok-2"}, Score: 40, TierName: "grok-2"},
		{Keywords: []string{"grok-3-mini"}, Score: 30, TierName: "grok-3-mini"},
		{Keywords: []string{"mini", "beta"}, Score: 20, TierName: "mini"},
	},
})
