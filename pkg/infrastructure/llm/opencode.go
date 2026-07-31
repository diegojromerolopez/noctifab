package llm

import "time"

type OpenCodeClient struct {
	*baseOpenAIClient
}

func NewOpenCodeClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &OpenCodeClient{
		baseOpenAIClient: newBaseOpenAIClient("opencode", "https://opencode.ai/zen/go/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "opencode",
		BaseURL:        "https://opencode.ai/zen/go/v1",
		EnvKeys:        []string{"OPENCODE_API_KEY"},
		ParseModelFunc: parseOpenCodeModel,
		Protocol:       "openai",
		NewClientFunc:  NewOpenCodeClient,
	})
}

var parseOpenCodeModel = NewModelParser(ParserConfig{
	DefaultVersion: 1.0,
	VersionRegexp:  `([0-9]+\.[0-9]+)`,
	Tiers: []KeywordTier{
		{Keywords: []string{"max", "5.2"}, Score: 40, TierName: "max"},
		{Keywords: []string{"plus", "5.1"}, Score: 30, TierName: "plus"},
		{Keywords: []string{"pro", "code", "k2.6"}, Score: 20, TierName: "pro"},
		{Keywords: []string{"flash", "lite"}, Score: 10, TierName: "flash"},
	},
})
