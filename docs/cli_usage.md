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
├── config.yaml          # Main YAML configuration file (safe to commit)
├── secrets.yaml         # Secret credentials — NEVER committed (gitignored)
├── .gitignore           # Ignores data/, secrets.yaml, logs, and lock files
├── data/
│   └── noctifab.db      # SQLite local database (runtime, gitignored)
├── profiles/
│   ├── default.yaml     # Role permission boundaries (read-only tools, local only)
│   └── orchestrator.yaml# Orchestrator permissions (allow all tools & external APIs)
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

### 2. `validate`
Loads and validates the current configurations, state, and directory constraints, ensuring that security policies, LLM keys, and local sandbox folders are correctly aligned.
```bash
noctifab validate
```


### 3. `start`
Spawns the background daemon process (`noctifab serve`) and starts a foreground interactive REPL loop. The REPL accepts operator orders (e.g. `start roadmap/US-0001.md`) and displays/prompts for clarification answers.
```bash
noctifab start
```

### 4. `start-one`
Plans and executes a single user story specification file end-to-end in a blocking execution loop until complete or failed, then exits.
```bash
noctifab start-one --input ./feature-spec.md
```

### 5. `stop`
Gracefully stops the background daemon process and saves state.
```bash
noctifab stop
```

### 6. `clean`
Wipes all noctifab state (deletes database, PID, and story/daemon logs).

```bash
noctifab clean           # asks for confirmation interactively
noctifab clean --yes     # skip confirmation (alias: -y)
noctifab clean --dry-run # preview what would be deleted without deleting
```

| Flag | Short | Description |
|------|-------|-------------|
| `--yes` | `-y` | Skip the `Are you sure? [y/N]` prompt |
| `--dry-run` | | Print what would be removed without deleting anything |

### 7. `maintenance`
Performs cleanup actions: prunes fully merged task branches from the local directory, cleans orphaned worktrees, and executes state database schema migrations.
```bash
noctifab maintenance
```

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
| `--llm-provider` | `-l` | `openai` | LLM client API provider (`openai`, `anthropic`, `gemini`, `ollama`) |
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
