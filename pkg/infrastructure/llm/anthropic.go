package llm

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

func (a *anthropicProviderClient) Call(ctx context.Context, model, apiKey, prompt string, maxTokens int, temperature float64) (*ProviderCallResult, error) {
	var url string
	if a.url != "" {
		url = a.url
	} else {
		url = "https://api.anthropic.com/v1/messages"
	}

	headers := make(map[string]string)
	headers["X-API-Key"] = apiKey
	headers["anthropic-version"] = "2023-06-01"
	headers["anthropic-beta"] = "prompt-caching-2024-07-31"
	headers["Content-Type"] = "application/json"

	if maxTokens <= 0 {
		maxTokens = 4096
	}

	useCacheControl := len(prompt) > 2048
	currentTemp := temperature
	currentMaxTokens := maxTokens

	timeout := a.timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		},
	}

	for attempt := 0; attempt < 3; attempt++ {
		var messageContent any = prompt
		if useCacheControl {
			messageContent = []map[string]any{
				{
					"type":          "text",
					"text":          prompt,
					"cache_control": map[string]string{"type": "ephemeral"},
				},
			}
		}

		payload := map[string]any{
			"model": model,
			"messages": []map[string]any{
				{"role": "user", "content": messageContent},
			},
			"max_tokens": currentMaxTokens,
		}
		if currentTemp > 0 {
			payload["temperature"] = currentTemp
		}

		reqBody, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		postCtx, cancel := context.WithTimeout(ctx, timeout)
		req, err := http.NewRequestWithContext(postCtx, "POST", url, bytes.NewBuffer(reqBody))
		if err != nil {
			cancel()
			return nil, err
		}

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		cancel()
		if err != nil {
			return nil, err
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusOK {
			return a.parseResponse(respBody)
		}

		bodyStr := string(respBody)
		if resp.StatusCode == http.StatusBadRequest {
			if looksLikeInvalidTemperature(bodyStr) && currentTemp > 0 {
				fmt.Fprintln(os.Stderr, "⚠ Server rejected the temperature value; retrying with the provider default.")
				currentTemp = 0
				continue
			}
			if looksLikeMaxTokensRejection(bodyStr) && currentMaxTokens > 4096 {
				fmt.Fprintln(os.Stderr, "⚠ Server rejected max_tokens parameter; retrying with max_tokens=4096.")
				currentMaxTokens = 4096
				continue
			}
			if useCacheControl && (strings.Contains(strings.ToLower(bodyStr), "cache_control") || strings.Contains(strings.ToLower(bodyStr), "prompt-caching")) {
				fmt.Fprintln(os.Stderr, "⚠ Server rejected cache_control parameter; retrying without prompt caching.")
				useCacheControl = false
				continue
			}
		}

		return nil, &httpError{StatusCode: resp.StatusCode, Body: bodyStr, Header: resp.Header}
	}

	return nil, fmt.Errorf("failed to complete Anthropic request after parameter retries")
}

func (a *anthropicProviderClient) parseResponse(respBody []byte) (*ProviderCallResult, error) {
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		return nil, fmt.Errorf("unexpected Anthropic response: %s", string(respBody))
	}

	var inputTokens, cacheReadTokens, cacheCreationTokens, outputTokens int64
	if usageMap, ok := result["usage"].(map[string]any); ok {
		if v, ok := usageMap["input_tokens"].(float64); ok {
			inputTokens = int64(v)
		}
		if v, ok := usageMap["cache_read_input_tokens"].(float64); ok {
			cacheReadTokens = int64(v)
		}
		if v, ok := usageMap["cache_creation_input_tokens"].(float64); ok {
			cacheCreationTokens = int64(v)
		}
		if v, ok := usageMap["output_tokens"].(float64); ok {
			outputTokens = int64(v)
		}
	}
	usage := ExtractAnthropicTokenUsage(inputTokens, cacheReadTokens, cacheCreationTokens, outputTokens)

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

	var bodyBytes []byte
	if len(textBlocks) > 0 {
		bodyBytes = []byte(strings.Join(textBlocks, "\n"))
	} else if len(fallbackTextBlocks) > 0 {
		bodyBytes = []byte(strings.Join(fallbackTextBlocks, "\n"))
	} else {
		return nil, fmt.Errorf("unexpected Anthropic response content: %s", string(respBody))
	}

	return &ProviderCallResult{
		Body:  bodyBytes,
		Usage: usage,
	}, nil
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
