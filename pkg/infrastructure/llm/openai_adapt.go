package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/respjson"
	"github.com/openai/openai-go/shared"
)

// completionOptions captures the per-request parameters that quirky
// OpenAI-compatible relays may reject. The adaptive retry in Call relaxes
// exactly one option per failed attempt so compliant models stay on the
// strict path while non-compliant gateways remain usable.
type completionOptions struct {
	enforceJSON bool
	// disableJSONMode overrides enforceJSON: when true, response_format is
	// never set, even if enforceJSON is true. Used for providers/models that
	// are incompatible with forced JSON mode (e.g. QwenCloud thinking models).
	disableJSONMode bool
	maxTokens       int
	// temperature is a pointer so the field can be omitted entirely: some
	// models (e.g. kimi-k3 behind the OpenCode Zen gateway) only accept a
	// single fixed temperature and reject any explicit value.
	temperature *float64
	// extraBody holds provider-specific key-value pairs to include verbatim
	// in the request body (e.g. enable_thinking for QwenCloud thinking mode).
	extraBody map[string]interface{}
}

// buildChatParams assembles SDK request params from completionOptions.
func buildChatParams(model, prompt string, opts completionOptions) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Model: openai.ChatModel(model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
	}
	if opts.temperature != nil {
		params.Temperature = openai.Float(tempOrDefault(*opts.temperature))
	}
	if opts.maxTokens > 0 {
		params.MaxTokens = openai.Int(int64(opts.maxTokens))
		params.MaxCompletionTokens = openai.Int(int64(opts.maxTokens))
	}
	// response_format=json_object is suppressed when disableJSONMode is set.
	// This is required for providers/models that cannot use forced JSON mode
	// (e.g. QwenCloud thinking models). ExtractJSONBlock handles the parsing.
	if opts.enforceJSON && !opts.disableJSONMode {
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
		}
	}
	return params
}

// ensureJSONKeyword guarantees the outgoing prompt contains the word "json"
// when response_format=json_object is requested. The OpenAI spec (and strict
// upstreams like DashScope/Qwen) reject json_object mode unless the messages
// mention JSON: "'messages' must contain the word 'json' in some form".
func ensureJSONKeyword(prompt string) string {
	if strings.Contains(strings.ToLower(prompt), "json") {
		return prompt
	}
	return prompt + "\n\nRespond with a single JSON object only."
}

// looksLikeRouterUnavailable detects gateway-side model routing failures such
// as the OpenCode Zen `{"type":"Router.Unavailable","modelID":"..."}` 5xx,
// observed when the request carries `max_tokens`/`response_format` for a
// model whose upstream cannot honour them. Retrying the identical request is
// futile; retrying with those fields stripped can succeed.
func looksLikeRouterUnavailable(he *httpError) bool {
	return he.StatusCode >= 500 && strings.Contains(strings.ToLower(he.Body), "router.unavailable")
}

// looksLikeInvalidTemperature detects 400s caused by models that pin their
// sampling temperature (e.g. "invalid temperature: only 1 is allowed for
// this model"). The retry omits the temperature field so the provider
// default applies.
func looksLikeInvalidTemperature(body string) bool {
	low := strings.ToLower(body)
	return strings.Contains(low, "temperature")
}

func looksLikeMaxTokensRejection(body string) bool {
	low := strings.ToLower(body)
	return strings.Contains(low, "max_tokens") || strings.Contains(low, "max_completion_tokens")
}

type providerCapabilityCache struct {
	mu            sync.RWMutex
	noTemperature map[string]bool
	noMaxTokens   map[string]bool
	noJSONMode    map[string]bool
}

var globalCapabilityCache = &providerCapabilityCache{
	noTemperature: make(map[string]bool),
	noMaxTokens:   make(map[string]bool),
	noJSONMode:    make(map[string]bool),
}

func (c *providerCapabilityCache) isTemperatureUnsupported(model string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.noTemperature[model]
}

func (c *providerCapabilityCache) isMaxTokensUnsupported(model string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.noMaxTokens[model]
}

func (c *providerCapabilityCache) isJSONModeUnsupported(model string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.noJSONMode[model]
}

func (c *providerCapabilityCache) markTemperatureUnsupported(model string) {
	if model == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.noTemperature[model] = true
}

func (c *providerCapabilityCache) markMaxTokensUnsupported(model string) {
	if model == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.noMaxTokens[model] = true
}

func (c *providerCapabilityCache) markJSONModeUnsupported(model string) {
	if model == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.noJSONMode[model] = true
}

// adaptOptionsForError inspects a provider error and, when the failure is
// attributable to a specific request option, returns a copy of opts with
// that single option relaxed and ok=true. It returns ok=false when the error
// is not recognised as parameter-induced (nothing left to adapt).
func adaptOptionsForError(opts completionOptions, err error, model string) (completionOptions, bool) {
	var he *httpError
	if !errors.As(err, &he) {
		return opts, false
	}
	switch {
	case he.StatusCode == http.StatusBadRequest && opts.enforceJSON && looksLikeResponseFormatRejection(he.Body):
		fmt.Fprintln(os.Stderr, "⚠ Server rejected response_format; retrying without JSON enforcement.")
		opts.enforceJSON = false
		globalCapabilityCache.markJSONModeUnsupported(model)
		return opts, true
	case looksLikeRouterUnavailable(he) && (opts.enforceJSON || opts.maxTokens > 0):
		fmt.Fprintln(os.Stderr, "⚠ Gateway router unavailable for this request shape; retrying without response_format/max_tokens.")
		opts.enforceJSON = false
		opts.maxTokens = 0
		globalCapabilityCache.markJSONModeUnsupported(model)
		globalCapabilityCache.markMaxTokensUnsupported(model)
		return opts, true
	case he.StatusCode == http.StatusBadRequest && opts.temperature != nil && looksLikeInvalidTemperature(he.Body):
		fmt.Fprintln(os.Stderr, "⚠ Server rejected the temperature value; retrying with the provider default.")
		opts.temperature = nil
		globalCapabilityCache.markTemperatureUnsupported(model)
		return opts, true
	case he.StatusCode == http.StatusBadRequest && opts.maxTokens > 0 && looksLikeMaxTokensRejection(he.Body):
		fmt.Fprintln(os.Stderr, "⚠ Server rejected max_tokens parameter; retrying without max_tokens.")
		opts.maxTokens = 0
		globalCapabilityCache.markMaxTokensUnsupported(model)
		return opts, true
	}
	return opts, false
}

// reasoningContentKeys are the non-standard response fields where some
// OpenAI-compatible gateways place the assistant text instead of `content`
// (e.g. glm-5.2 behind OpenCode Zen returns its whole answer in
// `reasoning_content` when response_format=json_object is set; OpenRouter
// exposes `reasoning` for some models).
var reasoningContentKeys = []string{"reasoning_content", "reasoning"}

// extractReasoningContent pulls assistant text from non-standard extra fields
// of a message or stream delta. Returns "" when absent or null.
func extractReasoningContent(extra map[string]respjson.Field) string {
	for _, key := range reasoningContentKeys {
		f, ok := extra[key]
		if !ok {
			continue
		}
		raw := f.Raw()
		if raw == "" || raw == "null" {
			continue
		}
		var s string
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			continue
		}
		if s != "" {
			return s
		}
	}
	return ""
}
