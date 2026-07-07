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

## Orchestrator Settings (`orchestrator`)

Configures the concurrency, polling, and human-in-the-loop limits.

```yaml
orchestrator:
  max_tools_per_response: 5
  concurrency: 3
  poll_interval: 5m
  max_clarification_wait: 30m
  clarification_timeout_action: abort
```

- **`max_tools_per_response`** (Integer): Maximum number of parallel tool calls the orchestrator allows the LLM to propose in a single completion response.
- **`concurrency`** (Integer): Max parallel agent workers to schedule and execute concurrently in the topological task graph.
- **`poll_interval`** (Duration): Cycle loop interval for polling VCS tasks, git repository changes, and queue statuses.
- **`max_clarification_wait`** (Duration): Maximum time the orchestrator blocks waiting for a human operator to resolve an ambiguous task clarification.
- **`clarification_timeout_action`** (String): Action to take if a clarification times out. Options: `abort` (fails the task) or `proceed` (continues execution using model defaults).

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

## LLM Configurations (`llm` and `llms`)

Defines primary LLM connections and resilient failover hierarchies. You can define a single `llm` block or a list under `llms`.

```yaml
llm:
  provider: openai
  model: gpt-4o
  temperature: 0.0
  api_key: "secret:OPENAI_API_KEY"
  max_retries: 5
  retry_backoff: 100ms
  retry_backoff_factor: 2.0
  max_budget_usd: 10.0
  reset_period: daily
  failover:
    enabled: false
    cooldown: 5m
    backends:
      - provider: gemini
        model: gemini-2.5-flash
        api_key_env: GEMINI_API_KEY
```

- **`provider`** (String): LLM provider client runtime implementation. Options: `openai`, `anthropic`, `gemini`, `ollama`, `hermes`, `huggingface`, `mistral`, `deepseek`, `opencode`.
- **`model`** (String): Specific model identifier (e.g. `claude-3-5-sonnet-latest`).
- **`temperature`** (Float): Creativity/determinism slider (typically `0.0` for code generation stability).
- **`api_key`** (String): API authentication key value. Must use `secret:<KEY>` syntax to load safely from `secrets.yaml`.
- **`api_key_env`** (String): Name of the environment variable containing the API key. Used as a fallback if `api_key` is not specified.
- **`url`** (String): Custom API endpoint URL override (required for self-hosted gateways or local `ollama` endpoints).
- **`max_retries`** (Integer): Number of retries on transient connection or model overload failures.
- **`retry_backoff`** (Duration): Starting wait time before retrying a failed API call.
- **`retry_backoff_factor`** (Float): Exponential factor multiplied by the backoff time for each retry.
- **`max_budget_usd`** (Float): Absolute financial budget cap enforced per day/period to prevent runaway LLM costs.
- **`reset_period`** (String): The timeframe to enforce the budget cap (e.g. `daily`, `monthly`).
- **`failover`**: Failover parameters:
  - **`enabled`** (Boolean): Auto-route failed calls to alternate providers when true.
  - **`cooldown`** (Duration): Time to temporarily quarantine a failed backend model.
  - **`max_call_limit`** (Integer): Maximum consecutive failover API calls allowed.
  - **`backends`** (List): Alternate backends definition list (containing `provider`, `model`, `api_key_env`, `url`, `max_retries`).

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
- **`formatter_command`** (String): Command executed to run code format checks.
- **`exclude_paths`** (List of Strings): Directory trees ignored by the repository indexer and file walker (e.g. `node_modules/`, `.git/`).
- **`allowed_commands`** (List of Strings): Whitelist of executable binaries permitted inside the sandbox process runner.
- **`auto_install_deps`** (Boolean): Allow sandbox to auto-detect and attempt to install missing build dependencies.
- **`package_managers`** (List of Strings): Authorized tool package managers (e.g. `pip`, `go`, `npm`, `brew`).
- **`forbidden_patterns`** (List of Strings): Regex patterns disallowed in tool inputs or parameters.

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
```

- **`enabled`** (Boolean): Enable OTel collection.
- **`exporter`** (String): Connection format protocols (e.g. `otlp`, `stdout`).
- **`endpoint`** (String): Host URL of the OpenTelemetry collector or Jaeger endpoint.
- **`service_name`** (String): Service metadata name tag.

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
