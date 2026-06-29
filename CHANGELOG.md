# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.1] - 2026-06-29

### Added
- **New Rust `wc` Validator Project**: Added a new E2E validator project replicating UNIX `wc` in Rust under `validation/projects/wc` with specifications and user stories (US-001, US-002, US-003) enforcing SOLID/DDD and memory-efficient streaming.
- **Rust Toolchain in Validation Container**: Added `rust` and `cargo` packages to the E2E verification image (`Dockerfile.validation`).
- **Rust validation check**: Updated `validate.sh` to check for `Cargo.toml` and `src/main.rs`.

## [0.2.0] - 2026-06-29

### Added
- **Simplified Permission Profiles**: Consolidated custom agent role profiles directly in `.noctifab/config.yaml` under the `profiles:` block.
- **Secure Built-in Defaults**: Enabled secure, built-in memory-based permission profiles for the four standard agent roles (`orchestrator`, `planner`, `tester`, `generator`). If overrides are not provided in `config.yaml`, the system automatically falls back to these secure defaults, resolving the write-blocking bug on missing `generator.yaml` profiles.

### Changed
- **Workspace Scaffolding Cleanup**: Removed profiles directory and YAML files generation from the `noctifab init` command.

## [0.1.5] - 2026-06-29

### Fixed
- **Documentation Discrepancies**: Removed non-existent CLI flags (`--llm-api-key`, `--vcs-token`, `--jira-token`) from `docs/cli_usage.md`, `docs/secrets.md`, and `SPEC.md` examples. Documented environment variables (`NOCTIFAB_LLM_API_KEY`, `NOCTIFAB_VCS_TOKEN`, `NOCTIFAB_JIRA_TOKEN`) as the supported mechanism to override credentials.
- **Secrets Precedence**: Documented specific precedence logic for sensitive credentials where CLI flags are disabled for security reasons.

### Changed
- **Pillar Highlights**: Updated `README.md` to detail the 3x Test Validator execution with majority vote consensus inside the Test-Driven Quality Gates pillar.

## [0.1.4] - 2026-06-29

### Added
- **Per-provider 429 retry delay parsing**: Introduced `httpError` struct that carries the HTTP response headers alongside the body. All provider `Call` methods (`gemini.go`, `openai.go`, `anthropic.go`) now return `*httpError` instead of a plain `fmt.Errorf`, making HTTP headers available to the retry layer.
- **`Retry-After` header support**: `parseRetryDelay` now reads the standard `Retry-After` header (integer or fractional seconds) returned by OpenAI, Anthropic, Mistral, and DeepSeek on 429 responses.
- **HuggingFace `ratelimit` header support**: Parses the `t=<seconds>` field from the HuggingFace `ratelimit` response header (e.g. `"api";r=0;t=55`).
- **Extended `TestParseRetryDelay`**: Added 7 new test cases covering `Retry-After` header (integer and fractional), HuggingFace `ratelimit` header, priority ordering (header beats body), Gemini complex duration strings (e.g. `7h2m3s`), and no-hint-present fallback.

### Fixed
- **Gemini `retryDelay` parsing**: Replaced `fmt.Sscanf` numeric fallback with `strconv.ParseFloat` for robustness; simplified the duration/numeric branching logic.

### Changed
- **Documentation Updates**: Updated `README.md` to reflect the latest orchestrator design, specifically describing the sequential execution flow, the relationship between the Generator and Tester agents, and the profile-based RBAC/security sandbox system.

### Research
- Investigated 429 rate-limit response formats for all 7 supported providers (Gemini, OpenAI, Anthropic, Mistral, DeepSeek, HuggingFace, Ollama). Key finding: only Gemini embeds retry timing in the JSON body; all others rely exclusively on HTTP headers. Ollama (local) never returns 429 — it uses 503 for queue-full conditions.

## [0.1.3] - 2026-06-29

### Added
- **Model Fallback Hierarchies for All Providers**: Extended `modelHierarchy` in `helpers.go` with static fallback chains for Mistral (`large → medium → small → open-mistral-7b`), DeepSeek (`coder → chat`), Hermes (`405b → 70b → 8b`), and Anthropic (`sonnet → haiku`). All providers now support automatic downgrade to a smaller model when the chosen model is unavailable.
- **Unit Tests for Gemini and Anthropic Provider Clients**: Added `gemini_test.go` and `anthropic_test.go` covering `Call` success/error paths and `GetAvailableModels`.
- **Extended `TestGetNextLowerModel`**: Added test cases for all new providers (Mistral, DeepSeek, Hermes, Anthropic) and bottom-of-chain boundary cases.
- **LLM Providers section in README**: Documented all supported providers with YAML configuration examples, secrets file examples, resilience features (retry, 429 handling, fallback), and the complete model fallback chain table.

### Fixed
- **Model matching precision**: Replaced fuzzy `strings.Contains` matching in `getNextLowerModel` with exact equality, preventing false matches between models sharing a common prefix (e.g., `gpt-4o` incorrectly matching `gpt-4o-mini`).

## [0.1.2] - 2026-06-29

### Changed
- **LLM Client Refactoring**: Refactored the monolithic LLM client into a modular interface-driven architecture. Created a `ProviderClient` interface (`provider.go`) and extracted OpenAI (`openai.go`), Gemini (`gemini.go`), and Anthropic (`anthropic.go`) clients into separate, dedicated implementations.


## [0.1.1] - 2026-06-29

