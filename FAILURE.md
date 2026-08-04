# Failure Report: `wc` Validation Run & LLM Infrastructure Audit

**Date:** 2026-08-04
**Run:** `make validate PROJECT=wc SKIP_BUILD=1` (image `noctifab-validation:wc`, built 48 min prior)
**Container:** `validate-wc-4255`
**Outcome:** STUCK — never completed a user story; stalled on the first LLM call (Product Manager agent roadmap audit).

---

## 1. Final Status Snapshot (T+32m)

| Project | Status | Stuck? | Completion (%) | Tests (Passed/Total) | Current Activity | Elapsed Time | Last Log Activity |
| :--- | :--- | :---: | :---: | :---: | :--- | :---: | :---: |
| `wc` | Stuck ⚠️ | **Yes** | 0% (PM audit phase, no stories started) | 0/0 | OpenCode call timed out after 30m34s; `client.go` retry loop restarted attempt 2/3 (repeating same hung pattern) | 32m | ~1m ago |

---

## 2. Codebase Soundness

### Build & tests: ✅ healthy
- `go build ./...` passes clean.
- `go test ./pkg/... ./tests` — all 10 packages pass (`domain`, `config`, `jira`, `llm`, `storage`, `telemetry`, `tty`, `vcs`, `services`, `tests`).
- All LLM provider files have matching `_test.go` files.

### Architecture: ✅ follows AGENTS.md
- Dispatch is data-driven via `ProviderSpec.NewClientFunc` — **zero protocol `switch` statements** in `client.go`.
- All 22 OpenAI-compatible providers embed `*baseOpenAIClient` and use the declarative `NewModelParser` engine.
- DI/SOLID respected; `ProviderClient` interface keeps providers mockable.

### Issues found

| # | Severity | File | Issue |
|---|---|---|---|
| 1 | Low | `pkg/infrastructure/llm/client.go` (503 lines), `pkg/infrastructure/llm/fallback_test.go` (505 lines) | Exceed the **500-line limit** (AGENTS.md §2.1) by 3–5 lines. Minor, but technically non-compliant. |
| 2 | Low | `README.md:316` vs `pkg/infrastructure/llm/opencode.go:11,18` | **Doc/code mismatch**: README lists OpenCode base URL as `https://opencode.ai/api/v1` but code uses `https://opencode.ai/zen/go/v1`. |
| 3 | Low | `README.md` provider table | Code registers 24 providers; README documents 20. Missing from README: `ai21`, `cerebras`, `nvidia`, `upstage`. |

---

## 3. LLM Providers

### Registrations: ✅
24 providers registered (`openai`, `anthropic`, `gemini`, `opencode`, `moonshot`/`kimi`, `groq`, `openrouter`, `qwen`/`dashscope`, `together`, `llama`/`meta`, `huggingface`, `mistral`, `deepseek`, `hermes`, `ollama`, `xai`/`grok`, `perplexity`, `fireworks`, `sambanova`, `cohere`, plus `ai21`, `cerebras`, `nvidia`, `upstage`).

### Provider implementations: ✅ sound
Gemini and Anthropic have bespoke protocol clients (correct — not OpenAI-compatible); the rest compose `*baseOpenAIClient`. Retry/backoff, rate-limit parsing (`parseRetryDelay`), and credit-exhaustion handling (`ErrCreditExhausted`) are well-structured.

**However — the `wc` run exposed serious resilience bugs (see §4).**

---

## 4. `wc` Validation Failure Analysis

The run **stalled on the very first LLM call** (Product Manager agent auditing the roadmap) and never started a user story. Root causes, in order of severity:

### 🔴 Critical: Layered retry amplification (effective timeout ≈ 105 min/provider)
The OpenCode upstream (`https://opencode.ai/zen/go/v1` → "Console Go" relay to `qwen3.8-max`) hangs without sending response headers. The timeout stacks multiply:
- **OpenAI SDK built-in retries:** noctifab never calls `option.WithMaxRetries`, so the SDK defaults to **2 retries (3 attempts)**. Each attempt waits the full `http.Client.Timeout` (600s). → **3 × 600s = 30m34s** (observed exactly).
- **`client.go` retry loop:** `max_retries: 2` → 3 attempts, repeating the whole cycle (streaming 400 → non-streaming hang).
- **Per-provider worst case ≈ 3 × (5m streaming + 30m non-streaming) ≈ 105 minutes** before the router fails over to OpenRouter.

The log confirms it: after the 30m34s timeout, `client.go` logged `"attempt 1/3. Retrying..."` and re-entered the same streaming→400→non-streaming-hang sequence.

