# LLM Provider Reference

This document covers all supported LLM providers, their configuration options, environment variables, API key setup, model capacity ranking, and dynamic fallback behaviour.

> [!TIP]
> noctifab fetches the live model list from each provider's `/models` endpoint at runtime. **Never hardcode model names in config if you want automatic fallback** — use the provider's recommended flagship model as `llm.model` and let the fallback engine handle the rest.

---

## Quick Reference

| Provider | `llm.provider` value | Environment Variable | Default Base URL | Protocol |
|---|---|---|---|---|
| OpenAI | `openai` | `OPENAI_API_KEY` | `https://api.openai.com/v1` | OpenAI |
| Anthropic | `anthropic` | `ANTHROPIC_API_KEY` | `https://api.anthropic.com/v1` | Anthropic |
| Google Gemini | `gemini` | `GEMINI_API_KEY` | `https://generativelanguage.googleapis.com/v1beta` | Gemini |
| OpenCode | `opencode` | `OPENCODE_API_KEY` | `https://opencode.ai/api/v1` | OpenAI-compat |
| Kimi / Moonshot | `kimi`, `moonshot` | `KIMI_API_KEY`, `MOONSHOT_API_KEY` | `https://api.moonshot.ai/v1` | OpenAI-compat |
| Groq | `groq` | `GROQ_API_KEY` | `https://api.groq.com/openai/v1` | OpenAI-compat |
| OpenRouter | `openrouter` | `OPENROUTER_API_KEY` | `https://openrouter.ai/api/v1` | OpenAI-compat |
| Qwen / QwenCloud / DashScope | `qwencloud`, `qwen`, `dashscope` | `QWENCLOUD_API_KEY`, `DASHSCOPE_API_KEY`, `QWEN_API_KEY` | `https://dashscope-intl.aliyuncs.com/compatible-mode/v1` | OpenAI-compat |
| Together AI | `together` | `TOGETHER_API_KEY` | `https://api.together.xyz/v1` | OpenAI-compat |
| Meta Llama | `llama`, `meta` | `LLAMA_API_KEY`, `META_API_KEY` | `https://api.together.xyz/v1` | OpenAI-compat |
| HuggingFace | `huggingface` | `HUGGINGFACE_API_KEY` | `https://api-inference.huggingface.co/v1` | OpenAI-compat |
| Mistral | `mistral` | `MISTRAL_API_KEY` | `https://api.mistral.ai/v1` | OpenAI-compat |
| DeepSeek | `deepseek` | `DEEPSEEK_API_KEY` | `https://api.deepseek.com/v1` | OpenAI-compat |
| Hermes (Nous Research) | `hermes` | `HERMES_API_KEY` | `https://api.together.xyz/v1` | OpenAI-compat |
| Ollama | `ollama` | `OLLAMA_API_KEY` *(optional)* | `http://localhost:11434/v1` | OpenAI-compat |
| xAI / Grok | `xai`, `grok` | `XAI_API_KEY`, `GROK_API_KEY` | `https://api.x.ai/v1` | OpenAI-compat |
| Perplexity | `perplexity` | `PERPLEXITY_API_KEY` | `https://api.perplexity.ai` | OpenAI-compat |
| Fireworks AI | `fireworks` | `FIREWORKS_API_KEY` | `https://api.fireworks.ai/inference/v1` | OpenAI-compat |
| SambaNova | `sambanova` | `SAMBANOVA_API_KEY` | `https://api.sambanova.ai/v1` | OpenAI-compat |
| Cohere | `cohere` | `COHERE_API_KEY`, `CO_API_KEY` | `https://api.cohere.com/v2` | OpenAI-compat |
| Cerebras | `cerebras` | `CEREBRAS_API_KEY` | `https://api.cerebras.ai/v1` | OpenAI-compat |
| NVIDIA NIM | `nvidia` | `NVIDIA_API_KEY` | `https://integrate.api.nvidia.com/v1` | OpenAI-compat |
| AI21 Labs | `ai21` | `AI21_API_KEY` | `https://api.ai21.com/studio/v1` | OpenAI-compat |
| Upstage | `upstage` | `UPSTAGE_API_KEY` | `https://api.upstage.ai/v1/solar` | OpenAI-compat |

