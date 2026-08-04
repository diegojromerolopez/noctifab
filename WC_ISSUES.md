# WC_ISSUES.md — Issues Found Running the `wc` Validation Project

Findings from running `make validate PROJECT=wc` repeatedly (5 runs) on
2026-08-04, including direct HTTP reproduction against the OpenCode Zen Go
gateway (`https://opencode.ai/zen/go/v1`) with a standalone Go harness that
mimics noctifab's request shape.

Issues are grouped by layer: validation config, the LLM gateway/models,
noctifab code, and the validation harness. Each issue includes a fix proposal.

---

## A. Validation project configuration issues

### A1. `claude-fable-5` is not a valid model on the zen go gateway

**Observed (run 1):** every call to the `opencode-zen` provider failed with
`HTTP 401 {"type":"ModelError","message":"Model claude-fable-5 is not supported"}`,
retried 3× per attempt despite being deterministic.

**Root cause:** the model does not exist in the gateway catalog
(`GET /models` lists: minimax-m3/m2.7/m2.5, kimi-k3/k2.7-code/k2.6/k2.5,
glm-5.2/5.1/5, deepseek-v4-pro/flash, qwen3.8-max/3.7-max/3.7-plus/3.6-plus/
3.5-plus, mimo-v2.5-pro/v2.5/v2-pro/v2-omni, hy3, hy3-preview, gpt-5.6-luna,
grok-4.5).

**Fix (applied):** switched `opencode-zen` to a cataloged model.
**Fix (proposal, noctifab):** validate configured model names against
`GET /models` during pre-flight and fail fast with the available list.

### A2. `idle_timeout: 8s` made every streaming call fail

**Observed (all runs):** `⚠ Streaming call failed (context deadline exceeded);
retrying with non-streaming POST.` on virtually every LLM call.

**Root cause:** two compounding problems:
1. noctifab bug: `openai.go:275-282` applies `idle_timeout` as a *total*
   stream deadline (see C1), so any completion > 8s was killed.
2. Even with a correct sliding idle timer, 8s is too aggressive: a measured
   glm-5.2 stream had a **12.3s max inter-chunk gap** on a 22KB prompt.

**Fix (applied):** raised `idle_timeout` to 600s in the wc config.
**Fix (proposal):** default validation configs should use idle timeouts of
60s+; fix C1 in code so idle means idle.

### A3. Pinned OpenRouter model is extremely slow

**Observed (runs 2–3):** `deepseek/deepseek-v4-flash-0731` completions took
**~5 minutes each** (`call OK after 5m1s`). Other validation projects use the
`-latest` alias.

**Fix (proposal):** use `deepseek/deepseek-v4-flash-latest`, or a different
fallback provider, and demote openrouter to last priority (applied).

---

## B. Gateway / model issues (opencode.ai zen go)

Confirmed with a standalone Go reproduction reading the API key from
`secrets.yaml` programmatically (never printed). Auth was verified OK — all
errors below are post-auth, model-level failures.

### B1. kimi-k3: `Router.Unavailable` 500 whenever `max_tokens` or `response_format` is present

| Request shape | Result |
|---|---|
| `response_format` and/or `max_tokens` set | `HTTP 500 {"type":"Router.Unavailable","modelID":"kimi-k3"}` |
| Bare minimal request | HTTP 200 |

noctifab always sends both fields, so **100% of noctifab calls to kimi-k3
fail** (observed: 18× HTTP 500 in run 2). Each failure costs 6 requests
(3 retries × streaming+non-streaming).

**Fix (applied):** removed kimi-k3 from the config.
**Fix (proposal, noctifab):** treat `Router.Unavailable` as a non-retryable
model-level error → skip to the next model immediately; optionally retry once
with `response_format`/`max_tokens` stripped (mirroring the existing
`looksLikeResponseFormatRejection` fallback).

### B2. glm-5.2 + `response_format: json_object` → content returned in `reasoning_content`, `content` empty

Reproduced deterministically:

```json
"message": {"content": "", "reasoning_content": "{\"greeting\":\"hello\"}"}
```

