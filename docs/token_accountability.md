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

* **OpenAI & OpenAI-Compatible (OpenCode, OpenRouter, DeepSeek, Qwen, Mistral, xAI, Cerebras, Fireworks, Moonshot)**:
  - Streaming requests specify `StreamOptions: { IncludeUsage: true }`.
  - Usage headers and final SSE stream chunks extract `PromptTokens`, `CompletionTokens`, `CompletionTokensDetails.ReasoningTokens`, and `PromptTokensDetails.CachedTokens`.
  - DeepSeek and OpenAI-compatible relays returning `prompt_cache_hit_tokens` in usage extra fields are automatically parsed and counted towards `CachedTokens`.
* **Anthropic Client**:
  - Extracts `input_tokens`, `cache_read_input_tokens`, `cache_creation_input_tokens`, and `output_tokens` from response objects.
  - Combines prompt tokens with cached read and creation tokens for accurate total input accountability (`InputTokens = input_tokens + cache_read_input_tokens + cache_creation_input_tokens`), recording `cache_read_input_tokens` as `CachedTokens`.
* **Gemini Client**:
  - Extracts `promptTokenCount`, `candidatesTokenCount`, and `cachedContentTokenCount` (with fallback to `totalCachedTokens` / `total_cached_tokens`) from `usageMetadata`.
* **Fallback Token Estimation**:
  - For non-standard or unmetered mock endpoints, `FallbackTokenUsage` estimates prompt tokens at ~4 characters per token and completion tokens based on response body length.

---

## 3. Telemetry & OpenTelemetry Attributes

All provider client completions emit standardized OpenTelemetry trace span attributes following the GenAI semantic conventions:

- `gen_ai.usage.input_tokens`: Number of prompt tokens sent to the LLM.
- `gen_ai.usage.output_tokens`: Number of completion/candidate tokens generated.
- `gen_ai.usage.cached_tokens`: Number of prompt tokens served from prefix/context cache.
- `gen_ai.usage.reasoning_tokens`: Number of internal chain-of-thought reasoning tokens generated.
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

---

## 5. Token Reduction & Generation Speed Optimizations

To maximize token economics and reduce wall-clock generation latency, Noctifab enforces several architectural controls across prompt construction and agent execution:

### 1. Context Deduplication Across File and Reader Phases
Source files sliced directly from `task.TargetFiles` and files resolved via the AST/ImportGraphWalker in `RunReaderPhase` are deduplicated via `DeduplicateFileAndReaderContexts`. Files are injected at most once into prompt context, eliminating 15%–30% of redundant input tokens per turn.

### 2. Sliding Window & Superseded Diagnostic Pruning
In multi-turn agent sessions (`allTurnOutputsHistory`), diagnostic failure outputs from `run_tests` and `run_linter` are pruned and superseded once subsequent test or linter runs take place. In addition, turns older than the configured window (default: 2 turns) have verbose output bodies condensed to single-line summaries via `PruneAndWindowToolOutputs`, preventing exponential context growth on turns 3+.

### 3. Relevant Subtree Workspace Tree Formatting
When the repository exceeds 60 files, `RunReaderPhase` formats the workspace file tree by including root build files and complete file listings for directories containing `task.TargetFiles`, while summarizing unrelated directories into concise file counts (e.g. `tests/e2e/ (15 files omitted)`).

### 4. Strict Static Prefix Locking for KV Cache Stability
`rendered.Body` remains completely invariant across all turns of a task session, guaranteeing that prompt prefixes match byte-for-byte across turns 1..N. Dynamic counters (`Turns remaining: %d`) are embedded within turn headers rather than splitting the trailing JSON output contract.

### 5. Elimination of Ghost Reasoning Output Tokens
Execution contracts for generator and tester roles restrict the `"reasoning"` requirement to a concise 1-sentence technical intent rather than requesting detailed essays, saving 200–600 output tokens and 5–15 seconds per turn.

### 6. Dynamic Turn Budgeting for Single-Pass Tasks
Single-pass generation tasks are capped at 8 turns (down from 20), combined with immediate fast-exit as soon as explicit test execution passes cleanly after file mutations.

### 7. Context-Aware Prompt-Scaled Hedging
Speculative hedging delays scale dynamically with prompt size (`+1s` per 2,000 tokens above 4,000 tokens), preventing premature concurrent hedging during large prompt KV cache evaluation.

