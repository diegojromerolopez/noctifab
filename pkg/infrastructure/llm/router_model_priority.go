package llm

import "strings"

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
		if cModel == target || strings.Contains(cModel, target) || strings.Contains(target, cModel) {
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
