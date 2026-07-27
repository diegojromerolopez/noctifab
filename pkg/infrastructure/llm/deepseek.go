package llm

import "time"

type DeepSeekClient struct {
	*baseOpenAIClient
}

func NewDeepSeekClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &DeepSeekClient{
		baseOpenAIClient: newBaseOpenAIClient("deepseek", "https://api.deepseek.com/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "deepseek",
		BaseURL:        "https://api.deepseek.com/v1",
		EnvKeys:        []string{"DEEPSEEK_API_KEY"},
		ParseModelFunc: parseDeepSeekModel,
		Protocol:       "openai",
		NewClientFunc:  NewDeepSeekClient,
	})
}

var parseDeepSeekModel = NewModelParser(ParserConfig{
	DefaultVersion: 1.0,
	Tiers: []KeywordTier{
		{Keywords: []string{"r1", "v3", "coder"}, Score: 30, TierName: "coder"},
		{Keywords: []string{"chat"}, Score: 20, TierName: "chat"},
	},
})
