package llm

import "time"

type HermesClient struct {
	*baseOpenAIClient
}

func NewHermesClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &HermesClient{
		baseOpenAIClient: newBaseOpenAIClient("hermes", "https://inference-api.nousresearch.com/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "hermes",
		BaseURL:        "https://inference-api.nousresearch.com/v1",
		EnvKeys:        []string{"HERMES_API_KEY"},
		ParseModelFunc: parseHermesModel,
		Protocol:       "openai",
		NewClientFunc:  NewHermesClient,
	})
}

var parseHermesModel = NewModelParser(ParserConfig{
	DefaultVersion: 3.0,
	Tiers: []KeywordTier{
		{Keywords: []string{"405b"}, Score: 30, TierName: "405b"},
		{Keywords: []string{"70b"}, Score: 20, TierName: "70b"},
	},
})
