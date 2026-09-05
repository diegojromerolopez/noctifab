# Configuration Examples

This document provides complete, production-ready configuration examples for various programming languages, environments, and advanced features in `noctifab`.

---

## 1. Go (SQLite + GitHub + Host Sandbox)

A standard local development setup for Go projects using a local SQLite database and host-level sandbox isolation.

```yaml
config_version: "2.0"

logging:
  level: "info"

storage:
  provider: "sqlite"
  conn_string: "./.noctifab/data/noctifab.db"

agents:
  architecture: "code_first" # Options: code_first (default), single_pass, breadth_first
  orchestrator:
    number: 1      # Task orchestration & state sync
    iterations: 2
  product_manager:
    number: 1      # Spec hardening & user story generation
    iterations: 2
  planner:
    number: 1      # Task DAG decomposition
    iterations: 5
  generators:
    number: 3      # Number of parallel Generator agents (default: 3)
    iterations: 20 # Maximum turns per task (default: 20)
  testers:
    number: 2      # Number of parallel Tester agents (default: 2)
    iterations: 15 # Maximum turns per task (default: 15)
  qa:
    enabled: false # Experimental capability; no Phase 0 runtime
    iterations: 1
  fallback:
    enabled: true  # Unified fallback agent: stall detection & sovereign repair
    temperature: 0.1
    max_turns: 2
    timeout: 180s

workspace_cache:
  enabled: true    # In-memory caching of workspace filesystem reads until mutation (default: true)
poll_interval: "5m0s"

llm:
  provider: "openai"
  model: "gpt-4o"
  temperature: 0.0
  api_key: "secret:OPENAI_API_KEY"
  max_retries: 3
  retry_backoff: "100ms"
  max_timeout: "60s"
  idle_timeout: "15s"
  streaming: true

vcs:
  provider: "github"
  repository: "myorg/mygo-project"
  base_branch: "main"
  token: "secret:GITHUB_TOKEN"
  pull_request:
    auto_create: true
    auto_merge: false

sandbox:
  mode: "host"
  test_command: "go test -v ./..."
  linter_command: "golangci-lint run"
  formatter_command: "go fmt ./..."
  allowed_commands:
    - "go"
    - "git"
```

---

## 2. Python (Docker Sandbox + Dependency Auto-Install)

An isolated development setup for Python projects. It executes tests and linters inside a Docker sandbox and automatically installs missing library dependencies (like `pytest` or `ruff`) using `pip`.

```yaml
config_version: "2.0"

logging:
  level: "info"

storage:
  provider: "sqlite"
  conn_string: "./.noctifab/data/noctifab.db"

llm:
  provider: "anthropic"
  model: "claude-3-5-sonnet-latest"
  temperature: 0.0
  api_key: "secret:ANTHROPIC_API_KEY"

sandbox:
  mode: "docker"
  timeout_seconds: 120
  test_command: "pytest tests/"
  linter_command: "ruff check ."
  formatter_command: "ruff format ."
  auto_install_deps: true
  package_managers:
    - "pip"
  allowed_commands:
    - "python"
    - "pip"
    - "pytest"
    - "ruff"
    - "git"
```

---

## 3. Node.js (GitHub + Automated Pull Requests)

A JavaScript/TypeScript repository configuration with automatic pull request generation.

```yaml
config_version: "2.0"

logging:
  level: "info"

storage:
  provider: "sqlite"
  conn_string: "./.noctifab/data/noctifab.db"

llm:
  provider: "gemini"
  model: "gemini-3.6-pro"
  api_key: "secret:GEMINI_API_KEY"

vcs:
  provider: "github"
  repository: "myorg/mynode-app"
  base_branch: "main"
  token: "secret:GITHUB_TOKEN"
  pull_request:
    auto_create: true
    auto_merge: true

sandbox:
  mode: "host"
  test_command: "npm test"
  linter_command: "npm run lint"
  formatter_command: "npx prettier --write ."
  allowed_commands:
    - "node"
    - "npm"
    - "npx"
    - "git"
```

---

## 4. Advanced Enterprise (PostgreSQL + Multi-LLM Failover + SAST)

A highly resilient configuration for enterprise-grade autonomous software factories. It uses a centralized PostgreSQL database, multi-provider LLM failover, token budgeting, and security vulnerability scanning.

