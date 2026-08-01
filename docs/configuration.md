# Configuration Reference

`noctifab` is configured via a YAML file located at `.noctifab/config.yaml` in your project workspace. This document provides a complete reference for all available configuration sections and settings.

---

## Root Configurations

These settings are defined at the root level of the configuration file.

| Key | Type | Default | Description |
|:---|:---|:---|:---|
| `config_version` | String | `1.0` | Configuration format version. |
| `input` | String | `""` | Default path or issue URL to fetch the feature specification. |
| `auto_commit` | Boolean | `false` | Enable automated branch checkouts, conventional commits, and PR creations. |
| `max_actions` | Integer | `100` | Maximum number of LLM actions permitted per task loop execution to avoid infinite loops. |
| `max_duration` | Duration | `0` (unlimited) | Max wall-clock time limit for the entire run. Supports duration strings (e.g. `2h`, `45m`). |
| `conversation_mode` | String | `sliding-window` | Context management strategy: `sliding-window` or `compaction`. |
| `max_history_messages`| Integer | `10` | Message limit before sliding window or compaction triggers. |
| `compaction_threshold` | Integer | `15` | Action log size at which history compaction occurs (if mode is `compaction`). |
| `max_history_tokens` | Integer | `4096` | Token count threshold for triggering history compaction. |
| `shutdown_grace_period`| Duration | `30s` | Time window to wait for active tasks to shut down gracefully before exit. |
| `occ_max_retries` | Integer | `5` | Maximum database transaction retries on Optimistic Concurrency Control failure. |
| `occ_backoff_base` | Duration | `50ms` | Baseline backoff duration for OCC retry loops. |
| `occ_backoff_factor` | Float | `2.0` | Exponential multiplier factor applied to subsequent OCC retries. |
| `token_usage_limit` | Integer | `0` (unlimited) | Total model token limit boundary for the current session. |
| `log_level` | String | `info` | Logs verbosity filter: `debug`, `info`, `warn`, `error`. |
| `log_file` | String | `""` | File path to write execution logs (empty prints to stderr). |

---

## Agent Settings (`agents`)

Configures multi-agent team concurrency, turn iterations, architecture mode, and audit/release panels. Setting `number: 0` disables any agent type.

```yaml
agents:
  architecture: code_first
  max_tools_per_response: 5

  orchestrator:
    number: 1
    iterations: 2

  product_manager:
    number: 1
    iterations: 2

  planner:
    number: 1
    iterations: 2

  architect:
    number: 1
    iterations: 2

  generators:
    number: 3
    iterations: 5

  testers:
    number: 2
    iterations: 3

  qa:
    number: 1
    iterations: 2

  security:
    number: 1
    iterations: 2

  performance:
    number: 1
    iterations: 2

  docs:
    number: 1
    iterations: 2

  devops:
    number: 1
    iterations: 2

  unblocker:
    number: 1
    iterations: 2

workspace_cache:
  enabled: true

poll_interval: 5m
max_clarification_wait: 30m
clarification_timeout_action: abort
```