---

## Named Provider Registries & Per-Agent Model Routing

`noctifab` supports declaring a named registry of LLM providers (`llm.providers`), setting a global failover priority list (`llm.priority`), and overriding provider priority chains per agent role (`roles.<agent>.providers`).

### Configuration Syntax

```yaml
llm:
  # Global Default Failover Priority Chain
  priority:
    - "openai-primary"
    - "anthropic-backup"
    - "deepseek-coder"

  # Named Provider Registry
  providers:
    - name: "openai-primary"
      provider: "openai"
      api_keys: "OPENAI_API_KEY"

    - name: "anthropic-backup"
      provider: "anthropic"
      api_keys: "ANTHROPIC_API_KEY"
      model: "claude-3-5-sonnet-latest"

    - name: "deepseek-coder"
      provider: "deepseek"
      api_keys: "DEEPSEEK_API_KEY"
      model: "deepseek-coder"

# Per-Agent Priority Overrides directly inside agents:
agents:
  generators:
    number: 4
    iterations: 5
    providers:
      - name: "deepseek-coder"
      - name: "openai-primary"

  testers:
    number: 2
    iterations: 3
    providers:
      - name: "openai-primary"
      - name: "anthropic-backup"

  qa:
    number: 1
    iterations: 2
    providers:
      - name: "anthropic-backup"
      - name: "openai-primary"
```

---

## Provider Details & Config Examples

---

### OpenAI

**Models**: `gpt-4o`, `gpt-4o-mini`, `gpt-4-turbo`, `gpt-3.5-turbo`
**Fallback chain**: `gpt-4o` → `gpt-4o-mini` → `gpt-3.5-turbo`
**Ranking**: tier keyword (`o1`, `flagship`, `o-mini`, `o-mini-high`, `gpt-4o`, `mini`, `turbo`, `instruct`, `lite`) + version multiplier.

```yaml
# .noctifab/secrets.yaml
OPENAI_API_KEY: "sk-proj-..."

# .noctifab/config.yaml
llm:
  provider: "openai"
  model: "gpt-4o"
  api_key: "secret:OPENAI_API_KEY"
  max_retries: 3
  streaming: true
```

**Multi-backend failover (OpenAI → Anthropic):**
```yaml
llms:
  - provider: "openai"
    model: "gpt-4o"
    api_key: "secret:OPENAI_API_KEY"
  - provider: "anthropic"
    model: "claude-3-5-sonnet-latest"
    api_key: "secret:ANTHROPIC_API_KEY"
```

---

### Anthropic (Claude)

**Models**: `claude-3-opus-*`, `claude-3-5-sonnet-*`, `claude-3-5-haiku-*`, `claude-3-7-sonnet-*`
**Fallback chain**: `opus` → `sonnet` → `haiku`
**Ranking**: tier keyword (`opus` > `sonnet` > `haiku`) + version multiplier × 10.

```yaml
# .noctifab/secrets.yaml
ANTHROPIC_API_KEY: "sk-ant-api03-..."

# .noctifab/config.yaml
llm:
  provider: "anthropic"
  model: "claude-3-5-sonnet-latest"
  api_key: "secret:ANTHROPIC_API_KEY"
  max_retries: 3
  streaming: true
```

> [!TIP]
> **Prompt Caching Support**: `noctifab` automatically includes Anthropic prompt caching beta headers (`anthropic-beta: prompt-caching-2024-07-31`) and attaches ephemeral cache markers (`cache_control: {"type": "ephemeral"}`) for payloads larger than 2,048 bytes, reducing input token processing by up to 90% and dramatically accelerating multi-turn task loops.