noctifab only reads `message.content` (and streaming `delta.content`), so it
logs `contentLen=0`, fails JSON-envelope parsing, and burns a "one-shot format
reminder" retry per call. Observed repeatedly in runs 3 and 5. Without
`response_format`, glm-5.2 returns a fenced ```json block in `content`
(parseable) plus verbose reasoning.

**Fix (proposal, noctifab):** in `openai.go`, fall back to
`reasoning_content` (message and stream-delta level) when `content` is empty.
This is a one-line accumulator change with broad benefit for reasoning-style
models behind OpenAI-compatible relays.

### B3. qwen3.8-max upstream enforces "prompt must contain the word 'json'" for `response_format`

**Observed (runs 4–5):**
`400 ... 'messages' must contain the word 'json' in some form, to use
'response_format' of type 'json_object'.`

noctifab's internal prompts request a "JSON envelope" but at least the PM
prompt evidently doesn't contain the literal token needed by the upstream
(DashScope-style rule from the OpenAI spec). The built-in rf-rejection
fallback recovers, but costs one failed round-trip per call.

**Fix (proposal, noctifab):** when `enforceJSON` is set, guarantee the word
"json" appears in the outgoing prompt (e.g., append a one-line "Respond in
JSON." suffix). Trivial and spec-compliant.

### B4. qwen3.8-max: extreme verbosity → streams exceed 600s; non-streaming hangs

- The gateway **never returns headers** for long non-streaming completions:
  a PM-sized request hung for the full 10-minute client timeout
  (`context deadline exceeded (Client.Timeout exceeded while awaiting headers)`).
- Streaming works (probe: first chunk in 2.1s, steady 1.1s max gap) but qwen
  is **very verbose**: 208KB of stream for a trivial summarization ask
  (vs. 96KB for glm-5.2, which finished 2.3× faster). Real PM prompts push
  qwen streams past even a 600s total deadline, where they are killed by bug
  C1. One 8-minute qwen SSE stream also completed with **0 bytes** of content.

**Fix (applied):** deprioritized qwen behind glm-5.2.
**Fix (proposal, noctifab):** fix C1 so long streams aren't killed; prefer
streaming universally for slow gateways (non-streaming "awaiting headers"
hangs are unrecoverable); cap effective `max_tokens` per role (a PM refinement
does not need a 32K-token budget — verbose models will happily use it).

### B5. Transient empty-body 500s on glm-5.2

Occasional `500 Internal Server Error {"type":"error","message":"Internal
server error"}`; the retry ladder recovered (`call OK after 2m43s (attempt 2)`).
Acceptable, but each retry doubles as streaming+non-streaming (C2).

---

## C. noctifab code issues surfaced by this run

### C1. `idle_timeout` implemented as a total-stream deadline (critical)

`pkg/infrastructure/llm/openai.go:275-282` wraps the entire SDK stream in
`context.WithTimeout(ctx, idleTimeout)`. The comment claims it only guards
time-to-first-byte; in reality it kills any stream whose *total* duration
exceeds the value. Every long completion is aborted and re-executed
non-streaming — doubling latency and token cost — and on this gateway the
non-streaming fallback can hang for 10 minutes awaiting headers (B4),
turning one call into a ~20-minute stall. This single bug interacted with
B2/B3/B4 to consume entire 30-minute run budgets in the PM phase.

**Fix:** implement a true sliding inter-chunk idle timer (one already exists
in `pkg/infrastructure/llm/stream_reader.go:42-74`; use it on the SDK path).

### C2. Deterministic errors retried through the full ladder

401 `ModelError` (A1), 400 temperature (below), and 500 `Router.Unavailable`
(B1) were each retried 3× per attempt, and each attempt is doubled by the
streaming→non-streaming fallback. A single misconfigured model costs ~6
wasted requests per LLM call, every cycle.

**Fix:** classify 400/401/403/404 and `Router.Unavailable`-style 500s as
non-retryable at the model level in `client.go`'s retry loop; move to the
next model/provider immediately.

### C3. Model-specific parameter constraints are not handled

`kimi-k3` rejects any `temperature != 1`
(`invalid temperature: only 1 is allowed for this model`). noctifab applies
one global temperature to every model, including those reached via the
dynamic lower-model fallback ladder — so the ladder itself can walk into a
deterministically failing model (run 1: claude-fable-5 → kimi-k3 with
temp 0.3 → hard 400 loop).

**Fix:** on a 400 mentioning an invalid parameter, retry once with the
offending parameter removed/defaulted; and don't inherit request params
blindly when the fallback ladder switches models.

### C4. Empty-content responses are not detected before JSON parsing

`contentLen=0` responses (B2) flow into envelope parsing, produce a generic
"JSON envelope not detected" message, and trigger a format-reminder retry
that fails the same way. The agent loop burns turns without ever surfacing
the actual cause (content in `reasoning_content`).

**Fix:** treat empty content as a distinct error class; check
`reasoning_content` first (B2 fix); log the raw finish_reason/usage to aid
diagnosis.

### C5. Unblocker reset an actively-working task (false positive)

Run 3: `🔧 [UnblockerAgent] Resetting task task-a0a0440d: Task stalled with
frozen progress at 50% ...` — while the task's LLM call was legitimately in
flight (calls take minutes due to C1 doubling + slow models). The reset threw
away in-progress work; the task later re-ran and passed.

**Fix:** the unblocker should consider LLM-call-in-flight as activity (e.g.,
last-LLM-request timestamp), not just persisted progress percentage; or scale
its stall threshold with observed provider latency.

### C6. Misleading "3x Consensus" log

Run 3: `✅ [3x Consensus Passed] Task task-a0a0440d ... passed 3x test
validation` — but `TestValidator.ValidateTask` runs the suite **once**
(`runWithCount(ctx, state, 1)`, `pkg/services/test_validator.go:78`).
The log overstates the verification actually performed.

**Fix:** either run the advertised 3 iterations (with the flaky detector) or
fix the log message.

### C7. PM "story refinement" silently degraded

Run 3: `Warning: Product Manager Agent story refinement skipped: roadmap
generation failed: LLM did not return any valid create_story actions` after
~10 minutes of PM-phase LLM calls. The run continued with the checked-in
roadmap fixtures (acceptable per the harness rules), but 1/3 of the run
budget was spent producing nothing.

**Fix:** cap PM-phase wall-clock/attempts; fall back to existing roadmap
faster when refinement responses repeatedly fail envelope parsing.

### C8. Prompt "compaction" is a no-op in practice

Every cycle logs `compacted prompt with simple_english: 21921 -> 21915 bytes`
— a 6-byte (0.03%) reduction, paid for on every call. The `context.compaction:
simple_english` mode adds no value for this workload.

**Fix:** skip compaction when projected savings are below a threshold; or
implement real compaction (section dedup/summarization) for the PM prompt.

---

## D. Validation harness issues

### D1. Project config is baked into the Docker image; `SKIP_BUILD=1` silently runs stale config

`validation/projects/wc/Dockerfile` copies `/app/projects` from the base
image at build time. After editing
`validation/projects/wc/.noctifab/config.yaml` on the host,
`make validate PROJECT=wc SKIP_BUILD=1` launched a container still containing
the **old** config (verified via `docker exec ... grep`), reproducing
already-fixed errors and wasting a full run.

**Fix:** bind-mount the project's `.noctifab/config.yaml` (and `SPEC.md`,
`roadmap/`) into the container at runtime, like `secrets.yaml` already is; or
have `run_one.sh` hash the project directory and refuse `SKIP_BUILD` when it
differs from the image.

