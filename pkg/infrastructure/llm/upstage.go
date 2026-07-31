package llm

import "time"

// UpstageClient wraps baseOpenAIClient to target the Upstage Solar API.
// Upstage (https://upstage.ai) develops the Solar model family — high-performance
// models optimised for enterprise document understanding and reasoning.
// Its API is OpenAI-compatible at https://api.upstage.ai/v1/solar.
type UpstageClient struct {
	*baseOpenAIClient
}

// NewUpstageClient creates an OpenAI-compatible client for the Upstage Solar API.
func NewUpstageClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &UpstageClient{
		baseOpenAIClient: newBaseOpenAIClient(
			"upstage",
			"https://api.upstage.ai/v1/solar",
			url,
			timeout,
			idleTimeout,
			streaming,
		),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "upstage",
		BaseURL:        "https://api.upstage.ai/v1/solar",
		EnvKeys:        []string{"UPSTAGE_API_KEY"},
		ParseModelFunc: parseUpstageModel,
		Protocol:       "openai",
		NewClientFunc:  NewUpstageClient,
	})
}

// parseUpstageModel ranks Upstage Solar models by capacity.
// Current model family: solar-pro > solar-mini.
// Generation numbers may appear as suffixes (e.g. "solar-pro-3").
var parseUpstageModel = NewModelParser(ParserConfig{
	RequiredPrefix: "solar",
	DefaultVersion: 1.0,
	VersionRegexp:  `([0-9]+)`,
	Tiers: []KeywordTier{
		{Keywords: []string{"pro"}, Score: 40, TierName: "pro"},
		{Keywords: []string{"mini", "lite"}, Score: 20, TierName: "mini"},
	},
})