### 🔴 Critical: `idle_timeout` is dead config for OpenAI-compatible providers
The `idle_timeout: 8s` in `validation/projects/wc/.noctifab/config.yaml` is **never enforced** on the OpenAI-SDK paths:
- `baseOpenAIClient.sdkHTTPClient()` (`pkg/infrastructure/llm/openai.go:133`) only uses `o.timeout` (MaxTimeout); `o.idleTimeout` is stored but ignored.
- `sendCompletionStreaming` uses the SDK's `client.Chat.Completions.NewStreaming()`, **not** the custom `readSSEResponse` (`pkg/infrastructure/llm/stream_reader.go`) that actually implements the sliding idle timer.
- So a hung upstream that never sends headers/blocks holds the connection for the *full* MaxTimeout per attempt instead of failing fast at 8s idle.

### 🟠 High: OpenCode provider is unusable for JSON-structured prompts
The `qwen3.8-max` upstream rejects `response_format: json_object` with a 400 unless the literal word "json" appears in the messages. The `looksLikeResponseFormatRejection` fallback (`pkg/infrastructure/llm/openai.go:92`) correctly drops the field and retries, but the upstream then returns **0-byte streamed responses after 5m18s** and hangs on non-streaming. Since noctifab's agent protocol *requires* a JSON envelope, this provider effectively cannot serve the dark factory loop.

### 🟡 Medium: Failover to OpenRouter never reached
Because each OpenCode attempt blocks ~30m and `client.go` retries 3× before surfacing the error to `ResilientLLMRouter`, the OpenRouter candidate (2nd in `llm.priority`) is never tried within a reasonable window. The `max_duration: 30m` story safety net should have fired but the 30m34s SDK timeout completed just before it could.

---

## 5. Key Log Evidence

Excerpt from `validation/projects/wc/output/log/wc.log` (and `docker logs validate-wc-4255`):

```
Pre-flight checks passed successfully.
Spawning Product Manager Agent to audit and refine existing roadmap user stories in ./roadmap...
ℹ [llm] compacted prompt with simple_english: 21921 -> 21915 bytes
⚠ Streaming call failed (HTTP error 400: POST "https://opencode.ai/zen/go/v1/chat/completions":
  400 Bad Request {"param":null,"type":"invalid_request_error","message":"Error from provider (Console Go):
  Upstream request failed: [invalid_parameter_error] <400> InternalError.Algo.InvalidParameter:
  'messages' must contain the word 'json' in some form, to use 'response_format' of type 'json_object'."});
  retrying with non-streaming POST.
⚠ Server rejected response_format; retrying without JSON enforcement.
ℹ [llm] SSE stream for model qwen3.8-max completed: 0 bytes, total=5m18.808431021s
⚠ Streaming call returned empty content; retrying with non-streaming POST.
⚠ [llm] opencode/qwen3.8-max call error after 30m34.629886627s (attempt 1/3):
  Post "https://opencode.ai/zen/go/v1/chat/completions": context deadline exceeded
  (Client.Timeout exceeded while awaiting headers)
⚠ LLM API error: Post "https://opencode.ai/zen/go/v1/chat/completions": context deadline exceeded
  (Client.Timeout exceeded while awaiting headers) (attempt 1/3). Retrying...
⚠ Streaming call failed (HTTP error 400: ...); retrying with non-streaming POST.
⚠ Server rejected response_format; retrying without JSON enforcement.
```

Pre-flight checks confirmed both providers were reachable before the run:
```
- LLM provider (openrouter / openrouter) ping: OK
- LLM provider (opencode / opencode) ping: OK
```

---

## 6. Recommended Fixes (in priority order)

1. **Disable SDK-level retries** — pass `option.WithMaxRetries(0)` in `baseOpenAIClient.sdkClient()` so only `client.go`'s explicit retry loop controls retries (no compounding). This alone collapses the 30m34s hung-call timeout back to the intended 600s.
2. **Wire `idle_timeout` into the SDK paths** — wrap the SDK call's context with `context.WithTimeout` based on `idleTimeout`, or switch streaming to the custom `readSSEResponse` which already implements the sliding idle timer correctly. This makes a hung upstream fail fast at 8s instead of 600s.
3. **Reorder `llm.priority`** in `validation/projects/wc/.noctifab/config.yaml` to put `openrouter` first (it pinged OK and serves a JSON-capable model), or drop `opencode` until the upstream stabilizes.
4. **Fix the 3 doc/size nits** — update README OpenCode URL (`https://opencode.ai/zen/go/v1`); document the 4 missing providers (`ai21`, `cerebras`, `nvidia`, `upstage`); trim `client.go` and `fallback_test.go` under 500 lines.

---

## 7. State at Report Time

- The validation container `validate-wc-4255` was still running (Up 32 min), stuck on `client.go` retry attempt 2/3 against the hung OpenCode upstream.
- No artifacts produced: `output/src/`, `output/dist/`, and `output/feedback/` are empty; no `WC_FEEDBACK.md` generated.
- Noctifab source code itself is architecturally sound and all unit tests pass — the failure is operational (resilience-layer timeouts vs. a misbehaving upstream), not a structural defect.
