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

	"github.com/diegojromerolopez/noctifab/pkg/domain"
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
	extraBody   map[string]interface{}
}

// SetExtraBody attaches provider-specific extra body parameters (such as disabling thinking).
func (a *anthropicProviderClient) SetExtraBody(params map[string]interface{}) {
	a.extraBody = params
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
		maxTokens = 8192
	}

	useCacheControl := len(prompt) > 1024
	currentTemp := temperature
	currentMaxTokens := maxTokens

	if hasThinkingEnabled(a.extraBody) || globalCapabilityCache.isTemperatureUnsupported(model) {
		currentTemp = 0
	}

	// Claude Extended Thinking Token Guard:
	// In Anthropic API, max_tokens bounds both thinking_tokens AND response text_tokens.
	// If thinking is enabled or budget_tokens is set, max_tokens MUST be larger than
	// budget_tokens to prevent output_tokens exhaustion (stop_reason: "max_tokens"
	// with zero text blocks).
	if a.extraBody != nil {
		if th, ok := a.extraBody["thinking"].(map[string]interface{}); ok {
			budgetTokens := 0
			if b, ok := th["budget_tokens"].(float64); ok {
				budgetTokens = int(b)
			} else if b, ok := th["budget_tokens"].(int); ok {
				budgetTokens = b
			}
			if budgetTokens > 0 && currentMaxTokens <= budgetTokens+2048 {
				currentMaxTokens = budgetTokens + 4096
			}
			if currentMaxTokens < 8192 {
				currentMaxTokens = 8192
			}
		}
	}

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
		messageContent := buildAnthropicUserMessageContent(ctx, prompt, useCacheControl)

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
		if a.extraBody != nil {
			if th, ok := a.extraBody["thinking"].(map[string]interface{}); ok {
				payload["thinking"] = th
			}
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
			callRes, pErr := a.parseResponse(respBody)
			if pErr != nil {
				// Check for thinking token exhaustion: stop_reason == "max_tokens" without text output
				var resMap map[string]any
				if json.Unmarshal(respBody, &resMap) == nil {
					stopReason, _ := resMap["stop_reason"].(string)
					if stopReason == "max_tokens" && currentMaxTokens < 32768 && attempt < 2 {
						fmt.Fprintf(os.Stderr, "⚠ [Anthropic] Thinking consumed entire max_tokens (%d); increasing max_tokens to %d and retrying.\n", currentMaxTokens, currentMaxTokens*2)
						currentMaxTokens = currentMaxTokens * 2
						continue
					}
				}
				return nil, pErr
			}
			return callRes, nil
		}

		bodyStr := string(respBody)
		if resp.StatusCode == http.StatusBadRequest {
			if looksLikeInvalidTemperature(bodyStr) && currentTemp > 0 {
				fmt.Fprintln(os.Stderr, "⚠ Server rejected the temperature value; retrying with the provider default.")
				currentTemp = 0
				globalCapabilityCache.markTemperatureUnsupported(model)
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

func (a *anthropicProviderClient) GetModelCapabilities(ctx context.Context, apiKey string) (map[string]ModelCapability, error) {
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
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return parseDynamicModelCapabilities(result.Data), nil
}

// buildAnthropicUserMessageContent constructs structured content blocks for Anthropic
// messages, placing ephemeral cache_control breakpoints on stable prompt prefixes.
func buildAnthropicUserMessageContent(ctx context.Context, prompt string, useCacheControl bool) any {
	if !useCacheControl {
		return prompt
	}

	prefixLen := domain.CacheablePrefixLen(ctx)
	if prefixLen <= 0 || prefixLen >= len(prompt) {
		// Auto-detect multi-turn continuation boundary
		if idx := strings.Index(prompt, "\n\nTOOL OUTPUTS FROM PREVIOUS TURN"); idx >= 500 {
			prefixLen = idx
		}
	}

	if prefixLen > 0 && prefixLen < len(prompt) {
		prefix := prompt[:prefixLen]
		suffix := prompt[prefixLen:]

		// Check if prefix can be split into static template instructions vs task details
		taskDetailsIdx := strings.Index(prefix, "\nTask Details:")
		if taskDetailsIdx < 0 {
			taskDetailsIdx = strings.Index(prefix, "\n\nTask Details:")
		}

		if taskDetailsIdx >= 1000 {
			staticPart := prefix[:taskDetailsIdx]
			dynamicPrefix := prefix[taskDetailsIdx:]
			var blocks []map[string]any
			if len(staticPart) > 0 {
				blocks = append(blocks, map[string]any{
					"type":          "text",
					"text":          staticPart,
					"cache_control": map[string]string{"type": "ephemeral"},
				})
			}
			if len(dynamicPrefix) > 0 {
				blocks = append(blocks, map[string]any{
					"type":          "text",
					"text":          dynamicPrefix,
					"cache_control": map[string]string{"type": "ephemeral"},
				})
			}
			if len(suffix) > 0 {
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": suffix,
				})
			}
			if len(blocks) > 0 {
				return blocks
			}
		}

		// Single breakpoint on prefixLen
		var blocks []map[string]any
		blocks = append(blocks, map[string]any{
			"type":          "text",
			"text":          prefix,
			"cache_control": map[string]string{"type": "ephemeral"},
		})
		if len(suffix) > 0 {
			blocks = append(blocks, map[string]any{
				"type": "text",
				"text": suffix,
			})
		}
		return blocks
	}

	// Auto-detect static template instructions in Turn 1 without explicit prefixLen
	taskDetailsIdx := strings.Index(prompt, "\nTask Details:")
	if taskDetailsIdx < 0 {
		taskDetailsIdx = strings.Index(prompt, "\n\nTask Details:")
	}
	if taskDetailsIdx >= 1000 && taskDetailsIdx < len(prompt) {
		return []map[string]any{
			{
				"type":          "text",
				"text":          prompt[:taskDetailsIdx],
				"cache_control": map[string]string{"type": "ephemeral"},
			},
			{
				"type": "text",
				"text": prompt[taskDetailsIdx:],
			},
		}
	}

	// Fallback to single block with cache_control
	return []map[string]any{
		{
			"type":          "text",
			"text":          prompt,
			"cache_control": map[string]string{"type": "ephemeral"},
		},
	}
}
