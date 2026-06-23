# CLI Usage & Integration Guide

`noctifab` is compiled as a single Go binary functioning as a Command Line Interface (CLI). It serves as the orchestrator to initialize, plan, check, run, and maintain your autonomous software development loops.

---

## Workspace Initialization

To set up a repository for use with `noctifab`, run the `git_init` command from the root of the target codebase:

```bash
noctifab git_init
```

This creates the default configuration structure in the `.noctifab/` directory:

```
.noctifab/
├── config.yaml          # Main YAML configuration file
├── .gitignore           # Ignores database, logs, and lock files
├── config/
│   └── noctifab.db      # SQLite local database
├── profiles/
│   ├── default.yaml     # Role permission boundaries (read-only tools, local only)
│   └── orchestrator.yaml# Orchestrator permissions (allow all tools & external APIs)
└── logs/                # Session and execution logs
```

---

## Subcommands Overview

`noctifab` provides several subcommands for orchestrating development pipelines:

### 1. `git_init`
Initializes a repository workspace with the necessary folder structure, database, default YAML configuration, and security permission profiles.
```bash
noctifab git_init [flags]
```
- **Security Check**: If the target directory contains existing project files but does not have a `.noctifab` directory, the command will warn the developer and abort with exit code `4` to prevent unintended code overwrites.

### 2. `validate`
Loads and validates the current configurations, state, and directory constraints, ensuring that security policies, LLM keys, and local sandbox folders are correctly aligned.
```bash
noctifab validate
```

### 3. `plan`
Loads a feature specification (from a markdown file or remote issue URL) and uses the LLM Planner model to decompose it into a Directed Acyclic Graph (DAG) of discrete task models.
```bash
noctifab plan --input ./feature-spec.md
```

### 4. `start`
Spawns the long-running daemon loop. It regularly polls the database for pending/ready tasks, coordinates the worker concurrency pool, binds a REST API command server, and executes agents.
```bash
noctifab start
```

### 5. `run-once`
Executes exactly one cycle of the event loop (Observation, Scheduling, Execution, Holdout Evaluation, and VCS Handoff) and exits immediately. This is ideal for manual verification or cron-based pipelines.
```bash
noctifab run-once
```

### 6. `maintenance`
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
| `--db-path` | | `.noctifab/config/noctifab.db` | Path to the local SQLite database file |
| `--storage-provider` | | `sqlite` | Storage provider (`sqlite`, `postgres`, `mysql`, `json`) |
| `--storage-conn` | | | Connection string or filepath for the storage database |
| `--input` | `-i` | | Path or issue URL to fetch the feature specification |
| `--auto-commit` | | `false` | Enable automated branch checkouts, conventional commits, and PRs |
| `--agents` | `-a` | `3` | Maximum number of parallel workers/agents to spawn |
| `--interval` | `-t` | `5m` | Cycle loop polling duration interval |
| `--vcs-provider` | `-p` | `github` | Version Control System target (`github`, `gitlab`) |
| `--vcs-token` | | | API Access Token for the VCS provider |
| `--vcs-repo` | `-r` | | Repository identifier format: `owner/repo` |
| `--llm-provider` | `-l` | `openai` | LLM client API provider (`openai`, `anthropic`, `gemini`, `ollama`) |
| `--llm-model` | `-m` | `gpt-4o` | LLM Model Identifier |
| `--llm-api-key` | `-k` | | API authentication key for LLM provider |
| `--sandbox-mode` | | `host` | Sandbox isolation mode (`host` or `docker`) |
| `--max-budget-usd` | | `10.00` | Daily LLM credit budget boundary in USD |
| `--log-level` | | `info` | Logging verbosity: `debug`, `info`, `warn`, `error` |

---

## End-to-End Workflow Example

Below is a typical sequence of commands to execute a feature specification:

```bash
# 1. Initialize the workspace
noctifab git_init

# 2. Add your LLM keys and configuration to .noctifab/config.yaml
# (e.g. set llm-provider: "openai" and llm-model: "gpt-4o")

# 3. Validate configuration
noctifab validate

# 4. Decompose a markdown specification into task DAGs
noctifab plan --input ./examples/markdown-to-html/spec.md

# 5. Execute the work loop
noctifab start
```
