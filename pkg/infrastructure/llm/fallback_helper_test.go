package llm

// parsedModelsFor is a test helper that runs a parser over a slice of model
// name strings and returns only successfully parsed results.
func parsedModelsFor(names []string, parser func(string) (*ProviderModelInfo, bool)) []*ProviderModelInfo {
	var out []*ProviderModelInfo
	for _, n := range names {
		if info, ok := parser(n); ok && info != nil {
			out = append(out, info)
		}
	}
	return out
}
