# CLI Usage & Integration Guide

`noctifab` is compiled as a single Go binary functioning as a Command Line Interface (CLI). It serves as the orchestrator to initialize, plan, check, run, and maintain your autonomous software development loops.

---

## Workspace Initialization

To set up a repository for use with `noctifab`, run the `init` command from the root of the target codebase:

```bash
noctifab init
```

This creates the default configuration structure in the `.noctifab/` directory:

```
.noctifab/
├── config.yaml          # Main YAML configuration file (safe to commit, includes role profiles)
├── secrets.yaml         # Secret credentials — NEVER committed (gitignored)
├── .gitignore           # Ignores data/, secrets.yaml, logs, and lock files
├── data/
│   └── noctifab.db      # SQLite local database (runtime, gitignored)
└── logs/                # Session and execution logs
```

---

## Subcommands Overview

`noctifab` provides several subcommands for orchestrating development pipelines:

### 1. `init`
Initializes a repository workspace with the necessary folder structure, database, default YAML configuration, and security permission profiles.
```bash
noctifab init [flags]
```
- **Security Check**: If the target directory contains existing project files but does not have a `.noctifab` directory, the command will warn the developer and abort with exit code `4` to prevent unintended code overwrites.

### 1. `init`
Initializes a Noctifab project workspace in the target directory (defaults to `.`). Automatically creates `.noctifab/config.yaml`, `.noctifab/secrets.yaml`, SQLite database, and a `SPEC.md` template if missing.

```bash
noctifab init [target_dir] [--profile <preset>]
```

| Flag | Description |
|------|-------------|
| `--profile` | Pre-configured LLM profile preset (`ollama-qwen`, `ollama-deepseek`, `vllm-local`, `openai-compat`) |

### 2. `demo`
Launches an instant, 2-minute, zero-config autonomous sandbox using deterministic offline mock replay. Ideal for testing Noctifab's dark factory loop with zero LLM API keys and zero cost.

```bash
noctifab demo [--project <archetype>] [--offline] [--speed <multiplier>] [--no-cleanup]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--project` | `cli` | Demo archetype template (`cli`, `rest`) |
| `--offline` | `true` | Deterministic offline replay mode without external network calls |
| `--speed` | `1.0` | Execution speed multiplier (e.g. `2.0` for 2x speed) |
| `--no-cleanup` | `false` | Preserve ephemeral `/tmp/noctifab-demo-*` workspace directory on exit |

### 3. `dashboard`
Launches the real-time progress dashboard to monitor active story and task orchestrations. By default, it opens the interactive Terminal User Interface (TUI). Pass `-w` / `--web` to launch the visual web dashboard in the browser instead.

```bash
# Launch interactive TUI dashboard in terminal
noctifab dashboard

# Launch visual web dashboard in browser and auto-open default browser
noctifab dashboard -w --web-open

# Launch visual web dashboard with custom port and host
noctifab dashboard -w --port 8080 --host 127.0.0.1

# Launch visual web dashboard in read-only mode
noctifab dashboard -w --readonly
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--web` | `-w` | `false` | Launch the real-time visual web dashboard in browser instead of TUI |
| `--web-open` | | `false` | Automatically open the visual web dashboard in the default browser |
| `--port` | | `8080` | Port for the visual web dashboard |
| `--host` | | `127.0.0.1` | Host address to bind the visual web dashboard |
| `--readonly` | | `false` | Read-only mode disabling prompt steering and order mutations |

* **Interactive TUI Keyboard Shortcuts**:
  * `q`: Quit the dashboard.
  * `p`: Pause / Resume execution of the active story.
  * `s`: Inject a mid-flight steering directive to the active worker.
  * `o` / `n`: Type a new feature prompt order.
  * `c`: Answer pending disambiguation/clarification questions.
  * `d`: Open the interactive Log / Failure Inspector.
  * `x`: Cancel execution of the active story.
* **CI/CD Non-Interactive Mode**: If stdin is not a terminal and `--web` is not set, the dashboard automatically falls back to dumping a plain-text status summary to stdout every 5 seconds, auto-exiting when all active stories have finished.

### 4. `steer`
Injects a mid-flight steering directive to guide active agent workers without stopping the execution loop.

