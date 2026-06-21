package llm

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ExtractJSONBlock locates the first matching outer JSON object using brace counting.
func ExtractJSONBlock(input string) (string, error) {
	start := strings.Index(input, "{")
	if start == -1 {
		return "", errors.New("error parsing response: no valid JSON object detected (no opening brace found); please return only the structured JSON block matching the schema")
	}

	count := 0
	for i := start; i < len(input); i++ {
		char := input[i]
		switch char {
		case '{':
			count++
		case '}':
			count--
			if count == 0 {
				return input[start : i+1], nil
			}
		}
	}

	return "", errors.New("error parsing response: no valid JSON object detected (brace counter did not resolve); please return only the structured JSON block matching the schema")
}

// LenientUnmarshal unmarshals the extracted JSON string into intermediate map interfaces
// and programmatically walks and coerces types to construct a validated domain.LLMResponse.
func LenientUnmarshal(jsonStr string) (*domain.LLMResponse, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, err
	}

	response := &domain.LLMResponse{}
	response.Reasoning, _ = raw["reasoning"].(string)

	actionsRaw, ok := raw["actions"].([]any)
	if !ok {
		// Try singular action fallback
		if actRaw, exists := raw["action"]; exists {
			actionsRaw = []any{actRaw}
		}
	}

	for _, actVal := range actionsRaw {
		actMap, ok := actVal.(map[string]any)
		if !ok {
			continue
		}

		tool, _ := actMap["tool"].(string)
		argsMap, _ := actMap["args"].(map[string]any)
		if argsMap == nil {
			argsMap = make(map[string]any)
		}

		// Coerce "depends_on" parameter
		if depRaw, exists := argsMap["depends_on"]; exists {
			if s, ok := depRaw.(string); ok {
				argsMap["depends_on"] = []string{s}
			} else if depSlice, ok := depRaw.([]any); ok {
				var stringSlice []string
				for _, val := range depSlice {
					if strVal, ok := val.(string); ok {
						stringSlice = append(stringSlice, strVal)
					}
				}
				argsMap["depends_on"] = stringSlice
			}
		}

		// Coerce "resolved" stringified booleans
		if resRaw, exists := argsMap["resolved"]; exists {
			if s, ok := resRaw.(string); ok {
				switch s {
				case "true":
					argsMap["resolved"] = true
				case "false":
					argsMap["resolved"] = false
				}
			}
		}

		response.Actions = append(response.Actions, domain.LLMAction{
			Tool: tool,
			Args: argsMap,
		})
	}

	return response, nil
}
