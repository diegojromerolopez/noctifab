# Configuration Guidelines & Best Practices

This guide outlines architectural recommendations, performance tuning strategies, and operational best practices for configuring `.noctifab/config.yaml` in autonomous Dark Factory environments.

Following these guidelines ensures that your projects achieve maximum throughput, avoid concurrency deadlocks, eliminate rate-limit stalls, and run completely unattended.

---

## 1. Golden Rules of Noctifab Configuration

1. **Enable Resilient OCC for SQLite**: Always tune Optimistic Concurrency Control (`storage.occ`) when running multiple parallel agents to avoid write collision aborts.
2. **Never Block on Unanswered Questions in CI/Autonomous Runs**: Set `clarification_timeout_action: continue` so agents synthesize assumptions and maintain momentum.
3. **Pre-Bake Toolchains in Containers**: Do not download heavy packages or compilers at runtime; pre-install them in your `Dockerfile` to eliminate network latency and timeout risks.
4. **Cap Reasoning Model Thinking Budgets**: Set `thinking_budget: 2048` on reasoning LLMs (such as Qwen or DeepSeek-R1) to prevent 30–60s per-turn response latency.
5. **Enforce Hard Watchdog Timers**: Set both `runtime.max_duration` and `runtime.max_silent_stall_duration` to catch deadlocks and trigger automatic recovery.
6. **Explicitly Whitelist Sandbox Binaries**: Ensure every CLI utility invoked by test runners, formatters, or linters is registered under `sandbox.allowed_commands`.

---

## 2. Storage & Optimistic Concurrency Control (OCC)

When `noctifab` runs 4–8 parallel Generator and Tester agents, multiple goroutines concurrently mutate the task state, story status, and execution logs in the database.

### Recommended SQLite Configuration

```yaml
storage:
  provider: sqlite
  conn_string: .noctifab/data/noctifab.db
  json_file_path: .noctifab/data/state.json
  occ:
    max_retries: 20
    backoff_base: 100ms
    backoff_factor: 2
```

### Why This Matters

| Parameter | Default / Anti-Pattern | Recommended | Rationale |
| :--- | :--- | :--- | :--- |
| `max_retries` | `5` | `20` | Prevents premature task failure during high-concurrency story completion and task merging. |
| `backoff_base` | `50ms` | `100ms` | Provides adequate spacing for SQLite transaction locks to clear without thrashing. |
| `backoff_factor` | `1.5` | `2` | Standard exponential backoff gives contention bursts time to subside. |

> [!TIP]
> For high-throughput distributed setups or shared CI clusters with $>10$ concurrent stories, consider switching `storage.provider` to `postgres` with connection pooling.

---

## 3. Autonomous Execution & Clarification Flow

During autonomous execution, an agent may formulate a clarification request when facing ambiguous requirements. In unattended CI/CD or overnight Dark Factory loops, there is no human operator to respond immediately.

### Recommended Clarification Settings

```yaml
poll_interval: 10s
max_clarification_wait: 30m0s
clarification_timeout_action: continue
```

### Action Strategies

- **`continue` (Recommended for Dark Factory & CI)**: When `max_clarification_wait` expires, the orchestrator instructs the agent to make a sensible engineering assumption based on the project specification and proceed immediately.
- **`block`**: Keeps the story suspended until an operator supplies an answer via `noctifab steer` or the Web Dashboard. Use this only for interactive local development.
- **`abort`**: Terminates the run immediately if a question arises. **Avoid in automated pipelines** as it causes unnecessary false-negative halts.

---

## 4. Runtime Limits & Stall Watchdogs

Every autonomous run must be guarded by wall-clock limits and progress watchdogs to prevent infinite loops, hung processes, or token leaks.

### Recommended Runtime Configuration

```yaml
runtime:
  spec_source: ""
  max_actions: 100
  max_duration: 10m
  max_silent_stall_duration: 10m
  max_tokens_per_story: -1
  max_tokens_per_task: -1
  max_tokens: -1
  loop:
    count: 1
```

### Key Guidelines

1. **Unlimited Token Budgets (`-1`)**: Use `-1` for `max_tokens_per_story`, `max_tokens_per_task`, and global `max_tokens` when you want tasks to complete without artificial cutoff mid-refactoring.
2. **`max_duration` (10m–30m)**: Sets the maximum total execution time for the run. For isolated validation tasks and unit builds, `10m` is standard. For full-stack applications with extensive test suites, use `30m`–`45m`.
3. **`max_silent_stall_duration` (10m)**: Monitors the workspace for task transitions. If no story or task advances within this window, the Unblocker Agent or Last-Resort Agent is triggered to recover execution.

---

## 5. LLM Provider Hierarchy & Rate-Limit Resilience

A robust LLM configuration uses multi-provider fallback ordering and exponential backoff to ensure transient provider outages or HTTP 429 rate limits do not crash the pipeline.

