package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func init() {
	RegisterProvider(&ProviderSpec{
		Name:           "openai",
		BaseURL:        "https://api.openai.com/v1",
		EnvKeys:        []string{"OPENAI_API_KEY"},
		ParseModelFunc: parseOpenAIModel,
		Protocol:       "openai",
		NewClientFunc: func(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
			return NewOpenAIClient(url, timeout, idleTimeout, streaming)
		},
	})
}

var parseOpenAIModel = NewModelParser(ParserConfig{
	ExcludedKeywords:  []string{"embed", "tts", "whisper", "dall-e", "moderation", "realtime", "transcription", "bison", "audio"},
	DefaultVersion:    4.0,
	VersionRegexp:     `gpt-([0-9]+(?:\.[0-9]+)?)`,
	VersionMultiplier: 5,
	Tiers: []KeywordTier{
		{Keywords: []string{"o3-mini", "o1-mini"}, Score: 25, TierName: "compact-reasoning"},
		{Keywords: []string{"gpt-4o-mini", "mini", "luna"}, Score: 20, TierName: "compact"},
		{Keywords: []string{"gpt-5", "gpt-4.5", "gpt-4o", "sol"}, Score: 60, TierName: "flagship"},
		{Keywords: []string{"terra", "gpt-4"}, Score: 40, TierName: "pro"},
		{Keywords: []string{"o3", "o1"}, Score: 35, TierName: "reasoning"},
		{Keywords: []string{"gpt-3.5"}, Score: 10, TierName: "lite"},
	},
})

type baseOpenAIClient struct {
	provider    string
	baseURL     string
	url         string
	timeout     time.Duration
	idleTimeout time.Duration
	streaming   bool
	// extraBody holds provider-specific parameters to be merged into the
	// request's extra_body field (e.g. enable_thinking for QwenCloud).
	extraBody map[string]interface{}
	// disableJSONMode disables response_format=json_object when set.
	// Used for providers/models that cannot accept forced JSON mode.
	disableJSONMode bool
}

func newBaseOpenAIClient(provider, baseURL, url string, timeout, idleTimeout time.Duration, streaming bool) *baseOpenAIClient {
	return &baseOpenAIClient{
		provider:    provider,
		baseURL:     baseURL,
		url:         url,
		timeout:     timeout,
		idleTimeout: idleTimeout,
		streaming:   streaming,
	}
}

// SetExtraBody attaches provider-specific extra body parameters to this client.
// Parameters are merged into every outgoing completion request.
func (o *baseOpenAIClient) SetExtraBody(params map[string]interface{}) {
	o.extraBody = params
}

// SetDisableJSONMode disables response_format=json_object for this client.
// Use when the provider/model cannot accept forced JSON mode (e.g. QwenCloud
// thinking models). ExtractJSONBlock will parse the JSON from the raw response.
func (o *baseOpenAIClient) SetDisableJSONMode(v bool) {
	o.disableJSONMode = v
}

type OpenAIClient struct {
	*baseOpenAIClient
}

func NewOpenAIClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	return &OpenAIClient{
		baseOpenAIClient: newBaseOpenAIClient("openai", "https://api.openai.com/v1", url, timeout, idleTimeout, streaming),
	}
}

// NewOpenAIProviderClient creates a ProviderClient for OpenAI and OpenAI-compatible APIs.
func NewOpenAIProviderClient(provider, url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
	baseURL := "https://api.openai.com/v1"
	if spec, ok := GetProviderSpec(provider); ok {
		baseURL = spec.BaseURL
	}
	return &OpenAIClient{
		baseOpenAIClient: newBaseOpenAIClient(provider, baseURL, url, timeout, idleTimeout, streaming),
	}
}

// looksLikeResponseFormatRejection inspects the body of an HTTP 400 from a
// `/chat/completions` endpoint to decide whether the failure is plausibly
// caused by the server refusing to honour `response_format`. Conservative:
// accepts both snake_case and camelCase variants plus the canonical OpenAI
// wording. When in doubt, returns false (so we surface the raw 400 instead
// of silently dropping the field on a non-typed server error).
func looksLikeResponseFormatRejection(body string) bool {
	low := strings.ToLower(body)
	if strings.Contains(low, "response_format") {
		return true
	}
	if strings.Contains(low, "unknown parameter") {
		return true
	}
	if strings.Contains(low, "unrecognized request argument") {
		return true
	}
	return false
}

// resolveProviderBaseURL returns the default base URL for a registered provider.
func resolveProviderBaseURL(provider string) string {
	if spec, ok := GetProviderSpec(provider); ok {
		return spec.BaseURL
	}
	return "https://api.openai.com/v1"
}

