package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var blacklistedModels sync.Map

// BlacklistModel permanently blacklists a model so Noctifab never attempts to call it again.
func BlacklistModel(model string) {
	if model != "" {
		if _, loaded := blacklistedModels.LoadOrStore(strings.ToLower(strings.TrimSpace(model)), true); !loaded {
			fmt.Fprintf(os.Stderr, "⛔ Blacklisted deprecated/unavailable model %q permanently.\n", model)
		}
	}
}

// IsModelBlacklisted reports whether a model has been blacklisted.
func IsModelBlacklisted(model string) bool {
	if model == "" {
		return false
	}
	_, ok := blacklistedModels.Load(strings.ToLower(strings.TrimSpace(model)))
	return ok
}

// ResetModelBlacklist clears all blacklisted models from memory.
// Primarily used for unit test cleanup and state resets.
func ResetModelBlacklist() {
	blacklistedModels.Range(func(key, _ any) bool {
		blacklistedModels.Delete(key)
		return true
	})
}

// isModelNotFoundOrDeprecated reports whether an error indicates a model is 404, deprecated, invalid, or no longer available.
func isModelNotFoundOrDeprecated(err error) bool {
	if err == nil {
		return false
	}
	var he *httpError
	if errors.As(err, &he) {
		if he.StatusCode == http.StatusNotFound {
			return true
		}
		bodyLower := strings.ToLower(he.Body)
		if strings.Contains(bodyLower, "no longer available") ||
			strings.Contains(bodyLower, "model_not_found") ||
			strings.Contains(bodyLower, "not a valid model") ||
			strings.Contains(bodyLower, "is not a valid model") ||
			strings.Contains(bodyLower, "invalid model") ||
			strings.Contains(bodyLower, "does not exist") ||
			strings.Contains(bodyLower, "not found") ||
			strings.Contains(bodyLower, "unknown model") ||
			strings.Contains(bodyLower, "model not supported") ||
			strings.Contains(bodyLower, "unsupported model") ||
			strings.Contains(bodyLower, "is not supported") {
			return true
		}
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "no longer available") ||
		strings.Contains(errStr, "model_not_found") ||
		strings.Contains(errStr, "not a valid model") ||
		strings.Contains(errStr, "is not a valid model") ||
		strings.Contains(errStr, "invalid model") ||
		strings.Contains(errStr, "does not exist") ||
		strings.Contains(errStr, "unknown model") ||
		strings.Contains(errStr, "model not supported") ||
		strings.Contains(errStr, "unsupported model") ||
		strings.Contains(errStr, "is not supported") ||
		strings.Contains(errStr, "404 not found")
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

// isNonRetryableHTTPError reports whether a provider error is a deterministic
// rejection that no amount of retrying the identical request can fix:
// client-side errors (invalid model/params/auth) and gateway "router
// unavailable" 5xxs that reject the request shape rather than signalling a
// transient fault. 408/429 and generic 5xxs remain retryable.
func isNonRetryableHTTPError(err error) bool {
	var he *httpError
	if !errors.As(err, &he) {
		return false
	}
	switch he.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
		http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUnprocessableEntity:
		return true
	}
	return looksLikeRouterUnavailable(he)
}

// shouldSkipModelFallback reports whether an error is a deterministic
// rejection that a cheaper/lower model cannot fix, so the lower-model
// fallback ladder should be skipped entirely. 401/403 (bad or missing API
// key), 400/422 (invalid request shape) and 405 all reject the caller, not
// the model. 404 (model not found) and invalid/deprecated model errors are
// deliberately NOT skipped: falling back to another model in the catalog IS the
// sensible reaction to an unknown model.
func shouldSkipModelFallback(err error) bool {
	if isModelNotFoundOrDeprecated(err) {
		return false
	}
	var he *httpError
	if !errors.As(err, &he) {
		return false
	}
	switch he.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden,
		http.StatusMethodNotAllowed, http.StatusUnprocessableEntity:
		return true
	}
	return false
}

// isTransientError reports whether an error is plausibly transient (rate
// limiting, overload, timeouts) so that a short provider cooldown is a
// sensible reaction. Deterministic rejections (401/403/404, invalid params)
// are NOT transient: cooling the provider down would not help, and the
// caller should simply move on to the next candidate.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	var he *httpError
	if errors.As(err, &he) {
		return he.StatusCode == http.StatusTooManyRequests ||
			he.StatusCode == http.StatusRequestTimeout ||
			he.StatusCode >= 500
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "overloaded") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded")
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