### Recommended Provider Settings

```yaml
llm:
  token_usage_limit: 0
  priority:
    - gemini-flash
    - claude
    - gemini
    - openai
    - qwen
    - deepseek-pro
    - opencode
    - openrouter
  providers:
    - name: claude
      provider: anthropic
      model: claude-sonnet-5
      api_keys: CLAUDE_API_KEY
      max_retries: 3
      retry_backoff: 500ms
      max_timeout: 60s
      idle_timeout: 60s
      max_tokens: -1
      temperature: 0.3
      streaming: true

    - name: gemini-flash
      provider: gemini
      model: gemini-3.6-flash
      api_keys: GEMINI_API_KEY
      max_retries: 3
      retry_backoff: 500ms
      max_timeout: 60s
      idle_timeout: 60s
      max_tokens: -1
      temperature: 0.3
      streaming: true

    - name: qwen
      provider: qwencloud
      model: qwen3.8-max
      api_keys: QWENCLOUD_API_KEY
      max_retries: 3
      retry_backoff: 500ms
      max_timeout: 60s
      idle_timeout: 60s
      max_tokens: -1
      temperature: 0.3
      streaming: true
      enable_thinking: true
      thinking_budget: 2048
```

### Provider Tuning Guidelines

- **`max_retries: 3` & `retry_backoff: 500ms`**: Essential for surviving brief API throttling without failing the entire task turn.
- **Thinking Budget Cap (`thinking_budget: 2048`)**: Uncapped reasoning models often spend 4,000–16,000 tokens reasoning on simple file edits, introducing 30–60 second turn delays. Capping to `2048` preserves deep logic analysis while keeping response times under 5 seconds.
- **Priority Fallback**: Order `llm.priority` with your fastest/highest-quota provider first (e.g. `gemini-flash` or `claude`), followed by high-capacity reasoning providers (`qwen`, `deepseek-pro`), and finally aggregators (`openrouter`).

---

## 6. Multi-Model Ensembles (MoM)

Multi-model ensembling allows specialized agent roles to leverage distinct model strengths.

### Recommended Role Topology Matrix

```yaml
agents:
  architecture: code_first

  product_manager:
    number: 1
    iterations: 2
    max_user_stories: 5
    max_tokens: -1
    ensemble:
      strategy: parallel
      timeout_seconds: 45
      soft_timeout_seconds: 15
      min_models: 2
      fallback_to_single: true
      models:
        - name: claude
          temperature: 0.2
        - name: openai
          temperature: 0.2
        - name: gemini
          temperature: 0.1
      synthesizer:
        name: gemini

  generators:
    number: 6
    iterations: 20
    max_tokens: -1
    ensemble:
      strategy: adaptive
      timeout_seconds: 45
      fast_tier:
        - name: gemini-flash
      standard_tier:
        - name: claude
      heavy_tier:
        - name: claude
        - name: openai
        - name: deepseek-pro

  testers:
    number: 2
    iterations: 15
    max_tokens: -1
    ensemble:
      strategy: best_of_n_scored
      timeout_seconds: 30
      models:
        - name: claude
          count: 2      # Spawns 2 parallel Claude candidate completions
        - name: openai
        - name: deepseek-pro

  auditor:
    number: 1
    iterations: 2
    max_tokens: -1
    ensemble:
      strategy: consensus
      timeout_seconds: 30
      voters:
        - name: claude
          count: 2      # Dual self-consistency Claude voters
        - name: gemini
      tie_breaker:
        name: openai
```

### Why This Topology Works

1. **Product Manager (`parallel` Quorum)**: Fan-out to multiple frontier models ensures user stories are exhaustive and free from hallucinated APIs. The synthesizer unifies them into a single cohesive story.
2. **Generators (`adaptive` Routing)**: Automatically sends simple edits (typos, docs) to Fast Tier (1–3s latency), standard code to Standard Tier, and complex logic/concurrency to Heavy Tier.
3. **Testers (`best_of_n_scored` with `count: N`)**: Generates candidate test suites in parallel—including multiple stochastic samples from top-tier models (`count: 2`)—and scores them locally via CPU (AST parsing, anti-stub scanner, assertion density) with **zero synthesis cost**.
4. **Auditor (`consensus` with Self-Consistency)**: Uses multi-voter consensus with tie-breaking for strict compliance checking against `SPEC.md`.

> [!TIP]
> **Homogeneous Sampling (`count: N`)**: Because LLMs are stochastic at $T > 0$, sampling multiple times from the *same* frontier model (e.g. `count: 3` on Claude or GPT-4.5) often outperforms mixing in weaker models. `count` defaults to `1` when omitted.


---

## 7. Sandbox Configuration & Toolchain Whitelisting

The Noctifab sandbox isolates code execution and enforces security by verifying every executed command against `sandbox.allowed_commands`.