- **`architecture`** (String): Execution loop architecture mode. Options: `code_first` (default: Generator implements code first, followed by independent Tester verification turns), `single_pass` (Fast-path single pass co-generating code and tests in 1 turn), or `breadth_first` (Iterative ~80% happy-path generation across all stories first, followed by benevolent judges refining edge cases). Legacy aliases (`code_first_verification_loop`, `single_pass_execution`, `breadth_first_generation`, `cfv`, `spe`, `bfg`) are fully supported.
- **`max_tools_per_response`** (Integer): Maximum number of parallel tool calls allowed per agent response turn.
- **`orchestrator`**: Configures Orchestrator agents managing task lifecycle and state synchronization (`number: 1`, `iterations: 2`).
- **`product_manager`**: Configures Product Manager agents generating new User Stories or auditing and enriching existing User Stories (`roadmap/US-xxx.md`) with explicit Definitions of Done (DoD), language-agnostic interface contracts, error message prefixes, exit status codes, and comprehensive edge-case scenario matrices before task planning (`number: 1`, `iterations: 2`, `max_user_stories: 5`). Supports an optional `max_user_stories` setting (e.g. `5`) to hard-cap the number of roadmap user stories generated during specification decomposition. If `max_user_stories` is omitted or 0, the Product Manager agent verifies against `SPEC.md` whether existing stories already implement all specification requirements before creating any new user story.
- **`planner`**: Configures Task Planner agents decomposing User Stories into task DAGs (`number: 1`, `iterations: 2`).
- **`architect`**: Configures Software Architect agents validating domain boundaries and package structures (`number: 1`, `iterations: 2`).
- **`generators`**: Configures Generator agents writing production code (`number: 3`, `iterations: 5`).
- **`testers`**: Configures Tester agents writing test suites (`number: 2`, `iterations: 3`).
- **`qa`**: Configures QA Auditor agents auditing code & test quality by feature domain (`number: 1`, `iterations: 2`).
- **`security`**: Configures Security Auditor agents running SAST scanners & auditing code vulnerabilities (`number: 1`, `iterations: 2`).
- **`performance`**: Configures Performance & Benchmark agents running profilers & leak detection (`number: 1`, `iterations: 2`).
- **`docs`**: Configures Documentation agents maintaining OpenAPI specs, READMEs & docstrings (`number: 1`, `iterations: 2`).
- **`devops`**: Configures DevOps & Release agents generating Dockerfiles, Makefiles & CI workflows (`number: 1`, `iterations: 2`).
- **`unblocker`**: Configures Unblocker agents monitoring pipelines for stalls and re-dispatching tasks (`number: 1`, `iterations: 2`).
- **`poll_interval`** (Duration): Cycle loop interval for polling VCS tasks, git repository changes, and queue statuses.
- **`max_clarification_wait`** (Duration): Maximum time the orchestrator blocks waiting for a human operator to resolve a task clarification.
- **`clarification_timeout_action`** (String): Action to take if a clarification times out (`abort` or `continue`).

---

## Storage Settings (`storage`)

Configures the state database persistence backend.

```yaml
storage:
  provider: sqlite
  conn_string: .noctifab/data/noctifab.db
  json_file_path: .noctifab/data/state.json
```

- **`provider`** (String): Persistent database backend. Supported values: `sqlite`, `postgres`, `mysql`, `json`.
- **`conn_string`** (String): Filepath or connection DSN string. (e.g. `postgres://user:pass@localhost:5432/dbname?sslmode=disable`). Can reference secrets using `secret:CONN_STRING`.
- **`json_file_path`** (String): Target backup file path used if the provider is `json`.

---

## LLM Configurations (`llm` and `roles`)

Defines named LLM provider registries, global default failover priorities, and per-agent model routing.

```yaml
llm:
  # Global Default Failover Priority Chain
  priority:
    - "openai-primary"
    - "anthropic-backup"
    - "deepseek-coder"

  # Named LLM Provider Registry
  providers:
    - name: "openai-primary"
      provider: "openai"
      api_key: "secret:OPENAI_API_KEY"
      max_retries: 5
      retry_backoff: 100ms
      max_timeout: 60s

    - name: "anthropic-backup"
      provider: "anthropic"
      api_keys: "ANTHROPIC_API_KEY"
      model: "claude-3-5-sonnet-latest"
      max_retries: 3

    - name: "deepseek-coder"
      provider: "deepseek"
      api_keys:
        - "DEEPSEEK_API_KEY_PRIMARY"
        - "DEEPSEEK_API_KEY_SECONDARY"
      model: "deepseek-coder"
      url: "https://api.deepseek.com"

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
        model: "gpt-4o-mini"

  unblocker:
    temperature: 0.0
    providers:
      - name: "openai-primary"
        model: "gpt-4o-mini"
      - name: "anthropic-backup"
```

