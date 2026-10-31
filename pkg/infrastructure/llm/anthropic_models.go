package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ActiveClaudeHaikuModel is the active Claude Haiku model identifier after Claude 3.5 Haiku was retired.
const ActiveClaudeHaikuModel = "claude-haiku-5-5"

// isRetiredAnthropicModel returns true if the model was retired by Anthropic.
// On February 19, 2026, Anthropic retired Claude Haiku 3.5 (claude-3-5-haiku-20241022)
// and all legacy Claude 3/3.5 Haiku models on the Claude API.
func isRetiredAnthropicModel(model string) bool {
	norm := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(norm, "claude-3-5-haiku") || strings.Contains(norm, "claude-3-haiku")
}

// rerouteRetiredAnthropicModel inspects candidate model identifiers and transparently
// redirects retired Claude 3.5 Haiku requests to the active Claude Haiku model.
func rerouteRetiredAnthropicModel(model string) string {
	if isRetiredAnthropicModel(model) {
		fmt.Fprintf(os.Stderr, "⚠ Anthropic retired %s on February 19, 2026; redirecting request to active model %s\n", model, ActiveClaudeHaikuModel)
		return ActiveClaudeHaikuModel
	}
	return model
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
		// Filter out retired models so Noctifab never attempts to fall back or send requests to them
		if !isRetiredAnthropicModel(m.ID) {
			models = append(models, m.ID)
		}
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

	// Filter out retired models before computing capabilities
	var activeData []map[string]interface{}
	for _, item := range result.Data {
		if id, ok := item["id"].(string); ok && isRetiredAnthropicModel(id) {
			continue
		}
		activeData = append(activeData, item)
	}

	return parseDynamicModelCapabilities(activeData), nil
}
