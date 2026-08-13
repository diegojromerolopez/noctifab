# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.32.3] - 2026-08-13

### Changed
- **Validation Projects Audit & Standardization**: Standardized `execution_report: "/app/report_mount/execution_report.md"` across all 15 validation projects (`auth-vault`, `buffonstream`, `djanban`, `fortune`, `frontpunch`, `searchthedocs`, `sqlasm`, `stricc`, `todo-cli`, `calculator`, `echo`, `notebook`, `pyedis`, `t4`, `wc`).
- **Validation Project Specifications**: Added explicit Definition of Done (DoD) sections, SOLID & Dependency Injection guidelines, and mandatory linter constraints across `SPEC.md` files.
- **`frontpunch` Sandbox Configuration**: Updated `linter_command` (`ruff check . && mypy .`) and `test_command` to enforce 100% test coverage gate.

## [0.32.2] - 2026-08-13

### Fixed
- **Listener Input Error Handling**: Added missing `scanner.Err()` check to the input scanning loop in `ListenerAgent.Start` (`pkg/services/listener.go`) to properly report scanner errors before exiting.

## [0.32.1] - 2026-08-13

### Changed
- **Validation Projects Consolidation**: Consolidated `searchreadthedocs` into `searchthedocs`, standardizing on Python 3.15 + FastAPI + PostgreSQL `pgvector` HNSW index architecture for RAG documentation search validation.

## [0.32.0] - 2026-08-13

