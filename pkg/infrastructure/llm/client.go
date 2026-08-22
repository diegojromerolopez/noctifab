package llm

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Client struct {
	Provider              string
	Model                 string
	APIKey                string
	APIKeys               []string
	keyIndex              uint64
	MaxRetries            int
	Backoff               time.Duration
	URL                   string
	Timeout               time.Duration
	IdleTimeout           time.Duration
	MaxTokens             int
	Temperature           float64
	Streaming             bool
	CavemanCompaction     bool
	Compaction            string
	SkipOnCreditExhausted bool
	// MaxPromptTokens caps the estimated size of an outgoing prompt so an
	// oversized prompt fails fast with a clear error instead of burning a
	// network round-trip (and the retry/fallback ladder) on a guaranteed
	// provider-side rejection. 0 means defaultMaxPromptTokens; a negative
	// value disables the check.
	MaxPromptTokens int64
	// ExtraParams holds provider-specific extra body parameters (e.g.
	// enable_thinking, thinking_budget for QwenCloud). These are merged
	// into the extra_body of each completion request as string-typed values.
	ExtraParams map[string]string
	// DisableJSONMode disables response_format=json_object for this client.
	// Set when the provider/model cannot use forced JSON mode (e.g. QwenCloud
	// thinking models). ExtractJSONBlock parses the JSON envelope instead.
	DisableJSONMode bool
	// EnableThinking enables chain-of-thought / reasoning output (e.g. QwenCloud thinking mode).
	EnableThinking *bool
	// ThinkingBudget caps the reasoning token budget.
	ThinkingBudget *int
	// catalogMu guards catalogCache: a small TTL cache of provider model
	// catalogs so the fallback ladder and latest-alias resolution do not
	// re-hit GetAvailableModels (a network call) on every invocation.
	// catalogTTL <= 0 means defaultCatalogTTL.
	catalogMu    sync.Mutex
	catalogCache map[string]catalogEntry
	catalogTTL   time.Duration
}

// ErrCreditExhausted is returned when a provider reports an HTTP 402 (or a
// credit/quota-limited 429) that cannot be resolved by retrying or by falling
// back to a lower model — the account is out of payable credits. The router
// treats it as a hard "skip this provider chain" signal so the next provider
// in priority is attempted immediately instead of burning wall-clock time.
var ErrCreditExhausted = errors.New("LLM provider credit exhausted")

