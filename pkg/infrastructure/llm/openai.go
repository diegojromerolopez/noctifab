package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
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
		{Keywords: []string{"o3-mini", "o1-mini"}, Score: 35, TierName: "compact-reasoning"},
		{Keywords: []string{"o3", "o1"}, Score: 60, TierName: "reasoning"},
		{Keywords: []string{"gpt-4o-mini", "mini", "luna"}, Score: 20, TierName: "compact"},
		{Keywords: []string{"sol", "gpt-4o"}, Score: 50, TierName: "flagship"},
		{Keywords: []string{"terra", "gpt-4"}, Score: 40, TierName: "pro"},
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
func (o *baseOpenAIClient) sdkBaseURL() string {
	base := o.baseURL
	if base == "" {
		base = resolveProviderBaseURL(o.provider)
	}
	if o.url != "" {
		base = o.url
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
		option.WithBaseURL(o.sdkBaseURL()),
		option.WithHTTPClient(o.sdkHTTPClient()),
		option.WithMaxRetries(0),
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return openai.NewClient(opts...)
}

func (o *baseOpenAIClient) Call(ctx context.Context, model, apiKey, prompt string, maxTokens int, temperature float64) ([]byte, error) {
	respBody, err := o.sendCompletion(ctx, model, apiKey, prompt, true, maxTokens, temperature)
	if err == nil {
		return respBody, nil
	}
	// Transparent single fallback: if a relay rejects the request because
	// it does not understand `response_format` (HTTP 400 with a hint in the
	// body), retry once without the field. This keeps compliant models on
	// the strong JSON path while not breaking older vLLM / Ollama setups.
	if he, ok := err.(*httpError); ok && he.StatusCode == http.StatusBadRequest && looksLikeResponseFormatRejection(he.Body) {
		_, _ = fmt.Fprintln(os.Stderr, "⚠ Server rejected response_format; retrying without JSON enforcement.")
		return o.sendCompletion(ctx, model, apiKey, prompt, false, maxTokens, temperature)
	}
	return nil, err
}

// sendCompletion performs a single chat completions POST via the official
// OpenAI SDK. When enforceJSON is true the request advertises
// response_format=json_object so the assistant is constrained to a JSON object.
func (o *baseOpenAIClient) sendCompletion(ctx context.Context, model, apiKey, prompt string, enforceJSON bool, maxTokens int, temperature float64) ([]byte, error) {
	if o.streaming {
		respBody, err := o.sendCompletionStreaming(ctx, model, apiKey, prompt, enforceJSON, maxTokens, temperature)
		if err == nil && len(respBody) > 0 {
			return respBody, nil
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "⚠ Streaming call failed (%v); retrying with non-streaming POST.\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "⚠ Streaming call returned empty content; retrying with non-streaming POST.\n")
		}
	}

	client := o.sdkClient(apiKey)

	params := openai.ChatCompletionNewParams{
		Model: openai.ChatModel(model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Temperature: openai.Float(tempOrDefault(temperature)),
	}
	if maxTokens > 0 {
		params.MaxTokens = openai.Int(int64(maxTokens))
	}
	if enforceJSON {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}
	}

	start := time.Now()
	completion, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, o.sdkError(err)
	}
	fmt.Fprintf(os.Stderr, "ℹ [llm] chat/completions for model %s completed after %v\n", model, time.Since(start))

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf("unexpected OpenAI-compatible response: no choices returned")
	}
	content := completion.Choices[0].Message.Content
	fmt.Fprintf(os.Stderr, "ℹ [llm] model %s returned finish=%q contentLen=%d first100=%q\n",
		model, completion.Choices[0].FinishReason, len(content), truncate(content, 100))
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
// idle_timeout enforcement: when idleTimeout > 0 the streaming context is
// wrapped with idleTimeout so that a hung upstream that never sends response
// headers/bytes fails fast at idleTimeout instead of blocking for the full
// max_timeout. The http.Client timeout (max_timeout) continues to govern the
// open TCP connection lifetime once headers are received, so long responses
// are not cut short by the context deadline.
func (o *baseOpenAIClient) sendCompletionStreaming(ctx context.Context, model, apiKey, prompt string, enforceJSON bool, maxTokens int, temperature float64) ([]byte, error) {
	client := o.sdkClient(apiKey)

	params := openai.ChatCompletionNewParams{
		Model: openai.ChatModel(model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Temperature: openai.Float(tempOrDefault(temperature)),
	}
	if maxTokens > 0 {
		params.MaxTokens = openai.Int(int64(maxTokens))
	}
	if enforceJSON {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}
	}

	// Apply idle timeout: a hung upstream that never sends response
	// headers/bytes will be cancelled after idleTimeout (e.g. 8s) instead
	// of blocking for the full max_timeout (e.g. 600s). This enforces the
	// config's idle_timeout field, which was previously stored but ignored
	// on all SDK streaming paths.
	streamCtx := ctx
	if o.idleTimeout > 0 {
		var idleCancel context.CancelFunc
		streamCtx, idleCancel = context.WithTimeout(ctx, o.idleTimeout)
		defer idleCancel()
	}

	stream := client.Chat.Completions.NewStreaming(streamCtx, params)

	var acc openai.ChatCompletionAccumulator
	streamStart := time.Now()
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)
	}
	if err := stream.Err(); err != nil {
		return nil, o.sdkError(err)
	}

	elapsed := time.Since(streamStart)
	content := ""
	if len(acc.Choices) > 0 {
		content = acc.Choices[0].Message.Content
	}
	fmt.Fprintf(os.Stderr, "ℹ [llm] SSE stream for model %s completed: %d bytes, total=%v\n", model, len(content), elapsed)

	return []byte(content), nil
}

// sdkError converts SDK errors into the codebase's httpError type when the
// failure carries an HTTP status so existing retry/credit-exhaustion handling
// keeps working.
func (o *baseOpenAIClient) sdkError(err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) && apiErr.StatusCode != 0 {
		return &httpError{StatusCode: apiErr.StatusCode, Body: err.Error()}
	}
	return err
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