---

### Google Gemini

**Models**: `gemini-3.6-pro`, `gemini-3.6-flash`, `gemini-2.5-pro`, `gemini-2.5-flash`, `gemini-1.5-pro`, `gemini-1.5-flash`
**Fallback chain**: `3.6-pro` → `3.6-flash` → `2.5-pro` → `2.5-flash` → `1.5-pro` → `1.5-flash`
**Ranking**: model family weight + version × 5. Uses the Gemini-specific `sortGeminiModels` with `GeminiModelInfo.Rank`.

```yaml
# .noctifab/secrets.yaml
GEMINI_API_KEY: "AIzaSy..."

# .noctifab/config.yaml
llm:
  provider: "gemini"
  model: "gemini-3.6-pro"
  api_key: "secret:GEMINI_API_KEY"
  max_retries: 3
  streaming: true
```

---

### OpenCode

**Models**: returned live from `https://opencode.ai/api/v1/models`
**Ranking**: OpenAI-compatible tier keywords.

```yaml
# .noctifab/secrets.yaml
OPENCODE_API_KEY: "oc-..."

# .noctifab/config.yaml
llm:
  provider: "opencode"
  model: "opencode-latest"
  api_key: "secret:OPENCODE_API_KEY"
```

---

### Kimi / Moonshot AI

**Models**: `kimi-k3`, `kimi-k2.7`, `kimi-k2.6`, `kimi-k2.5`, `kimi-k2`
**Fallback chain**: `kimi-k3` → `kimi-k2.7` → `kimi-k2.5`
**Ranking**: generation number extracted from `k<N>` suffix × 10.

```yaml
# .noctifab/secrets.yaml
KIMI_API_KEY: "..."

# .noctifab/config.yaml
llm:
  provider: "kimi"
  model: "kimi-k3"
  api_key: "secret:KIMI_API_KEY"
  streaming: true
```

> Both `kimi` and `moonshot` are accepted as provider names and map to the same client.

---

### Groq

**Models**: `llama-3.3-70b-versatile`, `llama-3.1-8b-instant`, `gemma2-9b-it`, `mixtral-8x7b-32768`
**Fallback chain**: size-based (`70b` → `8b`) or tier-based.
**Ranking**: `StandardSizeWeights` parameter count ranking.

```yaml
# .noctifab/secrets.yaml
GROQ_API_KEY: "gsk_..."

# .noctifab/config.yaml
llm:
  provider: "groq"
  model: "llama-3.3-70b-versatile"
  api_key: "secret:GROQ_API_KEY"
  streaming: true
```

---

### OpenRouter

**Models**: unified access to 200+ models from all providers. Model names use `provider/model` format (e.g. `anthropic/claude-3-5-sonnet`, `google/gemini-2.5-pro`).
**Ranking**: `StandardSizeWeights` + tier keywords across the unified catalog.

```yaml
# .noctifab/secrets.yaml
OPENROUTER_API_KEY: "sk-or-v1-..."

# .noctifab/config.yaml
llm:
  provider: "openrouter"
  model: "anthropic/claude-3-5-sonnet"
  api_key: "secret:OPENROUTER_API_KEY"
  streaming: true
```

---

### Qwen / QwenCloud / DashScope (Alibaba Cloud)

**Models**: `qwen3.8-max`, `qwen-max`, `qwen-plus`, `qwen-turbo`, `qwen-long`, `qwen-coder-plus`
**Fallback chain**: `qwen3.8-max` → `qwen-max` → `qwen-plus` → `qwen-turbo`
**Ranking**: tier keyword (`max` > `plus` > `turbo` > `long`) × base score.

#### Configuration Example (Standard & Thinking Models)