- **`llm.priority`** (List of Strings): Global ordered provider failover sequence. If an agent role does not define a custom `providers` list, it automatically inherits this global priority list.
- **`llm.providers`** (List of Provider Specs): Named provider registry.
  - **`name`** (String): Unique identifier for the provider (e.g. `openai-primary`, `anthropic-backup`, `ollama-local`).
  - **`provider`** (String): LLM provider client backend. Options: `openai`, `anthropic`, `gemini`, `opencode`, `kimi`, `moonshot`, `groq`, `openrouter`, `qwen`, `dashscope`, `together`, `llama`, `meta`, `huggingface`, `mistral`, `deepseek`, `hermes`, `ollama`, `xai`, `perplexity`, `fireworks`, `sambanova`, `cohere`, `cerebras`, `nvidia`, `ai21`, `upstage`.
  - **`model`** (String): Optional fixed model override (e.g. `claude-3-5-sonnet-latest`, `gpt-4o-mini`). Omit for dynamic capacity auto-selection.
  - **`api_key`** / **`api_keys`** (String or List of Strings): API authentication key value, secret reference, or secret name(s) in `secrets.yaml` / environment variables.
  - **`url`** (String): Endpoint URL override (required for self-hosted models or `ollama`).
  - **`max_retries`** / **`retry_backoff`** / **`max_timeout`**: Resilient retries and timeout constraints.
- **`roles.<agent>.providers`** (List of Agent Provider Refs): Role-specific provider priority list. Allows configuring different model priorities per agent role (`architect`, `planner`, `generator`, `tester`, `qa`, `security`, `performance`, `docs`, `devops`, `unblocker`).
- **`max_timeout`** (Duration): Maximum overall completion timeout allowed for LLM API calls (e.g. `60s`). Defaults to `60s` to allow complex planning/generation tasks without context deadlines.
- **`idle_timeout`** (Duration): Maximum stream/socket inactivity timeout allowed for LLM API calls (e.g. `15s`). Defaults to `15s` to cancel and fail over stalled stream connections without truncating active long responses.
- **`streaming`** (Boolean): Enable or disable HTTP Server-Sent Events (SSE) token streaming (e.g. `true`). Defaults to `true` to stream completion tokens in real time and enforce sliding socket idle timeouts.
- **`reset_period`** (String): The timeframe to enforce the budget cap (e.g. `daily`, `monthly`).
- **`failover`**: Failover parameters:
  - **`enabled`** (Boolean): Auto-route failed calls to alternate providers when true.
  - **`cooldown`** (Duration): Time to temporarily quarantine a failed backend model.
  - **`max_call_limit`** (Integer): Maximum consecutive failover API calls allowed.
  - **`backends`** (List): Alternate backends definition list (containing `provider`, `model`, `api_keys`, `url`, `max_retries`).

---

### Dynamic Model Fallback & Provider-Specific Capacity Ranking

One of `noctifab`'s core resilience features is its **Dynamic Model Fallback Engine**. When an LLM request fails due to rate limits (HTTP 429), quota limits (HTTP 401/402), or transient server errors (HTTP 5xx), `noctifab` automatically:
1. Queries the provider's API endpoint (`GET /models` or `/v1/models`) **live** to fetch currently available models.
2. Evaluates models using **custom provider-specific capacity ranking algorithms** to sort them by capability.
3. Automatically switches to the next lower model in capability without failing the task loop or losing progress.

#### Provider Capacity Ranking Formulas & Hierarchy

