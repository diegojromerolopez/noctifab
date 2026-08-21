package llm

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// estCharsPerToken is the rough number of characters per token used for
// token estimation (~4 chars/token for English-like text and code).
const estCharsPerToken = 4

// defaultMaxPromptTokens is the default pre-send cap on the estimated prompt
// size (~1 MiB of text). Prompts above it are guaranteed provider-side
// rejections, so failing fast saves a network round-trip plus the whole
// retry/fallback ladder.
const defaultMaxPromptTokens = 262_144

// ErrPromptTooLarge is returned when an outgoing prompt's estimated token
// count exceeds the configured pre-send cap.
var ErrPromptTooLarge = errors.New("prompt exceeds maximum estimated token size")

// checkPromptSize validates the estimated prompt size against maxTokens.
// maxTokens == 0 applies defaultMaxPromptTokens; maxTokens < 0 disables the
// check.
func checkPromptSize(prompt string, maxTokens int64) error {
	if maxTokens < 0 {
		return nil
	}
	if maxTokens == 0 {
		maxTokens = defaultMaxPromptTokens
	}
	if est := estimatePromptTokens(prompt); est > maxTokens {
		return fmt.Errorf("%w: estimated %d tokens exceeds limit %d", ErrPromptTooLarge, est, maxTokens)
	}
	return nil
}

// estimatePromptTokens estimates the number of tokens in a prompt.
func estimatePromptTokens(prompt string) int64 {
	return int64(len(prompt) / estCharsPerToken)
}

// estimateCompletionTokens estimates the number of tokens in a parsed LLM
// response. Always returns at least 1 for a non-nil response so successful
// calls are never accounted as free.
func estimateCompletionTokens(resp *domain.LLMResponse) int64 {
	if resp == nil {
		return 0
	}
	n := len(resp.Reasoning) / estCharsPerToken
	for _, a := range resp.Actions {
		n += len(a.Tool) / estCharsPerToken
		if a.Args != nil {
			data, err := json.Marshal(a.Args)
			if err == nil {
				n += len(data) / estCharsPerToken
			}
		}
	}
	if n < 1 {
		n = 1
	}
	return int64(n)
}

// EstimateUsageTokens estimates the total tokens consumed by one completed
// call (prompt + completion).
func EstimateUsageTokens(prompt string, resp *domain.LLMResponse) int64 {
	return estimateUsageTokens(prompt, resp)
}

// estimateUsageTokens estimates the total tokens consumed by one completed
// call (prompt + completion).
func estimateUsageTokens(prompt string, resp *domain.LLMResponse) int64 {
	return estimatePromptTokens(prompt) + estimateCompletionTokens(resp)
}