```yaml
# .noctifab/secrets.yaml
QWENCLOUD_API_KEY: "sk-..."

# .noctifab/config.yaml
llm:
  priority:
    - "qwencloud-max"
  providers:
    - name: "qwencloud-max"
      provider: "qwencloud"
      model: "qwen3.8-max"
      api_keys: "QWENCLOUD_API_KEY"
      streaming: true
      enable_thinking: true
      thinking_budget: 8192
```

> **Thinking Mode Support (`enable_thinking`, `thinking_budget`)**:
> - `enable_thinking: true` activates Qwen's chain-of-thought reasoning mode (sending `enable_thinking: true` in the API request body).
> - `thinking_budget`: Caps the reasoning token budget (e.g. `8192`).
> - When `enable_thinking: true` is configured, `noctifab` automatically bypasses `response_format: json_object` (which is incompatible with DashScope thinking traces) and relies on `ExtractJSONBlock` to extract clean JSON payloads from reasoning streams (`<think>...</think>`).
> - Valid provider alias names: `qwencloud` (uses `https://dashscope-intl.aliyuncs.com/compatible-mode/v1`), `qwen`, and `dashscope`.

---

### Together AI

**Models**: broad open-weight catalog — `meta-llama/Llama-3.3-70B-Instruct-Turbo`, `mistralai/Mistral-7B-Instruct-v0.3`, etc.
**Ranking**: `StandardSizeWeights` parameter count ranking.

```yaml
# .noctifab/secrets.yaml
TOGETHER_API_KEY: "..."

# .noctifab/config.yaml
llm:
  provider: "together"
  model: "meta-llama/Llama-3.3-70B-Instruct-Turbo"
  api_key: "secret:TOGETHER_API_KEY"
  streaming: true
```

---

### Meta Llama (via Together AI)

**Models**: `Llama-3.1-405B-Instruct`, `Llama-3.3-70B-Instruct`, `Llama-3.1-8B-Instruct`
**Fallback chain**: `405B` → `70B` → `8B`
**Ranking**: `StandardSizeWeights` (405b→500, 70b→400, 8b→200).

```yaml
# .noctifab/secrets.yaml
LLAMA_API_KEY: "..."

# .noctifab/config.yaml
llm:
  provider: "llama"
  model: "Llama-3.1-405B-Instruct"
  api_key: "secret:LLAMA_API_KEY"
  streaming: true
```

> Both `llama` and `meta` are accepted as provider names.

---

### HuggingFace Inference API

**Models**: any model hosted on HuggingFace Hub (e.g. `meta-llama/Meta-Llama-3.1-70B-Instruct`).
**Ranking**: `StandardSizeWeights` extracted from model name.

```yaml
# .noctifab/secrets.yaml
HUGGINGFACE_API_KEY: "hf_..."

# .noctifab/config.yaml
llm:
  provider: "huggingface"
  model: "meta-llama/Meta-Llama-3.1-70B-Instruct"
  api_key: "secret:HUGGINGFACE_API_KEY"
  streaming: true
```

---

### Mistral AI

**Models**: `mistral-large-latest`, `mistral-small-latest`, `codestral-latest`, `open-mistral-nemo`
**Fallback chain**: `mistral-large` → `mistral-small`
**Ranking**: tier keyword (`large`, `codestral` > `medium` > `small`, `nemo`) × base score.

```yaml
# .noctifab/secrets.yaml
MISTRAL_API_KEY: "..."

# .noctifab/config.yaml
llm:
  provider: "mistral"
  model: "mistral-large-latest"
  api_key: "secret:MISTRAL_API_KEY"
  streaming: true
```

---

### DeepSeek

**Models**: `deepseek-r1`, `deepseek-v3`, `deepseek-coder`, `deepseek-chat`
**Fallback chain**: `deepseek-r1` / `deepseek-coder` → `deepseek-chat`
**Ranking**: tier keyword (`r1`, `v3`, `coder` > `chat`).