| Provider (`llm.provider`) | Custom Parser | Ranking Formula & Hierarchy |
|---|---|---|
| **Anthropic** | `parseAnthropicModel` | `(Version * 10) + TierScore`<br>`opus` (400) > `sonnet` (300) > `haiku` (200).<br>*Order*: `claude-3-opus` (430) > `claude-3-7-sonnet` (337) > `claude-3-5-sonnet` (335) > `claude-3-5-haiku` (235) > `claude-3-haiku` (230). |
| **OpenAI** | `parseOpenAIModel` | `TierScore + (Version * 10) + (Date / 1,000,000)`<br>`o3`/`o1` reasoning (60) > `gpt-4o`/`sol` flagship (50) > `gpt-4-turbo`/`gpt-4`/`terra` (40) > `o3-mini`/`o1-mini` (30) > `gpt-4o-mini`/`luna` (20) > `gpt-3.5-turbo` (10). |
| **Gemini** | `parseGeminiModel` | `int(Version * 100) + TierScore`<br>`pro` (40) > `flash` (30) > `flash-lite` (20) > `nano` (10).<br>*Order*: `gemini-2.5-pro` (290) > `gemini-2.5-flash` (280) > `gemini-1.5-pro` (190) > `gemini-1.5-flash` (180). |
| **Kimi (Moonshot AI)** | `parseKimiModel` | `TierScore + ContextWindowBonus`<br>`k3` (50) > `k2.7`/`k2.7-code` (40) > `k2.6` (30) > `k2.5` (20) > `k2`/`v1` (10).<br>*Context bonus*: `128k` (+3) > `32k` (+2) > `8k` (+1). |
| **Meta Llama** | `parseLlamaModel` | `SizeScore + int(Version * 10)`<br>`405b` (500) > `90b`/`70b`/`72b` (400) > `34b`/`32b`/`27b`/`14b`/`13b` (300) > `11b`/`8b`/`7b` (200) > `3b`/`1b` (100). |
| **Qwen (DashScope)** | `parseQwenModel` | `TierScore + int(Version * 10)`<br>`qwen-max` / `qwen3-coder-max` (40) > `qwen-plus` / `qwen3-coder-plus` (30) > `qwen-turbo` (20) > `standard` (10). |
| **Mistral** | `parseMistralModel` | `mistral-large` / `codestral` (40) > `mistral-medium` (30) > `mistral-small` (20) > `open-mistral-7b` / `micro` (10). |
| **DeepSeek** | `parseDeepSeekModel` | `deepseek-r1` / `deepseek-v3` / `deepseek-coder` (30) > `deepseek-chat` (20) > `deepseek-flash` / `distill` (10). |
| **Nous Hermes** | `parseHermesModel` | `hermes-3-llama-3.1-405b` (30) > `hermes-3-llama-3.1-70b` (20) > `hermes-3-llama-3.1-8b` (10). |
| **Ollama / HuggingFace / Fireworks / SambaNova** | `parseOllamaModel`, `parseHuggingFaceModel` | `SizeScore + int(Version * 10)`<br>`405b` (500) > `70b`/`72b` (400) > `34b`/`32b`/`14b`/`13b` (300) > `8b`/`7b` (200) > `3b`/`1b` (100). |
| **xAI (Grok)** | `parseXAIModel` | `TierScore + int(Version * 5)`<br>`grok-3` (60) > `grok-2` (40) > `grok-3-mini` (30) > `mini`/`beta` (20). |
| **Perplexity AI** | `parsePerplexityModel` | `sonar-deep-research` (50) > `sonar-reasoning-pro` (40) > `sonar-reasoning` (30) > `sonar-pro` (20) > `sonar` (10). |
| **Cohere** | `parseCohereModel` | `command-r-plus` (40) > `command-r` (30) > `command-light` (20). |

---

## VCS & Integration Settings (`vcs`)

Configures code tracking, branch prefixes, pull requests, and CI feedback hooks.

```yaml
vcs:
  provider: github
  repository: owner/repo
  base_branch: master
  branch_prefix: noctifab/
  token: "secret:GITHUB_TOKEN"
  pull_request:
    auto_create: true
    auto_merge: true
    auto_rebase: true
    draft: false
    assignees:
      - "dev-user"
    labels:
      - "autonomous"
  ci:
    auto_fix: true
    max_retries: 3
```

- **`provider`** (String): Version Control System target host. Values: `github` or `gitlab`.
- **`repository`** (String): Remote repository path identifier (e.g. `owner/repo-name`).
- **`base_branch`** (String): Default integration target branch (e.g. `main` or `master`).
- **`branch_prefix`** (String): Namespace prefix applied to ephemeral feature task branches (default: `noctifab/`).
- **`token`** (String): OAuth or Personal Access Token value. Must use `secret:GITHUB_TOKEN` reference syntax.
- **`token_env`** (String): Fallback env name to extract token (default: `GITHUB_TOKEN` or `GITLAB_TOKEN`).
- **`conventional_commits`**:
  - **`enabled`** (Boolean): Commit messages will be formatted following Conventional Commits format when true.
  - **`default_scope`** (String): Commit scope fallback keyword (default: `core`).
- **`git_mutex_timeout`** (Duration): Timeout constraint when waiting for local Git file system lock acquisitions.
- **`git_operation_retries`** (Integer): Retry attempts on transient Git errors.
- **`git_retry_backoff`** (Duration): Backoff wait between Git retries.
- **`pull_request`**:
  - **`auto_create`** (Boolean): Automatically create a VCS Pull Request when a task successfully passes validation.
  - **`auto_merge`** (Boolean): Automatically merge the PR once all integration CI status checks pass.
  - **`auto_rebase`** (Boolean): Rebase PR branches on updates to the base branch.
  - **`draft`** (Boolean): Create the PR in Draft status.
  - **`assignees`** (List of Strings): VCS accounts automatically assigned to the PR.
  - **`labels`** (List of Strings): VCS tags automatically applied to the PR.
