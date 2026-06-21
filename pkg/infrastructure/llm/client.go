package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type Client struct {
	Provider   string
	Model      string
	APIKey     string
	MaxRetries int
	Backoff    time.Duration
}

var _ domain.LLMClient = (*Client)(nil)

func NewClient(provider, model, apiKey string, maxRetries int, backoff time.Duration) *Client {
	return &Client{
		Provider:   provider,
		Model:      model,
		APIKey:     apiKey,
		MaxRetries: maxRetries,
		Backoff:    backoff,
	}
}

func (c *Client) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	apiKey := c.APIKey
	if apiKey == "" {
		switch c.Provider {
		case "gemini":
			apiKey = os.Getenv("GEMINI_API_KEY")
		case "openai":
			apiKey = os.Getenv("OPENAI_API_KEY")
		case "anthropic":
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
	}

	if apiKey == "" {
		return nil, errors.New("missing API key for LLM provider")
	}

	var responseBody []byte
	var err error

	maxRetries := c.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 10
	}
	backoff := c.Backoff
	if backoff <= 0 {
		backoff = 100 * time.Millisecond
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		responseBody, err = c.doPost(ctx, apiKey, prompt)
		if err == nil {
			break
		}

		if attempt == maxRetries {
			return nil, fmt.Errorf("LLM completion failed after %d retries: %w", maxRetries, err)
		}

		// Exponential backoff with jitter
		jitter := time.Duration(float64(backoff) * (1.0 + rand.Float64()))
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(jitter):
		}
		backoff *= 2
	}

	extracted, err := ExtractJSONBlock(string(responseBody))
	if err != nil {
		return nil, err
	}

	return LenientUnmarshal(extracted)
}

func (c *Client) doPost(ctx context.Context, apiKey, prompt string) ([]byte, error) {
	var url string
	var reqBody []byte
	headers := make(map[string]string)

	switch c.Provider {
	case "openai":
		url = "https://api.openai.com/v1/chat/completions"
		headers["Authorization"] = "Bearer " + apiKey
		headers["Content-Type"] = "application/json"
		payload := map[string]any{
			"model": c.Model,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
			"temperature": 0.0,
		}
		reqBody, _ = json.Marshal(payload)

	case "gemini":
		model := c.Model
		if model == "" {
			model = "gemini-1.5-pro"
		}
		url = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
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
				"temperature": 0.0,
			},
		}
		reqBody, _ = json.Marshal(payload)

	case "anthropic":
		url = "https://api.anthropic.com/v1/messages"
		headers["X-API-Key"] = apiKey
		headers["anthropic-version"] = "2023-06-01"
		headers["Content-Type"] = "application/json"
		payload := map[string]any{
			"model": c.Model,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
			"max_tokens":  4096,
			"temperature": 0.0,
		}
		reqBody, _ = json.Marshal(payload)

	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", c.Provider)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 60 * time.Second}
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

	// Parse actual content out of the provider envelope
	switch c.Provider {
	case "openai":
		var result map[string]any
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, err
		}
		choices, ok := result["choices"].([]any)
		if !ok || len(choices) == 0 {
			return nil, fmt.Errorf("unexpected OpenAI response: %s", string(respBody))
		}
		choice := choices[0].(map[string]any)
		msg := choice["message"].(map[string]any)
		content, _ := msg["content"].(string)
		return []byte(content), nil

	case "gemini":
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

	case "anthropic":
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

	return respBody, nil
}