```yaml
# .noctifab/secrets.yaml
DEEPSEEK_API_KEY: "sk-..."

# .noctifab/config.yaml
llm:
  provider: "deepseek"
  model: "deepseek-r1"
  api_key: "secret:DEEPSEEK_API_KEY"
  streaming: true
```

---

### Hermes (Nous Research via Together AI)

**Models**: `hermes-3-llama-3.1-405b`, `hermes-3-llama-3.1-70b`
**Fallback chain**: `405b` → `70b`
**Ranking**: `StandardSizeWeights`.

```yaml
# .noctifab/secrets.yaml
HERMES_API_KEY: "..."

# .noctifab/config.yaml
llm:
  provider: "hermes"
  model: "hermes-3-llama-3.1-405b"
  api_key: "secret:HERMES_API_KEY"
```

---

### Ollama (Self-Hosted)

**Models**: any model pulled locally via `ollama pull`, e.g. `llama3.1:70b`, `llama3.1:8b`, `qwen2.5-coder:32b`.
**Fallback chain**: size-based (`70b` → `8b`).
**Ranking**: `StandardSizeWeights` extracted from the `:tag` suffix.

```yaml
# No API key needed for local Ollama.
# .noctifab/config.yaml
llm:
  provider: "ollama"
  model: "llama3.1:70b"
  url: "http://localhost:11434/v1"  # override if Ollama runs on a custom host/port
  streaming: true
```

> To use a remote Ollama instance with authentication:
> ```yaml
> llm:
>   provider: "ollama"
>   model: "llama3.1:70b"
>   url: "https://my-ollama-host.example.com/v1"
>   api_key: "secret:OLLAMA_API_KEY"
> ```

---

### xAI / Grok

**Models**: `grok-3`, `grok-3-mini`, `grok-2`, `grok-2-mini`, `grok-beta`
**Fallback chain**: `grok-3` → `grok-2` → `grok-3-mini` → `grok-2-mini`
**Ranking**: version number × 25 + mini penalty.

```yaml
# .noctifab/secrets.yaml
XAI_API_KEY: "xai-..."

# .noctifab/config.yaml
llm:
  provider: "xai"
  model: "grok-3"
  api_key: "secret:XAI_API_KEY"
  streaming: true
```

> Both `xai` and `grok` are accepted as provider names.

---

### Perplexity

**Models**: `sonar-deep-research`, `sonar-reasoning-pro`, `sonar-reasoning`, `sonar-pro`, `sonar`
**Fallback chain**: `sonar-deep-research` → `sonar-reasoning-pro` → `sonar-reasoning` → `sonar-pro` → `sonar`
**Ranking**: tier keyword (`deep-research` > `reasoning-pro` > `reasoning` > `pro`).

```yaml
# .noctifab/secrets.yaml
PERPLEXITY_API_KEY: "pplx-..."

# .noctifab/config.yaml
llm:
  provider: "perplexity"
  model: "sonar-pro"
  api_key: "secret:PERPLEXITY_API_KEY"
  streaming: true
```

---

### Fireworks AI

**Models**: broad open-weight catalog — `accounts/fireworks/models/llama-v3p1-70b-instruct`, `accounts/fireworks/models/deepseek-r1`, etc.
**Ranking**: `StandardSizeWeights` + DeepSeek tier keywords.

```yaml
# .noctifab/secrets.yaml
FIREWORKS_API_KEY: "fw_..."

# .noctifab/config.yaml
llm:
  provider: "fireworks"
  model: "accounts/fireworks/models/llama-v3p1-70b-instruct"
  api_key: "secret:FIREWORKS_API_KEY"
  streaming: true
```

---

### SambaNova

**Models**: `Meta-Llama-3.3-70B-Instruct`, `Meta-Llama-3.1-405B-Instruct`, `DeepSeek-R1`
**Fallback chain**: `405B` → `70B` (size-based).
**Ranking**: `StandardSizeWeights` + DeepSeek tier keywords.

