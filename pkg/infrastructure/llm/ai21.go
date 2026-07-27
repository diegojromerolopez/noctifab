package llm

import "time"

// AI21Client wraps baseOpenAIClient to target the AI21 Studio API.
// AI21 Labs develops the Jamba model family — a hybrid SSM-Transformer
// architecture offering uniquely long context windows (256k+).
// Its API is OpenAI-compatible at https://api.ai21.com/studio/v1.
type AI21Client struct {
	*baseOpenAIClient
}

// NewAI21Client creates an OpenAI-compatible client for the AI21 Studio API.
func NewAI21Client(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &AI21Client{
		baseOpenAIClient: newBaseOpenAIClient(
			"ai21",
			"https://api.ai21.com/studio/v1",
			url,
			timeout,
			idleTimeout,
			streaming,
		),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "ai21",
		BaseURL:        "https://api.ai21.com/studio/v1",
		EnvKeys:        []string{"AI21_API_KEY"},
		ParseModelFunc: parseAI21Model,
		Protocol:       "openai",
		NewClientFunc:  NewAI21Client,
	})
}

// parseAI21Model ranks AI21 Jamba models by capacity.
// Current model family: jamba-large > jamba-mini.
// Version numbers in the suffix (e.g. "-1.7") encode generation.
var parseAI21Model = NewModelParser(ParserConfig{
	RequiredPrefix:    "jamba",
	DefaultVersion:    1.0,
	VersionRegexp:     `([0-9]+\.[0-9]+)`,
	VersionMultiplier: 5,
	Tiers: []KeywordTier{
		{Keywords: []string{"large"}, Score: 40, TierName: "large"},
		{Keywords: []string{"mini"}, Score: 20, TierName: "mini"},
	},
	ContextBonus: true,
})
