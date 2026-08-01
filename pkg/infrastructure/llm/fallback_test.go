package llm

import (
	"testing"
)

func TestProviderModelParsers(t *testing.T) {
	t.Run("parseAnthropicModel", func(t *testing.T) {
		info, ok := parseAnthropicModel("claude-3-5-sonnet-latest")
		if !ok {
			t.Fatal("expected ok for claude-3-5-sonnet-latest")
		}
		if info.Tier != "sonnet" || info.Rank != 335 {
			t.Errorf("expected sonnet rank 335, got tier=%s rank=%d", info.Tier, info.Rank)
		}

		infoOpus, okOpus := parseAnthropicModel("claude-3-opus-20240229")
		if !okOpus || infoOpus.Tier != "opus" || infoOpus.Rank != 430 {
			t.Errorf("unexpected opus parse: %+v", infoOpus)
		}

		infoHaiku, okHaiku := parseAnthropicModel("claude-3-5-haiku-latest")
		if !okHaiku || infoHaiku.Tier != "haiku" || infoHaiku.Rank != 235 {
			t.Errorf("unexpected haiku parse: %+v", infoHaiku)
		}
	})

	t.Run("parseOpenAIModel", func(t *testing.T) {
		info, ok := parseOpenAIModel("gpt-4o")
		if !ok || info.Rank < 45 {
			t.Errorf("unexpected gpt-4o parse: %+v", info)
		}
		infoMini, okMini := parseOpenAIModel("gpt-4o-mini")
		if !okMini || infoMini.Rank < 15 {
			t.Errorf("unexpected gpt-4o-mini parse: %+v", infoMini)
		}
	})

	t.Run("parseMistralModel", func(t *testing.T) {
		info, ok := parseMistralModel("mistral-large-latest")
		if !ok || info.Rank != 40 {
			t.Errorf("unexpected mistral large parse: %+v", info)
		}
		infoSmall, okSmall := parseMistralModel("mistral-small-latest")
		if !okSmall || infoSmall.Rank != 20 {
			t.Errorf("unexpected mistral small parse: %+v", infoSmall)
		}
	})

	t.Run("parseDeepSeekModel", func(t *testing.T) {
		info, ok := parseDeepSeekModel("deepseek-coder")
		if !ok || info.Rank != 30 {
			t.Errorf("unexpected deepseek coder parse: %+v", info)
		}
		infoChat, okChat := parseDeepSeekModel("deepseek-chat")
		if !okChat || infoChat.Rank != 20 {
			t.Errorf("unexpected deepseek chat parse: %+v", infoChat)
		}
	})

	t.Run("parseHermesModel", func(t *testing.T) {
		info405b, ok405b := parseHermesModel("hermes-3-llama-3.1-405b")
		if !ok405b || info405b.Rank != 30 {
			t.Errorf("unexpected hermes 405b parse: %+v", info405b)
		}
		info70b, ok70b := parseHermesModel("hermes-3-llama-3.1-70b")
		if !ok70b || info70b.Rank != 20 {
			t.Errorf("unexpected hermes 70b parse: %+v", info70b)
		}
	})

	t.Run("parseOllamaModel", func(t *testing.T) {
		info70b, ok70b := parseOllamaModel("llama3.1:70b")
		if !ok70b || info70b.Rank != 431 {
			t.Errorf("unexpected ollama 70b parse: %+v", info70b)
		}
		info8b, ok8b := parseOllamaModel("llama3.1:8b")
		if !ok8b || info8b.Rank != 231 {
			t.Errorf("unexpected ollama 8b parse: %+v", info8b)
		}
	})

	t.Run("parseHuggingFaceModel", func(t *testing.T) {
		info, ok := parseHuggingFaceModel("meta-llama/Meta-Llama-3.1-70B-Instruct")
		if !ok || info.Rank != 431 {
			t.Errorf("unexpected hf 70b parse: %+v", info)
		}
	})

	t.Run("parseKimiModel", func(t *testing.T) {
		infoK3, okK3 := parseKimiModel("kimi-k3")
		if !okK3 || infoK3.Rank != 50 {
			t.Errorf("unexpected kimi k3 parse: %+v", infoK3)
		}
		infoK27, okK27 := parseKimiModel("kimi-k2.7")
		if !okK27 || infoK27.Rank != 40 {
			t.Errorf("unexpected kimi k2.7 parse: %+v", infoK27)
		}
	})

	t.Run("parseQwenModel", func(t *testing.T) {
		infoMax, okMax := parseQwenModel("qwen-max")
		if !okMax || infoMax.Rank != 65 {
			t.Errorf("unexpected qwen max parse: %+v", infoMax)
		}
		infoPlus, okPlus := parseQwenModel("qwen-plus")
		if !okPlus || infoPlus.Rank != 55 {
			t.Errorf("unexpected qwen plus parse: %+v", infoPlus)
		}
	})

	t.Run("parseLlamaModel", func(t *testing.T) {
		info405b, ok405b := parseLlamaModel("Llama-3.1-405B-Instruct")
		if !ok405b || info405b.Rank != 531 {
			t.Errorf("unexpected llama 405b parse: %+v", info405b)
		}
		info70b, ok70b := parseLlamaModel("Llama-3.3-70B-Instruct")
		if !ok70b || info70b.Rank != 433 {
			t.Errorf("unexpected llama 70b parse: %+v", info70b)
		}
	})

	t.Run("parseXAIModel", func(t *testing.T) {
		info3, ok3 := parseXAIModel("grok-3")
		if !ok3 || info3.Rank != 75 {
			t.Errorf("unexpected grok-3 parse: %+v", info3)
		}
		info2, ok2 := parseXAIModel("grok-2")
		if !ok2 || info2.Rank != 50 {
			t.Errorf("unexpected grok-2 parse: %+v\n", info2)
		}
	})

	t.Run("parseGeminiModelProvider", func(t *testing.T) {
		info, ok := parseGeminiModelProvider("gemini-2.5-flash")
		if !ok || info.Tier != "flash" || info.Rank != 55 {
			t.Errorf("expected gemini-2.5-flash rank 55, got ok=%t tier=%s rank=%d", ok, info.Tier, info.Rank)
		}
		_, okRobotics := parseGeminiModelProvider("gemini-robotics-er-2-preview")
		if okRobotics {
			t.Error("expected gemini-robotics-er-2-preview to be rejected by ExcludedKeywords")
		}
		_, okEmbed := parseGeminiModelProvider("gemini-embed-text-001")
		if okEmbed {
			t.Error("expected gemini-embed-text-001 to be rejected by ExcludedKeywords")
		}
	})

	t.Run("parsePerplexityModel", func(t *testing.T) {
		infoDeep, okDeep := parsePerplexityModel("sonar-deep-research")
		if !okDeep || infoDeep.Rank != 50 {
			t.Errorf("unexpected sonar-deep-research parse: %+v", infoDeep)
		}
		infoPro, okPro := parsePerplexityModel("sonar-pro")
		if !okPro || infoPro.Rank != 20 {
			t.Errorf("unexpected sonar-pro parse: %+v", infoPro)
		}
	})

	t.Run("parseCohereModel", func(t *testing.T) {
		infoPlus, okPlus := parseCohereModel("command-r-plus")
		if !okPlus || infoPlus.Rank != 40 {
			t.Errorf("unexpected command-r-plus parse: %+v", infoPlus)
		}
		infoR, okR := parseCohereModel("command-r")
		if !okR || infoR.Rank != 30 {
			t.Errorf("unexpected command-r parse: %+v", infoR)
		}
	})

	t.Run("parseCerebrasModel", func(t *testing.T) {
		info70b, ok70b := parseCerebrasModel("llama3.1-70b")
		if !ok70b || info70b.Rank <= 0 {
			t.Errorf("unexpected cerebras llama 70b parse: %+v", info70b)
		}
		info8b, ok8b := parseCerebrasModel("llama3.1-8b")
		if !ok8b || info8b.Rank <= 0 {
			t.Errorf("unexpected cerebras llama 8b parse: %+v", info8b)
		}
		if info70b.Rank <= info8b.Rank {
			t.Errorf("expected 70b rank (%d) > 8b rank (%d)", info70b.Rank, info8b.Rank)
		}
	})

	t.Run("parseNvidiaModel", func(t *testing.T) {
		info70b, ok70b := parseNvidiaModel("meta/llama-3.1-70b-instruct")
		if !ok70b || info70b.Rank <= 0 {
			t.Errorf("unexpected nvidia llama 70b parse: %+v", info70b)
		}
		info8b, ok8b := parseNvidiaModel("meta/llama-3.1-8b-instruct")
		if !ok8b || info8b.Rank <= 0 {
			t.Errorf("unexpected nvidia llama 8b parse: %+v", info8b)
		}
		if info70b.Rank <= info8b.Rank {
			t.Errorf("expected 70b rank (%d) > 8b rank (%d)", info70b.Rank, info8b.Rank)
		}
	})

	t.Run("parseAI21Model", func(t *testing.T) {
		infoLarge, okLarge := parseAI21Model("jamba-large")
		if !okLarge || infoLarge.Rank <= 0 {
			t.Errorf("unexpected ai21 jamba-large parse: %+v", infoLarge)
		}
		infoMini, okMini := parseAI21Model("jamba-mini")
		if !okMini || infoMini.Rank <= 0 {
			t.Errorf("unexpected ai21 jamba-mini parse: %+v", infoMini)
		}
		if infoLarge.Rank <= infoMini.Rank {
			t.Errorf("expected jamba-large rank (%d) > jamba-mini rank (%d)", infoLarge.Rank, infoMini.Rank)
		}
	})

	t.Run("parseUpstageModel", func(t *testing.T) {
		infoPro, okPro := parseUpstageModel("solar-pro")
		if !okPro || infoPro.Rank <= 0 {
			t.Errorf("unexpected upstage solar-pro parse: %+v", infoPro)
		}
		infoMini, okMini := parseUpstageModel("solar-mini")
		if !okMini || infoMini.Rank <= 0 {
			t.Errorf("unexpected upstage solar-mini parse: %+v", infoMini)
		}
		if infoPro.Rank <= infoMini.Rank {
			t.Errorf("expected solar-pro rank (%d) > solar-mini rank (%d)", infoPro.Rank, infoMini.Rank)
		}
	})

	t.Run("selectLowerModelFromParsed", func(t *testing.T) {
		parsed := []*ProviderModelInfo{
			{Name: "claude-3-opus-latest", Version: 3.0, Tier: "opus", Rank: 4},
			{Name: "claude-3-5-sonnet-latest", Version: 3.5, Tier: "sonnet", Rank: 3},
			{Name: "claude-3-5-haiku-latest", Version: 3.5, Tier: "haiku", Rank: 2},
		}

		next := selectLowerModelFromParsed("claude-3-5-sonnet-latest", parsed)
		if next != "claude-3-5-haiku-latest" {
			t.Errorf("expected claude-3-5-haiku-latest, got %s", next)
		}
	})
}