```yaml
# .noctifab/secrets.yaml
SAMBANOVA_API_KEY: "..."

# .noctifab/config.yaml
llm:
  provider: "sambanova"
  model: "Meta-Llama-3.3-70B-Instruct"
  api_key: "secret:SAMBANOVA_API_KEY"
  streaming: true
```

---

### Cohere

**Models**: `command-r-plus`, `command-r`, `command-light`, `command-nightly`
**Fallback chain**: `command-r-plus` → `command-r` → `command-light`
**Ranking**: tier keyword (`r-plus` > `r` > `light`) + context bonus for `08-2024` or `nightly` variants.

```yaml
# .noctifab/secrets.yaml
COHERE_API_KEY: "..."

# .noctifab/config.yaml
llm:
  provider: "cohere"
  model: "command-r-plus"
  api_key: "secret:COHERE_API_KEY"
  streaming: true
```

---

### Cerebras

**Models**: `llama3.1-70b`, `llama3.1-8b`, and other Llama/Qwen variants optimised for Cerebras wafer-scale silicon.
**Fallback chain**: size-based (`70b` → `8b`).
**Ranking**: `StandardSizeWeights` + `scout`/`maverick` frontier tier bonus.

> **Why Cerebras?** Cerebras delivers the highest raw token throughput available (orders of magnitude faster than GPU-based providers on large models), making it ideal for latency-critical agent loops.

```yaml
# .noctifab/secrets.yaml
CEREBRAS_API_KEY: "csk-..."

# .noctifab/config.yaml
llm:
  provider: "cerebras"
  model: "llama3.1-70b"
  api_key: "secret:CEREBRAS_API_KEY"
  streaming: true
```

---

### NVIDIA NIM

**Models**: large open-weight catalog hosted on NVIDIA GPU infrastructure — `meta/llama-3.1-70b-instruct`, `nvidia/nemotron-70b-instruct-hf`, `mistralai/mistral-7b-instruct-v0.3`, etc.
**Fallback chain**: size-based (`70b` → `8b`) or flagship (`nemotron`) → standard.
**Ranking**: `StandardSizeWeights` + `nemotron`/`starcoder` flagship tier bonus.

```yaml
# .noctifab/secrets.yaml
NVIDIA_API_KEY: "nvapi-..."

# .noctifab/config.yaml
llm:
  provider: "nvidia"
  model: "meta/llama-3.1-70b-instruct"
  api_key: "secret:NVIDIA_API_KEY"
  streaming: true
```

---

### AI21 Labs (Jamba)

**Models**: `jamba-large`, `jamba-mini` (and versioned variants like `jamba-large-1.7`)
**Fallback chain**: `jamba-large` → `jamba-mini`
**Ranking**: tier keyword (`large` > `mini`) + generation version multiplier.

> **Why AI21 Jamba?** Jamba uses a hybrid SSM (Mamba) + Transformer architecture supporting 256k+ context windows with high throughput compared to pure transformer models of equivalent quality.

```yaml
# .noctifab/secrets.yaml
AI21_API_KEY: "..."

# .noctifab/config.yaml
llm:
  provider: "ai21"
  model: "jamba-large"
  api_key: "secret:AI21_API_KEY"
  streaming: true
```

---

### Upstage (Solar)

**Models**: `solar-pro`, `solar-mini`
**Fallback chain**: `solar-pro` → `solar-mini`
**Ranking**: tier keyword (`pro` > `mini`) + generation number suffix.

```yaml
# .noctifab/secrets.yaml
UPSTAGE_API_KEY: "up-..."

# .noctifab/config.yaml
llm:
  provider: "upstage"
  model: "solar-pro"
  api_key: "secret:UPSTAGE_API_KEY"
  streaming: true
```

---

## Multi-Backend Failover Configuration

The `llms:` list enables cross-provider failover. If the primary provider exhausts its fallback chain entirely, the `FailoverClient` advances to the next backend in the list.

