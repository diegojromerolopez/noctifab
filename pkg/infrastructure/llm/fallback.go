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
			if i+1 < len(parsedModels) {
				return parsedModels[i+1].Name
			}
			// Current model is already the lowest-ranked — no further fallback.
			return ""
		}
	}

	// The current model was not found in the parsed list (unrecognised model name).
	// As a fault-tolerant safety valve, return the lowest-ranked model from the
	// parsed list so that execution can continue rather than failing immediately.
	return parsedModels[len(parsedModels)-1].Name
}

// normalizeModelID strips provider routing prefixes (e.g. OpenRouter's `~`
// pin marker or Gemini's `models/`) and lowercases a model identifier so
// alias-family matching is robust across providers.
func normalizeModelID(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.TrimPrefix(n, "~")
	n = strings.TrimPrefix(n, "models/")
	return n
}

// isMovingAlias reports whether a model identifier is a provider-managed
// "latest" pointer rather than a concrete, pinned model. OpenRouter exposes
// auto-updating aliases prefixed with `~` (e.g. `~deepseek/deepseek-v4-flash-latest`)
// that route to variable upstream providers; these must never be selected as
// the resolved target for a `*-latest` alias because their behaviour is
// non-deterministic. Concrete pinned snapshots (e.g. `deepseek/deepseek-v4-flash-0731`)
// are preferred.
func isMovingAlias(name string) bool {
	raw := strings.TrimSpace(name)
	if strings.HasPrefix(raw, "~") {
		return true
	}
	norm := normalizeModelID(raw)
	if norm == "latest" || strings.HasSuffix(norm, "-latest") {
		return true
	}
	return false
}

// filterModelsForAlias returns the subset of parsed models that belong to the
// same family as a `*-latest`/`auto` alias. A model belongs to the family when
// its normalized identifier equals the alias or shares the alias's base prefix
// (e.g. alias `deepseek/deepseek-v4-flash-latest` matches
// `deepseek/deepseek-v4-flash` and `deepseek/deepseek-v4-flash-0731`).
// Moving aliases (`~`-prefixed or `-latest`-suffixed) are excluded so the
// resolution lands on a concrete pinned model. Base-name matches
// (e.g. `deepseek/deepseek-v4-flash`) are ranked ahead of suffixed variants so
// the most canonical member wins deterministically even when rank/version tie.
// It returns an empty slice when no parsed model is in the family.
func filterModelsForAlias(alias string, parsedModels []*ProviderModelInfo) []*ProviderModelInfo {
	normAlias := normalizeModelID(alias)
	base := strings.TrimSuffix(normAlias, "-latest")
	var family []*ProviderModelInfo
	for _, m := range parsedModels {
		if isMovingAlias(m.Name) {
			continue
		}
		norm := normalizeModelID(m.Name)
		if norm == normAlias || strings.HasPrefix(norm, base) {
			family = append(family, m)
		}
	}
	sort.SliceStable(family, func(i, j int) bool {
		ni, nj := normalizeModelID(family[i].Name), normalizeModelID(family[j].Name)
		// Exact alias first, then more specific (dated/pinned) members, then
		// the bare base name. On OpenRouter the bare base name (e.g.
		// `deepseek/deepseek-v4-flash`) is itself a moving target that routes
		// to whatever version is current, while dated snapshots
		// (e.g. `-0731`) are stable — prefer the specific one.
		score := func(n string) int {
			switch n {
			case normAlias:
				return 4
			case base:
				return 2
			default:
				return 3
			}
		}
		if si, sj := score(ni), score(nj); si != sj {
			return si > sj
		}
		return family[i].Name < family[j].Name
	})
	return family
}
