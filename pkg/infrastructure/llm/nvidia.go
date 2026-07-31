package llm

import "time"

// NvidiaClient wraps baseOpenAIClient to target the NVIDIA AI Foundation / NIM API.
// NVIDIA NIM (https://build.nvidia.com) provides a large catalog of curated open-weight
// models (Llama, Mistral, Nemotron, etc.) optimised for NVIDIA GPU inference.
// Its API is fully OpenAI-compatible at https://integrate.api.nvidia.com/v1.
type NvidiaClient struct {
	*baseOpenAIClient
}

// NewNvidiaClient creates an OpenAI-compatible client for the NVIDIA NIM API.
func NewNvidiaClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &NvidiaClient{
		baseOpenAIClient: newBaseOpenAIClient(
			"nvidia",
			"https://integrate.api.nvidia.com/v1",
			url,
			timeout,
			idleTimeout,
			streaming,
		),
	}
}

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "nvidia",
		BaseURL:        "https://integrate.api.nvidia.com/v1",
		EnvKeys:        []string{"NVIDIA_API_KEY"},
		ParseModelFunc: parseNvidiaModel,
		Protocol:       "openai",
		NewClientFunc:  NewNvidiaClient,
	})
}

// parseNvidiaModel ranks NVIDIA NIM-hosted models by capacity.
// NVIDIA NIM hosts a broad open-weight catalog; model names typically include
// the parameter size (e.g. "70b", "8b") and sometimes a namespace prefix
// (e.g. "meta/llama-3.1-70b-instruct", "nvidia/nemotron-70b-instruct-hf").
var parseNvidiaModel = NewModelParser(ParserConfig{
	DefaultVersion: 1.0,
	SizeWeights:    StandardSizeWeights,
	Tiers: []KeywordTier{
		{Keywords: []string{"nemotron", "starcoder"}, Score: 300, TierName: "nvidia-flagship"},
	},
	ContextBonus: true,
})