```yaml
config_version: "2.0"

runtime:
  max_actions: 150
  max_duration: "2h"

logging:
  level: "info"
  file: "./.noctifab/logs/orchestrator.log"

storage:
  provider: "postgres"
  conn_string: "secret:POSTGRES_DSN"

# Primary LLM provider configuration
llm:
  provider: "opencode"
  model: "glm-5.2"
  temperature: 0.0
  api_key: "secret:OPENCODE_API_KEY"
  failover:
    enabled: true
    cooldown: "10m"
    max_call_limit: 15
    backends:
      - provider: "anthropic"
        model: "claude-3-5-sonnet-latest"
        api_keys: "ANTHROPIC_API_KEY"
      - provider: "openai"
        model: "gpt-4o"
        api_keys: "OPENAI_API_KEY"
      - provider: "gemini"
        model: "gemini-3.6-flash"
        api_keys: "GEMINI_API_KEY"

vcs:
  provider: "github"
  repository: "company/core-service"
  base_branch: "main"
  token: "secret:GITHUB_TOKEN"
  pull_request:
    auto_create: true
    auto_merge: false
    labels:
      - "autonomous"
      - "sast-approved"

# Security Scanning settings
sast:
  enabled: true
  scanners:
    - "gosec"
  fail_on_severity: "high"         # Block PR creation if high severity vulnerabilities are found

sandbox:
  mode: "docker"
  test_command: "make test"
  linter_command: "golangci-lint run"
  allowed_commands:
    - "go"
    - "make"
    - "git"

# Performance Metrics & Telemetry
telemetry:
  enabled: false                   # OpenTelemetry tracing (default: false)
  exporter: "otlp"
  metrics:
    enabled: true                  # Performance & Speed Metrics Instrumentation (default: true)

# Context Slicing & AST Symbol Indexing
context:
  mode: "tree_sitter"              # Options: "full" (default), "diff_window", "tree_sitter"
  diff_window_lines: 15
```

---

## 5. Multi-Provider Named Registry & Per-Agent Routing

Configure a multi-vendor LLM pool (`llm.providers`) with global default failover (`llm.priority`) and distinct model/provider routing per agent role (`roles.<agent>.providers`).

### 5.1. Multi-Model Peer Review Pipeline (Generate with DeepSeek, Test with GPT-4o, Audit with Claude Sonnet)
Assigning different models to generate code, write tests, and review code prevents self-confirmation bias and maximizes model specialization:
- **Generators (`roles.generator`):** `deepseek-coder` for fast, syntax-accurate code implementation.
- **Testers (`roles.tester`):** `openai-primary` (`gpt-4o`) for thorough unit test creation and boundary condition assertions.
- **QA (`roles.qa`):** `anthropic-backup` (`claude-3-5-sonnet-latest`) is reserved for the disabled experimental QA capability.
- **Orchestrator & Fallback (`roles.orchestrator`, `roles.fallback`):** `openai-primary` (`gpt-4o-mini`) for ultra-fast, lightweight state loop checks and diagnostics.

```yaml
config_version: "2.0"

logging:
  level: "info"

# 1. Named LLM Provider Registry & Global Failover
llm:
  priority:
    - "openai-primary"
    - "anthropic-backup"
    - "deepseek-local"

  providers:
    - name: "openai-primary"
      provider: "openai"
      model: "gpt-4o"
      url: "https://api.openai.com/v1"
      api_key_env: "OPENAI_API_KEY"

    - name: "anthropic-backup"
      provider: "anthropic"
      model: "claude-3-5-sonnet-latest"
      url: "https://api.anthropic.com/v1"
      api_key_env: "ANTHROPIC_API_KEY"

    - name: "deepseek-local"
      provider: "deepseek"
      model: "deepseek-coder"
      url: "http://localhost:11434/v1"
      api_key_env: "DEEPSEEK_API_KEY"

# 2. Per-Agent Model Routing & Fallback Chains
agents:
  product_manager:
    providers:
      - name: "anthropic-backup" # Claude 3.5 Sonnet for rich, detailed specification decomposition
      - name: "openai-primary"

  planner:
    providers:
      - name: "anthropic-backup"
      - name: "openai-primary"

  generators:
    number: 3
    providers:
      - name: "deepseek-local"  # Cheap, fast local coding
      - name: "openai-primary"  # Fallback to GPT-4o on syntax errors
      - name: "anthropic-backup"

  testers:
    number: 2
    providers:
      - name: "openai-primary"   # Independent validation by GPT-4o
      - name: "anthropic-backup"

  qa:
    enabled: false
    iterations: 1
    providers:
      - name: "anthropic-backup" # Audit code quality with Sonnet
      - name: "openai-primary"

  fallback:
    temperature: 0.1
    providers:
      - name: "anthropic-backup" # Claude Sonnet for deep sovereign code & test surgery
      - name: "openai-primary"
```