- **`ci`**:
  - **`auto_fix`** (Boolean): If true, `noctifab` will listen for CI build failures, checkout the task branch, package the build trace logs, and task the Generator agent to patch the code automatically.
  - **`max_retries`** (Integer): Max CI fix loop attempts.

---

## Sandbox Settings (`sandbox`)

Defines safety parameters, test/linter runners, and file jail protection bounds.

```yaml
sandbox:
  mode: host
  timeout_seconds: 300
  idle_timeout_seconds: 30
  grace_period_seconds: 30
  test_command: "go test -v ./..."
  linter_command: "golangci-lint run"
  formatter_command: "go fmt ./..."
  exclude_paths:
    - "node_modules/"
    - "vendor/"
  allowed_commands:
    - "go"
    - "git"
  auto_install_deps: false
```

- **`mode`** (String): Isolation strategy environment. Values: `host` (jail checks on the developer machine) or `docker` (complete container sandbox isolation).
- **`timeout_seconds`** (Integer): Absolute execution wall-clock time limit in seconds for test and script execution processes.
- **`idle_timeout_seconds`** (Integer): Active watchdog timeout. Kills processes immediately if they output no bytes on stdout/stderr for this duration.
- **`grace_period_seconds`** (Integer): Delay allowed for subprocesses to clean up after receiving `SIGTERM` before sending `SIGKILL`.
- **`test_command`** (String): Command executed by the Test Validator to run the unit/integration test suites (e.g. `npm test`, `pytest`).
- **`linter_command`** (String): Command executed to run project static analysis linter tasks.
- **`formatter_command`** (String): Command executed to run code format checks (e.g. `rubocop -A`, `go fmt ./...`, `prettier --write .`). When present, `run_linter` runs this pre-step auto-fixer first before linter diagnostics.
- **`max_linter_retries`** (Integer): Maximum linter fix retry turns per task (default: `3`). Prevents infinite agent loops on unfixable linter offenses.
- **`exclude_paths`** (List of Strings): Directory trees ignored by the repository indexer and file walker (e.g. `node_modules/`, `.git/`).
- **`allowed_commands`** (List of Strings): Whitelist of executable binaries permitted inside the sandbox process runner.
- **`auto_install_deps`** (Boolean): Allow sandbox to auto-detect and attempt to install missing build dependencies.
- **`package_managers`** (List of Strings): Authorized tool package managers (e.g. `pip`, `go`, `npm`, `brew`).
- **`forbidden_patterns`** (List of Strings): Regex patterns disallowed in tool inputs or parameters.
- **`context.caveman_compaction`** (Boolean): When set to `true`, enables caveman-style prompt & spec markdown compaction (stripping polite conversational fluff and decorative markdown headers before sending requests over HTTP). Defaults to `false`.

---

## Roles & Profiles (`roles` and `profiles`)

Used to segregates agent worker scopes, temperature limits, and tool permissions.

```yaml
roles:
  orchestrator:
    profile: orchestrator
    temperature: 0.0
  planner:
    profile: planner
    temperature: 0.5
  generator:
    profile: generator
    temperature: 0.0
  tester:
    profile: tester
    temperature: 0.0

profiles:
  generator:
    allowed_tools:
      - "read_file"
      - "write_file"
      - "edit_file"
      - "run_tests"
    allowed_commands:
      - "go"
      - "git"
```

### Roles Config (`roles`)
Assigns model override configurations, temperature boundaries, and security profile names to the agent workers:
- **`orchestrator`**: The coordinator handling state sync, VCS, and sandbox launches.
- **`planner`**: Parses feature specs into the DAG roadmap.
- **`generator`**: Writes feature implementation files in the sandbox.
- **`tester`**: Writes and aligns validation tests.

