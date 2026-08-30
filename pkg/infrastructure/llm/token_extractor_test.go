package llm_test

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/openai/openai-go"
)

func TestExtractOpenAITokenUsage(t *testing.T) {
	usage := openai.CompletionUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}
	usage.CompletionTokensDetails.ReasoningTokens = 20

	tu := llm.ExtractOpenAITokenUsage(usage)
	if tu.InputTokens != 100 || tu.OutputTokens != 50 || tu.ReasoningTokens != 20 || tu.TotalTokens != 150 {
		t.Errorf("unexpected OpenAI TokenUsage extraction: %+v", tu)
	}
}

func TestExtractAnthropicTokenUsage(t *testing.T) {
	tu := llm.ExtractAnthropicTokenUsage(100, 50, 10, 30)
	if tu.InputTokens != 160 || tu.OutputTokens != 30 || tu.CachedTokens != 50 || tu.TotalTokens != 190 {
		t.Errorf("unexpected Anthropic TokenUsage extraction: %+v", tu)
	}
}

func TestExtractGeminiTokenUsage(t *testing.T) {
	tu := llm.ExtractGeminiTokenUsage(200, 40, 60)
	if tu.InputTokens != 260 || tu.OutputTokens != 40 || tu.CachedTokens != 60 || tu.TotalTokens != 300 {
		t.Errorf("unexpected Gemini TokenUsage extraction: %+v", tu)
	}
}

func TestFallbackTokenUsage(t *testing.T) {
	resp := &domain.LLMResponse{
		Reasoning: "Short reasoning text here",
		Actions: []domain.LLMAction{
			{Tool: "write_file", Args: map[string]any{"path": "foo.go"}},
		},
	}
	prompt := "This is a prompt to test fallback token estimation."
	tu := llm.FallbackTokenUsage(prompt, resp)

	if tu.InputTokens <= 0 || tu.OutputTokens <= 0 || tu.TotalTokens != tu.InputTokens+tu.OutputTokens {
		t.Errorf("unexpected FallbackTokenUsage: %+v", tu)
	}
}
