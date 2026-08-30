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
		Name:           "gemini",
		BaseURL:        "https://generativelanguage.googleapis.com/v1beta",
		EnvKeys:        []string{"GEMINI_API_KEY"},
		ParseModelFunc: parseGeminiModelProvider,
		Protocol:       "gemini",
		NewClientFunc: func(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
			return NewGeminiProviderClient(url, timeout, idleTimeout, streaming)
		},
	})
}

// geminiTransport is a shared HTTP transport reused across all Gemini calls
// so TLS connections are pooled instead of re-handshaking per request. A
// non-nil empty TLSNextProto disables HTTP/2 (forcing HTTP/1.1), which works
// around mid-stream stalls observed with the Gemini endpoint over HTTP/2.
var geminiTransport = &http.Transport{
	TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
}

var parseGeminiModelProvider = NewModelParser(ParserConfig{
	RequiredPrefix:    "gemini",
	ExcludedKeywords:  []string{"robotics", "embed", "imagen", "image", "vision", "audio", "video", "bison", "tts", "stt"},
	DefaultVersion:    1.5,
	VersionRegexp:     `gemini-([0-9]+(?:\.[0-9]+)?)`,
	VersionMultiplier: 10,
	Tiers: []KeywordTier{
		{Keywords: []string{"pro"}, Score: 40, TierName: "pro"},
		{Keywords: []string{"flash"}, Score: 30, TierName: "flash"},
		{Keywords: []string{"flash-lite", "flash_lite"}, Score: 20, TierName: "flash-lite"},
		{Keywords: []string{"nano"}, Score: 10, TierName: "nano"},
	},
})

type geminiProviderClient struct {
	url         string
	timeout     time.Duration
	idleTimeout time.Duration
	streaming   bool
}

// NewGeminiProviderClient creates a ProviderClient for Gemini API.
func NewGeminiProviderClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &geminiProviderClient{url: url, timeout: timeout, idleTimeout: idleTimeout, streaming: streaming}
}

func (g *geminiProviderClient) Call(ctx context.Context, model, apiKey, prompt string, maxTokens int, temperature float64) (*ProviderCallResult, error) {
	var url string
	if g.url != "" {
		url = g.url
	} else {
		url = resolveGeminiURL(model, apiKey)
	}

	headers := make(map[string]string)
	headers["Content-Type"] = "application/json"

	generationConfig := map[string]any{
		"temperature":      tempOrDefault(temperature),
		"responseMimeType": "application/json",
	}
	if maxTokens > 0 {
		generationConfig["maxOutputTokens"] = maxTokens
	}
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": generationConfig,
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	timeout := g.timeout
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
		Timeout:   timeout,
		Transport: geminiTransport,
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

	var promptCount, candidatesCount, cachedCount int64
	if usageMeta, ok := result["usageMetadata"].(map[string]any); ok {
		if v, ok := usageMeta["promptTokenCount"].(float64); ok {
			promptCount = int64(v)
		}
		if v, ok := usageMeta["candidatesTokenCount"].(float64); ok {
			candidatesCount = int64(v)
		}
		if v, ok := usageMeta["cachedContentTokenCount"].(float64); ok {
			cachedCount = int64(v)
		}
	}
	usage := ExtractGeminiTokenUsage(promptCount, candidatesCount, cachedCount)

	candidates, ok := result["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		return nil, fmt.Errorf("unexpected Gemini response: %s", string(respBody))
	}
	candidate, ok := candidates[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected Gemini response: candidate is not a map")
	}
	content, ok := candidate["content"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected Gemini response: content is not a map")
	}
	parts, ok := content["parts"].([]any)
	if !ok || len(parts) == 0 {
		return nil, fmt.Errorf("unexpected Gemini response: parts list is missing or empty")
	}
	part, ok := parts[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected Gemini response: part is not a map")
	}
	text, _ := part["text"].(string)
	return &ProviderCallResult{
		Body:  []byte(text),
		Usage: usage,
	}, nil
}

func (g *geminiProviderClient) GetAvailableModels(ctx context.Context, apiKey string) ([]string, error) {
	var url string
	if g.url != "" {
		if strings.Contains(g.url, "generateContent") {
			idx := strings.Index(g.url, "/models/")
			if idx != -1 {
				url = g.url[:idx] + "/models"
				if qIdx := strings.Index(g.url, "?"); qIdx != -1 {
					url += g.url[qIdx:]
				}
			} else {
				url = g.url
			}
		} else {
			url = g.url + "/models?key=" + apiKey
		}
	} else {
		url = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", apiKey)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch models (HTTP %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Models []struct {
			Name                       string   `json:"name"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	var models []string
	for _, m := range result.Models {
		supportsGen := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsGen = true
				break
			}
		}
		if supportsGen {
			name := strings.TrimPrefix(m.Name, "models/")
			models = append(models, name)
		}
	}
	return models, nil
}