### Added
- **Dynamic Base Branch Detection**: Support `"git-detect"` as a base branch configuration value to dynamically resolve the base branch to the current active git branch.
- **Verification Script**: Created `scripts/verify_autonomy.sh` and containerized `scripts/verify_autonomy_docker.sh` + `scripts/Dockerfile.verify` to validate execution loop end-to-end.
- **Five New AI Providers**: Added support for Nous Research Hermes, HuggingFace, Mistral, DeepSeek, and Ollama Cloud LLM providers using OpenAI-compatible endpoints.
- **429 Quota Exceeded Warnings**: Warn the user on `os.Stderr` when 429 quota exhaustion errors occur.
- **API retryDelay Backoff Support**: Parse the API-returned `retryDelay` from error responses and apply it as the next backoff wait time.

### Fixed
- **Log Sanitizer / Synthesizer**: Implemented a failure log synthesizer that extracts key error messages and tracebacks, preventing LLM formatting confusion and JSON parsing crashes during retries.
- **Branch Cache Reset**: Force-reset the integration branch to the base branch on fresh story start if it already exists, avoiding dirty caches from previous runs.
- **Tester Agent Fix Prompt Handling**: Resolved a bug where `Fix the tests for task:` prompt types were not intercepted in the LLM client, resulting in unformatted Tester LLM responses.
- **Dynamic Project Context Prompts**: Preprocess and adapt prompt templates to inject specific target package details, target files examples, and robust design guidelines based on the detected target project.

## [0.1.0] - 2026-06-28

### Added
- **Sequential Agent Dependency Flow**: Reordered the task orchestrator execution cycle so that the Generator Agent always runs before the Tester Agent in both initial and refactoring/retry phases.
- **Task Retry State Reset Warning**: Added explicit context warnings to both the Generator and Tester agents during task retries to inform them that files have been reset to the clean base commit.
- **Git Revert Loop on Task Retry**: Implemented a recursive commit reset loop to clean previous refactoring attempts from task branches on retries.
- **Development Rule Updates (`AGENTS.md`)**: Documented rules restricting main branch commits and enforcing changelog updates on every commit.

### Fixed
- **Robust Isolated Test Runner**: Modified the isolated test discovery to read file contents and exclude helper python scripts that do not declare `TestCase` classes, avoiding false-positive validation failures on `0 tests ran`.

## [0.0.4] - 2026-06-28

### Added
- **Test Suite Isolation**: Intercepts python test discovery execution in `HostSandbox` to run each `test_*.py` file in a separate, isolated python process. This prevents global state mutations (such as disabling logging or changing global variables) from polluting subsequent tests.
- **Automatic Target Files Context Inheritance**: Implemented recursive dependency target files resolution in `Orchestrator.executeTask`. Downstream tasks automatically inherit the target files of their upstream dependencies.
- **Dynamic Read-Only Repository Visibility**: Updated the Context Gathering reader prompt to query and inject a list of all tracked repository files (via `git ls-files`), enabling agents to call `read_file` on any existing module.
- **Inter-Agent Communication Loop**: Added the `request_test_fix` tool and orchestrator intercept logic to let the Generator Agent dynamically request the Tester Agent to fix buggy tests, resolving retry deadlocks.
- **Agent Guidelines & Safeties**:
  - Added Tester Rule 20 to forbid global state mutations in unit/integration tests.
  - Added Generator Rule 9 to enforce dependency injection of logging objects.
  - Added Generator Rule 10 to require surgical merging via `edit_file` instead of wholesale `write_file` overwrites.
  - Added Generator Rule 11 to require checking and adding missing dependencies/manifests (e.g. `pyproject.toml`, `docker-compose.yml`) before implementing features.

## [0.0.3] - 2026-06-26

### Added
- **Secrets Management (`secrets.yaml`)**:
  - Introduced support for a gitignored `.noctifab/secrets.yaml` file to keep credentials out of version control.
  - Any string value in `config.yaml` prefixed with `secret:` (e.g. `secret:GEMINI_API_KEY`) is resolved at load time from the corresponding key in `secrets.yaml`.
  - Fields supporting secret references: `llm.api_key`, `llm.url`, `vcs.token`, `jira.token`, `jira.user`, `jira.url`.
  - `noctifab init` now automatically adds `secrets.yaml` to `.noctifab/.gitignore`.
  - Added `LoadSecrets`, `resolveSecretRef`, and `applySecretsToConfig` in `pkg/infrastructure/config/secrets.go` with 100% unit test coverage.
- **`clean` Command Improvements**:
  - Added `--yes` / `-y` flag to skip the interactive confirmation prompt (useful in scripts and CI).
  - Added `--dry-run` flag to preview which files and directories would be removed without performing any deletions.
  - Removed the `--force` flag entirely; daemon-running guard now uses `--yes` or the interactive prompt.
- **Documentation**:
  - Added `docs/secrets.md`: comprehensive secrets management reference covering setup, supported fields, priority precedence, CI/CD patterns, and a security checklist.
  - Updated `docs/cli_usage.md`: workspace structure now shows `secrets.yaml`; `clean` command docs reflect the new flags; added Secrets Management quick-start section.
  - Updated `README.md`: new Secrets Management section with quick-start examples.

### Changed
- **`clean` command** refactored into focused helper functions (`runClean`, `askConfirmation`, `runDryClean`, `runActualClean`, `cleanPostgres`, `cleanSQLiteDB`, `removePIDFile`, `removeStoryLogs`, `removeDaemonLog`) for improved readability and testability.

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
- **CI Workflow Optimization**:
  - Restricted the CI `push` trigger to only run on the `main` branch to prevent duplicate CI job runs when branches have active pull requests.

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

