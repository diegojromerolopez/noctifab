package llm

import (
	"sort"
	"strings"
)

// PrioritizeCandidatesByModel reorders candidates so that any candidate matching
// targetModel (exact or substring) is placed first, while preserving the relative
// order of other fallback candidates.
func PrioritizeCandidatesByModel(candidates []RouterCandidate, targetModel string) []RouterCandidate {
	target := strings.ToLower(strings.TrimSpace(targetModel))
	if target == "" || len(candidates) <= 1 {
		return candidates
	}

	var matched []RouterCandidate
	var others []RouterCandidate

	for _, c := range candidates {
		cModel := strings.ToLower(strings.TrimSpace(c.Model))
		cName := strings.ToLower(strings.TrimSpace(c.Name))
		if cModel == target || cName == target || strings.Contains(cModel, target) || (cModel != "" && strings.Contains(target, cModel)) {
			matched = append(matched, c)
		} else {
			others = append(others, c)
		}
	}

	if len(matched) == 0 {
		return candidates
	}

	return append(matched, others...)
}

// ModelQualityScore returns a deterministic quality score (1..100) for a provider/model pair.
// Lower scores represent fast draft models; higher scores represent capable standard and frontier models.
func ModelQualityScore(provider, model string) int {
	m := strings.ToLower(strings.TrimSpace(model))
	p := strings.ToLower(strings.TrimSpace(provider))

	if p == "ensemble" || m == "adaptive" || m == "parallel" || m == "serial" || m == "consensus" || m == "race" || m == "cascade" {
		return 50
	}

	// Tier 3: Frontier / highest-quality models (85..98)
	switch {
	case strings.Contains(m, "opus"):
		return 98
	case strings.Contains(m, "sonnet"):
		return 95
	case strings.Contains(m, "gemini") && strings.Contains(m, "pro"):
		return 91
	case strings.Contains(m, "o3") || strings.Contains(m, "o1"):
		return 90
	case strings.Contains(m, "gpt-5") && !strings.Contains(m, "luna") && !strings.Contains(m, "mini") && !strings.Contains(m, "nano"):
		return 88
	case strings.Contains(m, "gpt-4o") && !strings.Contains(m, "mini"):
		return 85
	}

	// Tier 2: High / standard capability coding models (60..82)
	switch {
	case (strings.Contains(m, "deepseek") && (strings.Contains(m, "pro") || strings.Contains(m, "r1") || strings.Contains(m, "v3"))) && !strings.Contains(m, "flash"):
		return 82
	case strings.Contains(m, "qwen") && (strings.Contains(m, "max") || strings.Contains(m, "plus") || strings.Contains(m, "coder")):
		return 76
	case strings.Contains(m, "glm"):
		return 64
	}

	// Tier 1: Fast / lightweight draft models (25..48)
	switch {
	case strings.Contains(m, "flash-lite") || strings.Contains(m, "nano") || strings.Contains(m, "8b") || strings.Contains(m, "7b"):
		return 28
	case strings.Contains(m, "flash"):
		return 36
	case strings.Contains(m, "haiku"):
		return 42
	case strings.Contains(m, "mini"):
		return 44
	case strings.Contains(m, "luna"):
		return 46
	}

	// Fallback by provider when model string is generic
	switch p {
	case "anthropic", "claude":
		return 92
	case "gemini":
		return 80
	case "openai":
		return 78
	case "deepseek", "qwencloud":
		return 74
	default:
		return 55
	}
}

// PrioritizeCandidatesForQualityStage orders candidates along a progressive quality ladder:
//   - stage <= 0 (initial draft): orders direct model candidates by ascending quality (fast draft model first,
//     climbing up to higher-quality models on failover) so initial drafts are generated rapidly.
//   - stage == 1 (first refinement/improvement pass, e.g. refactor, implement_breadth_first_fix, fix, turn 1):
//     steps up past fast/draft models (score < 60) to standard and frontier quality models ordered by ascending quality.
//   - stage >= 2 (deep refinement, surgical_repair, turn >= 2, retries > 0):
//     orders all real model candidates strictly by descending quality (highest-quality frontier models first),
//     placing fast draft models and generic ensemble wrappers last.
func PrioritizeCandidatesForQualityStage(candidates []RouterCandidate, stage int) []RouterCandidate {
	if len(candidates) <= 1 {
		return candidates
	}

	var direct []RouterCandidate
	var ensembles []RouterCandidate
	for _, c := range candidates {
		if strings.EqualFold(c.Provider, "ensemble") {
			ensembles = append(ensembles, c)
		} else {
			direct = append(direct, c)
		}
	}
	if len(direct) == 0 {
		return candidates
	}

	if stage <= 0 {
		sortedAsc := make([]RouterCandidate, len(direct))
		copy(sortedAsc, direct)
		sort.SliceStable(sortedAsc, func(i, j int) bool {
			return ModelQualityScore(sortedAsc[i].Provider, sortedAsc[i].Model) < ModelQualityScore(sortedAsc[j].Provider, sortedAsc[j].Model)
		})
		res := make([]RouterCandidate, 0, len(candidates))
		res = append(res, sortedAsc...)
		res = append(res, ensembles...)
		return res
	}

	if stage == 1 {
		// Step 1 improvement: promote high-quality & frontier models (score >= 70) ordered by descending quality
		// so that refactor / implement_breadth_first_fix / Turn 1 immediately improves the draft with top models.
		var upgraded []RouterCandidate
		var draftFallback []RouterCandidate
		for _, c := range direct {
			if ModelQualityScore(c.Provider, c.Model) >= 70 {
				upgraded = append(upgraded, c)
			} else {
				draftFallback = append(draftFallback, c)
			}
		}
		sort.SliceStable(upgraded, func(i, j int) bool {
			return ModelQualityScore(upgraded[i].Provider, upgraded[i].Model) > ModelQualityScore(upgraded[j].Provider, upgraded[j].Model)
		})
		sort.SliceStable(draftFallback, func(i, j int) bool {
			return ModelQualityScore(draftFallback[i].Provider, draftFallback[i].Model) > ModelQualityScore(draftFallback[j].Provider, draftFallback[j].Model)
		})
		res := make([]RouterCandidate, 0, len(candidates))
		res = append(res, upgraded...)
		res = append(res, ensembles...)
		res = append(res, draftFallback...)
		return res
	}

	// Stage >= 2: Strict highest-quality frontier models first (descending quality score)
	sortedDesc := make([]RouterCandidate, len(direct))
	copy(sortedDesc, direct)
	sort.SliceStable(sortedDesc, func(i, j int) bool {
		return ModelQualityScore(sortedDesc[i].Provider, sortedDesc[i].Model) > ModelQualityScore(sortedDesc[j].Provider, sortedDesc[j].Model)
	})
	res := make([]RouterCandidate, 0, len(candidates))
	res = append(res, sortedDesc...)
	res = append(res, ensembles...)
	return res
}
