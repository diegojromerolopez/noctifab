package llm

import "time"

// CerebrasClient wraps baseOpenAIClient to target the Cerebras Inference API.
// Cerebras operates wafer-scale AI chips delivering the fastest token throughput
// in the industry. Its API is fully OpenAI-compatible at https://api.cerebras.ai/v1.
type CerebrasClient struct {
	*baseOpenAIClient
}

// NewCerebrasClient creates an OpenAI-compatible client for the Cerebras Cloud API.
func NewCerebrasClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &CerebrasClient{
		baseOpenAIClient: newBaseOpenAIClient(
			"cerebras",
			"https://api.cerebras.ai/v1",
			url,
			timeout,
			idleTimeout,
			streaming,
		),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "cerebras",
		BaseURL:        "https://api.cerebras.ai/v1",
		EnvKeys:        []string{"CEREBRAS_API_KEY"},
		ParseModelFunc: parseCerebrasModel,
		Protocol:       "openai",
		NewClientFunc:  NewCerebrasClient,
	})
}

// parseCerebrasModel ranks Cerebras-hosted models by capacity.
// Cerebras exposes large open-weight models (Llama, Qwen) scaled to their
// wafer-scale hardware. Naming typically encodes the parameter count.
var parseCerebrasModel = NewModelParser(ParserConfig{
	DefaultVersion: 1.0,
	SizeWeights:    StandardSizeWeights,
	Tiers: []KeywordTier{
		{Keywords: []string{"scout", "maverick"}, Score: 400, TierName: "frontier"},
	},
	ContextBonus: true,
})