### Profiles Config (`profiles`)
Creates permission groups matching agent roles to whitelisted resources:
- **`allowed_tools`** (List of Strings): Exact agent workspace tool names permitted (e.g. `read_file`, `write_file`, `grep_search`, `run_tests`). Dangerous system tools are restricted by default.
- **`allowed_commands`** (List of Strings): Shell command binaries whitelisted for execution inside that specific profile's sandbox.

---

## Jira Integration (`jira`)

Enables pulling specifications and user stories directly from Jira tickets.

```yaml
jira:
  url: "https://mycompany.atlassian.net"
  user: "secret:JIRA_USER"
  token: "secret:JIRA_API_TOKEN"
```

- **`url`** (String): Full Jira instance domain URL.
- **`user`** (String): Jira user email. Can reference `secrets.yaml`.
- **`token`** (String): Jira API token value. Must reference `secrets.yaml`.

---

## Telemetry Config (`telemetry`)

Configures export settings for OpenTelemetry (OTel) metrics and distributed tracing.

```yaml
telemetry:
  enabled: false
  exporter: otlp
  endpoint: ""
  service_name: noctifab
  metrics:
    enabled: true
```

- **`enabled`** (Boolean): Enable OTel collection.
- **`exporter`** (String): Connection format protocols (e.g. `otlp`, `stdout`).
- **`endpoint`** (String): Host URL of the OpenTelemetry collector or Jaeger endpoint.
- **`service_name`** (String): Service metadata name tag.
- **`metrics`**:
  - **`enabled`** (Boolean): Enable or disable performance & speed metrics instrumentation (tracking Time To First Commit, phase latencies, LLM wait duration, tokens/sec, and sandbox build times). Default: `true`.

---

## SAST Settings (`sast`)

Performs security scans on generated code before making VCS pull requests.

```yaml
sast:
  enabled: true
  scanners:
    - gosec
  fail_on_severity: high
```

- **`enabled`** (Boolean): Turn on security scanner checking.
- **`scanners`** (List of Strings): Executable scanners to run (e.g. `gosec` for Go, `bandit` for Python).
- **`fail_on_severity`** (String): Minimum scan vulnerability level that blocks integration merges. Options: `high`, `medium`, `low`.

---

## Unblocker Agent Settings (`unblocker`)

Controls the autonomous **Unblocker Agent** — a background goroutine that periodically scans the pipeline for stalled or blocked tasks and injects corrective interventions.

```yaml
unblocker:
  enabled: true
  poll_interval: "30s"
  max_retries: 3
  stall_threshold: "5m"
  conflict_threshold: "15m"
  llm_assessment: true
```

- **`enabled`** (Boolean): Activate the unblocker goroutine (default: `true`). When `false`, no stall scanning is performed.
- **`poll_interval`** (Duration): How often the unblocker wakes up to scan the pipeline for stalls (default: `30s`). Configurable via `--unblocker-poll-interval` or `NOCTIFAB_UNBLOCKER_POLL_INTERVAL`.
- **`max_retries`** (Integer): Maximum number of unblock/reset attempts allowed for a single task before the unblocker permanently marks it as `FAILED` (default: `3`). Configurable via `--unblocker-max-retries` or `NOCTIFAB_UNBLOCKER_MAX_RETRIES`.
- **`stall_threshold`** (Duration): How long a task must be frozen `IN_PROGRESS` with no progress update before it is classified as stalled (default: `5m`).
- **`conflict_threshold`** (Duration): How long a `CONFLICT_BLOCKED` task waits before the unblocker intervenes (default: `15m`).
- **`llm_assessment`** (Boolean): When `true` (default), the unblocker calls the LLM to diagnose each stall and choose the corrective action. When `false`, deterministic heuristics are applied instead (no LLM call, lower token cost).

See [unblocker_agent.md](unblocker_agent.md) for a full developer reference including the stall detection algorithm, command dispatch, and tuning guidelines.

---

## Context Slicing Settings (`context`)

Controls how target workspace source files are formatted and sliced for agent prompts.

```yaml
context:
  mode: full
  diff_window_lines: 15
```

- **`mode`** (String): Context formatting strategy. Options:
  - `full`: Sends complete source file contents (default).
  - `diff_window`: Extracts modified git diff lines and error stack traces (+/- context lines).
  - `tree_sitter`: Universal AST parsing extracting class/struct definitions and function signatures.
