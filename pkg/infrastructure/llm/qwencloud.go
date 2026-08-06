package llm

import (
	"context"
	"time"
)

type QwenCloudClient struct {
	*baseOpenAIClient
}

func (q *QwenCloudClient) GetAvailableModels(ctx context.Context, apiKey string) ([]string, error) {
	if models, err := q.baseOpenAIClient.GetAvailableModels(ctx, apiKey); err == nil {
		return models, nil
	}
	return []string{"qwen3.8-max", "qwen-turbo", "qwen-plus"}, nil
}

func NewQwenCloudClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &QwenCloudClient{
		baseOpenAIClient: newBaseOpenAIClient("qwencloud", "https://dashscope-intl.aliyuncs.com/compatible-mode/v1", url, timeout, idleTimeout, streaming),
	}
}

func init() {
	qwenSpec := &ProviderSpec{
		Name:           "qwen",
		BaseURL:        "https://dashscope.aliyuncs.com/compatible-mode/v1",
		EnvKeys:        []string{"DASHSCOPE_API_KEY", "QWEN_API_KEY"},
		ParseModelFunc: parseQwenModel,
		Protocol:       "openai",
		NewClientFunc:  NewQwenCloudClient,
	}
	RegisterProvider(qwenSpec)
	RegisterProvider(&ProviderSpec{
		Name:           "dashscope",
		BaseURL:        "https://dashscope.aliyuncs.com/compatible-mode/v1",
		EnvKeys:        []string{"DASHSCOPE_API_KEY", "QWEN_API_KEY"},
		ParseModelFunc: parseQwenModel,
		Protocol:       "openai",
		NewClientFunc:  NewQwenCloudClient,
	})
	RegisterProvider(&ProviderSpec{
		Name:           "qwencloud",
		BaseURL:        "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		EnvKeys:        []string{"QWENCLOUD_API_KEY", "DASHSCOPE_API_KEY", "QWEN_API_KEY"},
		ParseModelFunc: parseQwenModel,
		Protocol:       "openai",
		NewClientFunc:  NewQwenCloudClient,
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
