# noctifab

[![CI Build Status](https://github.com/diegojromerolopez/noctifab/actions/workflows/ci.yml/badge.svg)](https://github.com/diegojromerolopez/noctifab/actions)
[![Go Version](https://img.shields.io/github/go-mod/go-version/diegojromerolopez/noctifab)](https://github.com/diegojromerolopez/noctifab)
[![Documentation Status](https://readthedocs.org/projects/noctifab/badge/?version=latest)](https://noctifab.readthedocs.io/en/latest/?badge=latest)
[![Autonomy Level](https://img.shields.io/badge/Autonomy-Level%203%20%2F%204-blueviolet)](https://noctifab.readthedocs.io)
[![License](https://img.shields.io/github/license/diegojromerolopez/noctifab)](/LICENSE)
[![Linter Status](https://img.shields.io/badge/Linter-Linting%20Clean-success)](https://github.com/diegojromerolopez/noctifab)

`noctifab` is an autonomous, long-running agentic harness that operates without human intervention to resolve issues, verify builds, run tests, and manage software project lifecycles. 

Designed as a **Dark Factory Platform** for GitHub and GitLab, it is compiled as a single Go binary and runs as a single-node autonomous loop engine to replace manual developer execution bottlenecks.

---

## Autonomy Matrix

The platform classifies development automation into distinct levels. `noctifab` is built to run at **Level 3** and **Level 4** autonomy:

| Level | Name | Platform Behavior |
| :--- | :--- | :--- |
| **Level 1** | Autocomplete | AI suggests code inline. Human drives the editor and makes all decisions. |
| **Level 2** | Interactive Assistant | AI generates entire files/functions. Human reviews every single change in the editor. |
| **Level 3** | Spec-Driven (Gated) | AI generates code autonomously from specifications. Holdout scenarios gate quality. Human clicks merge. |
| **Level 3.5** | Selective Auto-Merge | Same as Level 3, but low-risk modules merge automatically. Human can block. |
| **Level 4** | Full Dark Factory | Specs go in, tested code comes out fully merged. Human reviews only exceptions. |

---

## Core Pillars

1. **Stateless Agent, Stateful Orchestrator**: The AI agents have no memory of previous runs or actions. Instead, the orchestrator compiles and tracks system state (tasks, file indices, action logs, and clarifications) in a local database (SQLite/PostgreSQL) and feeds it to the agent at each step.
2. **Topological Task Scheduling**: Decomposes complex feature specifications into a Directed Acyclic Graph (DAG) of task models, running independent tasks concurrently.
3. **Behavior-Driven Quality Gates**: Employs BDD holdout scenario validation with majority voting to ensure that generated code is safe and completely regression-free before merging.
4. **Sandboxed Action Isolation**: Safely edits files and runs test commands inside host path jails or isolated Docker containers.

---

## Quick Start

### Installation

Clone the repository and compile the CLI using the provided `Makefile`:

```bash
git clone https://github.com/diegojromerolopez/noctifab.git
cd noctifab
make build
```

This compiles the binary to `./dist/noctifab`.

### Setup and Running

```bash
# 1. Initialize the noctifab workspace configurations
./dist/noctifab git_init

# 2. Validate configurations
./dist/noctifab validate

# 3. Plan a task DAG from a feature specification
./dist/noctifab plan --input ./examples/markdown-to-html/spec.md

# 4. Start the autonomous loop
./dist/noctifab start
```

---

## Command Reference

- **`git_init`**: Initializes workspace folder structure (`.noctifab/`), SQLite DB, default config, and security permission profiles.
- **`validate`**: Checks configuration files, databases, and sandbox settings.
- **`plan`**: Invokes the LLM Planner model to decompose a specification into task dependencies.
- **`start`**: Spawns the daemon workers, initializes the API server, and runs the polling event loop.
- **`run-once`**: Runs exactly one execution cycle and exits.
- **`maintenance`**: Cleans up completed branches, orphaned worktrees, and runs database schema migrations.

---

## Target Scenarios & Examples

`noctifab` contains pre-configured example targets in the `examples/` folder to validate autonomous software implementation capabilities:
- **`url-shortener`**: An API server that generates short URLs, tracks analytics, and redirects clients.
- **`todo-cli`**: A command-line checklist manager with file persistence.
- **`weather-api`**: A service caching weather data and querying external providers.
- **`markdown-to-html`**: A utility that parses markdown files and generates styled HTML.
- **`task-scheduler`**: An in-memory scheduler executing functions at scheduled time intervals.
- **`frontpunch`**: A task worker demonstration featuring SOLID patterns and Sidekiq-compatible components.

---

## Collaboration & Coding Standards

We welcome contributions! To maintain a highly clean and context-friendly repository, all code changes must adhere to the following directives:

1. **The 500-Line Limit**: No Go source code file (`.go`) may exceed **500 physical lines** (including comments and blank lines). Smaller, logically focused files prevent LLM context pollution.
2. **Dependency Injection**: Provide all clients, database connection objects, and configurations through struct constructors. Global state is strictly prohibited.
3. **100% Test Coverage**: Every package must be accompanied by unit tests (`_test.go` files). Ensure the test suite passes before submitting:
   ```bash
   go test -v ./...
   ```
4. **Code Quality and Lints**: Ensure that the code is formatted using `go fmt` and passes static analysis lints:
   ```bash
   docker run -t --rm -v $(pwd):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint run
   ```

---

## License

This project is licensed under the MIT License - see the [LICENSE](/LICENSE) file for details.
