# LLM Provider Evaluation & Guidelines

This document synthesizes empirical benchmarks, structural evaluations, and operational best practices for LLM providers integrated into Noctifab dark factory execution pipelines.

---

## 1. Overview & Architecture

Noctifab relies on a stateless LLM agent architecture controlled by a stateful orchestrator. Role-based routing ([pkg/infrastructure/llm/router.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/router.go)) dynamically selects providers based on agent assignments (`product_manager`, `planner`, `architect`, `generators`, `testers`, `unblocker`).

Key provider evaluation criteria:
1. **JSON Envelope & Schema Compliance**: Ability to strictly format tool calls matching Noctifab's JSON action envelope (`{"actions": [{"tool": "...", "args": {...}}]}`).
2. **Streaming Stability (SSE)**: Maintenance of persistent Server-Sent Event HTTP connections without premature EOF or chunk drops.
3. **Completion Latency**: Time-to-first-token and total completion time for large code generation prompts.
4. **Thinking Budget Overrides**: Support for disabling reasoning budgets on fast tool iterations.

---

## 2. Provider Evaluation Summary

| Provider | Key Model | Format Accuracy | SSE Streaming Stability | Average Latency | Overall Rating | Recommended Role |
| :--- | :--- | :---: | :---: | :---: | :---: | :--- |
| **QwenCloud** | `qwen3.8-max` | 100% ✅ | High (Persistent SSE) | 15s – 45s (Fast) | ⭐️⭐️⭐️⭐️⭐️ **(Optimal)** | **Primary (#1)** for all agents |
| **OpenRouter** | `deepseek-v4-flash-0731`, `claude-3.5-sonnet` | 95%+ ✅ | High (Standard OpenAI REST) | 3m – 5m (Model-dependent) | ⭐️⭐️⭐️⭐️ (Reliable) | **Secondary (#2)** / Failover |
| **OpenCode** | `glm-5.2` (Zen Gateway) | 40% ⚠️ (Needs retries) | Low (Frequent EOF drops) | 2m – 5m (POST fallback) | ⭐️⭐️ (High friction) | Experimental / Offline |

---

## 3. Detailed Provider Insights

### QwenCloud (`qwencloud`)
* **Endpoint / API**: DashScope compatible OpenAI endpoint.
* **Format Adherence**: Superior. Strictly generates valid JSON envelopes with opening `{` and correct key types (`"tool"`, `"args"`).
* **Streaming Performance**: Stable SSE streams; handles inter-chunk idle timeouts reliably.
* **Per-Agent Overrides**: Supports `enable_thinking` and `thinking_budget` configuration. Setting `enable_thinking: false` for generator and tester roles drops turn latency to ~2 seconds per tool call.
* **Verdict**: **Recommended #1 Primary Provider** for production dark factory execution.

### OpenRouter (`openrouter`)
* **Endpoint / API**: Standard OpenAI-compatible API (`https://openrouter.ai/api/v1`).
* **Format Adherence**: High. Models on OpenRouter (DeepSeek, Claude, Qwen Coder) follow structured tool calling schemas cleanly.
* **Latency**: Reasoning models and pinned snapshots (e.g. `deepseek/deepseek-v4-flash-0731`) exhibit higher latency (~3 to 5 minutes per turn), especially during peak API queue load.
* **Verdict**: **Recommended #2 Secondary / Fallback Provider**. Excellent fallback when primary API keys hit daily rate caps.

### OpenCode (`opencode`)
* **Endpoint / API**: OpenCode Zen Go proxy gateway (`glm-5.2`).
* **Format Adherence**: Frequently emits reasoning content in `reasoning_content` without root JSON braces `{}` or uses non-standard key aliases (`"cmd"` instead of `"tool"`).
* **Streaming Performance**: Frequently drops SSE connections (`unexpected end of JSON input`), triggering Noctifab fallback to non-streaming POST requests that take 2 to 5 minutes or hang.
* **Verdict**: **Not recommended as primary provider** due to streaming drops and schema formatting retry overhead.

---

## 4. Recommended Configuration Pattern

To maximize dark factory build speed and avoid format retries, use the following priority structure in `.noctifab/config.yaml`:

```yaml
llm:
  priority:
    - qwencloud
    - openrouter
    - opencode

  providers:
    - name: qwencloud
      provider: qwencloud
      model: qwen3.8-max
      api_keys: QWENCLOUD_API_KEY
      max_retries: 2
      retry_backoff: 50ms
      max_timeout: 600s
      idle_timeout: 600s
      max_tokens: 32768
      temperature: 0.3
      streaming: true
      enable_thinking: true
      thinking_budget: 8192

    - name: openrouter
      provider: openrouter
      model: deepseek/deepseek-v4-flash-0731
      api_keys: OPENROUTER_API_KEY
      max_retries: 2
      retry_backoff: 50ms
      max_timeout: 600s
      idle_timeout: 600s
      max_tokens: 32768
      temperature: 0.3
      streaming: true

agents:
  generators:
    number: 4
    iterations: 5
    providers:
      - name: qwencloud
        enable_thinking: false  # Drop completion latency to ~2s for code writers
      - name: openrouter
  testers:
    number: 2
    iterations: 3
    providers:
      - name: qwencloud
        enable_thinking: false
      - name: openrouter
```

---

## 5. Built-in Noctifab Resilience Mechanisms

To defend against LLM provider anomalies, Noctifab implements the following engine protections:
1. **Planner Story Retry Loop**: `PlanStory` retries story task DAG decomposition up to 3 times if an LLM returns malformed or empty task lists.
2. **Action JSON Alias Parsing**: `Action.UnmarshalJSON` automatically maps provider key aliases (`"cmd"`, `"name"`, `"command"`) to `Action.Tool`.
3. **UnblockerAgent Stall Recovery**: Detects frozen worker tasks (> 5 minutes without log activity) and automatically resets task status to `PENDING` for clean re-dispatch.