```yaml
# .noctifab/secrets.yaml
OPENAI_API_KEY: "sk-proj-..."
ANTHROPIC_API_KEY: "sk-ant-..."
GEMINI_API_KEY: "AIzaSy..."
GROQ_API_KEY: "gsk_..."

# .noctifab/config.yaml
llms:
  - provider: "openai"
    model: "gpt-4o"
    api_key: "secret:OPENAI_API_KEY"
    streaming: true
  - provider: "anthropic"
    model: "claude-3-5-sonnet-latest"
    api_key: "secret:ANTHROPIC_API_KEY"
    streaming: true
  - provider: "gemini"
    model: "gemini-3.6-pro"
    api_key: "secret:GEMINI_API_KEY"
    streaming: true
  - provider: "groq"
    model: "llama-3.3-70b-versatile"
    api_key: "secret:GROQ_API_KEY"
    streaming: true

llm:
  failover:
    cooldown: "5m"
    max_call_limit: 0
```

In the above config:
1. `gpt-4o` is tried first. On rate-limit, the engine queries OpenAI's `/models` endpoint and falls back to `gpt-4o-mini` → `gpt-3.5-turbo` within OpenAI.
2. If all OpenAI models are exhausted, the `FailoverClient` switches to Anthropic and repeats the same intra-provider chain.
3. After Anthropic, Gemini is tried, then Groq.

---

## Custom Endpoint Override

All providers support a `url:` field to override the default base URL. Useful for:
- Self-hosted vLLM / LMDeploy instances
- Azure OpenAI deployments
- Corporate API proxies

```yaml
llm:
  provider: "openai"
  model: "gpt-4o"
  url: "https://my-azure-openai.openai.azure.com/openai/deployments/gpt-4o"
  api_key: "secret:AZURE_OPENAI_KEY"
```

---

## Dynamic "latest" Model Alias Resolution

Setting `model: "latest"`, `model: "auto"`, or `model: "<provider>-latest"` in `.noctifab/config.yaml` instructs Noctifab to automatically discover and resolve the newest, highest-capability flagship model available from that LLM provider at runtime.

### How the `latest` Model is Computed Per Provider

When `model: "latest"` is configured:

1. **Live `/models` Endpoint Discovery**:
   - Noctifab issues a `GET` request to the provider's `/models` REST endpoint using the configured API key (or provider-specific headers).
2. **Specialized & Niche Model Exclusion**:
   - The provider's model parser applies `ExcludedKeywords` to discard non-chat/completion endpoints:
     - **OpenAI**: Excludes `embed`, `tts`, `whisper`, `dall-e`, `moderation`, `realtime`, `transcription`, `bison`, `audio`.
     - **Gemini**: Excludes `robotics`, `embed`, `imagen`, `bison`, `tts`, `stt`, and filters `supportedGenerationMethods` for `"generateContent"`.
     - **Anthropic / Mistral / DeepSeek / etc.**: Excludes embedding models (`embed`), moderation endpoints, and fine-tuning checkpoints.
3. **Provider-Specific Capacity & Version Scoring**:
   - Remaining models are parsed and scored using declarative provider rules:
     - **Rank Formula**: $\text{Rank} = \text{TierBaseScore} + (\text{Version} \times \text{VersionMultiplier}) + \text{ContextBonus} + \text{ParameterSizeWeight}$
4. **Top-Ranked Model Selection**:
   - Noctifab sorts candidate models by Rank (descending) and Version (descending), dynamically binding `model` to the top-ranked model (`parsedModels[0].Name`).

### Provider "latest" Resolution Reference Matrix

