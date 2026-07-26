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

type openaiProviderClient struct {
	provider    string
	url         string
	timeout     time.Duration
	idleTimeout time.Duration
	streaming   bool
}

// NewOpenAIProviderClient creates a ProviderClient for OpenAI and OpenAI-compatible APIs.
func NewOpenAIProviderClient(provider, url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &openaiProviderClient{provider: provider, url: url, timeout: timeout, idleTimeout: idleTimeout, streaming: streaming}
}

// osStderr returns the process stderr. Wrapped so tests can swap it without
// importing "os" elsewhere.
func osStderr() *os.File { return os.Stderr }

// looksLikeResponseFormatRejection inspects the body of an HTTP 400 from a
// `/chat/completions` endpoint to decide whether the failure is plausibly
// caused by the server refusing to honour `response_format`. Conservative:
// accepts both snake_case and camelCase variants plus the canonical OpenAI
// wording. When in doubt, returns false (so we surface the raw 400 instead
// of silently dropping the field on a non-typed server error).
func looksLikeResponseFormatRejection(body string) bool {
	low := strings.ToLower(body)
	if strings.Contains(low, "response_format") {
		return true
	}
	if strings.Contains(low, "unknown parameter") {
		return true
	}
	if strings.Contains(low, "unrecognized request argument") {
		return true
	}
	return false
}

func (o *openaiProviderClient) Call(ctx context.Context, model, apiKey, prompt string) ([]byte, error) {
	url := o.resolveEndpoint()
	headers := make(map[string]string)
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	headers["Content-Type"] = "application/json"

	respBody, err := o.sendCompletion(ctx, url, headers, model, prompt, true)
	if err == nil {
		return respBody, nil
	}
	// Transparent single fallback: if a relay rejects the request because
	// it does not understand `response_format` (HTTP 400 with a hint in the
	// body), retry once without the field. This keeps compliant models on
	// the strong JSON path while not breaking older vLLM / Ollama setups.
	if he, ok := err.(*httpError); ok && he.StatusCode == http.StatusBadRequest && looksLikeResponseFormatRejection(he.Body) {
		_, _ = fmt.Fprintln(osStderr(), "⚠ Server rejected response_format; retrying without JSON enforcement.")
		return o.sendCompletion(ctx, url, headers, model, prompt, false)
	}
	return nil, err
}

// resolveEndpoint returns the chat completions URL for this provider, honouring
// an explicit override URL when one is configured.
func (o *openaiProviderClient) resolveEndpoint() string {
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
		baseURL = "https://api.deepseek.ai/v1"
	case "ollama":
		baseURL = "https://ollama.com/v1"
	case "opencode":
		baseURL = "https://opencode.ai/zen/go/v1"
	default:
		baseURL = "https://api.openai.com/v1"
	}
	if o.url != "" {
		return o.url
	}
	return baseURL + "/chat/completions"
}

// sendCompletion performs a single chat completions POST. When enforceJSON
// is true the request advertises response_format=json_object so the assistant
// is constrained to a JSON object.
func (o *openaiProviderClient) sendCompletion(ctx context.Context, url string, headers map[string]string, model, prompt string, enforceJSON bool) ([]byte, error) {
	if o.streaming {
		respBody, err := o.sendCompletionStreaming(ctx, url, headers, model, prompt, enforceJSON)
		if err == nil {
			return respBody, nil
		}
		// On streaming error, fallback to standard non-streaming POST
	}

	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.0,
	}
	if enforceJSON {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	timeout := o.timeout
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
	choices, ok := result["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("unexpected OpenAI-compatible response: %s", string(respBody))
	}
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	content, _ := msg["content"].(string)
	return []byte(content), nil
}

func (o *openaiProviderClient) sendCompletionStreaming(ctx context.Context, url string, headers map[string]string, model, prompt string, enforceJSON bool) ([]byte, error) {
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.0,
		"stream":      true,
	}
	if enforceJSON {
		payload["response_format"] = map[string]string{"type": "json_object"}
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	timeout := o.timeout
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
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{
		Transport: &http.Transport{
			TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &httpError{StatusCode: resp.StatusCode, Body: string(respBody), Header: resp.Header}
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		defer func() { _ = resp.Body.Close() }()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
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

	var sb strings.Builder
	idleTimeout := o.idleTimeout
	if idleTimeout <= 0 {
		idleTimeout = 15 * time.Second
	}

	err = readSSEResponse(postCtx, resp.Body, idleTimeout, func(line string) error {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") {
			return nil
		}
		if !strings.HasPrefix(line, "data:") {
			return nil
		}
		dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if dataStr == "[DONE]" {
			return nil
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(dataStr), &chunk); err == nil {
			if len(chunk.Choices) > 0 {
				sb.WriteString(chunk.Choices[0].Delta.Content)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return []byte(sb.String()), nil
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
