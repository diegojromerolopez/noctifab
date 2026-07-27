package llm

import "time"

type QwenClient struct {
	*baseOpenAIClient
}

func NewQwenClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &QwenClient{
		baseOpenAIClient: newBaseOpenAIClient("qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	qwenSpec := &ProviderSpec{
		Name:           "qwen",
		BaseURL:        "https://dashscope.aliyuncs.com/compatible-mode/v1",
		EnvKeys:        []string{"DASHSCOPE_API_KEY", "QWEN_API_KEY"},
		ParseModelFunc: parseQwenModel,
		Protocol:       "openai",
		NewClientFunc:  NewQwenClient,
	}
	RegisterProvider(qwenSpec)
	RegisterProvider(&ProviderSpec{
		Name:           "dashscope",
		BaseURL:        "https://dashscope.aliyuncs.com/compatible-mode/v1",
		EnvKeys:        []string{"DASHSCOPE_API_KEY", "QWEN_API_KEY"},
		ParseModelFunc: parseQwenModel,
		Protocol:       "openai",
		NewClientFunc:  NewQwenClient,
	})
}

var parseQwenModel = NewModelParser(ParserConfig{
	RequiredPrefix: "qwen",
	DefaultVersion: 2.5,
	VersionRegexp:  `([0-9]+\.[0-9]+)`,
	Tiers: []KeywordTier{
		{Keywords: []string{"max"}, Score: 40, TierName: "max"},
		{Keywords: []string{"plus"}, Score: 30, TierName: "plus"},
		{Keywords: []string{"turbo"}, Score: 20, TierName: "turbo"},
	},
})