```bash
noctifab steer "Use PostgreSQL instead of SQLite" [--task-id <id>]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--task-id` | `-t` | Specific task ID to steer (defaults to active running task) |

### 5. `order`
Submits an ad-hoc user story or feature prompt order directly into the autonomous dark factory execution queue.

```bash
noctifab order "Implement JWT authentication middleware with refresh token rotation"
```

### 6. `validate`
Loads and validates the current configurations, state, and directory constraints, ensuring that security policies, LLM keys, and local sandbox folders are correctly aligned.
```bash
noctifab validate
```

### 7. `start`
Plans and executes code generation from a software specification file or project directory (defaults to `.`). Automatically initializes `.noctifab/config.yaml`, `.noctifab/secrets.yaml`, and `SPEC.md` template if missing in the target folder. Pass `-w` / `--web` to launch the concurrent live Visual Web Dashboard. Pass `--web-open` to automatically open the dashboard in your default browser. Pass `-i` / `--interactive` to launch the live TUI dashboard interface. Pass `--standby` to keep the daemon running persistently in standby mode after finishing initial stories.
```bash
# Run in current directory with live Visual Web Dashboard and auto-open in browser
noctifab start -w --web-open

# Run on a target project folder with live Web Dashboard on custom port
noctifab start /path/to/my-project -w --web-port 8080

# Run in interactive TUI dashboard mode
noctifab start -i

# Run in persistent standby mode (always-on dark factory)
noctifab start -w --standby

# Resume execution, skipping completed user stories
noctifab start --resume
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--web` | `-w` | `false` | Launch the real-time visual web dashboard concurrently during execution |
| `--web-open` | | `false` | Automatically open the visual web dashboard in the default browser |
| `--web-port` | | `8080` | Port for the concurrent visual web dashboard |
| `--web-host` | | `127.0.0.1` | Host address to bind the concurrent visual web dashboard |
| `--standby` | | `false` | Keep daemon alive in standby mode after finishing initial stories to accept prompt orders (defaults to true with `-w`) |
| `--interactive` | `-i` | `false` | Launch in live interactive TUI dashboard mode |
| `--resume` | | `false` | Resume execution from the first incomplete user story, skipping completed stories |
| `--spec` | `-s` | `SPEC.md` | Path to feature specification file |

### 8. `resume`
Resumes execution of an interrupted or partially completed project workspace, skipping user stories that have already reached `SUCCESS` with all tasks completed, and picking up execution at the first incomplete story. Supports `-w` / `--web` to launch the visual web dashboard concurrently, and `--web-open` to auto-open in browser.
```bash
# Resume execution in target project folder with concurrent web dashboard
noctifab resume /path/to/my-project -w --web-open
```

### 9. `serve`
Starts the long-running headless orchestrator daemon loop in the background, continuously polling and executing actions until completion or cancellation. Exposes the internal loopback REST API on `127.0.0.1:18080`.
```bash
noctifab serve
```

### 10. `prompts`
Inspects, initializes, customizes, and validates per-agent prompt templates without rebuilding the binary.
```bash
# List all 15 agent action prompts and their active sources (embedded vs override)
noctifab prompts list

# Show the active prompt template body and variable contract for an agent action
noctifab prompts show generator implement

# Scaffold editable template files into .noctifab/prompts/
noctifab prompts init generator implement
noctifab prompts init --all

# Validate all prompt overrides for syntax and parameter contract adherence
noctifab prompts validate
```

### 11. `stop`
Gracefully stops the background daemon process and saves state.
```bash
noctifab stop
```

### 12. `clean`
Resets all Noctifab state: wipes the state database, prunes logs, and removes PID files.
```bash
# Preview what would be cleaned
noctifab clean --dry-run

# Clean without interactive confirmation prompt
noctifab clean --yes
```

### 13. `maintenance`
Performs cleanup actions: prunes fully merged task branches from the local directory, cleans orphaned worktrees, and executes state database schema migrations.
```bash
noctifab maintenance
```

### 14. `version`
Displays Noctifab's semantic release version, Git commit hash, and commit date.