### D2. A full rebuild is required for any config iteration

Consequence of D1: every config tweak costs a multi-minute base+project image
rebuild. During iterative debugging this dominated wall-clock time.

**Fix:** same as D1 (runtime mounts).

### D3. No per-line timestamps in `wc.log`

The captured container log has no timestamps, making stall attribution
(e.g., the 10-minute "awaiting headers" hang) require external `stat` polling.

**Fix:** prefix log lines with timestamps in the harness (`ts` filter) or in
noctifab's logger.

---

## Run chronology (evidence summary)

| Run | Config | Outcome |
|---|---|---|
| 1 | `claude-fable-5` (zen), temp 0.3 | 401 ModelError loops (A1); ladder fell into kimi-k3 → 400 temperature loops (C3); openrouter succeeded at ~80s/call; stopped manually |
| 2 | `kimi-k3`, temp 1 — **stale image** (D1) | Re-ran run-1 config by accident; then rebuilt: 18× `Router.Unavailable` 500 (B1); openrouter at ~5min/call (A3); stopped |
| 3 | `glm-5.2` primary | Real progress: roadmap enqueued (4 stories), US-001 first task completed and validated; contentLen=0 retries (B2), unblocker false reset (C5); stopped manually |
| 4 | qwen first, idle 8s→600s mid-run | qwen rf-rejection (B3), non-streaming hang 10min awaiting headers (B4/C1); stuck; stopped |
| 5 | glm first, idle 600s | PM phase churning through B2/B3 retries; qwen fallback produced an 8-min 0-byte stream (B4) |

## Priority of fixes

1. **C1** — sliding idle timer (unblocks everything; every other issue is
   amplified by the doubled calls and killed streams).
2. **C2** — stop retrying deterministic errors (fail over in seconds, not
   minutes).
3. **B2 fix in code** — read `reasoning_content` fallback (makes glm-5.2, the
   best available model on this gateway, fully usable).
4. **B3 fix in code** — ensure the word "json" is present when enforcing
   JSON output (removes one failed round-trip per call on qwen-family).
5. **D1** — runtime-mount project config into validation containers.
6. **C5** — unblocker stall detection based on in-flight LLM activity.
