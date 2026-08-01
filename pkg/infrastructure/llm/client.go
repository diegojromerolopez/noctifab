package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Client struct {
	Provider    string
	Model       string
	APIKey      string
	APIKeys     []string
	keyIndex    uint64
	MaxRetries  int
	Backoff     time.Duration
	URL         string
	Timeout     time.Duration
	IdleTimeout time.Duration
	Streaming   bool
}

func (c *Client) getNextAPIKey() string {
	if len(c.APIKeys) > 0 {
		idx := atomic.AddUint64(&c.keyIndex, 1) - 1
		return c.APIKeys[idx%uint64(len(c.APIKeys))]
	}
	return c.APIKey
}

var _ domain.LLMClient = (*Client)(nil)

// parseAndUnmarshal runs ExtractJSONBlock followed by LenientUnmarshal in
// one step. On any failure it returns the first error encountered.
func parseAndUnmarshal(body []byte) (*domain.LLMResponse, error) {
	extracted, err := ExtractJSONBlock(string(body))
	if err != nil {
		return nil, err
	}
	return LenientUnmarshal(extracted)
}

// buildJSONReminderPrompt returns a single user-message prompt that re-states
// the JSON envelope demand and includes a truncated tail of the model's
// previous non-JSON answer. The model is asked to return ONLY the JSON
// envelope now. The tail is capped to keep the request under typical context
// limits while still surfacing enough context for the model to recognise its
// mistake and self-correct in a single turn.
const jsonReminderTailCap = 1500

func buildJSONReminderPrompt(originalPrompt string, prevBody []byte) string {
	tail := string(prevBody)
	if len(tail) > jsonReminderTailCap {
		tail = "...[truncated]...\n" + tail[len(tail)-jsonReminderTailCap:]
	}
	return fmt.Sprintf(`Your previous response did NOT contain the structured JSON envelope that this system requires. The system cannot continue without a single valid JSON object.

Original task:
%s

Your previous (rejected) response:
%s

CRITICAL INSTRUCTION (overrides anything above):
Respond with ONLY a single JSON object matching this schema. No markdown, no code fences, no prose before or after the JSON. Keys and string values must use double quotes.

Schema:
{
  "reasoning": "your reasoning",
  "actions": [
    { "tool": "write_file", "args": { "path": "...", "content": "..." } }
  ]
}

Return the JSON block now and nothing else.`, originalPrompt, tail)
}

func NewClient(provider, model, apiKey string, maxRetries int, backoff time.Duration, url string) *Client {
	return &Client{
		Provider:    provider,
		Model:       model,
		APIKey:      apiKey,
		MaxRetries:  maxRetries,
		Backoff:     backoff,
		URL:         url,
		Timeout:     5 * time.Second,
		IdleTimeout: 15 * time.Second,
		Streaming:   true,
	}
}

