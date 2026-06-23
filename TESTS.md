# noctifab Test Documentation (TESTS.md)

This document outlines the testing strategy, structures, and specifications of the test suites implemented in the `noctifab` repository.

---

## 1. Test Architecture Overview

`noctifab` implements a layered testing strategy to ensure reliability, correctness, and isolation across the multi-agent orchestration harness:

```
┌────────────────────────────────────────────────────────┐
│               E2E Integration Scenario                 │  <- tests/e2e/ (Containerized Docker Compose)
├────────────────────────────────────────────────────────┤
│                  CLI Integration Tests                 │  <- tests/integration_test.go (In-Process Commands)
├────────────────────────────────────────────────────────┤
│                       Unit Tests                       │  <- pkg/ (Config, DB Storage, Migrations)
└────────────────────────────────────────────────────────┘
```

The codebase guarantees:
- **100% test coverage** for database storage models and transaction logic.
- **Dual-Database Parity**: Automated validation running transparently against both SQLite (zero-setup local development) and PostgreSQL (production).
- **Environment Isolation**: Direct host path sandboxing during test runs with clean database teardowns.

---

## 2. Unit Tests (`pkg/`)

Unit tests reside adjacent to their target logic files inside the `pkg/` directory structure. They execute instantly and verify low-level components.

### 2.1. Configuration Parser (`pkg/infrastructure/config/`)
- [config_test.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/config/config_test.go): Verifies default config values, directory setup helper functions, and correct configuration file writing.
- [overrides_test.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/config/overrides_test.go): Validates that environment variables (prefixed with `NOCTIFAB_`) correctly override file-based YAML settings.
- [types_test.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/config/types_test.go): Tests custom parsing of Durations in config files and serialization constraints.

### 2.2. Database Storage Persistence (`pkg/infrastructure/storage/`)
- [migrator_test.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/storage/migrator_test.go): Validates that embedded migrations run successfully on both database backends and create the necessary tables on startup.
- [sqlite_repository_test.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/storage/sqlite_repository_test.go): Tests the SQLite repository's CRUD operations, relational joins mapping, and locks serialized via a global mutex under concurrent operations.
- [postgres_repository_save_test.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/storage/postgres_repository_save_test.go): Uses sqlmock to verify transactional state saving, Optimistic Concurrency Control (OCC) version mismatches, and rollbacks on save failures.
- [postgres_repository_load_test.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/storage/postgres_repository_load_test.go): Uses sqlmock to verify relational row mapping reconstruction, SQL errors handling, and resource leaks prevention.

---

## 3. CLI Integration Tests (`tests/`)

- [integration_test.go](file:///Users/diegoj/repos/noctifab/tests/integration_test.go): Tests the subcommands exposed by the Cobra CLI framework in-process by invoking the package-exported `cli.RootCmd.Execute()`.
  - **`git_init`**: Tests successful configuration bootstrap on a clean directory and asserts the directory guard returns **Exit Code 4** (Security Warning) when running on a dirty directory containing existing project files.
  - **`validate`**: Asserts config and env validations behave correctly.
  - **`plan` / `start` / `run-once` / `maintenance`**: Verifies CLI parser parameters routing, pre-flight checks output, and execution cycle initialization.
  - **`Execute` & `ExitError`**: Verifies in-process command execution error routing and custom shell exit code formatting.

---

## 4. E2E Integration Tests (`tests/e2e/`)

The E2E test suite executes offline integration tests to verify the orchestrator event loop and agent state coordination.

- [e2e_test.go](file:///Users/diegoj/repos/noctifab/tests/e2e/e2e_test.go): Executes CLI subcommands tests against the compiled binary and verifies migration parity on the active database target.

- [scenario_django_crud_test.go](file:///Users/diegoj/repos/noctifab/tests/e2e/scenario_django_crud_test.go): Evaluates multi-agent project lifecycle and repository conflict simulations.
  - **Observe Phase (Exclusions Walk)**: Asserts that files under build/asset folders (`node_modules/`, `.git/`, `.noctifab/`) are filtered out by the scanner and never synced.
  - **Decide Phase (Clarification & Cycle Detection)**:
    - Simulates an ambiguity pause where the Planner asks a question and blocks the loop until the operator replies.
    - Validates that task DAG topological sorting correctly handles prerequisites and fails if circular dependency cycles are planned.
  - **Execute Phase (Corrective Turn & Sandboxing)**:
    - Verifies that tasks are executed inside isolated branch environments (`noctifab/task-<id>-agent-...`).
    - Simulates the Evaluator failing a task due to a generator bug (missing HTML template) and verifies the Generator reads the error diagnostics logs to fix it in the subsequent turn.
    - **Flaky Build Quarantine**: Simulates majority voting over 3 test runs. Flaky builds are approved on a 2/3 pass vote but flagged with a `Warning: Potentially Flaky Build` in the database.
  - **Git Merge Conflict Handling**: Simulates a scenario where two parallel agents attempt to write conflicting edits on the same lines of the same file. Asserts that the second agent's integration fails with a merge conflict, the task status is marked as `CONFLICT_BLOCKED`, its branch is quarantined, and the orchestrator halts execution cleanly.
  - **Budget Safeguarding**: Asserts that before each LLM call, token estimations prevent overspending, halting execution with `ErrBudgetExhausted` if the daily limit is breached.
  - **Release Phase**: Verifies auto-bumping of version files (`VERSION`), updates to `CHANGELOG.md` matching Keep a Changelog formatting, and conventional git commits and pull requests.

---

## 5. How to Run the Test Suites

### 5.1. Running Unit & Integration Tests Locally
Ensure Go 1.25 is installed on your host system:

```bash
# Run all unit and in-process CLI integration tests
go test -v ./pkg/... ./tests
```

### 5.2. Running Containerized (PostgreSQL Target via Docker Compose)
The containerized testing environment spins up a PostgreSQL database and runs all E2E tests against it, ensuring zero host filesystem pollution:

```bash
# Start the Docker Compose test environment
docker compose -f tests/e2e/docker-compose.yml up --build --exit-code-from test-runner
```

### 5.3. Running Static Code Analysis (Linting)
Verify that your changes satisfy repository style guides:

```bash
docker run -t --rm -v $(pwd):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint run
```