// isCreditExhausted reports whether err signals provider credit/limit exhaustion.
// An HTTP 402 from the completion endpoint always qualifies. A 429 is only
// treated as credit exhaustion when the provider body explicitly mentions
// "credit" (e.g. OpenRouter's "You have depleted your monthly included
// credits") — a plain rate-limit 429 still falls through to normal handling.
func isCreditExhausted(err error) bool {
	var he *httpError
	if !errors.As(err, &he) {
		return false
	}
	if he.StatusCode == http.StatusPaymentRequired {
		return true
	}
	if he.StatusCode == http.StatusTooManyRequests {
		low := strings.ToLower(he.Body)
		return strings.Contains(low, "credit")
	}
	return false
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
Respond with ONLY a single JSON object matching this schema. Ensure the JSON starts with an opening brace '{' and ends with a closing brace '}'. No markdown, no code fences, no prose before or after the JSON. Keys and string values must use double quotes.

Schema:
{
  "reasoning": "your reasoning",
  "actions": [
    { "tool": "write_file", "args": { "path": "...", "content": "..." } }
  ]
}

Return the valid JSON block now and nothing else.`, originalPrompt, tail)
}

func NewClient(provider, model, apiKey string, maxRetries int, backoff time.Duration, url string) *Client {
	return &Client{
		Provider:   provider,
		Model:      model,
		APIKey:     apiKey,
		MaxRetries: maxRetries,
		Backoff:    backoff,
		URL:        url,
		// 60s matches the config default (llm.max_timeout in
		// pkg/infrastructure/config/defaults.go). A shorter fallback would
		// silently truncate LLM calls for any client constructed without a
		// config override.
		Timeout:     60 * time.Second,
		IdleTimeout: 15 * time.Second,
		Streaming:   true,
	}
}

// compactPrompt applies the configured compaction mode to the prompt. When
// the context marks a non-compactable tail (the machine-readable output
// contract at the end of prompts rendered by pkg/infrastructure/prompts),
// compaction applies only to the bytes before it: the JSON schema and tool
// list must reach the model verbatim.
func (c *Client) compactPrompt(ctx context.Context, prompt string) string {
	head, tail := prompt, ""
	if n := domain.UncompactableTailLen(ctx); n > 0 && n < len(prompt) {
		head, tail = prompt[:len(prompt)-n], prompt[len(prompt)-n:]
	}
	origPromptLen := len(prompt)
	switch strings.ToLower(strings.TrimSpace(c.Compaction)) {
	case "simple_english":
		prompt = CompactSimpleEnglish(head) + tail
		fmt.Fprintf(os.Stderr, "ℹ [llm] compacted prompt with simple_english: %d -> %d bytes\n", origPromptLen, len(prompt))
	case "caveman":
		prompt = CompactCaveman(head) + tail
		fmt.Fprintf(os.Stderr, "ℹ [llm] compacted prompt with caveman: %d -> %d bytes\n", origPromptLen, len(prompt))
	default:
		if c.CavemanCompaction {
			prompt = CompactCaveman(head) + tail
		}
	}
	return prompt
}

func (c *Client) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "Complete",
		trace.WithAttributes(
			attribute.String("provider", c.Provider),
			attribute.String("model", c.Model),
			attribute.Int("max_retries", c.MaxRetries),
		))
	defer span.End()

	// Prompts arrive fully assembled (rendered by pkg/infrastructure/prompts
	// or built inline by their hardcoded call sites); only compaction and the
	// size guard apply here.
	prompt = c.compactPrompt(ctx, prompt)

	// Pre-send size guard: reject prompts that would be refused by the
	// provider anyway, before spending a network call and the retry ladder.
	if err := checkPromptSize(prompt, c.MaxPromptTokens); err != nil {
		return nil, err
	}

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

	// activeModel carries the concrete model for this call. The shared Client
	// struct's Model field is never mutated here: Client instances are shared
	// across concurrent agent calls, so writing c.Model from Complete would be a
	// data race and would permanently pin a resolved "latest" alias (the alias
	// would never be re-resolved on later calls once the concrete name leaked
	// back into the struct). Keep everything on the local variable instead.
	activeModel := c.Model
	normModel := strings.ToLower(strings.TrimSpace(activeModel))
	if normModel == "latest" || normModel == "auto" || normModel == "" {
		if resolved := c.resolveLatestModel(ctx, apiKey); resolved != "" {
			fmt.Fprintf(os.Stderr, "ℹ Dynamically resolved model alias '%s' for provider %s to latest model: %s\n", activeModel, c.Provider, resolved)
			activeModel = resolved
		}
	}

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

		pClient := c.providerClient()

		activeKey := apiKey
		creditExhausted := false
		attemptStart := time.Now()
		for attempt := 0; attempt <= maxRetries; attempt++ {
			responseBody, err = pClient.Call(ctx, activeModel, activeKey, prompt, c.MaxTokens, c.Temperature)
			if err == nil {
				fmt.Fprintf(os.Stderr, "ℹ [llm] %s/%s call OK after %v (attempt %d)\n", c.Provider, activeModel, time.Since(attemptStart), attempt+1)
				break
			}

			if isModelNotFoundOrDeprecated(err) {
				BlacklistModel(activeModel)
				break
			}

			if isCreditExhausted(err) {
				creditExhausted = true
				fmt.Fprintf(os.Stderr, "⚠ LLM provider credit limit reached: %v\n", err)
				if c.SkipOnCreditExhausted {
					// Fast-fail: do not retry or fall back to a lower model —
					// every further attempt & fallback reuses the same spent
					// account and only delays the provider switch.
					break
				}
				// skip_on_credit_exhausted disabled: rotate to the next key in
				// the pool (the spent key is pushed to the back) and retry as
				// usual.
			}

			if isNonRetryableHTTPError(err) {
				// Deterministic rejection (bad model, invalid params, missing
				// auth, gateway router unable to serve the shape): retrying
				// the identical request cannot succeed. Break out so the
				// model/provider fallback ladder advances immediately.
				fmt.Fprintf(os.Stderr, "⚠ Non-retryable LLM API error for %s/%s; skipping retries.\n", c.Provider, activeModel)
				break
			}
			if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "RESOURCE_EXHAUSTED") || strings.Contains(err.Error(), "Quota exceeded") || strings.Contains(err.Error(), "quota") || creditExhausted {
				fmt.Fprintln(os.Stderr, "⚠ Warning: You have exceeded your LLM API quota (HTTP 429). Please check your plan and billing details.")
				if len(c.APIKeys) > 1 {
					activeKey = c.getNextAPIKey()
					fmt.Fprintf(os.Stderr, "ℹ Switching to next API key in pool for provider %s...\n", c.Provider)
				} else if c.SkipOnCreditExhausted {
					if delay, ok := parseRetryDelay(err); !ok || delay > 5*time.Second {
						fmt.Fprintf(os.Stderr, "⚠ Circuit-breaker: HTTP 429 quota exhausted for %s/%s; skipping retries to trigger model/provider fallback immediately.\n", c.Provider, activeModel)
						break
					}
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
				emitLLMEvent(ctx, c.Provider, activeModel, time.Since(attemptStart), prompt, resp, nil)
				return resp, nil
			}
			// Defensive one-shot format-reminder retry: when the model
			// returned a non-JSON blob (prose, code, a shell command), send
			// a single pullback prompt that re-states the JSON envelope
			// demand, append the offending tail, and try once more.
			fmt.Fprintf(os.Stderr, "⚠ LLM response was not a valid JSON envelope (%v). Sending a one-shot format reminder and retrying...\n", parseErr)
			reminderPrompt := buildJSONReminderPrompt(prompt, responseBody)
			reminderCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			reminderBody, rErr := pClient.Call(reminderCtx, activeModel, apiKey, reminderPrompt, c.MaxTokens, c.Temperature)
			cancel()
			if rErr == nil {
				resp2, pErr2 := parseAndUnmarshal(reminderBody)
				if pErr2 == nil {
					emitLLMEvent(ctx, c.Provider, activeModel, time.Since(attemptStart), reminderPrompt, resp2, nil)
					return resp2, nil
				}
				fmt.Fprintf(os.Stderr, "⚠ One-shot format reminder did not yield a parseable JSON response: %v\n", pErr2)
				emitLLMEvent(ctx, c.Provider, activeModel, time.Since(attemptStart), reminderPrompt, nil, pErr2)
				return nil, pErr2
			}
			fmt.Fprintf(os.Stderr, "⚠ Format reminder call failed: %v\n", rErr)
			emitLLMEvent(ctx, c.Provider, activeModel, time.Since(attemptStart), prompt, nil, parseErr)
			return nil, parseErr
		}

		// Attempt model fallback across all LLM providers when an error
		// response is returned, unless the error is deterministic in a way a
		// cheaper model cannot fix.
		shouldFallback := true
		if creditExhausted && c.SkipOnCreditExhausted {
			// Credit exhaustion is not recoverable with a cheaper model — the
			// account (or key) is out of payable credits. Skip the fallback
			// ladder and surface ErrCreditExhausted so the router moves to the
			// next provider in priority immediately.
			shouldFallback = false
		}
		if shouldSkipModelFallback(err) {
			// Deterministic non-retryable rejections such as 401/403 (bad
			// API key) or 400/422 (invalid request) cannot be fixed by a
			// cheaper model. 404 (model not found) is deliberately NOT
			// skipped: falling back to another model in the catalog IS the
			// correct reaction to an unknown model.
			fmt.Fprintf(os.Stderr, "⚠ Non-retryable LLM API error for %s/%s cannot be fixed by a lower model; skipping fallback ladder.\n", c.Provider, activeModel)
			shouldFallback = false
		}

		if shouldFallback {
			nextModel := c.getNextLowerModel(ctx, apiKey, activeModel)
			if nextModel != "" {
				fmt.Fprintf(os.Stderr, "⚠ Model %s returned error: %v. Falling back to model: %s...\n", activeModel, err, nextModel)
				activeModel = nextModel
				continue
			}
		}

		if creditExhausted && c.SkipOnCreditExhausted {
			errResult := fmt.Errorf("%w (provider %s, model %s): %v", ErrCreditExhausted, c.Provider, activeModel, err)
			emitLLMEvent(ctx, c.Provider, activeModel, time.Since(attemptStart), prompt, nil, errResult)
			return nil, errResult
		}

		errResult := fmt.Errorf("LLM completion failed after %d retries: %w", maxRetries, err)
		emitLLMEvent(ctx, c.Provider, activeModel, time.Since(attemptStart), prompt, nil, errResult)
		return nil, errResult
	}
}

func emitLLMEvent(ctx context.Context, provider, model string, duration time.Duration, prompt string, resp *domain.LLMResponse, err error) {
	obs := domain.ObserverFromContext(ctx)
	if obs == nil {
		return
	}
	durMS := duration.Milliseconds()
	outcome := domain.OutcomeSuccess
	if err != nil {
		outcome = domain.OutcomeFailed
	}
	pTokens := estimatePromptTokens(prompt)
	cTokens := estimateCompletionTokens(resp)
	event := domain.ExecutionEvent{
		Kind:             domain.EventLLMCallFinished,
		At:               time.Now().UTC(),
		Provider:         provider,
		Model:            model,
		DurationMillis:   &durMS,
		PromptTokens:     &pTokens,
		CompletionTokens: &cTokens,
		Outcome:          outcome,
	}
	obs.Observe(ctx, event)
}