func (c *Client) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "Complete",
		trace.WithAttributes(
			attribute.String("provider", c.Provider),
			attribute.String("model", c.Model),
			attribute.Int("max_retries", c.MaxRetries),
		))
	defer span.End()

	// Preprocess prompt to inject system instructions and schemas based on the target action type
	prompt = preprocessPrompt(prompt)

	spec, _ := GetProviderSpec(c.Provider)

	apiKey := c.getNextAPIKey()
	if apiKey == "" {
		if spec != nil {
			for _, envKey := range spec.EnvKeys {
				if val := os.Getenv(envKey); val != "" {
					apiKey = val
					break
				}
			}
		}
		if apiKey == "" {
			apiKey = os.Getenv("NOCTIFAB_LLM_API_KEY")
		}
	}

	if apiKey == "" {
		return nil, errors.New("missing API key for LLM provider")
	}

	normModel := strings.ToLower(strings.TrimSpace(c.Model))
	if normModel == "latest" || normModel == "auto" || strings.HasSuffix(normModel, "-latest") || normModel == "" {
		if resolved := c.resolveLatestModel(ctx, apiKey); resolved != "" {
			fmt.Fprintf(os.Stderr, "ℹ Dynamically resolved model alias '%s' for provider %s to latest model: %s\n", c.Model, c.Provider, resolved)
			c.Model = resolved
		}
	}

	originalModel := c.Model
	defer func() {
		c.Model = originalModel
	}()

	for {
		var responseBody []byte
		var err error

		maxRetries := c.MaxRetries
		if maxRetries <= 0 {
			maxRetries = 5
		}
		backoff := c.Backoff
		if backoff <= 0 {
			backoff = 100 * time.Millisecond
		}

		var pClient ProviderClient
		if spec != nil && spec.NewClientFunc != nil {
			pClient = spec.NewClientFunc(c.URL, c.Timeout, c.IdleTimeout, c.Streaming)
		} else {
			pClient = NewOpenAIProviderClient(c.Provider, c.URL, c.Timeout, c.IdleTimeout, c.Streaming)
		}

		activeKey := apiKey
		for attempt := 0; attempt <= maxRetries; attempt++ {
			responseBody, err = pClient.Call(ctx, c.Model, activeKey, prompt)
			if err == nil {
				break
			}

			fmt.Fprintf(os.Stderr, "⚠ LLM API error: %v (attempt %d/%d). Retrying...\n", err, attempt+1, maxRetries+1)
			if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "RESOURCE_EXHAUSTED") || strings.Contains(err.Error(), "Quota exceeded") || strings.Contains(err.Error(), "quota") {
				fmt.Fprintln(os.Stderr, "⚠ Warning: You have exceeded your LLM API quota (HTTP 429). Please check your plan and billing details.")
				if len(c.APIKeys) > 1 {
					activeKey = c.getNextAPIKey()
					fmt.Fprintf(os.Stderr, "ℹ Switching to next API key in pool for provider %s...\n", c.Provider)
				}
			}

			if attempt == maxRetries {
				break
			}

			// Exponential backoff with jitter
			jitter := time.Duration(float64(backoff) * (1.0 + rand.Float64()))
			if delay, ok := parseRetryDelay(err); ok {
				jitter = delay
				fmt.Fprintf(os.Stderr, "⚠ Rate limited. Backing off for %v as requested by the API.\n", delay)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(jitter):
			}
			backoff *= 2
		}

		if err == nil {
			resp, parseErr := parseAndUnmarshal(responseBody)
			if parseErr == nil {
				return resp, nil
			}
			// Defensive one-shot format-reminder retry: when the model
			// returned a non-JSON blob (prose, code, a shell command), send
			// a single pullback prompt that re-states the JSON envelope
			// demand, append the offending tail, and try once more.
			fmt.Fprintf(os.Stderr, "⚠ LLM response was not a valid JSON envelope (%v). Sending a one-shot format reminder and retrying...\n", parseErr)
			reminderPrompt := buildJSONReminderPrompt(prompt, responseBody)
			reminderCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			reminderBody, rErr := pClient.Call(reminderCtx, c.Model, apiKey, reminderPrompt)
			cancel()
			if rErr == nil {
				resp2, pErr2 := parseAndUnmarshal(reminderBody)
				if pErr2 == nil {
					return resp2, nil
				}
				fmt.Fprintf(os.Stderr, "⚠ One-shot format reminder did not yield a parseable JSON response: %v\n", pErr2)
				return nil, pErr2
			}
			fmt.Fprintf(os.Stderr, "⚠ Format reminder call failed: %v\n", rErr)
			return nil, parseErr
		}

		// Unconditionally attempt model fallback across all LLM providers when an error response is returned
		shouldFallback := true

		if shouldFallback {
			nextModel := c.getNextLowerModel(ctx, apiKey)
			if nextModel != "" {
				fmt.Fprintf(os.Stderr, "⚠ Model %s returned error: %v. Falling back to lower model: %s...\n", c.Model, err, nextModel)
				c.Model = nextModel
				continue
			}
		}

		return nil, fmt.Errorf("LLM completion failed after %d retries: %w", maxRetries, err)
	}
}

func (c *Client) getNextLowerModel(ctx context.Context, apiKey string) string {
	provider := strings.ToLower(c.Provider)

	var pClient ProviderClient
	spec, _ := GetProviderSpec(provider)
	if spec != nil && spec.NewClientFunc != nil {
		pClient = spec.NewClientFunc(c.URL, c.Timeout, c.IdleTimeout, c.Streaming)
	} else {
		pClient = NewOpenAIProviderClient(c.Provider, c.URL, c.Timeout, c.IdleTimeout, c.Streaming)
	}

	available, err := pClient.GetAvailableModels(ctx, apiKey)
	if err != nil || len(available) == 0 {
		fmt.Fprintf(os.Stderr, "⚠ Warning: failed to query available models from %s API endpoint: %v\n", c.Provider, err)
		return ""
	}

	var parsedModels []*ProviderModelInfo
	parser := parseOpenAIModel
	if spec != nil && spec.ParseModelFunc != nil {
		parser = spec.ParseModelFunc
	}
	for _, m := range available {
		if info, parsed := parser(m); parsed && info != nil {
			parsedModels = append(parsedModels, info)
		}
	}

	if len(parsedModels) == 0 {
		return ""
	}

	return selectLowerModelFromParsed(c.Model, parsedModels)
}