```bash
# Default single-line format
noctifab version
# Example: noctifab version 0.37.0 (commit: f85f9fd, date: 2026-08-17T12:35:00+02:00)

# Output raw semantic version string only
noctifab version --short
# or: noctifab version -s
# Example: 0.37.0

# Output structured multi-line details (includes Go runtime & OS/architecture)
noctifab version --verbose
# or: noctifab version -v

# Output machine-readable JSON format
noctifab version --json

# Root flag shortcut
noctifab --version
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--json` | | `false` | Output version and VCS build metadata in JSON format |
| `--short` | `-s` | `false` | Output only the raw semantic version string |
| `--verbose` | `-v` | `false` | Output detailed key-value metadata (version, commit, date, Go runtime, platform) |

---

## Pre-flight Checks

`noctifab start` runs a short pre-flight checklist before launching the daemon and prints one line per check:

```
Running pre-flight checks...
- Git CLI: OK
- Database connectivity (sqlite): OK
- LLM provider (opencode) ping: OK
- Sandbox mode (host): OK
Pre-flight checks passed successfully.
```

| Check | What it verifies | Failure modes |
|---|---|---|
| Git CLI | `git` is on `PATH` | `git` not installed |
| Database connectivity | Storage provider opens (SQLite file writable / Postgres reachable) | missing dir, permission denied, bad DSN |
| **LLM provider ping** | The configured provider's `/models` endpoint (or equivalent) is reachable with the configured API key | bad key (401), quota exceeded (429), network unreachable, wrong `url` override |
| Sandbox mode | The configured `sandbox.mode` is recognized (`host`/`docker`) | unknown mode string |

> **Note on "LLM provider ping".** The ping is a config/syntax sanity check, not a real model completion. A passing ping means your provider name, API key, and base URL resolve to a reachable models-listing endpoint — it does **not** guarantee that your specific `llm.model` (e.g. `glm-5.2`) is available under your plan, nor that you have completion quota. The first real model availability + quota test happens when the Planner Agent runs. If the ping fails, inspect the daemon log (`.noctifab/logs/daemon.log`) for the underlying HTTP error.

---

## Global Persistent Flags

The following flags can be passed to the root command or configured in `.noctifab/config.yaml`:

| Long Flag | Short Flag | Default Value | Description |
| :--- | :---: | :--- | :--- |
| `--config` | `-c` | `.noctifab/config.yaml` | Path to the YAML configuration file |
| `--db-path` | | `.noctifab/data/noctifab.db` | Path to the local SQLite database file |
| `--storage-provider` | | `sqlite` | Storage provider (`sqlite`, `postgres`, `mysql`, `json`) |
| `--storage-conn` | | | Connection string or filepath for the storage database |
| `--input` | `-i` | | Path or issue URL to fetch the feature specification |
| `--auto-commit` | | `false` | Enable automated branch checkouts, conventional commits, and PRs |
| `--agents` | `-a` | `3` | Maximum number of parallel workers/agents to spawn |
| `--interval` | `-t` | `5m` | Cycle loop polling duration interval |
| `--vcs-provider` | `-p` | `github` | Version Control System target (`github`, `gitlab`) |
| `--vcs-repo` | `-r` | | Repository identifier format: `owner/repo` |
| `--llm-provider` | `-l` | `openai` | LLM client API provider (`openai`, `anthropic`, `gemini`, `ollama`, `opencode`) |
| `--llm-model` | `-m` | `gpt-4o` | LLM Model Identifier |
| `--sandbox-mode` | | `host` | Sandbox isolation mode (`host` or `docker`) |
| `--sandbox-idle-timeout` | | `30s` | Kill subprocess if no stdout/stderr output for this duration (0 = disabled) |
| `--max-budget-usd` | | `10.00` | Daily LLM credit budget boundary in USD |
| `--pr-auto-create` | | `false` | Automatically create a PR from the task branch |
| `--pr-auto-merge` | | `false` | Automatically merge the PR when CI checks pass |
| `--pr-auto-rebase` | | `false` | Automatically rebase the PR branch on base updates |
| `--pr-draft` | | `false` | Create the PR as a draft |
| `--pr-assignees` | | | Comma-separated list of GitHub usernames to assign to the PR |
| `--pr-labels` | | | Comma-separated list of labels to apply to the PR |
| `--ci-auto-fix` | | `false` | Automatically attempt to fix CI pipeline failures |
| `--ci-max-retries` | | `3` | Max attempts to fix CI before giving up |
| `--log-level` | | `info` | Logging verbosity: `debug`, `info`, `warn`, `error` |

