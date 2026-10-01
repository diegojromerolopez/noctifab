package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// sendCompletionStreaming streams the completion via the official SDK SSE
// client and accumulates the full content. The SDK natively terminates on the
// `data: [DONE]` marker (handling OpenRouter's keepalive behaviour) and
// accumulates reasoning/content deltas correctly.
//
// idle_timeout enforcement is a true sliding inter-chunk timer: the stream is
// cancelled only when no chunk has arrived for idleTimeout. Long responses
// that keep streaming are never cut short — total duration remains capped by
// the http.Client timeout (max_timeout).
func (o *baseOpenAIClient) sendCompletionStreaming(ctx context.Context, model, apiKey, prompt string, opts completionOptions) (*ProviderCallResult, error) {
	opts.streaming = true
	client := o.sdkStreamingClient(apiKey)
	params := buildChatParams(model, prompt, opts)

	// Build extra request options for provider-specific body params and headers.
	var reqOpts []option.RequestOption
	for k, v := range opts.extraBody {
		reqOpts = append(reqOpts, option.WithJSONSet(k, v))
	}
	for k, v := range opts.extraHeaders {
		reqOpts = append(reqOpts, option.WithHeader(k, v))
	}

	streamTimeout := o.timeout
	if streamTimeout <= 0 || streamTimeout > 90*time.Second {
		streamTimeout = 90 * time.Second
	}
	streamCtx, cancelStream := context.WithTimeout(ctx, streamTimeout)
	defer cancelStream()

	idleTimeout := o.idleTimeout
	if idleTimeout <= 0 {
		idleTimeout = 20 * time.Second
	}
	var idleFired atomic.Bool
	idleTimer := time.AfterFunc(idleTimeout, func() {
		idleFired.Store(true)
		cancelStream()
	})
	defer idleTimer.Stop()

	stream := client.Chat.Completions.NewStreaming(streamCtx, params, reqOpts...)

	var acc openai.ChatCompletionAccumulator
	var reasoning strings.Builder
	var usage domain.TokenUsage
	streamStart := time.Now()
	tracker := domain.StreamLivenessTrackerFromContext(ctx)
	for stream.Next() {
		if idleTimer != nil {
			idleTimer.Reset(idleTimeout)
		}
		if tracker != nil {
			tracker.RecordChunk()
		}
		chunk := stream.Current()
		acc.AddChunk(chunk)
		if chunk.Usage.TotalTokens > 0 || chunk.Usage.PromptTokens > 0 {
			usage = ExtractOpenAITokenUsage(chunk.Usage)
		}
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content == "" {
			reasoning.WriteString(extractReasoningContent(chunk.Choices[0].Delta.JSON.ExtraFields))
		}
	}
	if err := stream.Err(); err != nil {
		if idleFired.Load() && ctx.Err() == nil {
			return nil, fmt.Errorf("stream idle timeout: no data received for %v: %w", idleTimeout, err)
		}
		return nil, o.sdkError(err)
	}

	if usage.TotalTokens == 0 && acc.Usage.TotalTokens > 0 {
		usage = ExtractOpenAITokenUsage(acc.Usage)
	}

	elapsed := time.Since(streamStart)
	content := ""
	if len(acc.Choices) > 0 {
		content = acc.Choices[0].Message.Content
	}
	if content == "" && reasoning.Len() > 0 {
		fmt.Fprintf(os.Stderr, "ℹ [llm] model %s streamed empty content; using reasoning_content (%d bytes)\n", model, reasoning.Len())
		content = reasoning.String()
	}
	fmt.Fprintf(os.Stderr, "ℹ [llm] SSE stream for model %s completed: %d bytes, total=%v\n", model, len(content), elapsed)

	return &ProviderCallResult{Body: []byte(content), Usage: usage}, nil
}
