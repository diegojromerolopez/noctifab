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

// isRoutineTask evaluates whether the given agent role represents a high-frequency,
// iterative code/test generation task where extended thinking delays should be suppressed.
func isRoutineTask(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "generator", "generators", "tester", "testers", "spike":
		return true
	default:
		return false
	}
}

// parseDynamicModelCapabilities inspects dynamic metadata objects returned by a provider's
// /models endpoint to determine capabilities (e.g. reasoning/thinking support) without hardcoding model names.
func parseDynamicModelCapabilities(rawModels []map[string]interface{}) map[string]ModelCapability {
	caps := make(map[string]ModelCapability, len(rawModels))

	for _, m := range rawModels {
		id, _ := m["id"].(string)
		if id == "" {
			// Some providers (e.g. Gemini) use "name"
			id, _ = m["name"].(string)
			id = strings.TrimPrefix(id, "models/")
		}
		if id == "" {
			continue
		}

		supportsThinking := false
		thinkingParam := ""

		// 1. Check supported_parameters / supportedParameters list
		for _, key := range []string{"supported_parameters", "supportedParameters", "parameters"} {
			if params, ok := m[key].([]interface{}); ok {
				for _, p := range params {
					if pStr, ok := p.(string); ok {
						pLow := strings.ToLower(pStr)
						if strings.Contains(pLow, "think") || strings.Contains(pLow, "reason") {
							supportsThinking = true
							if thinkingParam == "" {
								thinkingParam = pStr
							}
						}
					}
				}
			}
		}

		// 2. Check capabilities / features map
		for _, key := range []string{"capabilities", "features"} {
			if capMap, ok := m[key].(map[string]interface{}); ok {
				for cKey, cVal := range capMap {
					cKeyLow := strings.ToLower(cKey)
					if strings.Contains(cKeyLow, "think") || strings.Contains(cKeyLow, "reason") {
						if bVal, ok := cVal.(bool); ok && bVal {
							supportsThinking = true
							if thinkingParam == "" {
								thinkingParam = cKey
							}
						}
					}
				}
			}
		}

		// 3. Check direct thinking/reasoning configuration keys in model object
		for _, key := range []string{"thinkingConfig", "thinking_config", "thinking", "reasoning"} {
			if _, exists := m[key]; exists {
				supportsThinking = true
				if thinkingParam == "" {
					thinkingParam = key
				}
			}
		}

		// 4. Check architecture / instruct_type
		if arch, ok := m["architecture"].(map[string]interface{}); ok {
			for aKey, aVal := range arch {
				if aStr, ok := aVal.(string); ok {
					aStrLow := strings.ToLower(aStr)
					if strings.Contains(aStrLow, "think") || strings.Contains(aStrLow, "reason") {
						supportsThinking = true
						if thinkingParam == "" {
							thinkingParam = aKey
						}
					}
				}
			}
		}

		// 5. Check description or summary metadata returned by /models
		if desc, ok := m["description"].(string); ok {
			descLow := strings.ToLower(desc)
			if strings.Contains(descLow, "reasoning model") || strings.Contains(descLow, "extended thinking") || strings.Contains(descLow, "chain-of-thought") {
				supportsThinking = true
			}
		}

		caps[id] = ModelCapability{
			ID:               id,
			SupportsThinking: supportsThinking,
			ThinkingParam:    thinkingParam,
		}
	}

	return caps
}

// GetModelCapabilities queries the OpenAI-compatible /models endpoint dynamically and returns
// capability information for each available model without hardcoded model names.
func (o *baseOpenAIClient) GetModelCapabilities(ctx context.Context, apiKey string) (map[string]ModelCapability, error) {
	url := strings.TrimRight(o.sdkBaseURL(apiKey), "/") + "/models"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build /models request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query /models from %s: %w", o.provider, err)
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

	var parsed struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		// Try unmarshaling as direct slice
		var sliceData []map[string]interface{}
		if err2 := json.Unmarshal(body, &sliceData); err2 == nil {
			return parseDynamicModelCapabilities(sliceData), nil
		}
		return nil, fmt.Errorf("failed to parse /models response from %s: %w", o.provider, err)
	}

	return parseDynamicModelCapabilities(parsed.Data), nil
}

// adjustForRoutineTask inspects whether the active agent role is a routine task and disables extended
// thinking dynamically if the target model supports it according to the /models endpoint.
func (c *Client) adjustForRoutineTask(ctx context.Context, role, model string, extra map[string]interface{}) (map[string]interface{}, bool) {
	if !isRoutineTask(role) {
		return extra, false
	}

	pClient := c.providerClient()
	apiKey := c.getNextAPIKey()

	caps := c.availableCapabilitiesCached(ctx, pClient, apiKey)
	if len(caps) == 0 {
		return extra, false
	}

	normModel := strings.ToLower(strings.TrimSpace(model))
	normModel = strings.TrimPrefix(normModel, "models/")

	var capInfo *ModelCapability
	if ci, ok := caps[model]; ok {
		capInfo = &ci
	} else if ci, ok := caps[normModel]; ok {
		capInfo = &ci
	} else {
		// Prefix match against catalog items
		for id, ci := range caps {
			if strings.EqualFold(id, model) || strings.EqualFold(id, normModel) {
				capInfo = &ci
				break
			}
		}
	}

	if capInfo != nil && capInfo.SupportsThinking {
		if extra == nil {
			extra = make(map[string]interface{})
		}
		// Disable thinking across standard API dialect fields
		extra["enable_thinking"] = false
		extra["thinking_budget"] = 0
		extra["reasoning_effort"] = "low"
		extra["thinking"] = map[string]interface{}{"type": "disabled"}
		extra["thinkingConfig"] = map[string]interface{}{"thinkingBudget": 0}

		fmt.Fprintf(os.Stderr, "ℹ [llm] Dynamically disabled extended thinking for routine task role '%s' on model '%s' (discovered via /models)\n", role, model)
		return extra, true
	}

	return extra, false
}