---

## SAST Configuration

| Flag | Default | Description |
|------|---------|-------------|
| `--sast-enabled` | `false` | Enable SAST scanning before PR creation |
| `--sast-scanners` | `gosec` | Comma-separated SAST scanners (`gosec`, `bandit`) |
| `--sast-fail-on-severity` | `high` | Minimum severity to block the PR |

## Dependency Auto-Install Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `sandbox.auto_install_deps` | `false` | Auto-install missing toolchain dependencies |
| `sandbox.package_managers` | `["pip","go","brew","curl","npm"]` | Package managers to use for installation |

---

## Secrets Management

Sensitive credentials (API keys, VCS tokens) must **not** be written directly into `config.yaml`, which may be committed to version control. Instead, use the `secret:` reference syntax to load values from a gitignored `secrets.yaml` file.

### Quick Setup

**Step 1 — Create `.noctifab/secrets.yaml`** (already gitignored after `noctifab init`):

```yaml
# .noctifab/secrets.yaml — never commit this file
GEMINI_API_KEY: "AIzaSy..."
GITHUB_TOKEN: "github_pat_..."
```

**Step 2 — Reference secrets in `config.yaml`**:

```yaml
llm:
  api_key: "secret:GEMINI_API_KEY"
vcs:
  token: "secret:GITHUB_TOKEN"
```

During startup, noctifab resolves each `secret:<KEY>` reference from `secrets.yaml`. If the file does not exist, noctifab falls back to environment variables or config files (note that CLI flags are not provided for credentials to prevent secrets leakage in shell history).

**Precedence (highest wins):** Environment variable → `secrets.yaml` → literal value in `config.yaml`

For full details, supported fields, and CI/CD usage see [docs/secrets.md](secrets.md).

---

## LLM Provider Configurations

### OpenCode Go (GLM-5.2, Kimi K2.7, DeepSeek V4, …)

