package ensemble

import (
	"go/parser"
	"go/token"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

var (
	stubRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(TODO|FIXME|XXX|unimplemented)\b`),
		regexp.MustCompile(`panic\s*\(\s*["'](not implemented|unimplemented|TODO)["']\s*\)`),
		regexp.MustCompile(`(?m)^\s*pass\s*$`),
		regexp.MustCompile(`(?m)^\s*//\s*\.\.\.\s*(rest of (code|implementation|file)|remaining code)\s*(\.\.\.)?\s*$`),
	}
)

// ValidateCodeResponse checks if the LLMResponse contains valid, non-stub actions.
func ValidateCodeResponse(resp *domain.LLMResponse) (bool, string) {
	if resp == nil {
		return false, "response is nil"
	}
	if len(resp.Actions) == 0 {
		return false, "no actions returned"
	}

	for _, act := range resp.Actions {
		if act.Tool == "write_file" || act.Tool == "create_story" || act.Tool == "create_file" {
			path, _ := act.Args["path"].(string)
			if path == "" {
				path, _ = act.Args["filename"].(string)
			}
			content, _ := act.Args["content"].(string)
			if strings.TrimSpace(content) == "" {
				return false, "empty file content for " + path
			}

			// Check file size constraint (max 500 lines)
			lines := strings.Split(content, "\n")
			if len(lines) > 500 {
				return false, "file exceeds 500 line limit (" + path + ")"
			}

			// Check for blatant stubs
			for _, re := range stubRegexes {
				if re.MatchString(content) {
					return false, "stub pattern detected in " + path + ": " + re.String()
				}
			}

			// If Go file, check AST parseability
			if strings.HasSuffix(path, ".go") {
				fset := token.NewFileSet()
				if _, err := parser.ParseFile(fset, path, content, parser.AllErrors); err != nil {
					return false, "go syntax error in " + path + ": " + err.Error()
				}
			}
		}
	}

	return true, ""
}

// ScoreCodeResponse computes a deterministic quality score for a code completion.
func ScoreCodeResponse(resp *domain.LLMResponse) int {
	if resp == nil || len(resp.Actions) == 0 {
		return -1000
	}

	score := 100
	for _, act := range resp.Actions {
		if act.Tool == "write_file" || act.Tool == "create_story" || act.Tool == "create_file" {
			path, _ := act.Args["path"].(string)
			if path == "" {
				path, _ = act.Args["filename"].(string)
			}
			content, _ := act.Args["content"].(string)
			if strings.TrimSpace(content) == "" {
				score -= 200
				continue
			}

			lines := strings.Split(content, "\n")
			if len(lines) > 500 {
				score -= 100
			} else {
				score += 20 // Reward valid file creation
			}

			for _, re := range stubRegexes {
				if re.MatchString(content) {
					score -= 50
				}
			}

			if strings.HasSuffix(path, ".go") {
				fset := token.NewFileSet()
				if _, err := parser.ParseFile(fset, path, content, 0); err == nil {
					score += 50 // Reward clean Go AST
				} else {
					score -= 150
				}
			}

			// Reward test assertions
			if strings.Contains(path, "test") || strings.Contains(path, "spec") {
				assertions := strings.Count(content, "assert") + strings.Count(content, "require") + strings.Count(content, "t.Error") + strings.Count(content, "t.Fatal")
				score += assertions * 5
			}
		}
	}

	return score
}

// CombineUsage sums token usage from multiple completions.
func CombineUsage(usages ...domain.TokenUsage) domain.TokenUsage {
	var total domain.TokenUsage
	for _, u := range usages {
		total.InputTokens += u.InputTokens
		total.OutputTokens += u.OutputTokens
		total.ReasoningTokens += u.ReasoningTokens
		total.CachedTokens += u.CachedTokens
		total.TotalTokens += u.TotalTokens
	}
	return total
}

// MergeActions combines and deduplicates action lists by target file/tool.
func MergeActions(actionsLists ...[]domain.LLMAction) []domain.LLMAction {
	seen := make(map[string]bool)
	var merged []domain.LLMAction

	for _, list := range actionsLists {
		for _, act := range list {
			key := act.Tool
			if path, ok := act.Args["path"].(string); ok && path != "" {
				key += ":" + path
			} else if fn, ok := act.Args["filename"].(string); ok && fn != "" {
				key += ":" + fn
			}
			if !seen[key] {
				seen[key] = true
				merged = append(merged, act)
			}
		}
	}
	return merged
}
