package llm

import (
	"encoding/json"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/openai/openai-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ExtractOpenAITokenUsage parses usage information from an OpenAI completion response.
func ExtractOpenAITokenUsage(usage openai.CompletionUsage) domain.TokenUsage {
	reasoning := usage.CompletionTokensDetails.ReasoningTokens
	cached := usage.PromptTokensDetails.CachedTokens
	if cached == 0 && usage.JSON.ExtraFields != nil {
		if v, ok := usage.JSON.ExtraFields["prompt_cache_hit_tokens"]; ok {
			var hitTokens int64
			if err := json.Unmarshal([]byte(v.Raw()), &hitTokens); err == nil && hitTokens > 0 {
				cached = hitTokens
			}
		}
	}
	tu := domain.TokenUsage{
		InputTokens:     usage.PromptTokens,
		OutputTokens:    usage.CompletionTokens,
		ReasoningTokens: reasoning,
		CachedTokens:    cached,
		TotalTokens:     usage.TotalTokens,
	}
	if tu.TotalTokens <= 0 {
		tu.TotalTokens = tu.InputTokens + tu.OutputTokens
	}
	return tu
}

// ExtractAnthropicTokenUsage normalizes Anthropic prompt caching and completion usage counters.
func ExtractAnthropicTokenUsage(inputTokens, cacheReadTokens, cacheCreationTokens, outputTokens int64) domain.TokenUsage {
	totalInput := inputTokens + cacheReadTokens + cacheCreationTokens
	return domain.TokenUsage{
		InputTokens:  totalInput,
		OutputTokens: outputTokens,
		CachedTokens: cacheReadTokens,
		TotalTokens:  totalInput + outputTokens,
	}
}

// ExtractGeminiTokenUsage normalizes Gemini UsageMetadata counters.
func ExtractGeminiTokenUsage(promptCount, candidatesCount, cachedCount int64) domain.TokenUsage {
	totalInput := promptCount + cachedCount
	return domain.TokenUsage{
		InputTokens:  totalInput,
		OutputTokens: candidatesCount,
		CachedTokens: cachedCount,
		TotalTokens:  totalInput + candidatesCount,
	}
}

// FallbackTokenUsage computes an estimated TokenUsage when provider usage is missing.
func FallbackTokenUsage(prompt string, resp *domain.LLMResponse) domain.TokenUsage {
	in := estimatePromptTokens(prompt)
	out := estimateCompletionTokens(resp)
	return domain.TokenUsage{
		InputTokens:  in,
		OutputTokens: out,
		TotalTokens:  in + out,
	}
}

// AttachGenAIOtelAttributes enriches an active trace span with OpenTelemetry GenAI semantic conventions.
func AttachGenAIOtelAttributes(span trace.Span, usage domain.TokenUsage) {
	if span == nil || !span.IsRecording() {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.Int64("gen_ai.usage.input_tokens", usage.InputTokens),
		attribute.Int64("gen_ai.usage.output_tokens", usage.OutputTokens),
	}
	if usage.CachedTokens > 0 {
		attrs = append(attrs, attribute.Int64("gen_ai.usage.cached_tokens", usage.CachedTokens))
	}
	if usage.ReasoningTokens > 0 {
		attrs = append(attrs, attribute.Int64("gen_ai.usage.reasoning_tokens", usage.ReasoningTokens))
	}
	span.SetAttributes(attrs...)
}