// sdkBaseURL derives the SDK base URL. The official SDK appends the endpoint
// path (e.g. `chat/completions`) to the base, so any full endpoint override
// URL is reduced to its base origin. The result always ends in "/" as the SDK
// expects.
func (o *baseOpenAIClient) sdkBaseURL(apiKey string) string {
	base := o.baseURL
	if base == "" {
		base = resolveProviderBaseURL(o.provider)
	}
	if o.url != "" {
		base = o.url
	}
	if strings.HasPrefix(apiKey, "sk-nEx-") && (o.provider == "qwen" || o.provider == "dashscope" || o.provider == "qwencloud") {
		base = "https://opencode.ai/zen/go/v1"
	}
	if strings.HasPrefix(apiKey, "sk-ws-") && (o.provider == "qwen" || o.provider == "dashscope" || o.provider == "qwencloud") {
		base = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	}
	base = strings.TrimSuffix(base, "/chat/completions")
	base = strings.TrimSuffix(base, "/")
	return base + "/"
}

// sdkHTTPClient builds an *http.Client honouring the configured overall
// request timeout. The SDK otherwise uses http.DefaultClient.
func (o *baseOpenAIClient) sdkHTTPClient() *http.Client {
	timeout := o.timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	return &http.Client{Timeout: timeout}
}

// sdkClient builds an SDK client bound to this provider's base URL and key.
// SDK-level retries are disabled (WithMaxRetries(0)) so that only client.go's
// explicit retry loop controls retry cadence. Without this, the SDK adds 2
// implicit retries on top of client.go's own loop, multiplying a single hung
// call into up to 9 total attempts (3 SDK × 3 client.go).
func (o *baseOpenAIClient) sdkClient(apiKey string) openai.Client {
	opts := []option.RequestOption{
		option.WithBaseURL(o.sdkBaseURL(apiKey)),
		option.WithHTTPClient(o.sdkHTTPClient()),
		option.WithMaxRetries(0),
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return openai.NewClient(opts...)
}

// Call executes a chat completion with adaptive request-shape fallback: when
// the server rejects a specific request option (response_format not
// understood, gateway router unable to serve the shape, pinned temperature),
// exactly one option is relaxed and the call retried, up to three attempts.
// Unrecognised errors are returned immediately so the caller's retry/fallback
// ladder can classify them.
func (o *baseOpenAIClient) Call(ctx context.Context, model, apiKey, prompt string, maxTokens int, temperature float64) ([]byte, error) {
	opts := completionOptions{
		enforceJSON:     !globalCapabilityCache.isJSONModeUnsupported(model),
		disableJSONMode: o.disableJSONMode || globalCapabilityCache.isJSONModeUnsupported(model),
		maxTokens:       maxTokens,
		temperature:     &temperature,
		extraBody:       o.extraBody,
	}
	if globalCapabilityCache.isTemperatureUnsupported(model) {
		opts.temperature = nil
	}
	if globalCapabilityCache.isMaxTokensUnsupported(model) {
		opts.maxTokens = 0
	}
	var lastErr error
	for range 3 {
		respBody, err := o.sendCompletion(ctx, model, apiKey, prompt, opts)
		if err == nil {
			return respBody, nil
		}
		lastErr = err
		adapted, ok := adaptOptionsForError(opts, err, model)
		if !ok {
			return nil, err
		}
		opts = adapted
	}
	return nil, lastErr
}

// sendCompletion performs a single chat completions POST via the official
// OpenAI SDK. When opts.enforceJSON is true the request advertises
// response_format=json_object so the assistant is constrained to a JSON
// object, and the prompt is guaranteed to contain the word "json" as the
// OpenAI spec requires for that mode.
func (o *baseOpenAIClient) sendCompletion(ctx context.Context, model, apiKey, prompt string, opts completionOptions) ([]byte, error) {
	if opts.enforceJSON {
		prompt = ensureJSONKeyword(prompt)
	}

	if o.streaming {
		respBody, err := o.sendCompletionStreaming(ctx, model, apiKey, prompt, opts)
		if err == nil && len(respBody) > 0 {
			return respBody, nil
		}
		// A structured HTTP rejection is deterministic: the non-streaming
		// POST would receive the identical rejection, doubling latency and
		// token usage for nothing. Surface it so Call can adapt the request shape.
		var he *httpError
		if errors.As(err, &he) {
			return nil, err
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠ Streaming call failed (%v); retrying with non-streaming POST.\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "⚠ Streaming call returned empty content; retrying with non-streaming POST.\n")
		}
	}

	client := o.sdkClient(apiKey)
	params := buildChatParams(model, prompt, opts)

	// Build extra request options for provider-specific body params (e.g. enable_thinking).
	var reqOpts []option.RequestOption
	for k, v := range opts.extraBody {
		reqOpts = append(reqOpts, option.WithJSONSet(k, v))
	}

	start := time.Now()
	completion, err := client.Chat.Completions.New(ctx, params, reqOpts...)
	if err != nil {
		return nil, o.sdkError(err)
	}
	fmt.Fprintf(os.Stderr, "ℹ [llm] chat/completions for model %s completed after %v\n", model, time.Since(start))

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("unexpected OpenAI-compatible response: no choices returned")
	}
	choice := completion.Choices[0]
	content := choice.Message.Content
	if content == "" {
		// Some gateways (e.g. glm-5.2 behind OpenCode Zen with
		// response_format set) return the answer in a non-standard
		// reasoning_content field and leave content empty.
		if rc := extractReasoningContent(choice.Message.JSON.ExtraFields); rc != "" {
			fmt.Fprintf(os.Stderr, "ℹ [llm] model %s returned empty content; using reasoning_content (%d bytes)\n", model, len(rc))
			content = rc
		}
	}
	fmt.Fprintf(os.Stderr, "ℹ [llm] model %s returned finish=%q contentLen=%d first100=%q\n",
		model, choice.FinishReason, len(content), truncate(content, 100))
	return []byte(content), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// tempOrDefault returns the configured sampling temperature, defaulting to 0.0