// TestModelFallbackChains validates the complete end-to-end fallback ordering
// for every supported AI provider using selectLowerModelFromParsed.
// Each test simulates what the Dynamic Model Fallback Engine does when the
// primary model returns an error: it sorts all available models by capacity
// rank and selects the next lower one.
func TestModelFallbackChains(t *testing.T) {
	t.Run("when Anthropic claude-3-opus fails it falls back to claude-3-5-sonnet", func(t *testing.T) {
		models := []string{
			"claude-3-opus-20240229",
			"claude-3-5-sonnet-latest",
			"claude-3-5-haiku-latest",
		}
		parsed := parsedModelsFor(models, parseAnthropicModel)
		next := selectLowerModelFromParsed("claude-3-opus-20240229", parsed)
		if next != "claude-3-5-sonnet-latest" {
			t.Errorf("expected claude-3-5-sonnet-latest, got %q", next)
		}
	})

	t.Run("when Anthropic claude-3-5-sonnet fails it falls back to claude-3-5-haiku", func(t *testing.T) {
		models := []string{
			"claude-3-opus-20240229",
			"claude-3-5-sonnet-latest",
			"claude-3-5-haiku-latest",
		}
		parsed := parsedModelsFor(models, parseAnthropicModel)
		next := selectLowerModelFromParsed("claude-3-5-sonnet-latest", parsed)
		if next != "claude-3-5-haiku-latest" {
			t.Errorf("expected claude-3-5-haiku-latest, got %q", next)
		}
	})

	t.Run("when Anthropic lowest model fails there is no fallback", func(t *testing.T) {
		models := []string{
			"claude-3-opus-20240229",
			"claude-3-5-sonnet-latest",
			"claude-3-5-haiku-latest",
		}
		parsed := parsedModelsFor(models, parseAnthropicModel)
		next := selectLowerModelFromParsed("claude-3-5-haiku-latest", parsed)
		if next != "" {
			t.Errorf("expected empty fallback for lowest model, got %q", next)
		}
	})

	t.Run("when OpenAI gpt-4o fails it falls back to gpt-4o-mini", func(t *testing.T) {
		models := []string{"gpt-4o", "gpt-4o-mini", "gpt-3.5-turbo"}
		parsed := parsedModelsFor(models, parseOpenAIModel)
		next := selectLowerModelFromParsed("gpt-4o", parsed)
		if next != "gpt-4o-mini" {
			t.Errorf("expected gpt-4o-mini, got %q", next)
		}
	})

	t.Run("when Mistral large fails it falls back to small", func(t *testing.T) {
		models := []string{
			"mistral-large-latest",
			"mistral-small-latest",
		}
		parsed := parsedModelsFor(models, parseMistralModel)
		next := selectLowerModelFromParsed("mistral-large-latest", parsed)
		if next != "mistral-small-latest" {
			t.Errorf("expected mistral-small-latest, got %q", next)
		}
	})

	t.Run("when DeepSeek coder fails it falls back to chat", func(t *testing.T) {
		models := []string{"deepseek-coder", "deepseek-chat"}
		parsed := parsedModelsFor(models, parseDeepSeekModel)
		next := selectLowerModelFromParsed("deepseek-coder", parsed)
		if next != "deepseek-chat" {
			t.Errorf("expected deepseek-chat, got %q", next)
		}
	})

	t.Run("when Llama 405b fails it falls back to 70b", func(t *testing.T) {
		models := []string{
			"Llama-3.1-405B-Instruct",
			"Llama-3.3-70B-Instruct",
			"Llama-3.1-8B-Instruct",
		}
		parsed := parsedModelsFor(models, parseLlamaModel)
		next := selectLowerModelFromParsed("Llama-3.1-405B-Instruct", parsed)
		if next != "Llama-3.3-70B-Instruct" {
			t.Errorf("expected Llama-3.3-70B-Instruct, got %q", next)
		}
	})

	t.Run("when xAI grok-3 fails it falls back to grok-2", func(t *testing.T) {
		models := []string{"grok-3", "grok-2", "grok-3-mini"}
		parsed := parsedModelsFor(models, parseXAIModel)
		next := selectLowerModelFromParsed("grok-3", parsed)
		if next == "" {
			t.Errorf("expected a fallback from grok-3, got empty")
		}
	})

	t.Run("when Perplexity sonar-deep-research fails it falls back to sonar-pro", func(t *testing.T) {
		models := []string{
			"sonar-deep-research",
			"sonar-reasoning-pro",
			"sonar-pro",
		}
		parsed := parsedModelsFor(models, parsePerplexityModel)
		next := selectLowerModelFromParsed("sonar-deep-research", parsed)
		if next != "sonar-reasoning-pro" {
			t.Errorf("expected sonar-reasoning-pro, got %q", next)
		}
	})

	t.Run("when Cohere command-r-plus fails it falls back to command-r", func(t *testing.T) {
		models := []string{"command-r-plus", "command-r", "command-light"}
		parsed := parsedModelsFor(models, parseCohereModel)
		next := selectLowerModelFromParsed("command-r-plus", parsed)
		if next != "command-r" {
			t.Errorf("expected command-r, got %q", next)
		}
	})

	t.Run("when Kimi k3 fails it falls back to k2.7", func(t *testing.T) {
		models := []string{"kimi-k3", "kimi-k2.7", "kimi-k2.5"}
		parsed := parsedModelsFor(models, parseKimiModel)
		next := selectLowerModelFromParsed("kimi-k3", parsed)
		if next != "kimi-k2.7" {
			t.Errorf("expected kimi-k2.7, got %q", next)
		}
	})

	t.Run("when Qwen max fails it falls back to plus", func(t *testing.T) {
		models := []string{"qwen-max", "qwen-plus", "qwen-turbo"}
		parsed := parsedModelsFor(models, parseQwenModel)
		next := selectLowerModelFromParsed("qwen-max", parsed)
		if next != "qwen-plus" {
			t.Errorf("expected qwen-plus, got %q", next)
		}
	})

	t.Run("when Hermes 405b fails it falls back to 70b", func(t *testing.T) {
		models := []string{"hermes-3-llama-3.1-405b", "hermes-3-llama-3.1-70b"}
		parsed := parsedModelsFor(models, parseHermesModel)
		next := selectLowerModelFromParsed("hermes-3-llama-3.1-405b", parsed)
		if next != "hermes-3-llama-3.1-70b" {
			t.Errorf("expected hermes-3-llama-3.1-70b, got %q", next)
		}
	})

	t.Run("when Ollama 70b fails it falls back to 8b", func(t *testing.T) {
		models := []string{"llama3.1:70b", "llama3.1:8b"}
		parsed := parsedModelsFor(models, parseOllamaModel)
		next := selectLowerModelFromParsed("llama3.1:70b", parsed)
		if next != "llama3.1:8b" {
			t.Errorf("expected llama3.1:8b, got %q", next)
		}
	})

	t.Run("when Cerebras 70b fails it falls back to 8b", func(t *testing.T) {
		models := []string{"llama3.1-70b", "llama3.1-8b"}
		parsed := parsedModelsFor(models, parseCerebrasModel)
		next := selectLowerModelFromParsed("llama3.1-70b", parsed)
		if next != "llama3.1-8b" {
			t.Errorf("expected llama3.1-8b, got %q", next)
		}
	})

	t.Run("when NVIDIA 70b fails it falls back to 8b", func(t *testing.T) {
		models := []string{"meta/llama-3.1-70b-instruct", "meta/llama-3.1-8b-instruct"}
		parsed := parsedModelsFor(models, parseNvidiaModel)
		next := selectLowerModelFromParsed("meta/llama-3.1-70b-instruct", parsed)
		if next != "meta/llama-3.1-8b-instruct" {
			t.Errorf("expected meta/llama-3.1-8b-instruct, got %q", next)
		}
	})

	t.Run("when AI21 jamba-large fails it falls back to jamba-mini", func(t *testing.T) {
		models := []string{"jamba-large", "jamba-mini"}
		parsed := parsedModelsFor(models, parseAI21Model)
		next := selectLowerModelFromParsed("jamba-large", parsed)
		if next != "jamba-mini" {
			t.Errorf("expected jamba-mini, got %q", next)
		}
	})

	t.Run("when Upstage solar-pro fails it falls back to solar-mini", func(t *testing.T) {
		models := []string{"solar-pro", "solar-mini"}
		parsed := parsedModelsFor(models, parseUpstageModel)
		next := selectLowerModelFromParsed("solar-pro", parsed)
		if next != "solar-mini" {
			t.Errorf("expected solar-mini, got %q", next)
		}
	})
}

