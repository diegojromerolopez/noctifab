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
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Client struct {
	Provider   string
	Model      string
	APIKey     string
	MaxRetries int
	Backoff    time.Duration
	URL        string
	Timeout    time.Duration
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
		Provider:   provider,
		Model:      model,
		APIKey:     apiKey,
		MaxRetries: maxRetries,
		Backoff:    backoff,
		URL:        url,
		Timeout:    5 * time.Second,
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

	apiKey := c.APIKey
	if apiKey == "" {
		switch c.Provider {
		case "gemini":
			apiKey = os.Getenv("GEMINI_API_KEY")
		case "openai":
			apiKey = os.Getenv("OPENAI_API_KEY")
		case "anthropic":
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		case "opencode":
			apiKey = os.Getenv("OPENCODE_API_KEY")
		}
	}

	if apiKey == "" {
		return nil, errors.New("missing API key for LLM provider")
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
		switch strings.ToLower(c.Provider) {
		case "openai", "hermes", "huggingface", "mistral", "deepseek", "ollama", "opencode":
			pClient = NewOpenAIProviderClient(c.Provider, c.URL, c.Timeout)
		case "gemini":
			pClient = NewGeminiProviderClient(c.URL, c.Timeout)
		case "anthropic":
			pClient = NewAnthropicProviderClient(c.URL, c.Timeout)
		default:
			return nil, fmt.Errorf("unsupported LLM provider: %s", c.Provider)
		}

		for attempt := 0; attempt <= maxRetries; attempt++ {
			responseBody, err = pClient.Call(ctx, c.Model, apiKey, prompt)
			if err == nil {
				break
			}

			fmt.Fprintf(os.Stderr, "⚠ LLM API error: %v (attempt %d/%d). Retrying...\n", err, attempt+1, maxRetries+1)
			if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "RESOURCE_EXHAUSTED") || strings.Contains(err.Error(), "Quota exceeded") || strings.Contains(err.Error(), "quota") {
				fmt.Fprintln(os.Stderr, "⚠ Warning: You have exceeded your LLM API quota (HTTP 429). Please check your plan and billing details.")
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

		isFallbackError := strings.Contains(err.Error(), "HTTP error 503") ||
			strings.Contains(err.Error(), "503 Service Unavailable") ||
			strings.Contains(err.Error(), "HTTP error 429") ||
			strings.Contains(err.Error(), "429 Too Many Requests") ||
			strings.Contains(err.Error(), "RESOURCE_EXHAUSTED")

		if isFallbackError {
			nextModel := c.getNextLowerModel(ctx, apiKey)
			if nextModel != "" {
				fmt.Fprintf(os.Stderr, "⚠ Model %s returned transient/quota error. Falling back to lower model: %s...\n", c.Model, nextModel)
				c.Model = nextModel
				continue
			}
		}

		return nil, fmt.Errorf("LLM completion failed after %d retries: %w", maxRetries, err)
	}
}

func (c *Client) getNextLowerModel(ctx context.Context, apiKey string) string {
	list, ok := modelHierarchy[strings.ToLower(c.Provider)]
	if !ok || len(list) <= 1 {
		return ""
	}

	var pClient ProviderClient
	switch strings.ToLower(c.Provider) {
	case "openai", "hermes", "huggingface", "mistral", "deepseek", "ollama", "opencode":
		pClient = NewOpenAIProviderClient(c.Provider, c.URL, c.Timeout)
	case "gemini":
		pClient = NewGeminiProviderClient(c.URL, c.Timeout)
	case "anthropic":
		pClient = NewAnthropicProviderClient(c.URL, c.Timeout)
	default:
		return ""
	}

	available, err := pClient.GetAvailableModels(ctx, apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Warning: failed to query available models from %s: %v. Using static fallback hierarchy.\n", c.Provider, err)
		available = list
	}

	var filteredHierarchy []string
	for _, rankedModel := range list {
		rankedNorm := strings.TrimPrefix(strings.ToLower(rankedModel), "models/")
		isAvailable := false
		for _, availModel := range available {
			availNorm := strings.TrimPrefix(strings.ToLower(availModel), "models/")
			if rankedNorm == availNorm {
				isAvailable = true
				break
			}
		}
		if isAvailable {
			filteredHierarchy = append(filteredHierarchy, rankedModel)
		}
	}

	if len(filteredHierarchy) == 0 {
		filteredHierarchy = list
	}

	normCurrent := strings.TrimPrefix(strings.ToLower(normalizeModel(c.Model)), "models/")

	idx := -1
	for i, m := range filteredHierarchy {
		normM := strings.TrimPrefix(strings.ToLower(m), "models/")
		if normCurrent == normM {
			idx = i
			break
		}
	}

	if idx != -1 && idx+1 < len(filteredHierarchy) {
		return filteredHierarchy[idx+1]
	}
	return ""
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
