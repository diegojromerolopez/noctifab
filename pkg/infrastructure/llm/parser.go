package llm

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// jsonKeyRE matches a JSON key token that names a field of the LLMResponse
// schema (reasoning, actions, tool, args). Used to distinguish a real
// envelope from incidental `{ ... }` snippets in fenced code (Rust struct
// literals, Go composite literals, etc). Keys must be followed by an
// optional space and a colon to count.
var jsonKeyRE = regexp.MustCompile(`"(reasoning|actions|tool|args|path|content)"\s*:`)

// ExtractJSONBlock locates the outer JSON object of the LLM response envelope.
//
// It is string-literal-aware (braces inside JSON string values, such as Rust
// `mod tests { ... }` embedded in a write_file `content` argument, are not
// counted), tolerant of fenced code blocks surrounding the JSON
// (```{"reasoning": ...}```), and rejects blocks that look like the schema
// (containing "reasoning"/"actions" JSON keys) so code-block braces such as
// `CountStats { lines: 0, words: 0, bytes: 0 }` are never mistaken for a
// JSON object. When no candidate envelope is found it returns an error
// rather than guessing — guessing feeds junk to json.Unmarshal and produces
// misleading parse errors (e.g. "invalid character 'l'" on a Rust struct).
func ExtractJSONBlock(input string) (string, error) {
	cleaned := stripFencedCodeBlocks(input)
	blocks := findTopLevelJSONBlocks(cleaned)
	if len(blocks) == 0 {
		return "", errors.New("error parsing response: no valid JSON object detected (no opening brace found); please return only the structured JSON block matching the schema")
	}
	// Prefer the last block whose top-level keys match the LLMResponse
	// schema (reasoning / actions / tool / args / path / content).
	preferred := []string{}
	for _, b := range blocks {
		if looksLikeLLMResponseEnvelope(b) {
			preferred = append(preferred, b)
		}
	}
	if len(preferred) == 0 {
		return "", errors.New("error parsing response: JSON envelope not detected (found balanced blocks but none declares the expected keys; please return only the structured JSON block matching the schema)")
	}
	return preferred[len(preferred)-1], nil
}

// stripFencedCodeBlocks removes markdown fenced code blocks (```...```) so the
// scanner does not pick up `{` / `}` characters embedded in fenced source
// code. A fence is ` ``` ` optionally followed by a language tag (rust, go,
// `json`, bash, ...) and terminated by a closing ` ``` ` line. Only
// "fenced" code is stripped — JSON envelopes that happen to be wrapped in
// ` ```json ` are re-exposed as the JSON text they contain.
func stripFencedCodeBlocks(input string) string {
	var builder strings.Builder
	i := 0
	n := len(input)
	for i < n {
		// Detect a fence opening: a line starting with ``` (after optional
		// leading whitespace). We do a per-line scan so we keep newlines.
		lineEnd := strings.IndexByte(input[i:], '\n')
		var line string
		if lineEnd == -1 {
			line = input[i:]
		} else {
			line = input[i : i+lineEnd]
		}
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "```") {
			// Skip the entire fenced block through its matching closing ``` .
			start := i + lineEnd + 1
			if lineEnd == -1 {
				// Opening fence is the last line — nothing else to keep.
				return builder.String()
			}
			// Search for a closing fence line.
			closed := false
			j := start
			for j < n {
				end := strings.IndexByte(input[j:], '\n')
				var l string
				if end == -1 {
					l = input[j:]
				} else {
					l = input[j : j+end]
				}
				lt := strings.TrimLeft(l, " \t")
				if strings.HasPrefix(lt, "```") {
					// Found closing fence. Check if it's a `json` tagged fence
					// we should unwrap rather than drop.
					lang := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
					if lang == `` || strings.EqualFold(lang, "json") {
						// Re-expose the inner content of this fence as
						// un-fenced text so the JSON block scanner sees it.
						inner := input[start:j]
						if end != -1 {
							// Include the closing newline.
							inner += "\n"
						}
						builder.WriteString(inner)
					}
					if end == -1 {
						closed = true
						i = n
					} else {
						closed = true
						i = j + end + 1
					}
					break
				}
				if end == -1 {
					// ran out of input without closing fence — drop rest.
					i = n
					closed = true
					break
				}
				j = j + end + 1
			}
			if !closed {
				// No closing fence — never started a skip; leave the opening
				// fence line so subsequent JSON scanning still operates on its
				// trailing content.
				builder.WriteString(line)
				if lineEnd != -1 {
					builder.WriteByte('\n')
					i = i + lineEnd + 1
				} else {
					i = n
				}
			}
			continue
		}
		builder.WriteString(line)
		if lineEnd != -1 {
			builder.WriteByte('\n')
			i = i + lineEnd + 1
		} else {
			i = n
		}
	}
	return builder.String()
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
// to be the LLMResponse envelope by checking for JSON-key tokens from the
// schema. Uses a regex that requires `"key":` so prose comments like
// `// reasoning here` and bare struct literals like
// `CountStats { lines: 0, words: 0 }` do not match.
func looksLikeLLMResponseEnvelope(block string) bool {
	return jsonKeyRE.MatchString(block)
}

// escapeNewlinesInJSON escapes raw control characters (newline, carriage
// return, tab, backspace, form-feed) that appear inside JSON string literals
// so json.Unmarshal accepts models that emit pretty-printed code with
// literal newlines/tabs in their write_file content payloads. Control
// chars outside string literals are left untouched (json.Unmarshal accepts
// them at structural positions).
func escapeNewlinesInJSON(input string) string {
	var builder strings.Builder
	inString := false
	escaped := false

	for i := 0; i < len(input); i++ {
		char := input[i]

		if inString {
			if escaped {
				escaped = false
				builder.WriteByte(char)
				continue
			}
			switch char {
			case '\\':
				escaped = true
				builder.WriteByte(char)
				continue
			case '"':
				inString = false
				builder.WriteByte(char)
				continue
			case '\n':
				builder.WriteString(`\n`)
				continue
			case '\r':
				builder.WriteString(`\r`)
				continue
			case '\t':
				builder.WriteString(`\t`)
				continue
			case '\b':
				builder.WriteString(`\b`)
				continue
			case '\f':
				builder.WriteString(`\f`)
				continue
			}
			builder.WriteByte(char)
			continue
		}

		// Outside a string: track string opening.
		if char == '"' {
			inString = true
		}
		builder.WriteByte(char)
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