// (deterministic) when the provider leaves it unset.
func tempOrDefault(t float64) float64 {
	if t == 0 {
		return 0.0
	}
	return t
}

// sendCompletionStreaming streams the completion via the official SDK SSE
// client and accumulates the full content. The SDK natively terminates on the
// `data: [DONE]` marker (handling OpenRouter's keepalive behaviour) and
// accumulates reasoning/content deltas correctly.
//
// idle_timeout enforcement is a true sliding inter-chunk timer: the stream is
// cancelled only when no chunk has arrived for idleTimeout. Long responses
// that keep streaming are never cut short — total duration remains capped by
// the http.Client timeout (max_timeout).
func (o *baseOpenAIClient) sendCompletionStreaming(ctx context.Context, model, apiKey, prompt string, opts completionOptions) ([]byte, error) {
	client := o.sdkClient(apiKey)
	params := buildChatParams(model, prompt, opts)

	// Build extra request options for provider-specific body params (e.g. enable_thinking).
	var reqOpts []option.RequestOption
	for k, v := range opts.extraBody {
		reqOpts = append(reqOpts, option.WithJSONSet(k, v))
	}

	streamCtx := ctx
	var idleTimer *time.Timer
	var idleFired atomic.Bool
	if o.idleTimeout > 0 {
		var cancel context.CancelFunc
		streamCtx, cancel = context.WithCancel(ctx)
		defer cancel()
		idleTimer = time.AfterFunc(o.idleTimeout, func() {
			idleFired.Store(true)
			cancel()
		})
		defer idleTimer.Stop()
	}

	stream := client.Chat.Completions.NewStreaming(streamCtx, params, reqOpts...)

	var acc openai.ChatCompletionAccumulator
	var reasoning strings.Builder
	streamStart := time.Now()
	for stream.Next() {
		if idleTimer != nil {
			idleTimer.Reset(o.idleTimeout)
		}
		chunk := stream.Current()
		acc.AddChunk(chunk)
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content == "" {
			reasoning.WriteString(extractReasoningContent(chunk.Choices[0].Delta.JSON.ExtraFields))
		}
	}
	if err := stream.Err(); err != nil {
		if idleFired.Load() && ctx.Err() == nil {
			return nil, fmt.Errorf("stream idle timeout: no data received for %v: %w", o.idleTimeout, err)
		}
		return nil, o.sdkError(err)
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

	return []byte(content), nil
}

// sdkError converts SDK errors into the codebase's httpError type when the
// failure carries an HTTP status so existing retry/credit-exhaustion handling
// keeps working.
//
// The SDK's Error() string only embeds the body's `error` field; gateways
// that return a bare error object without that wrapper (e.g. OpenCode Zen's
// `{"type":"Router.Unavailable",...}`) would otherwise surface an empty body,
// hiding the rejection reason from error classification. The SDK re-populates
// Response.Body for debugging, so read it back and append anything missing.
// Response headers are propagated for Retry-After parsing.
func (o *baseOpenAIClient) sdkError(err error) error {
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode == 0 {
		return err
	}
	body := err.Error()
	var header http.Header
	if apiErr.Response != nil {
		header = apiErr.Response.Header
		if apiErr.Response.Body != nil {
			if raw, rerr := io.ReadAll(apiErr.Response.Body); rerr == nil {
				if trimmed := strings.TrimSpace(string(raw)); trimmed != "" && !strings.Contains(body, trimmed) {
					body += " " + trimmed
				}
			}
		}
	}
	return &httpError{StatusCode: apiErr.StatusCode, Body: body, Header: header}
}

func (o *baseOpenAIClient) GetAvailableModels(ctx context.Context, apiKey string) ([]string, error) {
	client := o.sdkClient(apiKey)

	var models []string
	page := client.Models.ListAutoPaging(ctx)
	for page.Next() {
		models = append(models, page.Current().ID)
	}
	if err := page.Err(); err != nil {
		return nil, err
	}
	return models, nil
}
