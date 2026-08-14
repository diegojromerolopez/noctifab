package llm

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

func (a *anthropicProviderClient) Call(ctx context.Context, model, apiKey, prompt string, maxTokens int, temperature float64) ([]byte, error) {
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

	if maxTokens <= 0 {
		maxTokens = 4096
	}
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": maxTokens,
	}
	if temperature > 0 {
		payload["temperature"] = temperature
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

	var textBlocks []string
	var fallbackTextBlocks []string

	for _, elem := range content {
		item, isMap := elem.(map[string]any)
		if !isMap {
			continue
		}
		blockType, _ := item["type"].(string)
		txt, _ := item["text"].(string)

		if blockType == "text" && txt != "" {
			textBlocks = append(textBlocks, txt)
		} else if txt != "" && blockType != "thinking" {
			fallbackTextBlocks = append(fallbackTextBlocks, txt)
		}
	}

	if len(textBlocks) > 0 {
		return []byte(strings.Join(textBlocks, "\n")), nil
	}
	if len(fallbackTextBlocks) > 0 {
		return []byte(strings.Join(fallbackTextBlocks, "\n")), nil
	}

	return nil, fmt.Errorf("unexpected Anthropic response content: %s", string(respBody))
}

func (a *anthropicProviderClient) GetAvailableModels(ctx context.Context, apiKey string) ([]string, error) {
	var url string
	if a.url != "" {
		if strings.HasSuffix(a.url, "/messages") {
			url = strings.TrimSuffix(a.url, "/messages") + "/models"
		} else {
			url = a.url + "/models"
		}
	} else {
		url = "https://api.anthropic.com/v1/models"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch Anthropic models (HTTP %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var models []string
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	return models, nil
}
