# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.2] - 2026-06-25

### Added
- **Interactive REPL & Background Daemon**:
  - Overhauled the `start` command to spawn `noctifab serve` as a background daemon process and run an interactive foreground command line REPL.
  - Implemented the `ListenerAgent` to interpret operator free-text commands using the LLM with a rule-based parser fallback.
  - Added the `ClarificationPoller` to query pending clarification questions from the daemon and prompt the developer for answers inline.
- **New CLI Commands**:
  - Added the `stop` command to gracefully stop the background daemon and save state.
  - Added the `clean` command to reset the database and logs. It includes a check to prevent clearing state if the daemon is currently running, unless overridden via `--force`.
- **Per-Story Log Files**:
  - Created spec-specific log files at `.noctifab/logs/roadmap/<spec-name>.log`.
- **Per-Story Pull Requests**:
  - Decoupled pull request finalization into `orchestrator_finalize.go`. The orchestrator now pushes branches and creates separate pull requests on a per-user-story completion basis.
- **Unit and Integration Tests**:
  - Added extensive BDD-style unit tests for `DaemonClient`, `ClarificationPoller`, and `FinalizeUserStory` with local bare Git remote mocks.
  - Added integration tests for process signaling, daemon graceful stop, and the clean command.

### Changed
- **Renamed Command**: Renamed the `create` command to `start-one`.

## [0.0.1] - 2026-06-24

### Added
- **Core Domain Layer (`pkg/domain`)**:
  - Implemented the `State` entity modeling tasks, validation criteria, role configurations, and client settings.
  - Implemented the topological `Task` dependency graph models and transition logic.
  - Defined the `LLMClient` and `VCSClient` interfaces to decouple use cases from external providers.
  - Created standardized `Clarification` models to represent human-in-the-loop interaction questions and responses.
  - Added dedicated domain errors, including `Sentinel` errors, for consistent engine diagnostics.
- **Orchestration & Use Cases (`pkg/usecase`)**:
  - Implemented the central `Orchestrator` engine executing the main autonomous loop (fetch, analyze, validate, execute, release).
  - Implemented the `Scheduler` to manage concurrent execution of independent tasks using topological sorting and worker goroutine pools.
  - Created the BDD `Holdout` execution engine featuring a majority voting consensus protocol across 3 sequential runs.
  - Added the `Sandbox` virtualization layer supporting host path jailing and warm Docker containers.
  - Added `CommandChannel` for serial FIFO control mailbox communication.
  - Added `RebaseQueue` to run serial Git merge/rebase processes in the background.
  - Added the `Release` orchestrator for automated semver bumping and changelog formatting.
  - Added policy-based safety checkers in the `Validator` service.
  - Created tool registries for bootstrapping and production environments.
- **Infrastructure & Adapters (`pkg/infrastructure`)**:
  - Built SQLite and PostgreSQL storage adapters utilizing migrations, SELECT FOR UPDATE row locking, connection pools, and WAL mode.
  - Created the LLM client adapter featuring a model failover/fallback client and a lenient format output parser.
  - Added the Git VCS adapter wrapping clone, checkout, push, and rebase commands.
  - Integrated the Jira API client featuring an ADF (Atlassian Document Format) parser to walk description payloads.
  - Implemented hierarchical configurations parsing environment variables, files, and CLI overrides with rigorous tests.
- **Command Line Interface (`cmd/noctifab`)**:
  - Structured standard subcommands using the Cobra framework: `start` (as a daemon), `run-once`, `plan`, `validate`, `git-init`, and `maintenance`.
- **E2E & Integration Testing (`tests/e2e`)**:
  - Setup Docker-compose orchestration for end-to-end integration tests.
  - Implemented mock LLM and mock Git VCS CGI services.
  - Added test scenarios covering compaction sandbox, DAG pruning, Django CRUD, flaky budgets, mock LLM, parallel refactoring, schema migrations, and integration hooks.
- **Project Specifications (`examples`)**:
  - Created spec-driven targets containing markdown specifications for validation validation targets: `frontpunch`, `weather-api`, `todo-cli`, `url-shortener`, `task-scheduler`, and `markdown-to-html`.
- **Documentation & Configuration**:
  - Detailed architectural guidelines and coding rules (`AGENTS.md`, `GEMINI.md`, `ROADMAP_MVP.md`, `TESTS.md`).
  - Created the extensive 2,300+ line technical specification (`SPEC.md`).
  - Added Sphinx documentation (`docs/`) with custom styling, conf.py, developer guide, and cli usage.
  - Configured CI pipeline with GitHub Actions (`ci.yml`) to separate unit and E2E tests, and configured linter check.
  - Added a release workflow (`release.yml`) for multi-platform binary compilation.
  - Created `Makefile` target configurations for compilation, testing, and linting.
  - Ignored `/dist/` build output folder in `.gitignore` and added `.readthedocs.yaml`.