### Common Failure Pitfall: Missing Whitelist Entries

If `test_command` or `linter.command` invokes a build tool (e.g. `make test` or `coverage run`), but the binary name (`make` or `coverage`) is omitted from `allowed_commands`, the sandbox immediately halts execution with:
`Sandbox violation: command '<binary>' is not in the whitelist`.

### Recommended Sandbox Configurations by Language

#### Go
```yaml
sandbox:
  mode: host
  timeout_seconds: 300
  test_command: go test -v ./...
  formatter_command: go fmt ./...
  linter:
    command: go vet ./...
    max_issues: 100
    consecutive_failures: 2
    max_retries: 3
  allowed_commands:
    - go
    - git
    - make
    - docker
```

#### Python
```yaml
sandbox:
  mode: host
  timeout_seconds: 300
  test_command: pytest
  formatter_command: ruff format .
  linter:
    command: ruff check . && mypy --strict .
    max_issues: 100
    consecutive_failures: 2
    max_retries: 3
  allowed_commands:
    - python3
    - python
    - pip
    - pip3
    - pytest
    - coverage
    - ruff
    - mypy
    - make
    - git
    - sh
    - bash
```

#### Rust
```yaml
sandbox:
  mode: host
  timeout_seconds: 600
  test_command: cargo test --workspace
  formatter_command: cargo fmt
  linter:
    command: cargo fmt --check && cargo clippy -- -D warnings
    max_issues: 100
    consecutive_failures: 2
    max_retries: 3
  allowed_commands:
    - cargo
    - rustc
    - gcc
    - clang
    - git
    - make
  forbidden_patterns:
    - \bunsafe\s*\{
```

#### TypeScript / Node.js
```yaml
sandbox:
  mode: host
  timeout_seconds: 300
  test_command: npm test
  formatter_command: npx prettier --write .
  linter:
    command: npx eslint . && npx tsc --noEmit
    max_issues: 100
    consecutive_failures: 2
    max_retries: 3
  allowed_commands:
    - npm
    - npx
    - node
    - tsc
    - vitest
    - eslint
    - prettier
    - git
    - make
```

#### C / C++
```yaml
sandbox:
  mode: host
  timeout_seconds: 600
  test_command: make test
  formatter_command: make format
  linter:
    command: make lint
    max_issues: 100
    consecutive_failures: 2
    max_retries: 3
  allowed_commands:
    - make
    - gcc
    - clang
    - clang-format
    - clang-tidy
    - sqlite3
    - git
    - sh
    - bash
```

---

## 8. Containerized Execution & Dockerfile Best Practices

When running validation runs or containerized sandboxes (`make validate PROJECT=<project>`), follow these Dockerfile rules:

### 1. Pre-Install All Tools in the Dockerfile
Avoid runtime `pip install`, `cargo install`, or package downloads during agent test loops.

```dockerfile
# GOOD: Pre-installed toolchain
FROM python:3.12-alpine

RUN apk add --no-cache git bash docker-cli curl build-base linux-headers \
    && python3 -m pip install --no-cache-dir --break-system-packages \
        "fastapi>=0.110" \
        "pytest>=8.0" \
        "ruff>=0.8" \
        "mypy>=1.13" \
    && rm -rf /root/.cache/pip
```

```dockerfile
# BAD: Missing formatters/linters require runtime network fetching
FROM rust:alpine
RUN apk add --no-cache git
# Missing rustup component add rustfmt clippy -> Causes cargo clippy / cargo fmt to fail offline
```

### 2. Workspace Mount & Secrets Isolation
- Never embed API keys into Dockerfiles or check in `secrets.yaml`.
- Noctifab mounts `secrets.yaml` securely at `/app/secrets.yaml` (or target project `.noctifab/secrets.yaml`) at container runtime.

---

## 9. Context & Prompt Optimization

```yaml
context:
  mode: full
  diff_window_lines: 15
  compaction: caveman
workspace_cache:
  enabled: true
```

- **`context.mode: full`**: Provides the agent with full file context for accurate AST reasoning, avoiding truncation errors common with heuristic tree-sitter extractors.
- **`compaction: caveman`**: Minimizes boilerplate in historical prompts to save up to 40% in prompt token overhead.
- **`workspace_cache.enabled: true`**: Caches unmodified filesystem reads in memory, dramatically reducing host I/O.

---

## 10. Pre-Flight Verification & Automatic Repair

Before launching production runs, use Noctifab's built-in validation utilities:

### 1. Validate Configuration Syntax & Constraints
```bash
noctifab validate
```

### 2. Automatic AI-Assisted Repair
If your configuration has deprecated keys, missing provider blocks, or invalid types, automatically diagnose and repair it with:
```bash
noctifab validate --fix
```

This creates a backup (`.noctifab/config.yaml.bak`), presents a colorized visual diff of the repaired configuration, and allows instant application.
