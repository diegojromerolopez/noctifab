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

// selectLowerModelFromParsed selects the next lower model from a parsed and sorted slice of models.
// If the currentModel is found in parsedModels, it returns the next lower model in the sorted list.
// If the currentModel is NOT found (e.g. it is a newly released model the parser does not yet recognise),
// it falls back to returning the lowest-ranked model still available, ensuring the fallback
// engine always has a safe candidate rather than surfacing an error prematurely.
func selectLowerModelFromParsed(currentModel string, parsedModels []*ProviderModelInfo) string {
	if len(parsedModels) == 0 {
		return ""
	}
	sortProviderModels(parsedModels)
	normCurrent := strings.TrimPrefix(strings.ToLower(currentModel), "models/")

	for i, m := range parsedModels {
		normM := strings.TrimPrefix(strings.ToLower(m.Name), "models/")
		if normM == normCurrent {
			for j := i + 1; j < len(parsedModels); j++ {
				if !IsModelBlacklisted(parsedModels[j].Name) {
					return parsedModels[j].Name
				}
			}
			// Current model is already the lowest non-blacklisted model — no further fallback.
			return ""
		}
	}

	// The current model was not found in the parsed list (unrecognised model name).
	// As a fault-tolerant safety valve, return the lowest non-blacklisted model.
	for j := len(parsedModels) - 1; j >= 0; j-- {
		if !IsModelBlacklisted(parsedModels[j].Name) && strings.TrimPrefix(strings.ToLower(parsedModels[j].Name), "models/") != normCurrent {
			return parsedModels[j].Name
		}
	}
	return ""
}
