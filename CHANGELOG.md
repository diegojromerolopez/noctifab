# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.18.4] - 2026-07-31

### Fixed
- **Unblocker Goroutine Leak in Story Loop**: Wrapped per-story execution in an anonymous function scope in `cmd/noctifab/cli/start.go` so `defer cancelUnblocker()` and `defer ticker.Stop()` execute at the end of each story iteration, preventing accumulating background unblocker goroutines during multi-story runs.

### Changed
- **CLI `-i` Flag Shorthand Documentation**: Documented breaking change in `cmd/noctifab/cli/root.go` where the persistent `-i` shorthand was reassigned from `--input` to `--interactive` (and `start-one` was merged into `start`).

## [0.18.3] - 2026-07-30

### Removed
- **Removed Markdown Specs**: Removed `BENCHMARK.md` and `BREATH_FIRST_GENERATION.md` documentation files and cleaned up stale reference links in `AGENTS.md` and `README.md`.

## [0.18.2] - 2026-07-30

### Fixed
- **E2E Test Parameter Alignment**: Updated `tokenLimit` parameters and token usage assertions across E2E scenario tests (`tests/e2e/`), added non-template `SPEC.md` and user story stubs for `TestE2E_StartCommand` / `TestE2E_StartOneCommand`, and added local `dist/noctifab` binary resolution fallback when running outside containers.
- **Environment Override Safety in Unit Tests**: Cleared `NOCTIFAB_E2E` environment overrides in `pkg/infrastructure/config/config_test.go` subtests to guarantee deterministic validation checks.

### Removed
- **Cleaned Up Plan & Feedback Docs**: Removed temporary review and feedback markdown files (`DARK_FACTORY_REVIEW.md`, `FORTUNE_FEEDBACK.md`, `UX.md`).

## [0.18.1] - 2026-07-30

### Changed
- **Documentation Alignment (`AGENTS.md` & `README.md`)**: Synced development guidelines in `AGENTS.md` and user documentation in `README.md` to reference `BREATH_FIRST_GENERATION.md`, document LLM Provider Struct Embedding composition rules, detail Verification vs. Validation testing strategy, incorporate the Product Manager Definition of Done (DoD) mandate, add short architecture names (`cfv`, `spe`, `bfg`), and standardize top-level `workspace_cache:` configuration syntax.

## [0.18.0] - 2026-07-30

