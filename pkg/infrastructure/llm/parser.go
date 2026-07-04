package llm

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ExtractJSONBlock locates the outer JSON object of the LLM response envelope.
//
// It is string-literal-aware (braces inside JSON string values, such as Rust
// `mod tests { ... }` embedded in a write_file `content` argument, are not
// counted) and, when several top-level balanced blocks are present (e.g. the
// model emits fenced code before the JSON envelope), it prefers the block
// that looks like the LLM response schema (containing "reasoning" or
// "actions" keys). This makes the parser robust to lower-tier models that
// prepend prose/code before the JSON block.
func ExtractJSONBlock(input string) (string, error) {
	blocks := findTopLevelJSONBlocks(input)
	if len(blocks) == 0 {
		return "", errors.New("error parsing response: no valid JSON object detected (no opening brace found); please return only the structured JSON block matching the schema")
	}
	// Prefer the last block that looks like the LLMResponse envelope. The JSON
	// envelope is emitted after any prose/code, so scanning from the end is the
	// most reliable heuristic when the model wraps code blocks before the JSON.
	for i := len(blocks) - 1; i >= 0; i-- {
		if looksLikeLLMResponseEnvelope(blocks[i]) {
			return blocks[i], nil
		}
	}
	// Fallback: return the last balanced block so LenientUnmarshal can produce
	// a precise parse error rather than a generic "not found" message.
	return blocks[len(blocks)-1], nil
}

// findTopLevelJSONBlocks scans input for every maximal top-level balanced
// substring starting with '{'. It tracks JSON string literals so braces
// inside string values (common in embedded source code) do not reset the
// depth counter. Overlapping/nested blocks are not returned; only the
// outermost block at each start position.
func findTopLevelJSONBlocks(input string) []string {
	var blocks []string
	i := 0
	for i < len(input) {
		if input[i] != '{' {
			i++
			continue
		}
		end, ok := scanBalancedObject(input, i)
		if !ok {
			i++
			continue
		}
		blocks = append(blocks, input[i:end+1])
		i = end + 1
	}
	return blocks
}

// scanBalancedObject returns the index of the '}' that closes the JSON object
// opening at input[start], assuming start points at a '{'. It is string-aware:
// braces occurring inside JSON string literals are ignored, and standard
// backslash escaping (\") is honoured. Returns (end, true) on success.
func scanBalancedObject(input string, start int) (int, bool) {
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(input); i++ {
		ch := input[i]
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return -1, false
}

// looksLikeLLMResponseEnvelope reports whether a balanced JSON block appears
// to be an LLMResponse envelope by checking for the presence of the
// "reasoning" or "actions" keys. This is a cheap substring heuristic that
// avoids a full unmarshal (the block may still contain raw newlines that
// escapeNewlinesInJSON will normalise later in LenientUnmarshal).
func looksLikeLLMResponseEnvelope(block string) bool {
	return strings.Contains(block, `"reasoning"`) || strings.Contains(block, `"actions"`)
}

func escapeNewlinesInJSON(input string) string {
	var builder strings.Builder
	inString := false
	isEscaped := false

	for i := 0; i < len(input); i++ {
		char := input[i]

		if char == '"' && !isEscaped {
			inString = !inString
		}

		if inString {
			if char == '\n' {
				builder.WriteString(`\n`)
				isEscaped = false
				continue
			}
			if char == '\r' {
				builder.WriteString(`\r`)
				isEscaped = false
				continue
			}
		}

		builder.WriteByte(char)

		if char == '\\' {
			isEscaped = !isEscaped
		} else {
			isEscaped = false
		}
	}

	return builder.String()
}

// LenientUnmarshal unmarshals the extracted JSON string into intermediate map interfaces
// and programmatically walks and coerces types to construct a validated domain.LLMResponse.
func LenientUnmarshal(jsonStr string) (*domain.LLMResponse, error) {
	jsonStr = escapeNewlinesInJSON(jsonStr)
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
