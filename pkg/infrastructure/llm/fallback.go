package llm

import (
	"sort"
	"strings"
)

type ProviderModelInfo struct {
	Name    string
	Version float64
	Tier    string
	Rank    int
}

func sortProviderModels(models []*ProviderModelInfo) {
	sort.Slice(models, func(i, j int) bool {
		if models[i].Rank != models[j].Rank {
			return models[i].Rank > models[j].Rank
		}
		return models[i].Version > models[j].Version
	})
}

func normalizeModelName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimPrefix(n, "~")
	n = strings.TrimPrefix(n, "models/")
	return n
}

// selectFallbackModel selects a replacement model when a model call fails or returns an invalid model error.
//  1. If currentModel is recognized in parsedModels, it steps down the lower-model ladder (from currentModel down).
//  2. If currentModel is not found in parsedModels (e.g. invalid, mistyped, or unsupported model name):
//     a. Prefix match: searches for the first/best non-blacklisted model in parsedModels that starts with the
//     configured model name (e.g. "claude-3-7-sonnet" matches "claude-3-7-sonnet-20250219") or where the
//     configured model name starts with the catalog model name.
//     b. Best available model: if no prefix match is found, selects the highest-ranked non-blacklisted model in parsedModels.
func selectFallbackModel(configuredModel, currentModel string, parsedModels []*ProviderModelInfo) string {
	if len(parsedModels) == 0 {
		return ""
	}
	sortProviderModels(parsedModels)

	normCurrent := normalizeModelName(currentModel)
	normConfigured := normalizeModelName(configuredModel)

	// Case 1: If currentModel is recognized in parsedModels, step down the lower-model ladder.
	for i, m := range parsedModels {
		if normalizeModelName(m.Name) == normCurrent {
			for j := i + 1; j < len(parsedModels); j++ {
				if !IsModelBlacklisted(parsedModels[j].Name) {
					return parsedModels[j].Name
				}
			}
			// Current model is already the lowest recognized non-blacklisted model.
			return ""
		}
	}

	// Case 2: Model is not recognized in parsedModels (invalid/unsupported model name).
	// Step 2a: Prefix matching against the configured model name with delimiter boundary safety.
	if normConfigured != "" {
		for _, m := range parsedModels {
			if IsModelBlacklisted(m.Name) {
				continue
			}
			normM := normalizeModelName(m.Name)
			if normM == normCurrent {
				continue
			}
			if isModelPrefixMatch(m.Name, configuredModel) {
				return m.Name
			}
		}
	}

	// Step 2b: Fallback to the best (highest-ranked) available non-blacklisted model.
	for _, m := range parsedModels {
		if !IsModelBlacklisted(m.Name) && normalizeModelName(m.Name) != normCurrent {
			return m.Name
		}
	}

	return ""
}

// isModelPrefixMatch checks if catalogName and configuredName share a model lineage on clean delimiter boundaries.
func isModelPrefixMatch(catalogName, configuredName string) bool {
	normCat := normalizeModelName(catalogName)
	normCfg := normalizeModelName(configuredName)
	if normCat == "" || normCfg == "" {
		return false
	}
	if normCat == normCfg {
		return true
	}

	// Catalog model extends configured model (e.g. "claude-3-7-sonnet-20250219" for "claude-3-7-sonnet")
	if strings.HasPrefix(normCat, normCfg) {
		rem := normCat[len(normCfg):]
		if len(rem) > 0 && isModelDelimiter(rem[0]) {
			return true
		}
	}

	// Configured model extends catalog model (e.g. "gpt-4o-mini-preview" for "gpt-4o-mini")
	if strings.HasPrefix(normCfg, normCat) {
		rem := normCfg[len(normCat):]
		if len(rem) > 0 && isModelDelimiter(rem[0]) {
			return true
		}
	}

	return false
}

func isModelDelimiter(b byte) bool {
	return b == '-' || b == '.' || b == '_' || b == ':' || b == '/'
}

// selectLowerModelFromParsed selects the next lower model from a parsed and sorted slice of models.
func selectLowerModelFromParsed(currentModel string, parsedModels []*ProviderModelInfo) string {
	return selectFallbackModel(currentModel, currentModel, parsedModels)
}