func (c *Client) resolveLatestModel(ctx context.Context, apiKey string) string {
	provider := strings.ToLower(c.Provider)

	var pClient ProviderClient
	spec, _ := GetProviderSpec(provider)
	if spec != nil && spec.NewClientFunc != nil {
		pClient = spec.NewClientFunc(c.URL, c.Timeout, c.IdleTimeout, c.Streaming)
	} else {
		pClient = NewOpenAIProviderClient(c.Provider, c.URL, c.Timeout, c.IdleTimeout, c.Streaming)
	}

	available, err := pClient.GetAvailableModels(ctx, apiKey)
	if err != nil || len(available) == 0 {
		return ""
	}

	var parsedModels []*ProviderModelInfo
	parser := parseOpenAIModel
	if spec != nil && spec.ParseModelFunc != nil {
		parser = spec.ParseModelFunc
	}
	for _, m := range available {
		if info, parsed := parser(m); parsed && info != nil {
			parsedModels = append(parsedModels, info)
		}
	}

	if len(parsedModels) == 0 {
		return ""
	}

	sortProviderModels(parsedModels)
	return parsedModels[0].Name
}

// httpError is returned by provider Call methods on non-2xx responses.
// It carries the raw response body and the HTTP headers that providers use
// to signal retry timing (e.g. Retry-After, ratelimit).
type httpError struct {
	StatusCode int
	Body       string
	Header     http.Header
}

func (e *httpError) Error() string {
	return fmt.Sprintf("HTTP error %d: %s", e.StatusCode, e.Body)
}

// hfRatelimitRe matches the HuggingFace `ratelimit` header value, e.g.
// `"api";r=0;t=55`  — we want the `t=<seconds>` component.
var hfRatelimitRe = regexp.MustCompile(`t=(\d+)`)

// geminiRetryDelayRe is used to extract retryDelay strings from Gemini JSON
// bodies without a full unmarshal when the body may already be partially
// embedded in an error message string.
type geminiErrorResponse struct {
	Error struct {
		Details []struct {
			Type       string `json:"@type"`
			RetryDelay string `json:"retryDelay"`
		} `json:"details"`
	} `json:"error"`
}

// parseRetryDelay extracts a provider-specific retry wait duration from an
// error returned by a provider Call method.  It tries, in order:
//  1. The Retry-After HTTP header (integer seconds) — used by OpenAI,
//     Anthropic, Mistral, DeepSeek.
//  2. The HuggingFace `ratelimit` header  t=<seconds> field.
//  3. The Gemini JSON body  error.details[].retryDelay  duration string.
func parseRetryDelay(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}

	// 1. Structured httpError — check headers first.
	var he *httpError
	if errors.As(err, &he) {
		// 1a. Standard Retry-After header (integer seconds).
		if ra := he.Header.Get("Retry-After"); ra != "" {
			if secs, e := strconv.ParseFloat(strings.TrimSpace(ra), 64); e == nil {
				return time.Duration(secs * float64(time.Second)), true
			}
		}
		// 1b. HuggingFace `ratelimit` header — extract t=<seconds>.
		if rl := he.Header.Get("ratelimit"); rl != "" {
			if m := hfRatelimitRe.FindStringSubmatch(rl); len(m) == 2 {
				if secs, e := strconv.Atoi(m[1]); e == nil {
					return time.Duration(secs) * time.Second, true
				}
			}
		}
	}

	// 2. Gemini JSON body retryDelay — works whether error is *httpError or
	//    a plain fmt.Errorf (legacy path).
	errStr := err.Error()
	firstBrace := strings.Index(errStr, "{")
	if firstBrace == -1 {
		return 0, false
	}
	var resp geminiErrorResponse
	if jsonErr := json.Unmarshal([]byte(errStr[firstBrace:]), &resp); jsonErr != nil {
		return 0, false
	}
	for _, detail := range resp.Error.Details {
		if detail.RetryDelay == "" {
			continue
		}
		delayStr := strings.TrimSpace(detail.RetryDelay)
		if d, e := time.ParseDuration(delayStr); e == nil {
			return d, true
		}
		// Numeric seconds without unit suffix.
		if secs, e := strconv.ParseFloat(delayStr, 64); e == nil {
			return time.Duration(secs * float64(time.Second)), true
		}
	}
	return 0, false
}
