package llm

import "time"

type MoonshotClient struct {
	*baseOpenAIClient
}

func NewMoonshotClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &MoonshotClient{
		baseOpenAIClient: newBaseOpenAIClient("moonshot", "https://api.moonshot.ai/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	kimiSpec := &ProviderSpec{
		Name:           "kimi",
		BaseURL:        "https://api.moonshot.ai/v1",
		EnvKeys:        []string{"KIMI_API_KEY", "MOONSHOT_API_KEY"},
		ParseModelFunc: parseKimiModel,
		Protocol:       "openai",
		NewClientFunc:  NewMoonshotClient,
	}
	RegisterProvider(kimiSpec)
	RegisterProvider(&ProviderSpec{
		Name:           "moonshot",
		BaseURL:        "https://api.moonshot.ai/v1",
		EnvKeys:        []string{"KIMI_API_KEY", "MOONSHOT_API_KEY"},
		ParseModelFunc: parseKimiModel,
		Protocol:       "openai",
		NewClientFunc:  NewMoonshotClient,
	})
}

var parseKimiModel = NewModelParser(ParserConfig{
	RequiredPrefix: "kimi",
	DefaultVersion: 2.0,
	ContextBonus:   true,
	Tiers: []KeywordTier{
		{Keywords: []string{"k3"}, Score: 50, TierName: "k3"},
		{Keywords: []string{"k2.7"}, Score: 40, TierName: "k2.7"},
		{Keywords: []string{"k2.6"}, Score: 30, TierName: "k2.6"},
		{Keywords: []string{"k2.5"}, Score: 20, TierName: "k2.5"},
		{Keywords: []string{"k2", "v1"}, Score: 10, TierName: "k2"},
	},
})