### Added
- **Complexity Unit ($CU$) Roadmap Sizing & Micro-Task Prevention**: incorporated Function Point Analysis and multi-dimensional Complexity Units ($CU$) into Product Manager prompt templates (`generate.tmpl`) and Planner prompt templates (`decompose.tmpl`) to enforce proportional story/task granularity ($CU_{\text{story}} \in [15, 30]$, $CU_{\text{task}} \in [4, 8]$) and eliminate micro-tasks for concise specs ($CU < 25$).
- **Turn 1 Context Enrichment**: updated Reader Phase (`RunReaderPhase` in `pkg/services/orchestrator_helper.go`) to automatically pre-load the workspace file tree (`git ls-files`) and project manifests (`Cargo.toml`, `go.mod`, `package.json`, `pyproject.toml`, `Makefile`, `CMakeLists.txt`) before Turn 1 code generation, eliminating import path guesses and retries.
- **`git diff` Task Retry Context**: enriched Generator Agent task retries in `RunGeneratorAgent` with formatted `git diff` output from failed attempts so agents fix syntax and logic errors in 1 turn.
- **Project-Agnostic Execution Report Formatting**: enhanced `pkg/services/reporting/renderer.go` with unified duration formatting (omitting zero units), clear Phase Performance Execution Windows explanations, task title & story correlation, detailed error breakdown tables, and project/language-agnostic engineering insights.
- **Unified Parent Cache Volume Mounts**: updated `validation/run_one.sh` to consolidate host package/compiler cache mounts under `${HOME}/.noctifab/cache`.
- **Real-Time Live Execution Report Documentation**: updated `README.md`, `docs/execution_report.md`, and `validation/run_all.sh` to document real-time live checkpointing and atomic file writes.
- **Lead Time Metric Standard**: renamed **Execution Wall Time** metric to **Lead Time** across the execution report renderer (`renderer.go`) and documentation (`docs/execution_report.md`).
- **Standard Terminology Alignment**: standardized Phase Performance metrics to **Phase Cycle Time** (net de-duplicated physical clock time) and **Execution Spans** in `renderer.go` and documentation.
- **User Story Title Correlation**: added story title parsing (`extractStoryTitle`) and display in the `### User Stories` table (`Story ID & Title`), matching task title formatting.
- **Codebase Changes & Workspace Impact**: renamed Code Churn section to **Codebase Changes & Workspace Impact** across `renderer.go` and `docs/execution_report.md`, computing full cumulative line deltas against the root commit.
- **Black-Box Contract Scenarios Table**: rendered public contract scenarios (`Contract ID`, `Interface`, `Executable Path`, `Observable Expectations`, `Verification Status`) under `## Verification & Testing Strategy`.
- **Permanent Model 404 Deprecation Blacklisting**: added thread-safe `BlacklistModel` and `IsModelBlacklisted` registry in `pkg/infrastructure/llm/` to permanently skip HTTP 404 / deprecated models across all future LLM fallback ladder selections.
- **Soft DAG Dependency Pruning**: updated `ResolveTaskDependencies` in `pkg/services/task_dependencies.go` to prune unknown/hallucinated task dependencies with a warning log to `os.Stderr` instead of failing execution.
- **Flexible Validation Artifact Matching**: updated `validation/validate.sh` to accept alternative TypeScript entry points in `src/` (e.g. `src/server.ts`, `src/app.ts`) for `notebook`.
- **Language-Independent Standard Library First Prompt Mandate**: updated 11 prompt templates across `pkg/infrastructure/prompts/defaults/` to enforce standard library primitives over un-scaffolded external packages unless explicitly required by `SPEC.md`.
- **Tool Sandboxing & Package Resolution Documentation**: updated `docs/architecture.md` and `docs/prompts.md` documenting agent tool permissions (`exec` disabled), manifest editing vs terminal package installation, and standard library fallback behavior.
- **Advisory Linter Soft-Pass**: updated task turn evaluation in `pkg/services/orchestrator_generator.go` and `orchestrator_helper.go` to complete tasks as `SUCCESS` with an advisory log warning when unit/integration tests pass 100% and linter failures occur 2+ consecutive times.
- **Aggressive Circuit-Breaker for HTTP 429 Rate Limits**: updated `pkg/infrastructure/llm/client.go` to immediately skip retries on HTTP 429 quota exhaustion when no short `Retry-After` header is supplied, triggering instant model/provider fallback.
- **Concurrent DAG Task Worker Dispatch**: updated `pkg/services/orchestrator_dispatch.go` to default concurrency to `GeneratorsNumber` (default 3) when unset, executing independent ready tasks in parallel worker goroutines.
- **Configurable Task Execution Order & Pre-Seeded Stub Generation**: added `agents.task_execution_order` setting (`"generator_first"` default vs `"tester_first"` TDD mode). In `tester_first` mode, Noctifab automatically pre-seeds minimal compilation stub files (`ensureTargetStubFilesExist`) for missing target files so Turn 1 `run_tests` compiles cleanly.
- **Black-Box Contract Scenario Prompt Injection**: updated `orchestrator_execute.go` and `story_contract.go` to parse and inject machine-readable contract expectations (`AllowedExecutables`, `ExitCodes`, `StderrPrefixes`, `StdoutContains`) into Generator and Tester agent prompts.
- **Incremental Story Resume (`noctifab resume` & `noctifab start --resume`)**: added `--resume` flag to `noctifab start` and created dedicated `noctifab resume` CLI command in `cmd/noctifab/cli/resume.go` to skip already completed stories and resume execution from the first pending/failed story.
- **Configurable Product Manager Passes (`agents.product_manager.passes`)**: added `passes` setting (default `2`) to `AgentRoleConfig` and `GenerateRoadmapWithPasses`, enabling multi-pass roadmap refinement (Pass 1: Decomposition; Pass 2+: Cross-story contract and dependency audit). Configured `passes: 2` / `passes: 3` across `t4`, `notebook`, `pyedis`, and `calculator` validation project configurations.
- **Source-Code-Only Line Delta Filter**: updated `computeWorkspaceChurn` in `pkg/services/reporting/collector.go` to exclude third-party vendor and build artifact directories (`node_modules/`, `vendor/`, `dist/`, `.next/`, `venv/`, `__pycache__`, `target/`) from git diff and untracked line churn calculations.
- **Standardized Sandbox Path Normalization**: updated `resolveSandboxPath` in `pkg/services/production_tools.go` to normalize backslashes `\`, strip leading `./` prefixes, and clean paths before verifying sandbox boundaries.

### Removed
- **Validation Project Feedback Report (`gen_feedback.py`)**: removed legacy `gen_feedback.py` script and `*_FEEDBACK.md` artifact generation in favor of Noctifab's native, structured Execution Report (`validation/projects/<project>/output/report/*.md`).

## [0.31.0] - 2026-08-12

### Added
- **Fine-Grained Telemetry Instrumentation (`Observe`)**: wired structured `ExecutionEvent` emission across `LLMClient` (`EventLLMCallFinished`), `Sandbox` (`EventSandboxFinished`), `ProductManager` (`EventAgentStarted`/`EventAgentFinished`), and `Orchestrator` (`EventTaskAttemptStarted`/`EventTaskAttemptFinished`) for fine-grained reporting metrics without performance overhead.
- **Context Propagation**: added `WithObserver` and `ObserverFromContext` helper primitives to `pkg/domain/execution_event.go`.
- **Validation Monitoring Updates**: updated `AGENTS.md` and `validation/projects/TESTING_GUIDE.md` to rely on the automatically generated execution report (`validation/projects/<project>/output/report/*.md`) instead of token-heavy 60-second polling loops.

## [0.30.0] - 2026-08-11

### Added
- **Structured Execution Reports & Logs** (`execution_report: ".noctifab/reports/execution_report.md"`): deterministic execution measurements and Markdown diagnostic artifact documenting process and story timings, active vs. waiting breakdown by agent role, deterministic bottlenecks (`BN-*`), evidence-backed issues (`ISSUE-*`), fallback recommendations (`PROP-*`), and single bounded read-only model analysis without parsing raw container logs.
- **Documentation**: added [`docs/execution_report.md`](file:///Users/diegoj/repos/noctifab/docs/execution_report.md), updated [`docs/index.md`](file:///Users/diegoj/repos/noctifab/docs/index.md), and updated [`README.md`](file:///Users/diegoj/repos/noctifab/README.md) with comprehensive guides on execution reporting, path resolution, and telemetry event streams.
- **Domain Refactoring**: split event telemetry model (`ExecutionEvent`) into [`pkg/domain/execution_event.go`](file:///Users/diegoj/repos/noctifab/pkg/domain/execution_event.go) and synthesized diagnostic report model (`ExecutionReport`) into [`pkg/domain/execution_report.go`](file:///Users/diegoj/repos/noctifab/pkg/domain/execution_report.go).
- **Validation Harness Integration**: updated `run_one.sh`, `validate.sh`, and `gen_feedback.py` to consume host-mounted `execution_report.md` diagnostic reports.

## [0.29.0] - 2026-08-09

### Added
- **Experimental QA agent** (opt-in, `qa.enabled: true`): end-to-end acceptance review of completed tasks against generated user stories. Enforcing config validation restricts QA to `code_first` architectures with `vcs.use_worktrees: true`, `blocking: true`, and `network: "none"`. QA runs in hardened Docker sandboxes with read-only source mounts, ephemeral per-review workspaces (build/tester/QA worktrees verified against the source commit manifest), generated scenarios and findings persisted with fingerprints (SQLite/PostgreSQL migration `0006`), bounded generator fix rounds, global token budget enforcement, and a crash-recovery service that resumes or fails reviews after interruption.
- **`StoryContract` extraction** (`noctifab-contract` fenced JSON block parsed from generated user stories, `pkg/services/story_contract.go`): explicit public API signatures, binary paths, and I/O invariants that QA verifies against.
- **Static QA reporting** via the capability registry: `role qa: experimental-disabled` / `experimental-enabled`.

### Changed
- **Config schema version 2.0**: `config_version` now defaults to `2.0`; version `1.0` configuration files fail with the exact error `unsupported config_version "1.0": migrate to "2.0"`. All bundled fixtures and validation project configs updated.
- **Dead placeholder roles removed**: `architect`, `security`, `performance`, `docs`, and `devops` agents are deleted from the config, router, CLI, documentation, and validation project configs; they fail with `unsupported agent role %q: delete the %s.%s section`. The `resolver` role is retained for conflict-resolution.

## [0.28.9] - 2026-08-09

### Fixed
- **Prompt CLI config-path consistency**: `promptsWorkspace` and `loadPromptOverrides` now share one config-path resolver, preventing convention templates and config overrides from being loaded from different paths.

## [0.28.8] - 2026-08-09

### Changed
- **WatchdogRepair dormant-state tracking**: the injected-but-never-invoked `WatchdogRepair.AttemptRepair` flow is now tracked in [issue #15](https://github.com/diegojromerolopez/noctifab/issues/15) (wire it into the task-failure path or remove it), replacing the vague "revisit if the flow is ever triggered" note. Added a DORMANT doc comment on `AttemptRepair` and updated the CUSTOM_PROMPTS.md assessment table to reference the issue. No behavior change.

## [0.28.7] - 2026-08-09

### Added
- **Explicit regression test for the product_manager audit prefix quirk** (`pkg/infrastructure/prompts/audit_quirk_test.go`): the legacy pipeline trimmed the dispatch prefix for `generate` but kept the whole raw prompt for `audit`, so the audit instruction line intentionally sits inside the `INPUT CONTEXT:` section of `audit.tmpl`. Previously this was only covered implicitly by the byte-identical golden comparison against the legacy replica (a transitional artifact); the new self-documenting test survives any future removal of that replica and explains why the asymmetry must not be "cleaned up".

## [0.28.6] - 2026-08-09

### Fixed
- **Compaction protection extended to all hardcoded prompts with schema suffixes**: the Repair Agent (`repairPromptTail`), Reader/context-gathering (`readerPromptTail`), and Unblocker (`unblockerPromptTail`) prompts now keep their tool-list/JSON-schema suffixes as separate constants and mark them via `domain.WithUncompactableTail`, so `caveman`/`simple_english` compaction can never rewrite their output contracts (completing the v0.28.1 fix, which only covered renderer-produced prompts). The listener prompt is exempt by design: its schema sits mid-prompt with the dynamic operator command as the suffix.

## [0.28.5] - 2026-08-09

### Changed
- **Consistent breadth-first prompt action naming**: renamed the generator prompt actions `breadth_first` → `implement_breadth_first` and `breadth_first_fix` → `implement_breadth_first_fix` for symmetry with `tester/write_breadth_first` (`<base>_breadth_first` convention). Action names are the public customization contract (template directory names, config keys, CLI arguments); renamed before first release of the feature.

## [0.28.4] - 2026-08-09

### Added
- **Strict template placeholder validation**: prompt templates are now parsed with `missingkey=error`, and combined with the startup fixture render, a typo'd placeholder in a user override (e.g. `{{.Titel}}`) aborts startup with a key-named error instead of silently rendering `<no value>` into live prompts. Documented brace escaping (`{{"{{"}}`) in `docs/prompts.md`.

## [0.28.3] - 2026-08-09

### Changed
- **Append/override conflicts now fail fast at startup**: configuring both a full-template override (config `path` or convention `.tmpl`) and an append (config `append` or `.append.tmpl`) for the same prompt action is now a startup error naming the key, instead of silently ignoring the append with a log warning. Both mechanisms are explicit opt-ins; ignoring one would mask a configuration mistake. `prompts validate` reports the conflict with a non-zero exit code.

## [0.28.2] - 2026-08-09

### Fixed
- **Prompt inventory completeness**: Added the live `listener/interpret` prompt (`listenerSystemPrompt`, `pkg/services/listener.go`) to the CUSTOM_PROMPTS.md assessment table and non-goals as a Hardcode entry; it was missing from the "full inventory" claim. No code change — the listener prompt stays hardcoded protocol machinery.

## [0.28.1] - 2026-08-09

### Fixed
- **Compaction can no longer rewrite the prompt output contract**: `CompactCaveman`/`CompactSimpleEnglish` in `llm.Client` now skip the non-overridable JSON/tool contract block at the end of rendered prompts (the renderer reports the contract length via `domain.WithUncompactableTail`), so the machine-readable schema always reaches the model verbatim. Multi-turn continuation prompts now keep the output contract at the END of the prompt (tool outputs are inserted before it), instead of appending turn outputs after the schema.

## [0.28.0] - 2026-08-09

### Added
- **Per-Agent Prompt Customization System**: New `pkg/infrastructure/prompts` package with 14 customizable `(agent, action)` prompt templates across 4 agents (`product_manager`, `planner`, `tester`, `generator`), embedded via `go:embed` and resolved per key: config `prompts.<agent>.<action>.path` > convention file `.noctifab/prompts/<agent>/<action>.tmpl` > embedded default. Small additions supported via `append` (config string or `<action>.append.tmpl`), always applied to the default body, never to an override. The JSON/tool output contract is a non-overridable block appended by code after rendering. Invalid overrides fail fast at startup with file-named errors.
- **`noctifab prompts` CLI**: New command group with `list` (agent/action tree + effective source), `show <agent> <action>` (effective template + contract), `init [agent] [action]` (materialize embedded defaults, never overwriting), and `validate` (parse + test-render all effective templates).
- **Prompts documentation**: New `docs/prompts.md` readthedocs page (registered in the `index.md` toctree) covering the resolution order, the append mechanism, per-agent template data contracts, hardcoded prompts, and CLI usage.

### Fixed
- **Prefix-dispatch prompt bypass (CUSTOM_PROMPTS.md §1.1)**: Four live prompt variants (tester code_first retry, tester breadth_first write, generator breadth_first implement/refine) matched no `preprocessPrompt()` prefix and were silently sent WITHOUT their role body (no persona, no tool list, no JSON output schema). Prompts are now rendered by explicit `(agent, action)` key — never by prefix matching — so these variants receive the full role body and output contract. `preprocessPrompt()` is deleted; golden tests assert byte-identical default output to the legacy assembly for the 10 unaffected variants.

### Changed
- **`services.NewOrchestrator` and `services.GenerateRoadmap`** now take a `PromptRenderer` (nil falls back to embedded defaults). The Repair Agent role body moved from the deleted prefix dispatch into `pkg/services/watchdog_repair.go` (byte-identical, still hardcoded).

## [0.27.3] - 2026-08-09

### Removed
- **Dead Code Removal (prompt customization groundwork, Phase 3b)**: Deleted `PromptBuilder` and the language concurrency-invariant constants (`pkg/infrastructure/llm/prompts.go`, `prompts_test.go`), the flaky-test detector (`DetectFlaky`/`BuildFlakyStabilizationPrompt` in `pkg/services/flaky_detector.go`, `flaky_detector_test.go`), and the `IntentDisambiguator` (`pkg/services/intent_disambiguator.go`, `intent_disambiguator_test.go`), plus their subtests in `tests/e2e/scenario_comprehensive_test.go`. All three were verified to have no production call sites; removal is a runtime no-op. The live `TestRunResult` type used by multi-run test validation moved into `pkg/services/test_validator.go`.

## [0.27.2] - 2026-08-08

### Fixed
- **Flaky `TestOrchestrator_ConcurrentWorktreeIsolation`**: Made `mockRepo.Save` in `pkg/services/orchestrator_test.go` enforce the same optimistic concurrency contract as the real Postgres/SQLite repositories (rejecting stale `state.Version` writes with `domain.ErrVersionConflict` and bumping the version on success). Concurrent `executeTask` goroutines now exercise the production OCC retry path instead of silently losing state updates, eliminating the timing-dependent `SUCCESS`/`IN_PROGRESS` assertion failure in CI.

## [0.27.1] - 2026-08-08

### Fixed
- **Validate Command Output Contract**: Restored the `Configuration is valid.` output line in `noctifab validate` (`cmd/noctifab/cli/validate.go`), fixing the E2E test `TestE2E_Validate_Configuration` which asserts this public CLI output string.

## [0.27.0] - 2026-08-08

### Added
- **30-Minute Provider Eviction Circuit-Breaker**: Implemented 30-minute candidate eviction in `ResilientLLMRouter` triggered on HTTP 401/402 or `CreditsError`/credit exhaustion, skipping depleted providers instantly during routing and informing the user in terminal logs and status views.
- **Asynchronous Background Catalog Refresh**: Updated `availableModelsCached` in `client_catalog.go` to serve model catalogs instantly from cache while refreshing expiring entries asynchronously in background goroutines.
- **CLI Pre-Flight Health Diagnostic & Credit Exhaustion Notices**: Added sandbox build tool auditing (`go`, `docker`, `python3`, `rustc`, `make`, `gcc`) and explicit credit exhaustion alerts in `noctifab start` and `noctifab validate`.
- **Flicker-Free Terminal Dashboard**: Added cursor hiding (`\033[?25l`) on dashboard render start to eliminate terminal flicker.
- **Decoupled Interactive Dashboard Prompt Overlay**: Decoupled keyboard input prompts into floating screen overlays (`cmd/noctifab/cli/dashboard.go`), allowing background status polling and dashboard UI renders to continue uninterrupted while input prompts are active.
- **Interactive Log & Failure Inspector Modal**: Implemented interactive modal (`HandleLogInspectorModal`) accessible via `d` in the TUI dashboard, rendering full error logs, colorized stack traces, and assertion diffs inline.

## [0.26.3] - 2026-08-08

### Added
- **Serial Execution Mode & Project Selection Parameters**: Added `--serial` (`-s`) and `--projects` (`-p`) flags to `validation/run_all.sh` and updated `Makefile` (`SERIAL=1`, `PROJECT=...`), allowing serial execution (one project at a time) and explicit project list selection.

## [0.26.2] - 2026-08-08

### Fixed
- **Deprecated `temperature` Parameter Removal**: Stripped the `temperature` parameter from Anthropic requests (`pkg/infrastructure/llm/anthropic.go`) and automatically omitted `temperature` for all Claude (`claude*`) and OpenAI reasoning (`o1*`, `o3*`) models in the OpenAI adapter (`pkg/infrastructure/llm/openai_adapt.go`), resolving HTTP 400 rejection errors on newer Opus/Sonnet models.
- **`sqlasm` Container Architecture Alignment**: Updated `validation/projects/sqlasm/Dockerfile` to `alpine:3.21` with `nasm`, `gcc`, `make`, `valgrind`, `musl-dev`, fixing cross-architecture `Exec format error`.

## [0.26.1] - 2026-08-08

### Added
- **Validation Projects 10-Minute Execution Timeout Mandate**: Documented the strict 10-minute maximum execution time limit rule per validation project execution across `AGENTS.md`, `validation/projects/TESTING_GUIDE.md`, and `validation/README.md`.

## [0.26.0] - 2026-08-08

### Added
- **Validation Projects Matrix**: Added technical specifications (`SPEC.md`), container definitions (`Dockerfile`, `docker-compose.yml`), and configuration profiles (`.noctifab/config.yaml`) for validation projects: `auth-vault`, `buffonstream`, `djanban`, `searchthedocs`, `sqlasm`, and `stricc`.
- **LLM Provider Prioritization**: Standardized top-tier LLM provider fallback hierarchy (`claude`, `gemini`, `openai`, `deepseek-pro`, `qwen`, `opencode`, `openrouter`) across all 15 validation projects.
- **Architectural Reviews & Hardening Guidelines**: Audited and hardened all `SPEC.md` files for Dark Factory autonomous execution by lower-level LLMs.

## [0.25.4] - 2026-08-07

### Enhanced
- **`djanban` Validation SPEC.md**: Expanded [`validation/projects/djanban/SPEC.md`](file:///Users/diegoj/repos/noctifab/validation/projects/djanban/SPEC.md) with explicit domain calculators (`WIPCalculator`, `RegressionTracker`, `PlusForTrelloParser`, `LeadCycleCalculator`, `ScheduleAnalyzer`), strict wire contract schemas (`/api/v1/...`), number precision invariants, and DI interfaces.

## [0.25.3] - 2026-08-07

### Added
- **`djanban` Validation Project**: Added the `djanban` legacy modernization project (`validation/projects/djanban/`) to test `noctifab` upgrading outdated Python/Django codebases to Python 3.12+ and Django 5.x while deprecating legacy AngularJS frontends.

## [0.25.2] - 2026-08-07

### Added
- **Comma-Separated Project Validation (`Makefile` & `run_all.sh`)**: Updated `Makefile` `validate` target and `validation/run_all.sh` to accept comma-separated project lists (e.g., `make validate PROJECT=pyedis,echo,calculator`).

## [0.25.1] - 2026-08-07

### Documentation
- **Self-Correcting & Dynamic Prompts Framework**: Updated `README.md`, `docs/index.md`, `docs/getting_started.md`, and `docs/architecture.md` detailing dynamic prompt adaptation, legacy codebase characterization (`scanLegacyFiles`), pre-flight LLM provider capability caching (`providerCapabilityCache`), and parallel context compaction engine.

## [0.25.0] - 2026-08-07

## [0.23.0] - 2026-08-07

### Removed
- **Documentation Cleanup**: Consolidated architectural insights, dynamic prompt injection features, and 5x–10x acceleration strategies into primary documentation (`README.md`, `docs/`) and removed obsolete root markdown files (`BOTTLENECKS.md`, `DYNAMIC_PROMPTS.md`, `FAILURE.md`, `LLM_PROVIDERS.md`, `SPEEDUP_EXTENSION.md`, `SPEEDUP.md`).

### Changed
- **Documentation Consolidation (`README.md` & `docs/`)**:
  - Updated `README.md` and `docs/unblocker_agent.md` with **Dynamic Prompt Enhancement** features, documenting live execution log tailing, secret scrubbing (`log_tailer.go`), 0-token fast-path regex pre-filtering (`unblocker_fastpath.go`), 10x progressive log window escalation (50 $\rightarrow$ 500 $\rightarrow$ 5,000 lines), and task stall recovery directives.
  - Added **Dark Factory Acceleration Engine (5x–10x Speedup)** documentation to `README.md` and `docs/architecture.md`, highlighting parallel DAG task worker pools with Git worktree sandboxing, tiered LLM provider routing, parallel 3x test majority-vote validation, unified diff multi-file patching (`apply_patch`), pre-baked base images, prompt history pruning, and speculative prefetching.

### Fixed
- **Pre-Flight Provider Banning Name Collision (`start.go`)**: Fixed a bug where pre-flight ping failures appended both provider name (`p.Name`) and provider type (`p.Provider`) to `bannedNames`, causing sibling provider instances sharing the same provider protocol (e.g. `gemini-backup`) to be collateral-banned when `gemini-primary` failed. Provider banning is now strictly scoped by unique instance name (`p.Name`).

## [0.22.0] - 2026-08-06

### Added
- **Dynamic Prompt Enhancement via Unblocker Log Injection (`DYNAMIC_PROMPTS.md`)**: Implemented dynamic log tailing, secret sanitization, zero-token fast-path regex unblocking, and 10x progressive log escalation in `UnblockerAgent`:
  - **Live Log Tailing & Secret Scrubbing (`log_tailer.go`)**: Added `TailLogFile` and `SanitizeLog` to read standard output logs and redact sensitive credentials (API keys, bearer tokens, passwords).
  - **Fast-Path Regex Classifier (`unblocker_fastpath.go`)**: Added static regex pre-filtering for 0-token unblocking of routine CLI stalls (interactive stdin prompts, port binding collisions, test watch mode spinners).
  - **10x Progressive Log Escalation**: Configured 3-tier log tail windowing based on `task.StallCount` (50 lines $\rightarrow$ 500 lines $\rightarrow$ 5,000 lines, capped at 3 escalations before failing task).
  - **Task Recovery Directives**: Attached `RecoveryDirective` to task state upon reset and injected `[STALL RECOVERY DIRECTIVE]` into `Generator` prompts on retry attempts to prevent repeating stalls.

## [0.21.1] - 2026-08-06

### Added
- **Unified Diff Multi-File Patching Tool (`apply_patch`, Proposal 11)**: Created `ApplyPatchTool` (`pkg/services/apply_patch_tool.go`) allowing Generator and Tester agents to apply single- or multi-file unified diff patches (`diff -u` / Git patch format) in a single turn. Features fuzzy line matching, file creation/deletion support, in-process syntax checks, and full sandbox security validation. Added `ApplyPatchTool` to `validator.go` role profiles and updated LLM prompt templates in `prompt_templates.go`.

### Changed
- **Task Entity & Atomicity Mandates**: Updated Product Manager and Planner system prompts in `prompt_templates.go` to enforce strict task entity and atomicity. Prohibited test-only tasks, mandated co-located application functionality and tests in every task, and enforced single-responsibility atomic tasks (1–2 turns).
- **Dark Factory Acceleration Documentation**: Updated `SPEEDUP.md` and `SPEEDUP_EXTENSION.md` marking completed acceleration proposals (Proposals 1–8, 10, 15, 17, 19, 20) with `[DONE] ✅` status indicators.

## [0.21.0] - 2026-08-06

### Added
- **SPEEDUP.md Proposals Implementation**: Implemented all 9 dark factory acceleration proposals to achieve **5x–10x** overall code generation throughput speedup:
  - **Proposal 1: Parallel DAG Worker Pools**: Enabled multi-worker topological task dispatch with per-task file locking, isolated Git worktree execution, and asynchronous `RebaseQueue` branch merging.
  - **Proposal 2: Tiered LLM Provider Routing**: Updated per-role provider routing defaults to direct deep reasoning models for PM/Planner and high-throughput coding models for Generator/Tester agents.
  - **Proposal 3: Parallel 3x Majority-Vote Validation**: Parallelized test validation runs in `test_validator.go` using concurrent goroutines and `sync.WaitGroup` to cut test verification latency from ~15s to ~3s.
  - **Proposal 4: Pre-baked Base Images**: Updated validation Dockerfiles (`pyedis/Dockerfile`) to pre-install common test dependencies (`pytest-asyncio`, `httpx`, `coverage`).
  - **Proposal 5: Deterministic Mock Clock Invariants**: Added Product Manager DoD prompt invariants in `prompt_templates.go` requiring deterministic mock clock patterns for time- and date-dependent features.
  - **Proposal 6: Native JSON Schema Enforcement & Parameter Sanitization**: Enforced native JSON schema response formatting and parameter sanitization across OpenAI and compatible providers.
  - **Proposal 7: Implicit Orchestrator Verification**: Automatically trigger `run_tests` implicitly upon file modification when generator returns `noop`.
  - **Proposal 8: Aggressive Prompt History Pruning on Retries**: Implemented suffix-only prompt history truncation on retries in `orchestrator_generator.go` to preserve LLM KV prompt cache prefixes.
  - **Proposal 9: Speculative Next-Task Prefetching**: Prefetched target file contexts for candidate downstream tasks while current task 3x validation executes.

## [0.20.7] - 2026-08-06

### Changed
- **WC Validation LLM Provider Configuration**: Configured `openai` provider (`OPENAI_API_KEY`) and prioritized high-speed `gemini` (`gemini-2.5-flash`) at #1 priority across global defaults and all agent role provider ladders (`product_manager`, `architect`, `generators`, `testers`, `qa`) in `validation/projects/wc/.noctifab/config.yaml`. Moved error-prone `opencode` and high-latency `openrouter` to last-resort fallbacks.

## [0.20.6] - 2026-08-05

### Fixed
- **Cross-Story Dependency Resolution**: Added `ResolveTaskDependencies` (`pkg/services/task_dependencies.go`) to validate and resolve task dependencies across story boundaries. Prerequisite user story dependencies (e.g. `US-001`) referencing valid existing story files on disk are recognized as satisfied and omitted from the active DAG, while references to non-existent story files or unknown task IDs fail fast during `ValidatePlannedTasks`.
- **Orchestrator Deadlock Detection**: Added deadlock detection in `RunOnce` (`pkg/services/orchestrator_dispatch.go`). When 0 ready tasks exist, 0 active workers are working, and 0 tasks are in progress while pending tasks remain, the orchestrator logs detailed diagnostic information for blocked tasks, sets story status to `StoryFailed`, and aborts execution cleanly instead of looping silently.
- **State Metadata & Active Agent Reset on Story Switch**: Fixed state initialization when switching user stories in `cmd/noctifab/cli/start.go`. Always updates `state.Metadata.FeatureName`, `state.Metadata.InputPath`, and `state.Metadata.IntegrationBranch` to match the active story file, and clears `state.ActiveAgents` and `state.Tasks` between story runs.
- **Enhanced Execution Tracing Logs**: Added structured cycle tracing logs in `RunOnce` to provide end-to-end visibility of task readiness, active workers, task completions, and story finalization.

## [0.20.5] - 2026-08-05

### Changed
- **WC Validation LLM Provider Configuration**: Configured `deepseek-pro` (using `qwencloud` provider with `deepseek-v4-pro` model and `QWENCLOUD_API_KEY`) as top default priority. Assigned high-throughput `gemini` (`gemini-2.5-flash`) as the primary provider for `generators`, `testers`, and `qa` agent roles for optimized execution latency.

## [0.20.4] - 2026-08-05

### Added
- **LLM Provider Evaluation Guide**: Created [LLM_PROVIDERS.md](file:///Users/diegoj/repos/noctifab/LLM_PROVIDERS.md) providing comprehensive benchmarks, JSON formatting accuracy comparisons, latency evaluations, and recommended configuration priority patterns for `qwencloud`, `openrouter`, and `opencode`.
- **Agent-Level LLM Provider Overrides**: Extended `AgentProviderRef` configuration struct with `enable_thinking` and `thinking_budget` fields, allowing agents (e.g. `generators`, `testers`) to override provider-level thinking modes (e.g., setting `enable_thinking: false` for `qwencloud` to drop completion latency from 180s to ~2s). Added `Scenario 11` unit tests in `router_test.go`.
- **Worktree Root Manifest Syncing**: Added `syncRootManifests` in `orchestrator_execute.go` to automatically copy project root manifests (`Cargo.toml`, `package.json`, `go.mod`, `Makefile`, etc.) into fresh Git worktrees when initialized.
- **Enhanced Sandbox Policy Guidance**: Updated `validator.go` to include explicit lists of authorized tools and commands in `ValidationResult.Reason` when an unauthorized action or command is blocked.

### Fixed
- **Per-Agent Provider Context Resolution**: Added `Scenario 10` & `Scenario 12` unit test coverage verifying role resolution across `agent_role`, `role`, and `RoleContextKey{}` for all agent roles (`product_manager`, `planner`, `architect`, `generators`, `testers`, `unblocker`).
- **Authorized Tools for Generator & Tester Roles**: Added `delete_file` to `defaultRoleProfiles` for `generator` and `tester` agents in `validator.go`, allowing agents to delete redundant or conflicting files during refactoring tasks (e.g. resolving module path ambiguity between `src/domain.rs` and `src/domain/mod.rs`).
- **Resilient Planner Story Decomposition**: Added a 3-attempt retry loop to `PlanStory` in `orchestrator_server.go` so transient LLM streaming or formatting glitches during task DAG generation automatically retry instead of failing the story immediately.
- **Robust LLM Action Field Alias Unmarshaling**: Added custom `UnmarshalJSON` implementation for `Action` in `pkg/domain/action.go` to support LLM field aliases (`cmd`, `name`, `command`) mapping transparently to `Action.Tool`. Added unit test coverage in `pkg/domain/action_test.go`.

## [0.20.3] - 2026-08-04

### Fixed
- **Batch-Synchronous Dispatch Stalled the Pipeline (Critical, BOTTLENECKS #2)**: `RunOnce` launched all ready tasks and blocked on `wg.Wait()`, so a single slow task (up to the 30-minute cap) prevented newly-unblocked tasks from being scheduled. Replaced with continuous dispatch (`orchestrator_dispatch.go`): on every task completion the orchestrator re-loads state, re-evaluates readiness, and dispatches newly-ready tasks while concurrency slots are free.
- **Unbounded `State.LastActions` Growth (BOTTLENECKS #3)**: actions are now appended via `domain.AppendAction`, which caps the log at the 200 most recent entries and truncates each `Action.Result` to 4,000 tail characters, keeping OCC saves from degrading over long runs. `syncWorkspaceFiles` also skips the full-state save when the rebuilt workspace file index is unchanged.
- **Misleading "3x Consensus" Validation (BOTTLENECKS #9)**: `ValidateTask` claimed 3-run flaky consensus but ran the suite once. The run count is now constructor-injected (default 1) with real majority voting when configured >1, and all logs/comments describe the actual behavior.
- **Untimed Subprocesses Under Global Locks (BOTTLENECKS #5)**: `GitClient.Run` gets a per-command timeout (default 2m) and `GIT_TERMINAL_PROMPT=0`; `DockerSandbox.RunCommand` gets its own max duration (default 5m) plus an in-container `timeout` prefix so processes inside the container are actually killed; `checkPythonSyntax` now uses a 10-second context timeout; the Python isolated runner no longer leaks one goroutine per test file.
- **Retry/Fallback Amplification (BOTTLENECKS #6)**: model catalogs (`GetAvailableModels`, `latest`/`auto` alias resolution) are cached with a 5-minute TTL; the lower-model fallback ladder is skipped for deterministic 400/401/403/405/422 rejections; the router applies its 5-minute cooldown only to transient errors (429/408/5xx/timeouts); router candidates are memoized per role instead of rebuilding clients and re-scanning `os.Getenv` on every completion; Gemini reuses a shared HTTP transport (connection pooling) instead of a fresh one per call; `NewClient`'s hardcoded 5-second timeout fallback is now 60s.
- **Inconsistent Token Budget Accounting (BOTTLENECKS #13)**: the router recorded 1 "token" per call while `FailoverClient` recorded estimates — a daily token limit meant different things depending on which client the factory built. Both now share the same estimation helpers (`token_estimate.go`, `estCharsPerToken`), and `FailoverClient` includes the pending request's estimated prompt tokens in the pre-send budget check.
- **Unbounded Prompt/Memory Growth (BOTTLENECKS #7, #11)**: tool outputs embedded in agent prompts are capped at 8,000 chars and file contexts at 16,000 chars (head+tail truncation); `grep_search` skips files >1MB and binary files, caps results at 200 matches and 500 chars/line; subprocess output buffers (watchdog capturer, docker, python runner) are bounded to a 1MB tail.
- **Concurrency-Safety Gaps (BOTTLENECKS #8)**: task goroutines receive a deep `State.Clone()` instead of a shallow copy sharing slice backing arrays; `SetMetricsCollector` and `storyStartedAt`/`lastWorkspaceSync` are mutex-guarded; `UnblockerAgent.Start` is idempotent; `RebaseQueue.Push` fails fast when the queue was never started.
- **Swallowed OCC-Exhaustion Errors (BOTTLENECKS #13)**: all `_ = o.updateStateWithRetry(...)` sites now log failures with context; `markTaskFailed` propagates its error.
- **Dead Per-Role `iterations` Config (BOTTLENECKS #13)**: generator and tester agent loops now honor `agents.generators.iterations` / `agents.testers.iterations` (default 5) instead of a hardcoded `maxTurns = 5`.
- **Scheduler O(T²)–O(T³) Dependency Resolution (BOTTLENECKS #10)**: dependency IDs are resolved against a normalization index built once per `GetReadyTasks` call, preserving exact→normalized→substring matching semantics.
- **Daemon HTTP Hardening (BOTTLENECKS #12, #13)**: the command-channel server sets Read/Write/Idle/ReadHeader timeouts; pause/resume/cancel OCC retries use exponential backoff with jitter instead of fixed 50ms sleeps; `/api/v1/status` returns lightweight per-state summaries instead of every historical state with all relations; `/statusz` strips the file index and caps embedded actions; the Unblocker's LLM assessment is rate-limited to once per 5 minutes per stalled task; the Postgres pool sets `SetConnMaxLifetime(30m)`/`SetConnMaxIdleTime(5m)`.

- **`use_worktrees` Never Reached the Orchestrator (Latent Bug)**: `OrchestratorConfig.UseWorktrees` was never populated from `vcs.use_worktrees` (default `true`), so the orchestrator always ran in the non-worktree shared-workdir mode with concurrency 3 — the exact workspace-corruption hazard flagged in BOTTLENECKS #8. The flag is now plumbed through `serve`/`start`, and task concurrency is clamped to 1 (with a warning) whenever worktrees are disabled.
- **Full-Rewrite Saves Replaced by Dirty-Group Incremental Saves (BOTTLENECKS #3/#4)**: `Save` previously did a DELETE + per-row re-INSERT of every relation group (tasks, actions, workspace files, …) on every write. Both SQLite and Postgres repositories now fingerprint each relation group and rewrite only the groups whose content changed; fingerprints are updated only after commit and invalidated on any failure or OCC version conflict.
- **Silently Discarded Merge-Back Errors**: a failed `RebaseQueue.Push` after a successful task left the integration branch missing "successful" work with no trace; the error is now logged loudly. The dashboard's hardcoded fabricated "3x Consensus: PASS (2/3)" badge was removed.

### Added
- **Storage Retention (`storage.keep_finished_states`)**: terminal (SUCCESS/FAILED) story states beyond the most recent N (default 20; negative disables) are pruned at daemon startup via the new `PruneFinishedStates` repository method, so a long-running daemon's database no longer grows monotonically.
- **SQL-Level Status Summaries**: new `LoadAllSummaries` repository method computes per-story summaries (status, task counts, timestamps) with GROUP BY queries; `/api/v1/status` no longer loads every historical state with all relations per request.
- **Configurable Story Tick (`story_exec_interval`)**: the story execution loop frequency (previously hardcoded 2s in `serve`/`start`) is now configurable, defaulting to 2s.
- **Pre-Send Prompt Size Guard (`llm.max_prompt_tokens`)**: outgoing prompts whose estimated token count exceeds the cap (default 262,144; negative disables) fail fast with `ErrPromptTooLarge` before spending a network call and the retry/fallback ladder.

### Removed
- **Dead SSE Stream Reader**: `stream_reader.go` (`readSSEResponse`), superseded by the sliding inter-chunk idle timer on the SDK streaming path, was production-dead code and has been deleted.

## [0.20.2] - 2026-08-04

### Fixed
- **`idle_timeout` Applied as Total-Stream Deadline (Critical)**: the previous fix wrapped the entire SDK stream in `context.WithTimeout(ctx, idleTimeout)`, so any completion streaming longer than `idle_timeout` (e.g. 8s) was aborted mid-flight and transparently re-executed non-streaming — doubling latency and token cost of virtually every code-generation call, and stalling for up to `max_timeout` when gateways (e.g. OpenCode Zen) never return headers for long non-streaming completions. Replaced with a true sliding inter-chunk idle timer: the stream is cancelled only when no chunk arrives for `idle_timeout`; total duration remains governed by `max_timeout`. Regression tests: `TestSlidingIdleTimeout` (steady stream longer than `idle_timeout` survives; stalled stream fails fast with an explicit "stream idle timeout" error).
- **Deterministic LLM Errors Burned the Full Retry Ladder**: 400/401/403/404/405/422 responses and gateway `Router.Unavailable` 5xxs (deterministic rejections) were retried `max_retries` times with exponential backoff before model fallback, wasting minutes per misconfigured model (observed: 18 consecutive 500s for `kimi-k3`). `client.go` now classifies these as non-retryable (`isNonRetryableHTTPError`) and advances the model/provider fallback ladder immediately. 408/429 and generic 5xxs remain retryable.
- **Empty `body` on SDK Errors Without `error` Wrapper**: the OpenAI SDK's `Error()` string only embeds the response body's `error` field; gateways returning bare error objects (e.g. OpenCode Zen's `{"type":"Router.Unavailable",...}`) surfaced as empty-bodied errors, hiding the rejection reason from classification and logs. `sdkError` now reads back the SDK's re-populated `Response.Body` and propagates response headers (enabling `Retry-After` parsing on SDK paths).
- **Streaming HTTP Rejections No Longer Re-Run Non-Streaming**: a structured HTTP error on the streaming path is deterministic — the non-streaming POST receives the identical rejection. `sendCompletion` now surfaces such errors directly instead of doubling the call.
- **Assistant Text in `reasoning_content` Dropped (glm-5.2 and reasoning-style models)**: some OpenAI-compatible gateways return the whole answer in a non-standard `reasoning_content`/`reasoning` field with `content` empty (observed with glm-5.2 + `response_format: json_object` on OpenCode Zen), producing `contentLen=0` and JSON-envelope retry loops. Both streaming and non-streaming paths now fall back to accumulated reasoning content when `content` is empty.

### Added
- **Adaptive Request-Shape Fallback (`openai_adapt.go`)**: `baseOpenAIClient.Call` now relaxes exactly one request option per failed attempt (up to 3 attempts) based on the server's rejection: `response_format` rejection → drop JSON enforcement (pre-existing behaviour, now part of the loop); gateway `Router.Unavailable` → strip `response_format` + `max_tokens` (makes kimi-family models usable on OpenCode Zen); "invalid temperature: only N is allowed" 400s → omit the `temperature` field so the provider default applies.
- **OpenAI-Spec `json` Keyword Guarantee**: when enforcing `response_format: json_object`, the outgoing prompt is guaranteed to contain the word "json" (appending a one-line instruction when absent), as required by the OpenAI spec and enforced by strict upstreams (DashScope/Qwen: "'messages' must contain the word 'json' in some form").
- **Documentation**: `BOTTLENECKS.md` (codebase-wide bottleneck review) and `WC_ISSUES.md` (issues found running the `wc` validation project, with fix proposals).

## [0.20.1] - 2026-08-04

### Fixed
- **Layered Retry Amplification (Critical)**: The OpenAI SDK was configured without `option.WithMaxRetries(0)`, causing it to silently add 2 implicit retries on top of `client.go`'s own `max_retries` loop. A single hung upstream call (e.g. OpenCode timing out) could trigger up to 9 total HTTP attempts (3 SDK × 3 client.go), resulting in a 30m34s wall-clock block before failover to the next provider. Fixed by passing `option.WithMaxRetries(0)` in `baseOpenAIClient.sdkClient()` so only `client.go`'s explicit retry loop governs retry cadence. Regression test `TestSDKMaxRetriesDisabled` verifies exactly 1 HTTP attempt is made per `client.go` retry cycle.
- **`idle_timeout` Dead Config on SDK Streaming Paths (Critical)**: `baseOpenAIClient.sdkHTTPClient()` only set the overall `http.Client.Timeout` (`max_timeout`); the `idle_timeout` field was stored but never applied to the SDK streaming path. A hung upstream that never sent response headers would hold the connection for the full `max_timeout` (e.g. 600s) instead of timing out at `idle_timeout` (e.g. 8s). Fixed by wrapping the streaming context with `context.WithTimeout(ctx, o.idleTimeout)` in `sendCompletionStreaming()`, mirroring the sliding idle timer already present in `readSSEResponse` for the raw-HTTP path. Regression test `TestStreamingIdleTimeoutEnforced` verifies the streaming call fails within `2×idleTimeout` (150ms) instead of `max_timeout` (10s).

### Changed
- **Validation Project Provider Priority**: Reordered `llm.priority` in all 9 validation project `config.yaml` files (`wc`, `notebook`, `echo`, `frontpunch`, `t4`, `todo-cli`, `pyedis`, `calculator`, `fortune`) to put `openrouter` before `opencode`. OpenRouter pinged successfully pre-run and serves JSON-capable models; OpenCode's `qwen3.8-max` upstream was rejecting `response_format: json_object` and returning 0-byte streamed responses, making it unusable for the dark factory loop.

## [0.20.0] - 2026-08-03

### Added
- **Three New Validation Projects** (`t4`, `pyedis`, `notebook`) extending the E2E autonomy matrix to 9 projects:
  - **`t4`** (C): a bucket-less, simplified S3-style object store. C17 HTTP server with a pinned black-box contract (`PUT`/`GET`/`HEAD`/`DELETE`/list + `Range` → `206`/`416`, deterministic `ETag` = `"<size>-<fnv1a64>"`), atomic file-backed store, and a `docker-compose.yml` e2e harness that treats the server as a black box.
  - **`pyedis`** (Python 3.14 + FastAPI): a Redis-flavored key-value store exposed over HTTP (`POST /commands` with `SET`/`GET`/`DEL`/`EXISTS`/`INCR`/`DECR`/`EXPIRE`/`TTL`/`KEYS`/`FLUSHALL`), deterministic reply/error envelopes, AOF persistence with `fsync`, injected clock/store (DI), `mypy --strict`, and docker-compose e2e.
  - **`notebook`** (TypeScript strict + Fastify + PostgreSQL): a notes CRUD REST API with a pinned JSON wire contract, SQL migrations applied at startup, integration tests against a real ephemeral PostgreSQL, and a `db` + `api` + `e2e` docker-compose harness.
  - Each project ships a `SPEC.md` (pinning mandatory linter gates — `clang-tidy` + strict `gcc` for C, `ruff` + `mypy --strict` for Python, `eslint` + `tsc --noEmit` for TypeScript — plus SOLID/DDD/DI engineering guidelines and unit/integration/black-box-e2e test requirements), a toolchain `Dockerfile`, and a `.noctifab/config.yaml`.
- **Harness Wiring**: `validate.sh` artifact checks and dist compile steps, `run_one.sh` target lists, and config-load test coverage for all 9 projects (`pkg/infrastructure/config/validation_projects_test.go`).
- **`validation/projects/TESTING_GUIDE.md`**: documents the tier classification (Tier 0–3), the diagnostic run order with per-project failure attribution, and a mandatory 60-second monitoring/status-loop spec (status table columns: completion, user stories, tests, progress delta, pace, loop count, errors, token budget, last log activity, verdict).

### Changed
- **Docs**: `README.md` and `validation/README.md` updated with the full 9-project matrix; `validation/README.md` gains the tier-based effectiveness classification; `AGENTS.md` links the new `TESTING_GUIDE.md`.

### Removed
- **`VALIDATION_PROJECT_FEEDBACK.md`**: deleted from the repository. Feedback is a per-run runtime artifact generated by `gen_feedback.py` and git-ignored via `*_FEEDBACK.md`, not tracked source.

## [0.19.2] - 2026-08-03

### Fixed
- **Latest-Model Alias No Longer Mutates the Shared Client (Race)**: `Client.Complete` used to write the dynamically-resolved model back into `c.Model`. Because clients are shared across concurrent agent calls this was a data race, and the deferred restore captured the *resolved* value — so the `latest`/`-latest` alias was permanently pinned after the first call and never re-resolved. The concrete model now lives only on a local `activeModel` variable threaded through `Complete` and `getNextLowerModel`; the shared `Client.Model` is never mutated. Added regression tests (alias stays intact across repeated calls; concurrent `Complete` under `go test -race`).
- **Formatter Auto-Fix Failures Are Logged**: The `run_linter` formatter pre-step no longer silently swallows a failing formatter command (`_, _ = ...`). A failure is now reported to stderr while remaining non-fatal to the linter step.

## [0.19.1] - 2026-08-03

### Changed
- **Pin Validation Projects to `qwen3.8-max`**: The `opencode-primary` LLM provider in all 6 validation project configs (`calculator`, `echo`, `fortune`, `frontpunch`, `todo-cli`, `wc`) now pins `model: qwen3.8-max` (verified against the opencode `/models` endpoint) instead of the dynamically-resolved `latest` alias. Other provider blocks (`huggingface-kimi`, `openrouter-backup`) are unchanged.

## [0.19.0] - 2026-08-03

### Added
- **LLM Credit Exhaustion Fast-Fail (`llm.skip_on_credit_exhausted`)**: New config toggle (default `true`) that stops attempting a provider chain the moment an HTTP 402 credit-limit response (or a credit/quota-limited 429) is detected. `noctifab` no longer burns wall-clock time retrying and falling back to lower models on a spent account — the router moves straight to the next provider in `llm.priority`. When disabled, the client rotates to the next key in the `api_keys` pool and keeps retrying. Detection is robust: HTTP 402 always qualifies; a 429 only when the provider body explicitly mentions `credit` (e.g. OpenRouter's `openrouter_key_limit`).

### Fixed
- **`max_duration` Now Enforced During Execution**: The orchestrator's wall-clock cap previously only aborted a story while `StoryStatus == StoryIdle`, so a story stuck mid-execution (a hung LLM/sandbox call inside a running task) never hit the deadline. The guard now aborts as soon as the deadline elapses and any task is still unfinished, regardless of transient story status.
- **Validation Projects Enforce a 30-Minute Cap**: All 6 validation project configs (`calculator`, `echo`, `fortune`, `frontpunch`, `todo-cli`, `wc`) now set `max_duration: 30m` instead of the previously unlimited `0s`.
- **`fortune` SPEC Requires a Complete Makefile**: Added the missing `format` target to the SPEC's required Makefile targets (the harness invokes `make format`) and explicitly forbid omitting `lint`/`format` or defining any target more than once.

## [0.18.15] - 2026-08-02

### Changed
- **Remove Gemini from Validation Project LLM Providers**: Removed `gemini`/`gemini-primary` from all 6 validation project configs (`calculator`, `echo`, `fortune`, `frontpunch`, `todo-cli`, `wc`), including the `priority` lists, the provider registry entries, and the generator agent provider assignment in `calculator` and `fortune`. Each project now resolves through `opencode-primary` → `openrouter-backup` (plus `huggingface-kimi` where present).

## [0.18.14] - 2026-08-02

### Changed
- **Canonical LLM Provider Env Var Names**: Standardized HuggingFace and Hermes provider auth to the canonical environment variable names, `HUGGINGFACE_API_KEY` and `HERMES_API_KEY`. Removed the legacy `HF_TOKEN` fallback from HuggingFace provider registration and docs, and the `NOUS_API_KEY` reference, so config validation and runtime key resolution consistently use a single documented env var per provider. Added `TestResolveProviderSpecSecret_CanonicalEnvNames` covering both.

## [0.18.13] - 2026-08-02

### Fixed
- **Config Validation Rejects Registered LLM Providers**: Added the `openrouter`, `groq`, `qwen`, `dashscope`, `together`, `llama`, `meta`, `xai`, `grok`, `perplexity`, `fireworks`, `sambanova`, `cohere`, `cerebras`, `nvidia`, `ai21`, `upstage`, `kimi`, and `moonshot` provider names to the `validLLM` allowlist in `pkg/infrastructure/config/config.go` so `config.Validate()` accepts providers that are already registered in the LLM package (previously `provider: openrouter` in validation project configs failed with `invalid LLM provider`).
- **Ping Dispatch Now Uses Data-Driven ProviderSpec Registry**: `newProviderClientForPing` in `pkg/infrastructure/llm/ping.go` now resolves the client via `GetProviderSpec(...).NewClientFunc` before falling back to the hardcoded switch, so ping works for all registry-registered providers (e.g. `openrouter`).
- **Registry Drift Guard**: Added `config.IsValidLLMProvider` and `llm.RegistrySnapshot`, plus a unit test asserting every registered provider is accepted by config validation, preventing future divergence between the LLM registry and the config allowlist.
- **Validation Project Config Load Test**: Added `TestLoadValidationProjectConfigs` in `pkg/infrastructure/config/validation_projects_test.go` that loads every validation project config (`calculator`, `echo`, `fortune`, `frontpunch`, `todo-cli`, `wc`) through the full config pipeline (skipping when local `secrets.yaml` is absent, as it is gitignored).

## [0.18.12] - 2026-08-01

### Added
- **Universal "latest" Model Alias Support Across All LLM Providers**: Implemented dynamic `/models` endpoint model resolution and `ExcludedKeywords` filtering for all LLM providers (`openai`, `anthropic`, `gemini`, `mistral`, `deepseek`, `hermes`, `ollama`, `huggingface`, `kimi`, `qwen`, `llama`, `xai`, `perplexity`, `opencode`, etc.).
- **Comprehensive Multi-Provider Latest Model Resolution Unit Tests**: Implemented `TestProvidersLatestAliasResolution` in `pkg/infrastructure/llm/providers_latest_test.go` with authentic mock HTTP `/models` responses verifying automatic resolution of `"latest"` model aliases across all LLM providers.
- **Validation Project Monitoring Guidelines in AGENTS.md**: Added periodic 60-second status reporting specifications and stuck-detection rules (`Stuck?`, `Tests (Passed/Total)`, `Elapsed Time`, `Last Log Activity`, and `Completed ✅` emoji status indicator) for validation project execution matrix.

## [0.18.11] - 2026-08-01

### Added
- **Dynamic "latest" / "auto" Model Alias Resolution**: Added automatic model alias resolution in `Client.Complete` for model configurations specifying `"latest"`, `"auto"`, or `"<provider>-latest"`. Noctifab dynamically queries the provider's `/models` endpoint, filters out specialized niche models, ranks available general-purpose models by version and tier, and resolves the client model to the top-ranked available endpoint.
- **`ExcludedKeywords` Filtering in Declarative Model Parsers**: Added `ExcludedKeywords []string` support to `ParserConfig` in `pkg/infrastructure/llm/provider_registry.go` and configured Gemini's model parser to reject niche/specialized endpoints (`robotics`, `embed`, `imagen`, `bison`, `tts`, `stt`), preventing niche preview models from slipping through as general LLM fallbacks.

## [0.18.10] - 2026-07-31

### Fixed
- **E2E Simulation Harness & Linter Verification Fixes**: Fixed unused functions in `tests/e2e/scenario_test_utils.go` (`scanWorkspaceFiles` and `resolveDependencies`) by wiring workspace scanning and DAG dependency validation into `runSimulatedOrchestrator`. Fixed E2E test failures in `TestScenario_DjangoCRUD`, `TestScenario_UpstreamFailurePruning`, `TestScenario_BudgetExceededMidExecution`, `TestScenario_ShutdownResumption`, and `TestScenario_ContextCompaction` by wrapping `domain.ErrBudgetExhausted`, processing LLM clarification and task creation actions, logging graceful shutdown actions, allowing retries for views template generation, and pruning downstream tasks upon upstream failures.

## [0.18.9] - 2026-07-31

### Fixed
- **500-Line Code File Limit Compliance**: Refactored Go source code and test files exceeding 500 lines into smaller domain helper modules (`pkg/services/orchestrator_generator.go`, `pkg/services/orchestrator_execute_helpers.go`, `pkg/infrastructure/config/config_validation_test.go`, and `tests/e2e/scenario_simulation_test.go`), achieving 100% compliance with `AGENTS.md` section 2.1 rules.
- **Provider Struct Composition Data-Driven Dispatching**: Removed legacy `if provider == "gemini"` hardcoded logic from `getNextLowerModel()` in `pkg/infrastructure/llm/client.go` by registering `ParseModelFunc` on Gemini's `ProviderSpec`, making LLM lower-model fallback 100% data-driven across all providers.
- **Command Context Wiring**: Replaced un-cancellable `context.Background()` calls for `rebaseQueue` and `mailbox` daemons in `cmd/noctifab/cli/start.go` with the cancellable root command context, enabling graceful shutdown on process termination signals.

## [0.18.8] - 2026-07-31

### Fixed
- **TestValidationProjectsConfigs_StrictSchemaValidation Removed**: Removed the `TestValidationProjectsConfigs_StrictSchemaValidation` test from `pkg/infrastructure/config/config_test.go`. This test hardcoded paths to validation project config files that are not present in the CI runner environment, causing systematic failures for all subtests (calculator, echo, fortune, frontpunch, todo-cli, wc).

## [0.18.7] - 2026-07-31

### Fixed
- **TestWriteDefaultConfig Root Compatibility**: Replaced `/nonexistent-dir-12345/foo/bar` with a path that uses an existing regular file as a parent directory, ensuring `MkdirAll` always fails even when the test runs as root in Linux CI containers.
- **E2E BudgetStore Type Mismatch**: Fixed `assert.Equal(t, 0.0, usage)` to `assert.Equal(t, int64(0), usage)` in `tests/e2e/scenario_comprehensive_test.go` to match the `int64` return type of `GetDailyUsage`, eliminating the `float64 vs int64` mismatch that caused the E2E suite to fail.

## [0.18.6] - 2026-07-31

### Fixed
- **Gosec Binary Detection in Unit Tests**: Replaced `exec.Command("gosec").Run()` with `exec.LookPath("gosec")` in `pkg/services/sast_scanner_test.go` to correctly detect if `gosec` is installed on `PATH`, preventing false test failures in environments where `gosec` is present (such as CI Linux runners).

## [0.18.5] - 2026-07-31

### Fixed
- **Thread-Safety in Unit Test Mocks**: Added `sync.Mutex` synchronization and JSON state cloning to `mockRepo` and `mockVCS` in `pkg/services/orchestrator_test.go` to eliminate data races during concurrent unit test executions (e.g. `TestOrchestrator_ConcurrentWorktreeIsolation`).

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