### Added
- **Breadth-First Generation (`breadth_first`) Architecture**: Implemented a new execution architecture mode ([BREATH_FIRST_GENERATION.md](file:///Users/diegoj/repos/noctifab/BREATH_FIRST_GENERATION.md)) where Generator and Tester agents focus on delivering ~80% core happy-path functionality across all tasks first. Non-critical linter nitpicks, formatting guidelines, and obscure corner cases are deferred to subsequent refinement passes under Benevolent Judge evaluation.
- **Benevolent Judge Zero-Regression Enforcement**: Integrated Zero-Regression checks in `executeTaskBreadthFirst` ([pkg/services/orchestrator_execute_breadth_first.go](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator_execute_breadth_first.go)) to ensure iterative refinements never degrade previously validated happy-path features.
- **Short Architecture Names & Normalization**: Added support for concise architecture names (`code_first`, `single_pass`, `breadth_first`) and acronyms (`cfv`, `spe`, `bfg`) in `agents.architecture` via `NormalizeArchitecture` ([pkg/infrastructure/config/config.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/config/config.go#L280)) while maintaining full backward compatibility for legacy strings (`code_first_verification_loop`, `single_pass_execution`, `breadth_first_generation`).

## [0.17.1] - 2026-07-30

### Added
- **Product Manager Definition of Done (DoD) Mandate**: Injected a language-agnostic Definition of Done & Interface Contract rule into `buildProductManagerPrompt` ([pkg/infrastructure/llm/prompt_templates.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/prompt_templates.go#L48)). Generated User Stories (`roadmap/US-xxx.md`) are now required to specify explicit public API signatures, binary executable paths, I/O formatting invariants, error prefixes, exit codes, number precision representations, and zero-failure test pass criteria before downstream task planning starts.

### Fixed
- **Per-Tool Formatter Execution Overhead Removed**: Removed synchronous `runFormatterIfConfigured` calls after every single `write_file` and `edit_file` execution in `orchestrator_helper.go`. Code formatting is now executed only during explicit linter passes, eliminating ~180 blocking RuboCop subprocess boots per run.
- **Top-Level `workspace_cache` Configuration**: Relocated `workspace_cache` from `agents.workspace_cache` to root-level `workspace_cache:` in `.noctifab/config.yaml` and `Config` struct (with fallback for backward compatibility).

## [0.17.0] - 2026-07-29

### Added
- **Verification vs. Validation Engineering Strategy**: Decoupled task execution into two distinct stages: *Verification* (building minimal working code that compiles and passes basic functional checks) and *Validation* (iteratively refactoring and hardening code under test safety rails).
- **Black-Box Testing Mandate**: Updated Tester Agent prompts to strictly mandate black-box testing against public API contracts, return values, and CLI/system outputs. Tests are explicitly forbidden from asserting or depending on internal module structures, private struct fields, or method layouts.
- **Generator Pre-Submission Self-Verification**: Updated Generator Agent prompts to require running `run_tests` in-session before returning `noop`, fixing build/syntax errors in-place to avoid 30–45s Orchestrator task failure/re-queueing cycles.
- **Product Manager Roadmap Consolidation**: Added Product Manager prompt handling (`buildProductManagerPrompt`) enforcing single-story generation (`roadmap/US-001.md`) for standalone utilities or specifications under 500 LOC to prevent multi-story over-decomposition overhead.
- **GNU Makefile & C Scaffolding Best Practices**: Injected standard GNU Makefile multi-directory wildcard patterns (`SRCS = $(foreach dir,$(SRC_DIRS),$(wildcard $(dir)/*.c))`) and non-empty compilation unit stubs into system prompts.

## [0.16.1] - 2026-07-29

### Fixed
- **Unblocker Agent Wiring**: Wired up `UnblockerAgent` initialization in `cmd/noctifab/cli/start.go` and `cmd/noctifab/cli/serve.go`. The autonomous unblocker daemon now starts automatically alongside the orchestrator loop whenever `unblocker.enabled` is active in `.noctifab/config.yaml`.


## [0.16.0] - 2026-07-27

### Added
- **Dedicated Provider Files (Go Struct Embedding Composition)**: Refactored LLM provider infrastructure into dedicated per-provider Go files (`mistral.go`, `moonshot.go`, `deepseek.go`, `qwen.go`, `llama.go`, `xai.go`, `perplexity.go`, `cohere.go`, `opencode.go`, `ollama.go`, `huggingface.go`, `fireworks.go`, `sambanova.go`, `hermes.go`, `groq.go`, `openrouter.go`, `together.go`). Each file is self-contained: it defines a typed client struct embedding `*baseOpenAIClient`, a declarative `NewModelParser`-based capacity parser, and its `ProviderSpec` registration in `init()`.
- **`baseOpenAIClient` Composition Base**: Extracted `baseOpenAIClient` as a reusable composition base in `openai.go` exposing the full OpenAI HTTP wire protocol (`Call`, `GetAvailableModels`, `sendCompletion`, `sendCompletionStreaming`, `resolveEndpoint`). All OpenAI-compatible provider structs embed `*baseOpenAIClient` to inherit all methods without code duplication.
- **`NewClientFunc` in `ProviderSpec`**: Added `NewClientFunc` field to `ProviderSpec` in `provider_registry.go`, enabling `client.go` to instantiate provider clients via a single zero-switch call: `spec.NewClientFunc(url, timeout, idleTimeout, streaming)`.
- **`NewModelParser` Declarative Composition Engine**: Moved `NewModelParser`, `ParserConfig`, `KeywordTier`, `ModelParser`, and `StandardSizeWeights` into `provider_registry.go` for shared access across all provider files. Each provider now defines its model capacity parser in 5-10 declarative lines instead of verbose procedural functions.
- **Interactive Mode & Asset Directory**: Created the `assets/` directory for repository media and added an Interactive Mode overview section with screenshot references to `README.md`.
- **Interactive Dashboard Total Elapsed Time Metric**: Added total execution elapsed time calculation (`computeTotalElapsed`, `formatDuration`) to the interactive TUI dashboard header telemetry panel and control footer.

### Changed
- **Eliminated `provider_parsers.go`**: Removed the monolithic 400+ line file containing all procedural `parse<Provider>Model` functions. Parser logic is now co-located with each provider's own file.
- **Zero `switch` Statements in Core Dispatch**: `client.go` no longer contains any protocol `switch` blocks for client creation; dispatch is fully data-driven through `ProviderSpec.NewClientFunc`.

## [0.15.0] - 2026-07-27

### Added
- **Context Slicing & AST Symbol Indexing (`context.mode`)**: Added configurable context slicing service (`ContextSlicer`) supporting `full` (default, full source files), `diff_window` (git diff line windows and test stack traces), and `tree_sitter` (universal AST symbol map parsing) modes in `.noctifab/config.yaml`.
- **Workspace Inspection Caching (`agents.workspace_cache.enabled`)**: Added in-memory workspace inspection tool caching (`list_directory`, `read_file`, `find_files`, `grep_search`) and diagnostic tool caching (`run_tests`, `run_linter`) in `TaskDiagnosticCache`, automatically invalidated when file mutations occur (`write_file`, `edit_file`, `delete_file`), controlled by `agents.workspace_cache.enabled` (defaulting to `true`).
- **Build Script Symbol Linking Directive**: Added prompt directive to universal anti-stalling mandates instructing agents to link all implementation source files alongside test files in Makefiles and build scripts.
- **Validation Project Matrix Updates**: Added `fortune` project spec & configuration, and enabled performance metrics, workspace inspection caching, and context slicing configurations across all 6 validation target projects (`calculator` set to `mode: tree_sitter`; `echo`, `fortune`, `frontpunch`, `todo-cli`, `wc` set to `mode: full`).
- **Documentation Updates**: Updated `README.md`, `docs/architecture.md`, `docs/configuration.md`, and `docs/configuration_examples.md` with metrics, streaming, workspace caching, and context slicing configurations and architectural details.

## [0.14.0] - 2026-07-26

### Added
- **Performance & Speed Metrics Instrumentation (`telemetry.metrics.enabled`)**: Added thread-safe performance and speed metrics collector (`MetricsCollector`) to track Time To First Commit (TTFC), per-phase execution latencies (`Reader`, `Planner`, `Generator`, `Tester`, `Validator`), LLM API wait duration, token output throughput (tokens/sec), sandbox build times, and retry counts, exported to `.noctifab/data/metrics.json`.
- **Dark Factory Architecture Review**: Added `DARK_FACTORY_REVIEW.md` document outlining optimizations for speed, architectural completeness, and reliability.

## [0.13.0] - 2026-07-26

### Added
- **HTTP SSE Streaming Transport (`llm.streaming`)**: Added configurable Server-Sent Events (SSE) token streaming transport across OpenAI-compatible, Gemini, and Anthropic LLM provider clients, controlled by the `llm.streaming` configuration boolean (defaulting to `true`).
- **Sliding Idle Socket Timeout**: Integrated sliding socket inactivity timer (`readSSEResponse`) that resets on every chunk arrival and triggers instant provider failover if 0 tokens are received for 15 consecutive seconds (`llm.idle_timeout`).
- **Documentation Updates**: Updated `docs/configuration.md` and `docs/architecture.md` with `streaming` configuration details and SSE streaming architecture breakdown.

## [0.12.3] - 2026-07-24

### Added
- **LLM Idle Timeout Configuration (`llm.idle_timeout`)**: Added configurable stream and socket inactivity timeout `idle_timeout` (defaulting to `15s`) in `LLMConfig` and `FailoverBackend` structs. Automatically triggers client failover when zero response bytes are received for 15 seconds continuously without truncating active long responses.
- **Validation Project Configurations**: Added `idle_timeout: 15s` to all 6 target validation project configurations (`calculator`, `echo`, `fortune`, `frontpunch`, `todo-cli`, `wc`).
- **Documentation Updates**: Updated `README.md`, `docs/configuration.md`, and `docs/configuration_examples.md` with `idle_timeout` settings and descriptions.

## [0.12.2] - 2026-07-23

### Changed
- **Reference Test Guidelines**: Updated `AGENTS.md` to explicitly link to `TESTS.md` for test suite execution and strategy details.
- **Removed Autonomy Roadmap**: Deleted `AUTONOMY.md` after verifying all autonomous software factory objectives (budget tracking, OCC, failover, watchdogs, flaky tests, hot-reload, SAST, dependency management, and intent disambiguation) are fully implemented and tested.
- **Documented Hidden/Undocumented Features**:
  * Documented the `noctifab dashboard` command and its interactive keyboard shortcuts in `docs/cli_usage.md`.
  * Created `docs/api.md` containing detailed descriptions of all loopback REST API endpoints (pause, resume, cancel, status, manual tasks, override-merge).
- **Added Config Examples**: Created `docs/configuration_examples.md` containing complete configuration templates for Python, Node.js, Go, and resilient multi-provider enterprise environments.

## [0.12.1] - 2026-07-15

### Fixed
- **Avoid Introspection in Scaffold/Environment Testing**: Updated the Tester Agent's system prompt instructions to explicitly forbid checking package installations, environment variables, or library configurations using API introspection/reflection (e.g., asserting internal RSpec config hashes). Instead, instruct verifying installations through basic smoke/sanity tests (e.g., executing a dummy test file).

## [0.12.0] - 2026-07-15

### Added
- **Resilient Gemini Model Fallback Strategy**: Implemented dynamic `models.list` querying on Gemini API failures (any error). The client parses, groups, and sorts the returned active models descending by `Version` (e.g. 3.5, 3.1, 3.0, 2.5, 2.0, 1.5, 1.0) and `Tier Rank` (`pro`: 4, `flash`: 3, `flash-lite`: 2, `nano`: 1). It automatically falls back to the immediate lower model in the active hierarchy, providing robust and self-healing LLM resolution.

## [0.11.1] - 2026-07-15

### Fixed
- **Increased Task and Watchdog Max Retries**: Increased default `max_retries` for tasks and `watchdog_repair` from 3 to 10 in `bootstrap_tools.go`, `watchdog_repair.go`, `start_one.go`, and `serve.go` to provide agents sufficient turns to align test assertions and correct subtle linter/compiler issues before failing validation permanently.

## [0.11.0] - 2026-07-13

### Added
- **Directory User Stories Support in start/serve**: Extended the `/api/v1/stories` REST API endpoint and the interactive REPL listener command parser to automatically detect and support folder paths. If the passed path is a folder, `noctifab` automatically resolves and enqueues all markdown files in lexicographical order as user stories.
- **Directory Stories support in validate.sh**: Updated the validation harness `validate.sh` to configure `roadmap` folders for the `wc`, `frontpunch`, and `todo-cli` validation projects, expanding them sequentially on the Bash side for `start-one` runs to execute the full sequence of stories.

## [0.10.1] - 2026-07-12

### Fixed
- **`edit_file` Directive Error Message**: When `target_content` is not found in a file, the error message now explicitly instructs the agent to call `read_file` first, then retry `edit_file` with the correct target or fall back to `write_file`. This breaks the retry loop where agents exhausted all turns repeating the same mismatched `edit_file` call.

## [0.10.0] - 2026-07-12

### Added
- **Multi-Turn Agent Loop**: Added iterative code-generation and validation loop (up to 5 turns) for Generator and Tester agents to immediately resolve compile, test, or lint errors using inline tool execution feedback.
- **Configurable Tool/Test Execution Timeouts**: Made `RunTestsTool` and `RunLinterTool` timeouts configurable via `sandbox.timeout_seconds` configuration to prevent premature timeouts on long test suites.
- **General Watchdog Repair**: Expanded the `WatchdogRepair` handler to trigger for compilation and test logic failures in addition to execution timeouts, providing self-healing capability for all validation issues.
- **Global Action Ceiling Enforcement**: Wired the `max_actions` setting to act as a global story execution circuit breaker in the orchestrator, preventing infinite task cycles.

## [0.9.1] - 2026-07-08

### Added
- **Interactive Validation Flag**: Added `-i` option support to `run_one.sh` and `INTERACTIVE=1` argument inside `Makefile` to launch the validation Docker container interactively with a TTY attached and without standard output redirection.
- **macOS Docker Mount Sync Fix**: Configured `run_one.sh` to preserve directory nodes (cleaning file contents instead of deleting output directories) and added a filesystem synchronization pause to eliminate macOS Docker Desktop FUSE mount race conditions.
- **Docker Runner curl and Build Dependencies**: Installed `curl` in `Dockerfile.validation` and all project runner Dockerfiles, and added `build-base` to the `wc` Dockerfile to support linking Rust proc-macros like `clap_derive` on Alpine.
- **Rust Toolchain Upgrade**: Upgraded `wc` Dockerfile toolchain base to `rust:alpine` (Rust 1.85+) to support Cargo Edition 2024 dependencies.
- **Harness Logging Redirection**: Redirected all setup messages and configuration yaml dumps inside `validate.sh` to standard error (`>&2`) to prevent terminal stdout pollution and screen corruption prior to TUI rendering.

## [0.9.0] - 2026-07-08

### Added
- **Interactive TUI Dashboard**: Implemented `noctifab dashboard` providing real-time tracking of multiple user stories and tasks with visual progress bars.
- **Story Control REST APIs**: Added endpoints (`POST /api/v1/pause`, `POST /api/v1/resume`, `POST /api/v1/cancel`) to pause, resume, or cancel active story orchestrations.
- **Daemon Pause/Cancel Integration**: Wired daemon worker cycle ticker in `serve.go` to suspend cycles when paused and safely interrupt tasks, revert branches, and clear locks when cancelled.
- **SQLite and PostgreSQL LoadAll and LoadByID**: Added custom repository methods with database connection pool starvation safety.
- **Milestone Progress Tracking**: Orchestrator task execution updates task completion percentages (25%, 50%, 75%, 100%) and updates working agents in active agents registry.

## [0.8.3] - 2026-07-08

### Changed
- **README Emoji Update**: Added 🤖🌌 emojis to the [README.md](file:///Users/diegoj/repos/noctifab/README.md) title to reflect the platform's autonomous agent and night/nocturnal software dark factory theme.

## [0.8.2] - 2026-07-08

### Added
- **Exposed Database and Tool Registry Getters**: Added public `DB()` methods to `SQLiteRepository` and `PostgresRepository`, and a `Tools()` method to `Registry`/`ToolRegistry` to support instantiating the budget and repair handlers in downstream commands.

### Fixed
- **Autonomy Wiring in CLI Commands**: Wired `SQLiteBudgetStore` / `PostgresBudgetStore` into the failover LLM client, enabled `WatchdogRepair` as the default orchestrator `repairHandler`, and configured the `TestValidator` with the LLM client and tools map to enable self-repair and flaky-test stabilization in both the `serve` and `start-one` subcommands.

## [0.8.1] - 2026-07-07

### Fixed
- **Makefile validation-images target**: Added the missing recipe implementation for the `validate-images` target, which builds the base validation image and all per-project Docker validation images.
- **Budget Save Error Handling**: Changed `FailoverClient` to propagate database persistence errors returned by `IncrementUsage` during API usage logging, rather than silently swallowing them and bypassing daily budget safeguards.
- **Spec Improvements**: Resolved a missing `"strings"` import in `AUTONOMY.md` for `CostForTokens` and fully completed the missing `sqliteBudgetStore` stubs (`LoadBudget` and `ListBudgets`) in the level-5 specifications.
- **PostgreSQL Cleanup and Rollbacks**: Integrated `budget_usage`, `validation_criteria`, and `active_agents` tables into the `noctifab clean` CLI command's PostgreSQL drop sequence and validation allowlist, preventing orphaned tables.

## [0.8.0] - 2026-07-07

### Added
- **Echo Validation Project**: Added the `echo` minimal validation project in Go, including `SPEC.md`, user story roadmap, and configs.
- **Validation Workspace Bind-Mounting**: Updated validation runner and harness to automatically create and bind-mount `output/` folders (`src/`, `dist/`, `log/`, and `feedback/`) from the host under `validation/projects/<project>/output/`.
- **Gitignore and Dockerignore Exclusions**: Excluded the outputted validation project `output/` directory from git tracking and docker build context.
- **Conditional PR Creation**: Modified `noctifab` to make PR creation conditional on `VCS.PullRequest.AutoCreate` in `OrchestratorConfig`, allowing E2E runs to bypass PR creation and just commit changes to local Git.
- **Validation Host Pre-Clean**: Configured validation runner (`run_one.sh`) to automatically clean and delete the target project's host `output/` directory before recreating directories and running the validation container, ensuring fresh test executions.
- **Auto-create Configuration**: Added configuration for `pull_request.auto_create` set to `false` directly inside the echo project's `.noctifab/config.yaml` to control PR creation behavior directly.

## [0.7.0] - 2026-07-06

### Added
- **Language-Agnostic Linter Tooling**: Implemented `RunLinterTool` (`run_linter` tool) that executes the project's configured `linter_command` inside the sandbox workspace, exposing it to both Generator and Tester agents for local formatting and style checks.
- **Language-Agnostic System Prompts**: Generalized agent prompts in `client.go`, removing Python-specific and project-specific instructions (e.g. `pytest`, `threading`, `redis`) to comply with repository agnosticism guidelines.
- **Instruction to Avoid Truncation Placeholders**: Added specific instructions to prompts warning agents not to include `[TRUNCATED]` placeholders in `target_content` parameters for file editing tools.

### Changed
- **Incremental Agent State Retention**: Refactored the orchestrator's task retry logic in `orchestrator_execute.go` to keep and build on top of previous worker branch commits instead of executing a hard reset (`git reset --hard HEAD~1`) on validation failures.

## [0.6.0] - 2026-07-06

### Added
- **Autonomous User Story Generation from SPEC.md**: Added support for spawning a dynamic **Product Manager Agent** to decompose `SPEC.md` into detailed user stories when the `roadmap/` directory is empty or missing, or when `SPEC.md` is passed directly as the input.

## [0.5.0] - 2026-07-05

### Added
- **Multiple LLM Client Configurations Support**: Added support for configuring a list of LLM backends under `llms:` in `config.yaml`.
- **Dynamic Secrets Resolution for Backend list**: Updated secrets processing to recursively resolve `secret:` references for all backends in the `llms:` configurations list.
- **Failover Client Integration**: Updated client bootstrap factory to automatically instantiate a `FailoverClient` wrapping the backend configurations in order when multiple LLMs are defined.

### Fixed
- **wc validation roadmap renumbering**: Renumbered the user stories for the `wc` validation project to be strictly sequential (`US-001.md` through `US-004.md`), updated their titles, dependencies, and internal references, and adjusted the E2E verification harness and READMEs to execute the correct story targets.

## [0.4.2] - 2026-07-05

### Fixed
- **E2E Validation Configuration & Sandbox Whitelist**: Corrected `todo-cli` configuration to whitelist `go` and `git` commands, preventing sandbox violations during test execution. Re-architected `todo-cli` Dockerfile to build from `golang:alpine`.
- **E2E Validation target path for frontpunch**: Corrected the target file validation check for `frontpunch` to check `frontpunch/client.py` (which is the actual output of US-001) instead of `frontpunch/worker.py`.
- **Feedback Generator false positive test failures**: Refined `gen_feedback.py` to pre-process container logs by stripping out code blocks from tool call arguments and raw LLM response chunks, and ignoring system lines in `test_failures` matching, eliminating noisy false-positive test failures.
- **Documentation cleanup**: Fixed a typo in `validation/README.md` describing `todo-cli` as implemented in Python instead of Go.

## [0.4.1] - 2026-07-05

### Fixed
- **GITHUB_TOKEN Standardization**: Ensured all validation projects (`frontpunch`, `todo-cli`, `wc`) and documentation (`docs/secrets.md`) use `GITHUB_TOKEN` consistently instead of the legacy `GITHUB_FRONTPUNCH_TOKEN`.
- **E2E/Integration Test Liveness**: Skipped the real LLM provider ping during tests/E2E runs (when `NOCTIFAB_E2E=true` is set), preventing test suites from attempting external LLM API connections and failing without credentials.

## [0.4.0] - 2026-07-03

### Added
- **Validation Project Spec/Story Quality Pass**: Reviewed and hardened the SPEC.md and roadmap user stories of all three validation projects (`wc`, `todo-cli`, `frontpunch`) so an LLM agent (GLM-5.2) can build them autonomously with minimal guessing. Per-project changes:
  - **wc**: Pinned Rust toolchain/edition/crate name + allowed deps (clap, assert_cmd, tempfile); added a pinned DDD directory layout to SPEC §2.1; rewrote SPEC §3.3 output format with an exact `format!("{:>7} {:>7} ...")` reference formatter; added SPEC §3.4 (counting semantics for `-w`/`-c`/`-m`/`-L` including invalid-UTF-8 fallback, `\r\n` handling, no-final-newline handling) and §3.5 (exact stderr templates + exit codes 1 vs 2); fixed the wrong UTF-8 example arithmetic in §4.5 (`"Hello, 世界!\n"` = 11 chars / 15 bytes, not 13 chars); added `US-000a` (project scaffold + domain core) so US-001 fits the ~4096-token completion budget; added `depends_on`/`change_type`/`target_files` front-matter to US-001/002/003; pinned the wc sandbox `linter_command` to `cargo fmt --check && cargo clippy -- -D warnings`.
  - **todo-cli**: Pinned the language to **Go 1.22+** (was "Go, Python, or Node.js" — a contradiction with AGENTS.md tooling); added SPEC §2.3 (DDD directory layout: `cmd/todo/`, `internal/task/`, `internal/storage/`, `internal/cli/`), §2.4 (ID auto-increment + `rm` non-reindexing + idempotent `done` semantics), §2.5 (byte-level list output format), §2.6 (exact exit codes + stderr message templates), §2.8 (explicit waiver of the AGENTS.md Postgres compose stack for this project); rewrote US-001 and US-002 with `depends_on`/`change_type`/`target_files` metadata, replaced `python3 todo.py` with the Go binary invocation form, and replaced ambiguous "table or formatted list" with byte-exact expected strings and actionable unit/integration/BDD test splits.
  - **frontpunch**: Fixed the broken JSON in SPEC §2.3 (missing closing `}`); pinned the web framework to **FastAPI** (was "Flask or FastAPI"); pinned dependencies (`redis>=5.0,<6.0`, `fastapi`, `cryptography`, `croniter`, `click`); added SPEC §2.7 (`frontpunch.configure(...)` singleton contract + the full `frontpunch.exceptions` hierarchy); added SPEC §3.1 (DDD project layout with `pyproject.toml`, `domain/`/`application/`/`infrastructure/valkey/`/`interfaces/`); reconciled the `class` field to require a fully-qualified dotted import path (fixed US-001's bare `"log_event"` example); clarified the §3 SSA rule (`ruff RET504`) and the "no broad Exception catch" rule (worker execution envelope exemption); replaced deprecated `assertEquals` with `assertEqual` in §4.2 and pinned the coverage tooling; added `US-000` (foundation story owning scaffold, `configure`, `exceptions`, Clock/FS ports, test harness) so subsequent stories stop guessing on package layout; added `depends_on`/`change_type` to US-001.

### Added
- **OpenCode Go LLM Provider**: Added `opencode` as a supported LLM provider, routing through OpenAI-compatible transport to the OpenCode Go subscription endpoint (`https://opencode.ai/zen/go/v1/chat/completions`). Provides access to curated open coding models including GLM-5.2, GLM-5.1, Kimi K2.7 Code, Kimi K2.6, MiniMax M3/M2.7, Qwen3.7 Max/Plus, Qwen3.6 Plus, MiMo V2.5/V2.5-Pro, DeepSeek V4 Pro/Flash. API key resolves from the `OPENCODE_API_KEY` environment variable. Includes a static model fallback hierarchy for quota/transient-error model stepping and unit tests for `Call` and `GetAvailableModels`.
- **Forbidden Patterns Policy**: Added a configurable `sandbox.forbidden_patterns` list (Go regexes) enforced at write time on `write_file`/`edit_file` content and `edit_file` replacement_content. Any match rejects the action with a `SPEC violation` reason, giving the agent immediate feedback instead of failing later at the test-validation stage. Wired through `PolicyValidator.SetForbiddenPatterns` from `cfg.Sandbox.ForbiddenPatterns` in both `serve` and `start-one` CLI entry points. Invalid regexes are skipped (not fatal). The `wc` validation project now configures `\bunsafe\s*\{` to enforce the SPEC's `#![deny(unsafe_code)]` constraint.
- **Build Status Gating**: The orchestrator now sets `BuildStatus = FAILING` immediately when a task fails test validation (compilation or test failure), so the operator dashboard and auto-merge policy see the red signal in real time instead of staying `UNKNOWN`. Finalization now uses `StoryStatus` (previously declared but never set) as the once-only guard: when all tasks finish, `StorySuccess`+`BuildPassing` is set only if every task succeeded; otherwise `StoryFailed`+`BuildFailing` is set and release finalization (version bump / PR) is skipped. Added `allTasksSucceeded` helper.
- **Non-TTY Status Rendering**: The `noctifab start --wait` polling loop and the daemon-ready wait loop now detect whether stdout is a terminal (`pkg/infrastructure/tty.IsTerminal`, via `golang.org/x/term`). In a TTY the existing dot-accumulation progress animation is preserved; when stdout is not a TTY (CI logs, `--wait < script`, `2>&1 | tee`), each poll emits one timestamped status line separated by a newline instead of dots on one ever-growing physical line, keeping captured logs readable.
- **Real Pre-flight LLM Ping**: `noctifab start` and `noctifab start-one` now perform a genuine reachability check before launching the daemon: `pkg/infrastructure/llm.Ping` constructs the provider transport for the configured provider and calls `GetAvailableModels` with a 15s timeout. On failure the pre-flight prints `FAIL: <classified reason>` and aborts startup instead of unconditionally printing `OK`. Ping errors are classified into operator-readable categories (rejected API key / 403 forbidden / 429 quota / network unreachable / generic) so the user can distinguish a bad key from a network issue without reading a raw HTTP dump. Unit tests cover success, unsupported provider, 401, 429, and timeout.
- **Story Wall-Clock Enforcement (`max_duration`)**: The previously-declared-but-unused `max_duration` config field is now enforced. When set to a non-zero duration, the orchestrator tracks the story start time (the first cycle in which any task becomes ready) and, if the elapsed time exceeds the limit while the story is still `StoryIdle`, fails every non-finished task with a `story exceeded max_duration` failure log, sets `BuildStatus=FAILING` + `StoryStatus=StoryFailed`, and stops dispatching new work — preventing runaway LLM spend on a stuck story. A `MaxDuration=0` (the default) disables the check. Wired through `OrchestratorConfig.MaxDuration` in both `serve` and `start-one` entry points. Unit tests cover both enabled-abort and disabled-passthrough cases.
- **Pre-flight & Provider Docs**: `docs/cli_usage.md` now documents the pre-flight checklist (Git CLI, database, LLM provider ping, sandbox mode) with a per-check failure-mode table and an explicit note that the LLM "ping" is a reachability + key check against the provider's `/models` endpoint, not a quota or model-availability guarantee. Added an "LLM Provider Configurations / OpenCode Go" section with a full `secrets.yaml` + `config.yaml` example for the `opencode` provider (GLM-5.2 etc.) and the `--llm-provider` flag description updated to include `opencode`.

### Fixed
- **LLM Response JSON Parser String-Awareness**: `ExtractJSONBlock` (`pkg/infrastructure/llm/parser.go`) previously used naive brace counting from the first `{`, which miscounted braces inside JSON string values (e.g. Rust `mod tests { ... }` embedded in a `write_file` `content` argument) and returned a truncated/invalid substring, causing deterministic `invalid character 'B' looking for beginning of object key string` failures on code-heavy Tester-role output (observed 4/4 with GLM-5.2). The parser is now string-literal-aware (honours `\"` escapes, ignores braces inside JSON strings) and, when several top-level balanced blocks are present (e.g. fenced code before the JSON envelope), prefers the block containing `"reasoning"`/`"actions"` keys. Regression tests added with a real Rust-in-content response shape.
- **Structured Phase Log Markers**: The orchestrator now emits distinct, summary-bearing log lines for each agent phase: `[Reader] phase ok/failed`, `[Tester] write phase ok` (with reasoning + action count) / `[Tester] write phase failed`, `[Tester] write phase summary: N executed, M blocked`, `[Generator] write phase ok/failed/summary`, and blocked actions now include the validator reason instead of the bare `validation failed` string. This disambiguates the previous conflated `Tester LLM completion failed` log that did not distinguish "tests were written" from "verify turn failed".
- **REPL EOF & Welcome-Example Consistency**: The `noctifab REPL exited.` message now reads `noctifab REPL exited. Daemon continues in background; polling status every <interval>.` so a piped-stdin EOF no longer sounds like a daemon crash. The REPL welcome string and the listener system-prompt example were changed from `roadmap/US-0001.md` (4-digit, no matching file) to `roadmap/US-001.md` (3-digit, matching the actual roadmap files in `validation/projects/`), removing a footgun for new users typing the example.
- **Rust `unsafe` Prompt Prohibition**: The Rust project context in `pkg/infrastructure/llm/helpers.go` now injects an explicit `CRITICAL: The use of unsafe blocks is STRICTLY FORBIDDEN` instruction into both the Generator (`Instructions`) and Tester (`TestWriterInstructions`) system prompts, listing the safe idioms to use instead (`BufReader`, `str::from_utf8`, slices/iterators). This complements the write-time `forbidden_patterns` gate so the model is told the constraint upfront rather than discovering it via a rejected write.
- **Per-Project `.noctifab/.gitignore` for Secret Safety**: The `wc`, `frontpunch`, and `todo-cli` validation projects now each ship a `.noctifab/.gitignore` excluding `secrets.yaml`, `data/`, `logs/`, `*.lock`, and `*.pid`. Previously only the noctifab repo-root `.gitignore` covered `validation/projects/*/.noctifab/secrets.yaml`, so running `noctifab init` inside one of those project directories could have staged `secrets.yaml` into git.

### Changed
- **Span Naming Convention**: Renamed all OpenTelemetry spans from `noctifab.*` prefixed names to bare Go function names (e.g., `noctifab.cycle` → `RunOnce`, `noctifab.postgres_save` → `Save`) for cleaner observability UX.
- **OpenTelemetry Instrumentation**: Added named spans with input attributes (secrets redacted) to 9 additional functions: `SQLiteRepository.Save/Load`, `DockerSandbox.RunCommand`, `Orchestrator.FinalizeUserStory/PlanStory`, `Scheduler.GetReadyTasks`, `ListenerAgent.Start/interpretCommand/routeIntent`, `DaemonClient.IsAlive/SendStartStory/SendStartDirectory/GetStatus`, and `BumpVersion`. All sensitive keys (`api_key`, `token`, `secret`, `password`, `auth`, `credential`, `private_key`, `access_key`) are automatically redacted to `[REDACTED]` via `telemetry.Attr`/`AttrInt` helpers. Tracer configuration uses only `OTEL_*` environment variables.
- **Package Rename**: Renamed `pkg/usecase` to `pkg/services` for idiomatic Go convention. All imports and internal references updated accordingly.

## [0.3.0] - 2026-07-01

### Added
- **Level 5 Autonomy Specification**: Expanded the `AUTONOMY.md` proposal to define Level 5 (Maximum) Autonomy for `noctifab`.
- **Level 5 Core Pillars**: Outlined requirements for environmental self-healing, closed-loop telemetry staging/production feedback, autogenous flaky test stabilization, binary self-evolution (metaprogramming), automated security/SAST auditing, and zero-clarification intent resolution.
- **Level 5 Roadmap**: Added Phase 5 (Self-Healing & Telemetry) and Phase 6 (Self-Evolution & Security Auditing) to the implementation roadmap.
- **Watchdog Liveness Monitor**: Added `pkg/usecase/watchdog.go` with idle output timeout detection for subprocess executions. The `Watchdog` tracks wall-clock duration and resets an idle timer on every byte of stdout/stderr output, killing the process group via `SIGKILL` when either limit is exceeded. Integrated into `HostSandbox.RunCommand`, replacing the previous goroutine-based context cancellation pattern. Sentinels `ErrWatchdogMaxDuration` and `ErrWatchdogIdleTimeout` are wrapped with `%w` so callers can distinguish hang events from normal test failures. Configured via `sandbox.idle_timeout_seconds` in `config.yaml` (default: 30s).
- **Interruptible OCC Backoff**: Added `SleepWithInterrupt` to `CommandMailbox` with a `Wakeup()` notification channel that fires when a command is enqueued. The OCC retry loop in `Orchestrator.updateStateWithRetry` now selects on this channel instead of blocking on `time.After()`, making the daemon responsive to operator commands (abort, model switch) during database conflict backoff.
- **LLM Call Budget Enforcement**: Added `maxCalls` parameter to `NewFailoverClient`. When set, `Complete` returns `domain.ErrBudgetExhausted` after the configured number of total calls, preventing runaway API spending when no backends are available.
- **Unit Test Coverage**: Added 11 new tests — 6 for `Watchdog` (normal exit, max duration, idle timeout, output resets idle timer, context cancellation, no-limits mode) and 5 for `SleepWithInterrupt` (timer expiry, zero duration, cancelled context, immediate wakeup, deferred wakeup). Extended `FailoverClient` test suite with budget exhaustion coverage.
- **Budget Persistence (AUT-102)**: Added `PostgresBudgetStore` and `SQLiteBudgetStore` implementations of the `BudgetStore` interface with UPSERT semantics. Backed by dedicated migrations (`0003_add_budget.sql`) creating the `budget_usage` table. Daily usage now survives daemon restarts across both SQLite and PostgreSQL backends.
- **Config Types for Level 5 Features**: Extended `Config` struct with `PullRequestConfig` (`auto_create_pr`, `auto_merge`), `CIConfig` (`auto_fix`), `SASTConfig` (`enabled`, `scanners`, `fail_on_severity`), `TelemetryConfig` (`enabled`, `provider`, `endpoint`), and `FailoverConfig` (`max_calls`). All wired through CLI flags, environment variables, and config file defaults.
- **SAST Scanner (AUT-603)**: Added `SASTScanner` supporting `gosec` and `bandit` integrations. Parses JSON output from both tools into structured `SecurityIssue` objects. Configurable severity threshold (`fail_on_severity`) blocks builds on `HIGH`/`MEDIUM`/`LOW` findings. Includes dedicated `parseGosecJSON` and `parseBanditJSON` parsers with unit tests for real-world output formats.
- **Intent Disambiguation (AUT-604)**: Added `IntentDisambiguator` that synthesizes git history, workspace file listings, and feature context into a targeted LLM prompt, inferring answers to clarification questions without operator intervention.
- **Flaky Test Detection & Stabilization (AUT-502)**: Added `DetectFlaky` with 3-run majority vote consensus (≥2 pass + ≥1 fail = flaky). Added `BuildFlakyStabilizationPrompt` that generates a structured remediation prompt identifying common flakiness patterns (time.Sleep, shared state, missing mutexes, network dependencies, order-dependent tests).
- **Dependency Auto-Install (AUT-501)**: Added `DependencyManager` with `DetectMissingTool` (pattern-matches error output for `cargo`, `pytest`, `golangci-lint`, `node`, `npm`) and `InstallTool` with configurable allowed package manager whitelist.
- **Hot-Reload & Handoff File (AUT-602)**: Added `HotReloadManager` with JSON handoff file protocol (`HandoffState` with `pending`/`handing_off`/`active`/`failed` states), health check polling on the new binary's `/healthz` endpoint, and active handoff confirmation.
- **Watchdog Repair & Failure Categorization (AUT-401/402)**: Added `WatchdogRepair` that classifies subprocess failures into `FailureCategory` constants (`ExitCode`, `SignalKilled`, `MaxDuration`, `IdleTimeout`). Wired watchdog timeout and idle settings through HostSandbox configuration.
- **Comprehensive E2E Scenario Test**: Added `TestScenario_ComprehensiveAutonomy` with 12 subtests covering budget persistence, OCC version conflicts, watchdog process killing, flaky detection, dependency manager detection, self-update patch validation, hot-reload handoff round-trip, intent disambiguation, SAST scanner disabled mode, cost calculation, complex state persistence through PostgreSQL, and failover budget alerts.
- **Specification & Documentation Updates**: Updated SPEC.md with sections §3.6.8–§3.6.11 (PR, CI, SAST, Telemetry config), §3.10 (Failover), §3.9.2 (Config loading 2-phase). Updated README.md with new feature descriptions. Updated docs/cli_usage.md with new CLI flags for PR, CI, SAST, and telemetry configuration.

## [0.2.3] - 2026-07-01

### Fixed
- **Stale E2E Test Assertion**: Removed the `profiles/default.yaml` and `profiles` directory existence checks from `TestE2E_Init_CleanDirectory` in `tests/e2e/e2e_test.go`. The simplified permission profiles feature moved profiles into `config.yaml`, making the separate `profiles/` folder obsolete and causing E2E tests to fail.

## [0.2.2] - 2026-06-30

### Fixed
- **Stale Integration Test Assertion**: Removed the `profiles/default.yaml` file existence check from `TestInitCommand/Clean_directory_initialization` in `tests/integration_test.go`. The `feat/simplified-permission-profiles` feature moved permission profiles out of separate YAML files and into the `profiles:` block inside `config.yaml`, so `noctifab init` no longer creates a `profiles/` directory.

## [0.2.1] - 2026-06-29

### Added
- **New Rust `wc` Validator Project**: Added a new E2E validator project replicating UNIX `wc` in Rust under `validation/projects/wc` with specifications and user stories (US-001, US-002, US-003) enforcing SOLID/DDD and memory-efficient streaming.
- **Rust Toolchain in Validation Container**: Added `rust` and `cargo` packages to the E2E verification image (`Dockerfile.validation`).
- **Rust validation check**: Updated `validate.sh` to check for `Cargo.toml` and `src/main.rs`.
- **Dynamic Base Branch Detection**: Implemented base branch resolution in `serve` and `start-one` commands when the base branch is configured as `"git-detect"`, falling back to the current active Git branch.
- **Centralized LLM Base URL Config**: Added support for custom LLM API URLs (`url` under `llm` block) in command initialization to support alternative and custom endpoints.

### Changed
- **Default LLM Max Retries**: Reduced the default `max_retries` setting from 10 to 5 in the global configuration defaults to improve validation recovery speeds.
- **Flexible Test Command Execution**: Updated `DockerSandbox.RunCommand` to dynamically execute Python unittest discovery (`python -m unittest discover tests`) when `go.mod` is not found, enabling out-of-the-box non-Go verification.
- **Validation README**: Updated `validation/README.md` to document the new `wc` Rust project as a third available validation target, including its success criteria (`Cargo.toml` + `src/main.rs` present) and added the `make validate PROJECT=wc` example command.
- **Main README E2E Validation Section**: Added a new "E2E Autonomy Validation" section to `README.md` with a summary table of all three validation projects (`frontpunch`, `todo-cli`, `wc`), their languages, user stories, and checked outputs. Updated the test command in the Collaboration section to use `go test -v ./pkg/... ./tests` per `AGENTS.md`.

### Fixed
- **Broadened Gitignore Protections**: Updated the main `.gitignore` file to ignore `.noctifab/secrets.yaml`, `.noctifab/data/`, and `.noctifab/logs/` directories recursively across all validation projects.

### Removed
- **Obsolete Permission Profiles**: Removed legacy `.noctifab/profiles/` folders and YAML profiles from all template validation projects (`frontpunch`, `todo-cli`, and `wc`) to enforce the new centralized `profiles:` config block.

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

