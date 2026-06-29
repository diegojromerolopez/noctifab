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

type geminiProviderClient struct{}

// NewGeminiProviderClient creates a ProviderClient for Gemini API.
func NewGeminiProviderClient() ProviderClient {
	return &geminiProviderClient{}
}

func (g *geminiProviderClient) Call(ctx context.Context, model, apiKey, prompt, customURL string) ([]byte, error) {
	var url string
	if customURL != "" {
		url = customURL
	} else {
		url = resolveGeminiURL(model, apiKey)
	}

	headers := make(map[string]string)
	headers["Content-Type"] = "application/json"

	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]any{
			"temperature":      0.0,
			"responseMimeType": "application/json",
		},
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
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	candidates, ok := result["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		return nil, fmt.Errorf("unexpected Gemini response: %s", string(respBody))
	}
	candidate := candidates[0].(map[string]any)
	content := candidate["content"].(map[string]any)
	parts := content["parts"].([]any)
	part := parts[0].(map[string]any)
	text, _ := part["text"].(string)
	return []byte(text), nil
}

func (g *geminiProviderClient) GetAvailableModels(ctx context.Context, apiKey string) ([]string, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", apiKey)
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