`noctifab` supports the [OpenCode Go](https://opencode.ai/docs/en/go/) subscription as an LLM provider. The OpenCode models are served through an OpenAI-compatible endpoint at `https://opencode.ai/zen/go/v1/chat/completions`; `noctifab` routes the `opencode` provider name through that transport.

**Step 1 — Add your OpenCode API key to `.noctifab/secrets.yaml`:**

```yaml
# .noctifab/secrets.yaml
OPENCODE_API_KEY: "sk-..."
GITHUB_TOKEN: "github_pat_..."
```

**Step 2 — Configure the `opencode` provider in `config.yaml`:**

```yaml
llm:
  provider: opencode
  model: glm-5.2          # or glm-5.1, kimi-k2.7-code, kimi-k2.6, deepseek-v4-pro, …
  temperature: 0
  api_key: "secret:OPENCODE_API_KEY"
  api_keys: OPENCODE_API_KEY
  url: ""                  # leave blank to use https://opencode.ai/zen/go/v1
  max_retries: 5
  retry_backoff: 100ms
```

Key points:
- The API key resolves from `secret:OPENCODE_API_KEY` in `secrets.yaml`, or from the `OPENCODE_API_KEY` environment variable if the secrets file is absent.
- The default base URL is `https://opencode.ai/zen/go/v1` (`/chat/completions` is appended automatically). Override `url` only if you run a custom OpenAI-compatible gateway.
- A static model fallback hierarchy is built in for `opencode` (GLM‑5.2 → GLM‑5.1 → Kimi K2.7 Code → … → DeepSeek V4 Flash): on a transient HTTP 429/503 the client steps down to the next model in that list and retries.

### Resilient Failover Configuration (Multiple LLM Clients)

`noctifab` supports configuring a list of multiple LLM clients under `llms:` in `config.yaml` to enable high-availability failover. If the primary provider experiences transient errors (such as HTTP `429` Rate Limits, HTTP `503` Service Unavailable, timeouts, or model overload errors), the client automatically switches to alternative backends in the defined order.

**Step 1 — Configure multiple backends in `config.yaml`:**
```yaml
llms:
  - provider: opencode
    model: glm-5.2
    temperature: 0
    api_key: "secret:OPENCODE_API_KEY"
    url: ""
    max_retries: 5
    retry_backoff: 100ms
    retry_backoff_factor: 2
    max_budget_usd: 10
  - provider: gemini
    model: gemini-2.5-flash
    temperature: 0
    api_key: "secret:GEMINI_API_KEY"
    url: ""
    max_retries: 5
    retry_backoff: 100ms
    retry_backoff_factor: 2
    max_budget_usd: 10
```

**Step 2 — Define your API keys in `secrets.yaml`:**
```yaml
# .noctifab/secrets.yaml
OPENCODE_API_KEY: "sk-..."
GEMINI_API_KEY: "AIzaSy..."
```

**Key Points on Failover Logic:**
- **Order of Try:** The first provider listed in the `llms:` configuration is the primary client. `noctifab` uses it for the startup pre-flight check and attempts all completions through it first.
- **Failover Cooldown:** When a provider encounters a transient failure (HTTP `429`/`503`/overload/timeout), that backend is marked on **cooldown** (default: 5 minutes) and bypassed on subsequent calls. completions are automatically routed to the next configured backend in the list.
- **Secrets Resolution:** All `secret:` references inside the `llms:` list are recursively resolved against `secrets.yaml` at load time, ensuring fallback credentials remain protected.
- **Budget Monitoring:** Each backend enforces its own locally monitored daily monetary limit (`max_budget_usd`) independently.

---

## End-to-End Workflow Example

Below is a typical sequence of commands to execute a feature specification:

```bash
# 1. Initialize the workspace
noctifab init

# 2. Add credentials to .noctifab/secrets.yaml (gitignored)
cat >> .noctifab/secrets.yaml <<'EOF'
GEMINI_API_KEY: "AIzaSy..."
GITHUB_TOKEN: "github_pat_..."
EOF

# 3. Reference them in .noctifab/config.yaml
#    llm.api_key: "secret:GEMINI_API_KEY"
#    vcs.token:   "secret:GITHUB_TOKEN"

# 4. Validate configuration
noctifab validate

# 5. Start the background daemon and interactive REPL
noctifab start

# 6. From the REPL prompt, queue a user story
> start examples/markdown-to-html/spec.md
```

---

## Sandbox Language Configurations

For language-specific workspaces, you should configure the `sandbox` block in your `.noctifab/config.yaml` to specify the correct test runner, linter, formatter, and allowed binaries.

Below are configurations for common programming languages:

### Python
```yaml
sandbox:
  mode: host
  idle_timeout_seconds: 30
  test_command: "coverage run --branch -m unittest discover -s tests -p \"test_*.py\" && coverage report --fail-under=80"
  linter_command: "ruff check ."
  formatter_command: "black ." # or "ruff format ."
  allowed_commands:
    - python
    - git
    - pip
    - coverage
    - ruff
    - black
```

### Ruby
```yaml
sandbox:
  mode: host
  idle_timeout_seconds: 30
  test_command: "bundle exec rspec"
  linter_command: "bundle exec rubocop"
  formatter_command: "bundle exec rubocop -A"
  allowed_commands:
    - ruby
    - bundle
    - git
    - rspec
    - rubocop
```

### Node.js (JavaScript / TypeScript)
```yaml
sandbox:
  mode: host
  idle_timeout_seconds: 30
  test_command: "npm test" # or "jest" / "vitest"
  linter_command: "npm run lint" # or "eslint ."
  formatter_command: "npx prettier --write ."
  allowed_commands:
    - node
    - npm
    - npx
    - git
```

### Java
```yaml
sandbox:
  mode: host
  idle_timeout_seconds: 30
  test_command: "mvn test" # or "./gradlew test"
  linter_command: "mvn checkstyle:check"
  formatter_command: "mvn spotless:apply"
  allowed_commands:
    - java
    - mvn
    - gradle
    - ./gradlew
    - git
```

### Go (Golang)
```yaml
sandbox:
  mode: host
  idle_timeout_seconds: 30
  test_command: "go test -v ./..."
  linter_command: "golangci-lint run"
  formatter_command: "go fmt ./..."
  allowed_commands:
    - go
    - git
    - make
```

### Rust
```yaml
sandbox:
  mode: host
  idle_timeout_seconds: 30
  test_command: "cargo test"
  linter_command: "cargo clippy -- -D warnings"
  formatter_command: "cargo fmt"
  allowed_commands:
    - cargo
    - rustc
    - git
```