- **`diff_window_lines`** (Integer): Number of context lines surrounding diff modifications in `diff_window` mode (default: `15`).

---

## Workspace Inspection Caching Settings (`agents.workspace_cache`)

Controls in-memory deduplication of read-only filesystem reads (`list_directory`, `read_file`, `find_files`, `grep_search`) and diagnostic test/linter runs during an agent task execution loop. The cache is automatically invalidated when any file mutation (`write_file`, `edit_file`, `delete_file`) occurs.

```yaml
agents:
  workspace_cache:
    enabled: true
```

- **`enabled`** (Boolean): Enable in-memory workspace inspection and diagnostic tool caching (default: `true`).

---

## Full Configuration Example (`.noctifab/config.yaml`)

Below is a complete, annotated example configuration demonstrating all options in a typical spec-driven (Level 3.5) setup:

```yaml
config_version: "1.0"
input: "./roadmap/US-001.md"
auto_commit: true
max_actions: 100
max_duration: "45m"
conversation_mode: "compaction"
max_history_messages: 20
compaction_threshold: 25
max_history_tokens: 32768
shutdown_grace_period: "30s"
occ_max_retries: 5
occ_backoff_base: "50ms"
occ_backoff_factor: 2.0
token_usage_limit: 0
log_level: "info"
log_file: "./.noctifab/logs/noctifab.log"

orchestrator:
  max_tools_per_response: 5
  concurrency: 3
  poll_interval: "10s"
  max_clarification_wait: "30m"
  clarification_timeout_action: "abort"

storage:
  provider: "sqlite"
  conn_string: "./.noctifab/data/noctifab.db"

llm:
  provider: "opencode"
  model: "opencode-v1"
  temperature: 0.0
  api_key: "secret:OPENCODE_API_KEY"
  max_retries: 5
  retry_backoff: "100ms"
  retry_backoff_factor: 2.0
  max_budget_usd: 10.0
  reset_period: "daily"
  failover:
    enabled: true
    cooldown: "5m"
    max_call_limit: 10
    backends:
      - provider: "openai"
        model: "gpt-4o"
        api_keys: "OPENAI_API_KEY"

vcs:
  provider: "github"
  repository: "owner/repo"
  base_branch: "main"
  branch_prefix: "noctifab/feature-"
  token: "secret:GITHUB_TOKEN"
  git_mutex_timeout: "5m"
  git_operation_retries: 3
  git_retry_backoff: "1s"
  pull_request:
    auto_create: true
    auto_merge: true
    auto_rebase: true
    draft: false
    assignees:
      - "dev-user"
    labels:
      - "autonomous"
      - "noctifab"
  ci:
    auto_fix: true
    max_retries: 3

sandbox:
  mode: "docker"
  timeout_seconds: 300
  idle_timeout_seconds: 30
  grace_period_seconds: 10
  test_command: "cargo test"
  linter_command: "cargo clippy -- -D warnings"
  formatter_command: "cargo fmt --check"
  exclude_paths:
    - "target/"
    - ".noctifab/"
  allowed_commands:
    - "cargo"
    - "rustc"
    - "git"

roles:
  orchestrator:
    profile: "orchestrator"
    temperature: 0.0
  planner:
    profile: "planner"
    temperature: 0.5
  generator:
    profile: "generator"
    temperature: 0.0
  tester:
    profile: "tester"
    temperature: 0.0

profiles:
  generator:
    allowed_tools:
      - "read_file"
      - "write_file"
      - "edit_file"
      - "list_directory"
      - "find_files"
      - "grep_search"
      - "run_tests"
      - "run_linter"
      - "request_test_fix"
      - "noop"
  tester:
    allowed_tools:
      - "read_file"
      - "write_file"
      - "edit_file"
      - "list_directory"
      - "find_files"
      - "grep_search"
      - "run_tests"
      - "run_linter"
      - "noop"

jira:
  url: "https://mycompany.atlassian.net"
  user: "secret:JIRA_USER"
  token: "secret:JIRA_API_TOKEN"

telemetry:
  enabled: false
  exporter: "otlp"
  endpoint: "http://localhost:4317"
  service_name: "noctifab"

sast:
  enabled: true
  scanners:
    - "gosec"
  fail_on_severity: "high"
```

