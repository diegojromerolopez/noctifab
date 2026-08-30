# 3-Tier Token Accountability System

`noctifab` features a comprehensive **3-Tier Token Accountability System** to track, persist, report, and audit LLM token consumption across all execution levels:

```
[Tier 1: Global Run & State Metadata]
       ├── Total Input Tokens
       └── Total Output Tokens
             │
[Tier 2: User Story Milestone Level]
       ├── Input Tokens per Story
       └── Output Tokens per Story
             │
[Tier 3: Task & Active Agent Goroutine Level]
       ├── Input Tokens per Task / Agent Attempt
       └── Output Tokens per Task / Agent Attempt
```

---

## 1. The 3 Tiers of Token Accounting

1. **Global Run & State Metadata (Tier 1)**
   - Tracks total prompt (`TotalInputTokens`) and candidate (`TotalOutputTokens`) tokens consumed across the entire dark factory run lifetime in `domain.StateMetadata`.
   - Persisted in the `state` table columns `total_input_tokens` and `total_output_tokens`.

2. **User Story Milestone Level (Tier 2)**
   - Tracks accumulated input and output tokens consumed by generator, tester, and reviewer agents working on a specific user story (`US-001`, `US-002`, etc.).
   - Persisted in the `stories` table columns `input_tokens` and `output_tokens`.
   - Rendered in execution reports as the `### Story Token Breakdown` table.

3. **Task & Active Agent Goroutine Level (Tier 3)**
   - Tracks precise input and output tokens per individual task execution attempt and agent worker goroutine.
   - Persisted in the `tasks` and `active_agents` table columns `input_tokens` and `output_tokens`.

---

## 2. LLM Provider Token Extraction

Token usage is extracted directly from the response payloads of all supported LLM provider clients:

* **OpenAI & OpenAI-Compatible (OpenCode, OpenRouter, DeepSeek, Qwen)**:
  - Streaming requests specify `StreamOptions: { IncludeUsage: true }`.
  - Usage headers and final SSE stream chunks extract `PromptTokens` and `CompletionTokens`.
* **Anthropic Client**:
  - Extracts `input_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens`, and `output_tokens` from response objects.
  - Combines prompt tokens with cached read and creation tokens for accurate total input accountability (`InputTokens = input_tokens + cache_read_input_tokens + cache_creation_input_tokens`).
* **Gemini Client**:
  - Extracts `promptTokenCount`, `candidatesTokenCount`, and `cachedContentTokenCount` from `usageMetadata`.
* **Fallback Token Estimation**:
  - For non-standard or unmetered mock endpoints, `FallbackTokenUsage` estimates prompt tokens at ~4 characters per token and completion tokens based on response body length.

---

## 3. Telemetry & OpenTelemetry Attributes

All provider client completions emit standardized OpenTelemetry trace span attributes following the GenAI semantic conventions:

- `gen_ai.usage.input_tokens`: Number of prompt tokens sent to the LLM.
- `gen_ai.usage.output_tokens`: Number of completion/candidate tokens generated.
- `gen_ai.response.model`: Model identifier used for the completion.
- `gen_ai.provider`: LLM infrastructure provider name.

---

## 4. Visualization & Reporting

### Terminal TUI Dashboard (`noctifab start` / `noctifab dashboard`)
Displays 3-tier token breakdown in the header telemetry ribbon:
```
Tokens Used: 14,500 total (12,500 in / 2,000 out)
```

### Execution Reports (`.noctifab/output/report/*.md`)
Renders exact input, output, and total token breakdowns for the overall run and individual user stories:
```markdown
## LLM and Token Usage

- **Total Input Tokens:** 12000
- **Total Output Tokens:** 1800
- **Total Tokens:** 13800

### Story Token Breakdown

| Story ID | Input Tokens | Output Tokens | Total Tokens |
| :--- | ---: | ---: | ---: |
| US-001 | 12000 | 1800 | 13800 |
```

### Web Dashboard & Telemetry API (`/api/v1/metrics`)
Returns structured JSON metrics including `total_input_tokens`, `total_output_tokens`, and `total_tokens`.
