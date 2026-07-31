package llm

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "anthropic",
		BaseURL:        "https://api.anthropic.com/v1",
		EnvKeys:        []string{"ANTHROPIC_API_KEY"},
		ParseModelFunc: parseAnthropicModel,
		Protocol:       "anthropic",
		NewClientFunc: func(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
			return NewAnthropicProviderClient(url, timeout, idleTimeout, streaming)
		},
	})
}

var parseAnthropicModel = NewModelParser(ParserConfig{
	RequiredPrefix: "claude",
	DefaultVersion: 3.0,
	VersionRegexp:  `claude-([0-9]+(?:[\.-][0-9]+)?)`,
	Tiers: []KeywordTier{
		{Keywords: []string{"opus"}, Score: 400, TierName: "opus"},
		{Keywords: []string{"sonnet"}, Score: 300, TierName: "sonnet"},
		{Keywords: []string{"haiku"}, Score: 200, TierName: "haiku"},
	},
})

type anthropicProviderClient struct {
	url         string
	timeout     time.Duration
	idleTimeout time.Duration
	streaming   bool
}

// NewAnthropicProviderClient creates a ProviderClient for Anthropic (Claude) API.
func NewAnthropicProviderClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &anthropicProviderClient{url: url, timeout: timeout, idleTimeout: idleTimeout, streaming: streaming}
}

func (a *anthropicProviderClient) Call(ctx context.Context, model, apiKey, prompt string) ([]byte, error) {
	var url string
	if a.url != "" {
		url = a.url
	} else {
		url = "https://api.anthropic.com/v1/messages"
	}

	headers := make(map[string]string)
	headers["X-API-Key"] = apiKey
	headers["anthropic-version"] = "2023-06-01"
	headers["Content-Type"] = "application/json"

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens":  4096,
		"temperature": 0.0,
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	timeout := a.timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	postCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(postCtx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{StatusCode: resp.StatusCode, Body: string(respBody), Header: resp.Header}
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		return nil, fmt.Errorf("unexpected Anthropic response: %s", string(respBody))
	}
	item := content[0].(map[string]any)
	text, _ := item["text"].(string)
	return []byte(text), nil
}

func (a *anthropicProviderClient) GetAvailableModels(ctx context.Context, apiKey string) ([]string, error) {
	return nil, fmt.Errorf("provider anthropic does not support listing models")
}