| Provider | Configured Model | Discovered `/models` Example | Resolved Flagship Model |
|---|---|---|---|
| **OpenAI** | `latest` | `[text-embedding-3, gpt-3.5-turbo, gpt-4o-mini, gpt-4o]` | `gpt-4o` |
| **Anthropic** | `latest` | `[claude-3-5-haiku, claude-3-5-sonnet, claude-3-opus]` | `claude-3-opus-20240229` / `claude-3-5-sonnet-20241022` |
| **Google Gemini** | `latest` | `[gemini-embed, gemini-robotics, gemini-1.5-flash, gemini-2.5-flash, gemini-3.6-flash]` | `gemini-3.6-flash` |
| **Mistral** | `latest` | `[mistral-embed, mistral-small, mistral-large-latest]` | `mistral-large-latest` |
| **DeepSeek** | `latest` | `[deepseek-chat, deepseek-coder]` | `deepseek-coder` |
| **Hermes (Nous)** | `latest` | `[hermes-8b, hermes-70b, hermes-405b]` | `hermes-3-llama-3.1-405b` |
| **Qwen** | `latest` | `[qwen-turbo, qwen-plus, qwen-max]` | `qwen-max` |
| **Meta Llama** | `latest` | `[Llama-3.1-8B, Llama-3.3-70B, Llama-3.1-405B]` | `Llama-3.1-405B-Instruct` |
| **xAI / Grok** | `latest` | `[grok-2-mini, grok-2, grok-3]` | `grok-3` |
| **Perplexity** | `latest` | `[sonar-small, sonar-pro, sonar-deep-research]` | `sonar-deep-research` |
| **Kimi / Moonshot** | `latest` | `[kimi-k1.5, kimi-k2.7, kimi-k3]` | `kimi-k3` |
| **OpenCode** | `latest` | `[glm-4-flash, glm-4, glm-5.2]` | `glm-5.2` |

---

## Dynamic Model Fallback Behaviour

See [architecture.md](architecture.md#3-dynamic-model-fallback-engine-provider-specific-capacity-ranking) for the full explanation of how models are ranked and selected. The core behaviour:

1. The provider's live `/models` endpoint is queried on every fallback.
2. Each model is scored by the provider's `ParseModelFunc`.
3. The next lower-ranked model is selected and execution resumes transparently.
4. If the current model is unrecognised (e.g. a brand-new model release), the safety valve selects the **lowest-ranked known model**, preventing a hard failure.

---

## 1-Click Local LLM Profiles & DeepSeek-R1 Reasoning Support

Noctifab provides built-in, pre-tuned configuration profiles for local model deployments (Ollama, vLLM, LMDeploy, and OpenAI-compatible local engines) via `noctifab init --profile <preset>`:

```bash
noctifab init --profile ollama-qwen
noctifab init --profile ollama-deepseek
noctifab init --profile vllm-local
noctifab init --profile openai-compat
```

### Supported Profile Presets

| Profile Preset | Provider | Default Model | Base URL | Max Context | Notes |
|---|---|---|---|---|---|
| `ollama-qwen` | `ollama` | `qwen2.5-coder:32b` | `http://127.0.0.1:11434/v1` | 32,768 | Tuned for coding and fast agent cycles |
| `ollama-deepseek`| `ollama` | `deepseek-r1:32b` | `http://127.0.0.1:11434/v1` | 64,000 | Optimized for deep chain-of-thought planning |
| `vllm-local` | `openai` | `Qwen/Qwen2.5-Coder-32B-Instruct` | `http://127.0.0.1:8000/v1` | 32,768 | High-throughput concurrent vLLM engine |
| `openai-compat` | `openai` | `default-local-model` | `http://127.0.0.1:1234/v1` | 16,384 | Generic local server (LM Studio, LocalAI) |

### Automatic `<think>` Reasoning Tag Stripping
Reasoning models like DeepSeek-R1 and Qwen-Thinking output chain-of-thought blocks wrapped in `<think>...</think>` tags before their actual JSON action envelope. Noctifab's response parser automatically intercepts and strips `<think>` blocks before JSON unmarshaling, allowing reasoning models to execute structured tool calling seamlessly without formatting errors.
