# Configuration Examples

This document provides complete, production-ready configuration examples for various programming languages, environments, and advanced features in `noctifab`.

---

## 1. Go (SQLite + GitHub + Host Sandbox)

A standard local development setup for Go projects using a local SQLite database and host-level sandbox isolation.

```yaml
config_version: "1.0"
log_level: "info"

storage:
  provider: "sqlite"
  conn_string: "./.noctifab/data/noctifab.db"

llm:
  provider: "openai"
  model: "gpt-4o"
  temperature: 0.0
  api_key: "secret:OPENAI_API_KEY"
  max_retries: 3
  retry_backoff: "100ms"

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
config_version: "1.0"
log_level: "info"

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

## 3. Node.js (GitHub + Automated CI Fixes)

A JavaScript/TypeScript repository configuration. It enables **CI Auto-Fix**, allowing `noctifab` to listen for GitHub Action failures, check out the task branch, and prompt the Generator agent to fix compile or test issues autonomously.

```yaml
config_version: "1.0"
auto_commit: true
log_level: "info"

storage:
  provider: "sqlite"
  conn_string: "./.noctifab/data/noctifab.db"

llm:
  provider: "gemini"
  model: "gemini-2.5-pro"
  api_key: "secret:GEMINI_API_KEY"

vcs:
  provider: "github"
  repository: "myorg/mynode-app"
  base_branch: "main"
  token: "secret:GITHUB_TOKEN"
  pull_request:
    auto_create: true
    auto_merge: true
  ci:
    auto_fix: true
    max_retries: 5

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

A highly resilient configuration for enterprise-grade autonomous software factories. It uses a centralized PostgreSQL database, multi-provider LLM failover, token cost budgeting, and security vulnerability scanning.

```yaml
config_version: "1.0"
max_actions: 150
max_duration: "2h"
log_level: "info"
log_file: "./.noctifab/logs/orchestrator.log"

storage:
  provider: "postgres"
  conn_string: "secret:POSTGRES_DSN"

# Primary LLM provider configuration
llm:
  provider: "opencode"
  model: "glm-5.2"
  temperature: 0.0
  api_key: "secret:OPENCODE_API_KEY"
  max_budget_usd: 15.00            # Stop calls if daily limit is exceeded
  reset_period: "daily"
  failover:
    enabled: true
    cooldown: "10m"
    max_call_limit: 15
    backends:
      - provider: "anthropic"
        model: "claude-3-5-sonnet-latest"
        api_key_env: "ANTHROPIC_API_KEY"
      - provider: "openai"
        model: "gpt-4o"
        api_key_env: "OPENAI_API_KEY"
      - provider: "gemini"
        model: "gemini-2.5-flash"
        api_key_env: "GEMINI_API_KEY"

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
```
