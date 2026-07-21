package llm

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type GeminiModelInfo struct {
	Name    string
	Version float64
	Tier    string
	Rank    int
}

func parseGeminiModel(name string) (*GeminiModelInfo, bool) {
	norm := strings.TrimPrefix(strings.ToLower(name), "models/")
	if !strings.HasPrefix(norm, "gemini") && !strings.Contains(norm, "nano") {
		return nil, false
	}

	var tier string
	var rank int
	if strings.Contains(norm, "nano") {
		tier = "nano"
		rank = 1
	} else if strings.Contains(norm, "flash-lite") || strings.Contains(norm, "flash_lite") {
		tier = "flash-lite"
		rank = 2
	} else if strings.Contains(norm, "flash") {
		tier = "flash"
		rank = 3
	} else if strings.Contains(norm, "pro") {
		tier = "pro"
		rank = 4
	} else {
		return nil, false
	}

	version := 1.5
	re := regexp.MustCompile(`gemini-([0-9]+(?:\.[0-9]+)?)`)
	matches := re.FindStringSubmatch(norm)
	if len(matches) > 1 {
		if v, err := strconv.ParseFloat(matches[1], 64); err == nil {
			version = v
		}
	} else {
		reBroad := regexp.MustCompile(`([0-9]+\.[0-9]+)`)
		broadMatches := reBroad.FindStringSubmatch(norm)
		if len(broadMatches) > 1 {
			if v, err := strconv.ParseFloat(broadMatches[1], 64); err == nil {
				version = v
			}
		}
	}

	return &GeminiModelInfo{
		Name:    name,
		Version: version,
		Tier:    tier,
		Rank:    rank,
	}, true
}

func sortGeminiModels(models []*GeminiModelInfo) {
	sort.Slice(models, func(i, j int) bool {
		if models[i].Version != models[j].Version {
			return models[i].Version > models[j].Version
		}
		return models[i].Rank > models[j].Rank
	})
}

var modelHierarchy = map[string][]string{
	"openai": {
		"gpt-4o",
		"gpt-4o-mini",
	},
	"mistral": {
		"mistral-large-latest",
		"mistral-medium-latest",
		"mistral-small-latest",
		"open-mistral-7b",
	},
	"deepseek": {
		"deepseek-coder",
		"deepseek-chat",
	},
	"hermes": {
		"hermes-3-llama-3.1-405b",
		"hermes-3-llama-3.1-70b",
		"hermes-3-llama-3.1-8b",
	},
	"anthropic": {
		"claude-3-5-sonnet-latest",
		"claude-3-5-haiku-latest",
	},
	"opencode": {
		"glm-5.2",
		"glm-5.1",
		"kimi-k2.7-code",
		"kimi-k2.6",
		"qwen3.7-max",
		"qwen3.7-plus",
		"minimax-m3",
		"minimax-m2.7",
		"qwen3.6-plus",
		"mimo-v2.5-pro",
		"deepseek-v4-pro",
		"mimo-v2.5",
		"deepseek-v4-flash",
	},
}

func normalizeModel(model string) string {
	return strings.TrimPrefix(strings.ToLower(model), "models/")
}

func resolveGeminiURL(modelInput, apiKey string) string {
	normModel := normalizeModel(modelInput)
	return fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", normModel, apiKey)
}