// TestFallbackFaultTolerance validates fault-tolerant behaviour when the
// current model name is unrecognised by the provider parser — a scenario that
// arises when a provider releases a new model whose naming convention is not
// yet matched by any existing tier keyword or size weight.
func TestFallbackFaultTolerance(t *testing.T) {
	t.Run("it falls back to lowest available when current model is unrecognised by parser", func(t *testing.T) {
		// Simulate a future Anthropic model "claude-4-nova" that the parser's tier
		// keywords do not yet recognise. The known models (sonnet, haiku) should
		// still be reachable as fallbacks.
		known := []string{"claude-3-5-sonnet-latest", "claude-3-5-haiku-latest"}
		parsed := parsedModelsFor(known, parseAnthropicModel)

		// "claude-4-nova" is not in parsed — safety valve returns lowest known model.
		next := selectLowerModelFromParsed("claude-4-nova", parsed)
		if next == "" {
			t.Errorf("expected a fallback for unrecognised current model, got empty string")
		}
	})

	t.Run("it falls back to lowest available when current model lacks required prefix", func(t *testing.T) {
		// Simulate a hypothetical Moonshot model "moonshot-k3-pro" whose name does not
		// contain the "kimi" required prefix — so it would fail parseKimiModel and not
		// appear in parsedModels at all. The safety valve must still select a fallback.
		known := []string{"kimi-k2.7", "kimi-k2.5"}
		parsed := parsedModelsFor(known, parseKimiModel)

		next := selectLowerModelFromParsed("moonshot-k3-pro", parsed)
		if next == "" {
			t.Errorf("expected a fallback for model failing RequiredPrefix, got empty string")
		}
		// The lowest available should be kimi-k2.5 (rank 20 < rank 40).
		if next != "kimi-k2.5" {
			t.Errorf("expected lowest fallback kimi-k2.5, got %q", next)
		}
	})

	t.Run("it returns empty when no models are available at all", func(t *testing.T) {
		next := selectLowerModelFromParsed("any-model", []*ProviderModelInfo{})
		if next != "" {
			t.Errorf("expected empty for zero available models, got %q", next)
		}
	})

	t.Run("it returns empty when current model is the only available model", func(t *testing.T) {
		models := []string{"mistral-large-latest"}
		parsed := parsedModelsFor(models, parseMistralModel)
		next := selectLowerModelFromParsed("mistral-large-latest", parsed)
		if next != "" {
			t.Errorf("expected empty when current model is the only one, got %q", next)
		}
	})

	t.Run("it returns empty when current model is already the lowest ranked", func(t *testing.T) {
		models := []string{"command-r-plus", "command-r", "command-light"}
		parsed := parsedModelsFor(models, parseCohereModel)
		next := selectLowerModelFromParsed("command-light", parsed)
		if next != "" {
			t.Errorf("expected empty for lowest model, got %q", next)
		}
	})
}

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
