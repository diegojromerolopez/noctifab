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

type openaiProviderClient struct {
	provider string
	url      string
}

// NewOpenAIProviderClient creates a ProviderClient for OpenAI and OpenAI-compatible APIs.
func NewOpenAIProviderClient(provider, url string) ProviderClient {
	return &openaiProviderClient{provider: provider, url: url}
}

func (o *openaiProviderClient) Call(ctx context.Context, model, apiKey, prompt string) ([]byte, error) {
	var baseURL string
	switch strings.ToLower(o.provider) {
	case "openai":
		baseURL = "https://api.openai.com/v1"
	case "hermes":
		baseURL = "https://inference-api.nousresearch.com/v1"
	case "huggingface":
		baseURL = "https://api-inference.huggingface.co/v1"
	case "mistral":
		baseURL = "https://api.mistral.ai/v1"
	case "deepseek":
		baseURL = "https://api.deepseek.com/v1"
	case "ollama":
		baseURL = "https://ollama.com/v1"
	case "opencode":
		baseURL = "https://opencode.ai/zen/go/v1"
	default:
		baseURL = "https://api.openai.com/v1"
	}

	var url string
	if o.url != "" {
		url = o.url
	} else {
		url = baseURL + "/chat/completions"
	}

	headers := make(map[string]string)
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	headers["Content-Type"] = "application/json"

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.0,
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	postCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(postCtx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{
		Timeout: 10 * time.Minute,
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
	choices, ok := result["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("unexpected OpenAI-compatible response: %s", string(respBody))
	}
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	content, _ := msg["content"].(string)
	return []byte(content), nil
}

func (o *openaiProviderClient) GetAvailableModels(ctx context.Context, apiKey string) ([]string, error) {
	var baseURL string
	switch strings.ToLower(o.provider) {
	case "openai":
		baseURL = "https://api.openai.com/v1"
	case "hermes":
		baseURL = "https://inference-api.nousresearch.com/v1"
	case "huggingface":
		baseURL = "https://api-inference.huggingface.co/v1"
	case "mistral":
		baseURL = "https://api.mistral.ai/v1"
	case "deepseek":
		baseURL = "https://api.deepseek.com/v1"
	case "ollama":
		baseURL = "https://ollama.com/v1"
	case "opencode":
		baseURL = "https://opencode.ai/zen/go/v1"
	default:
		baseURL = "https://api.openai.com/v1"
	}

	var url string
	if o.url != "" {
		if strings.HasSuffix(o.url, "/chat/completions") {
			url = strings.TrimSuffix(o.url, "/chat/completions") + "/models"
		} else {
			url = o.url + "/models"
		}
	} else {
		url = baseURL + "/models"
	}

	headers := make(map[string]string)
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
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