### 5.2. Local-First Privacy Setup (Ollama Default + Cloud Planning)
- **Local Workers:** Generators, Testers, the retained disabled QA route, and Orchestrator use local Ollama (`llama3.1:70b` / `qwen2.5-coder`).
- **Cloud Escalation:** Only `planner` uses cloud Claude Sonnet for story decomposition.

```yaml
config_version: "2.0"

llm:
  priority:
    - "ollama-local"

  providers:
    - name: "ollama-local"
      provider: "ollama"
      url: "http://localhost:11434"

    - name: "anthropic-cloud"
      provider: "anthropic"
      api_keys: "ANTHROPIC_API_KEY"
      model: "claude-3-5-sonnet-latest"

roles:
  planner:
    providers:
      - name: "anthropic-cloud" # Cloud planning
      - name: "ollama-local"    # Local fallback if offline
```

### 5.3. Version-Agnostic Dynamic Model Fallback per Role
Omit hardcoded model version strings entirely (`model: ""`). Bind `generator` to OpenAI, `tester` to DeepSeek Coder, and `qa` to Anthropic Claude. At runtime, `noctifab` queries `/models` to auto-discover the highest capacity flagship model for each provider and steps down through lower-ranked tiers automatically if rate limits occur.

```yaml
config_version: "2.0"

llm:
  priority:
    - "openai-provider"
    - "anthropic-provider"
    - "deepseek-provider"

  providers:
    - name: "openai-provider"
      provider: "openai"
      api_keys: "OPENAI_API_KEY"
      # model omitted -> auto-discovers flagship model (gpt-4o -> gpt-4o-mini)

    - name: "anthropic-provider"
      provider: "anthropic"
      api_keys: "ANTHROPIC_API_KEY"
      # model omitted -> auto-discovers flagship model (opus -> sonnet -> haiku)

    - name: "deepseek-provider"
      provider: "deepseek"
      api_keys: "DEEPSEEK_API_KEY"
      # model omitted -> auto-discovers flagship coder model

roles:
  # STEP 1: Code Generation with OpenAI (Dynamic version selection & step-down)
  generator:
    temperature: 0.0
    providers:
      - name: "openai-provider"

  # STEP 2: Test Creation with DeepSeek Coder (Dynamic version selection)
  tester:
    temperature: 0.0
    providers:
      - name: "deepseek-provider"

  # STEP 3: Code Audit & Review with Anthropic Claude (Dynamic version selection)
  qa:
    temperature: 0.0
    providers:
      - name: "anthropic-provider"

---

## 6. Model-Per-Agent Specialized Routing (ninline Pattern)

A clean, high-performance configuration routing a single specialized model per agent role without ensemble overhead. Featured in the `ninline` validation project (Generalized $N$-in-a-Line game engine):

- **Product Manager**: Claude 3.5 Sonnet for exhaustive Definition of Done and edge-case matrices.
- **Planner**: GPT-4o for structured task graph decomposition.
- **Generators**: DeepSeek Coder for fast, idiomatic algorithmic implementations.
- **Testers**: Gemini Flash for rapid, adversarial test synthesis.
- **Auditor**: Claude Sonnet for zero-tolerance compliance checks.

```yaml
config_version: "2.0"

agents:
  architecture: "code_first"
  product_manager:
    number: 1
    user_stories:
      max_count: 5
      complexity:
        min: 15
        max: 35
    providers:
      - name: "claude"
  planner:
    number: 1
    providers:
      - name: "openai"
  generators:
    number: 4
    iterations: 20
    providers:
      - name: "deepseek-pro"
  testers:
    number: 2
    iterations: 15
    providers:
      - name: "gemini-flash"
  auditor:
    number: 1
    providers:
      - name: "claude"
  unblocker:
    number: 1
    providers:
      - name: "gemini-flash"

llm:
  priority:
    - "claude"
    - "openai"
    - "deepseek-pro"
    - "gemini-flash"
  providers:
    - name: "claude"
      provider: "anthropic"
      model: "claude-sonnet-5"
      api_keys: "CLAUDE_API_KEY"
    - name: "openai"
      provider: "openai"
      model: "gpt-5.6-luna"
      api_keys: "OPENAI_API_KEY"
    - name: "deepseek-pro"
      provider: "qwencloud"
      model: "deepseek-v4-pro"
      api_keys: "QWENCLOUD_API_KEY"
    - name: "gemini-flash"
      provider: "gemini"
      model: "gemini-3.6-flash"
      api_keys: "GEMINI_API_KEY"

sandbox:
  mode: "host"
  test_command: "make test"
  linter_command: "make lint"
  formatter_command: "make format"
```
