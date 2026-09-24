# AGENTS.md: Development Guidelines for AI Coding Assistants

Welcome, agent. This document outlines the rules, architecture, and coding constraints of the `noctifab` repository. Read and follow these directives strictly before modifying any files.

---

## 1. Core Reference File

Before planning or executing any task in this codebase, you **must** read and understand:
*   [SPEC.md](/SPEC.md) - The technical specification of the project.
*   [TESTS.md](/TESTS.md) - The testing strategy, structures, and verification specifications.

---

## 2. Mandatory Coding Constraints

To maintain modularity and high context compatibility, the following guidelines are absolute:

1.  **File Size Limits:**
    *   No single Go source code file (`.go`) may exceed **500 lines** of code.
    *   If a file you are working on or creating approaches or exceeds this limit, you **must** split/refactor it into smaller, logically coherent domain models or helper packages.
2.  **Architecture & Design:**
    *   **Dependency Injection (DI):** Do not hardcode dependencies. Provide all objects, configurations, and clients through constructors. Code must be built in a way that is easy to test (utilizing dependency injection to make components highly mockable and isolated).
    *   **SOLID:** Keep classes/structs focused on a single responsibility.
    *   **Domain-Driven Design (DDD):** Align packaging boundaries to domain logic (e.g., domain entities, value objects, and service interfaces), not technical categories.
    *   **Provider Struct Composition (LLM Infrastructure):** All LLM provider clients must reside in dedicated per-provider source files (`pkg/infrastructure/llm/<provider>.go`). OpenAI-compatible providers must embed `*baseOpenAIClient` and use the declarative `NewModelParser` engine. Core dispatching in `client.go` must be data-driven via `ProviderSpec.NewClientFunc` with zero protocol `switch` statements.
    *   **Thin Shell Entrypoint & Test Scope Alignment Pattern:** Primary executable/daemon entrypoints (`main.go`, `main.rs`, `main.py`, `server.ts`, etc.) must be thin wrappers (< 15 lines) delegating immediately to pure, callable application core functions/factories (e.g. `run(args, io) -> Result`, `create_app()`, `WorkerEngine`). Intermediate component tasks must author fast, in-process unit and integration tests against library APIs directly in memory, reserving external compiled binary OS invocations (`cargo_bin`, child process spawns) for entrypoint and black-box E2E tasks.
    *   **Verification vs. Validation Engineering Strategy:** Task execution is divided into two distinct stages: *Verification* (building minimal working functionality that compiles and satisfies baseline checks) and *Validation* (black-box behavioral testing against public contracts, CLI outputs, and API signatures). Tests must never assert internal module implementation details.
    *   **Product Manager Definition of Done (DoD) Mandate:** Generated user stories (`roadmap/US-xxx.md`) must specify explicit public API signatures, binary executable paths, I/O formatting invariants, error prefixes, exit codes, number precision representations, and zero-failure test pass criteria before downstream task planning starts.
    *   **Product Manager Modular File Decomposition & Anti-Monolith Mandate:** When generating user stories and refining specifications, the Product Manager agent must enforce strict separation of concerns and modular source file decomposition by responsibility (e.g. dedicated files per command, handler, codec, parser, model, or domain entity). Even if `SPEC.md` does not specify a directory layout, or even if `SPEC.md` explicitly specifies or implies a lone gigantic source file, the Product Manager MUST override it, mandating a modular layout and listing exact per-responsibility implementation and test file paths in user story Definitions of Done. This prevents monolithic code files, eliminates worktree merge conflicts, maximizes task parallelism, and minimizes LLM context token consumption.
    *   **Project & Language Agnosticism:** Noctifab MUST NOT HAVE validation project-specific or language-specific code in its codebase. Noctifab is a dark factory agent; do not add specific instructions, context helpers, or code rules for particular validation projects or programming languages.
    *   **Resilient Code Fence & Output Parsing:** LLM responses often wrap structured output (JSON, YAML, diffs, code) in markdown code fences (````go ... ````, ````json ... ````) containing inner backticks or commentary within string literals and comments. Parsing utilities (`pkg/services/`, `pkg/infrastructure/llm/parser/`) must never use naive substring matching (`strings.Index("```")`) that breaks on inner code blocks. Parsing must be string-literal aware and safely strip outer markdown envelopes without corrupting payload data or panicking.
    *   **Pre-Flight Path Exclusion & Context Hygiene:** Agents must preserve LLM context economics and prevent repository pollution. All workspace scanning, context packing, and VCS staging routines must strictly enforce exclusion policies via `IsPathExcluded` (ignoring `.git`, `.noctifab`, build outputs, binaries, toolchain caches, and third-party dependencies). Ephemeral worktrees, build artifacts, or secret files must never be loaded into LLM prompts or committed to git.
    *   **Incremental State Persistence & Append-Only Telemetry:** When implementing or modifying database storage layers (`pkg/infrastructure/storage/`), destructive `DELETE FROM <table> WHERE state_id = ?` table-wiping loops inside state persistence transactions are strictly forbidden. Operational entities (`tasks`, `stories`, `workspace_files`) must use `INSERT ... ON CONFLICT DO UPDATE` upserts, selectively pruning only items removed from the domain model. High-frequency event streams and telemetry logs (`actions`) must be stored in append-only tables with unique identifiers (`ON CONFLICT(action_id) DO NOTHING`), guaranteeing stable row IDs and eliminating SQLite write lock thrashing on progress updates. High-frequency telemetry loaders must bound queries to the configured window (`domain.MaxLastActions = 200`) ordered chronologically, ensuring $O(1)$ memory consumption and load latency regardless of total execution history.
    *   **Database Migration Pair Discipline:** When modifying database schemas (`pkg/infrastructure/storage/migrations/`), every change requires both creating a new numbered migration file under `sqlite/` (e.g. `0009_*.sql`) and updating the baseline schema file `sqlite.sql`. All migration DDL must be idempotent and fully SQLite-compatible (e.g. avoiding unsupported Postgres constructs, and adhering to SQLite UPSERT index requirements).
    *   **Process-Aware Lock Governance:** Never use arbitrary short timers (e.g., 5 seconds) to clean filesystem or Git locks (`.git/index.lock`, worktree locks). Agents must verify lock holder process liveness via OS signal checks (`kill -0 <pid>`) before unlinking locks, and enforce a safe fallback grace threshold (at least 60 seconds) if PID is unrecorded to prevent Git index corruption during long compilations.
    *   **Shared Toolchain Dependency Caches in Worktrees:** When spawning isolated Git worktrees for parallel tasks, agents must redirect heavy toolchain build caches (e.g. `CARGO_TARGET_DIR`, `GOCACHE`, `node_modules`, Python virtual environments) to shared repository cache directories (`.noctifab/cache/`) rather than letting each worktree re-download and re-compile dependencies from scratch.
    *   **Hermetic In-Memory Doubles & Goroutine Leak Prevention:** Intermediate unit and integration tests must execute hermetically in memory. When testing asynchronous messaging, event brokers (`pkg/infrastructure/broker/`), or worker loops, always use buffered channels or mock subscribers with explicit timeout contexts (`context.WithTimeout`). Never allow unbuffered channel sends or missing subscribers to block goroutines indefinitely and deadlock the test suite. External filesystem and database tests should leverage `t.TempDir()` or `:memory:` SQLite rather than touching host files.
3.  **Testing Strategy:**
    *   All code must be **100% unit tested**. Every Go package must be accompanied by unit tests.
    *   After making any change to the codebase, you **must** run the test suite to verify correctness.
    *   Tests must reside in files ending with `_test.go` in the same directory as the target logic. Detailed testing context and architecture details are documented in [TESTS.md](/TESTS.md).
    *   When writing new features, ensure corresponding unit tests are implemented concurrently.
    *   **How to Run Unit & Local Integration Tests:**
        *   Run all unit and in-process CLI integration tests locally:
            ```bash
            go test -v ./pkg/... ./tests
            # Or with race detection:
            go test -race ./pkg/...
            ```
        *   Alternatively, use the Makefile target:
            ```bash
            make test
            ```
    *   **How to Run End-to-End (E2E) Tests:**
        *   Run the containerized E2E test suite (which sets up a PostgreSQL instance via Docker Compose):
            ```bash
            docker compose -f tests/e2e/docker-compose.yml up --build --exit-code-from test-runner
            ```
    *   **BDD Specifications:** Acceptance tests must always run under a test runner using BDD format with the context pattern: `when <scenario>`, `it <action happens>`. Generated tests must be e2e as much as possible for the happy paths, input validations/edge cases must be unit tests, and complex internal validation flows must be integration tests.
4.  **Formatting & Linting:**
    *   **Formatting:** All Go source code must strictly follow the standard `go fmt` format. Ensure `go fmt ./...` runs clean.
    *   **Linting:** Code must pass static analysis checks. Run the linter via:
        ```bash
        make lint
        # or directly via Docker:
        docker run -t --rm -v $(pwd):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint run
        ```
    *   **Efficient String Writing (`writestring`):** Inefficient string concatenations inside calls to `WriteString` (e.g., `sb.WriteString("a" + b + "\n")` or `io.WriteString(w, a + b)`) are strictly forbidden. Always call `WriteString` sequentially for individual string components to prevent unnecessary memory allocations and comply with the `writestring` static analyzer (`golang.org/x/tools/go/analysis/passes/writestring`).
5.  **Continuous Integration (CI):**
    *   A GitHub Actions workflow configured in `.github/workflows/ci.yml` executes on every push and pull request.
    *   All unit tests and static analysis linting checks must pass successfully in the CI pipeline before merging.
6.  **Branching & Commit Guidelines:**
    *   **No Commits on Main**: Never create commits directly on the `main` branch. Always create a new branch with the changes.
    *   **CHANGELOG & Version Updates**: Every commit must contain the corresponding changes documented in `CHANGELOG.md`, incrementing the version accordingly (`VERSION`, `pkg/version/version.go`, and `CHANGELOG.md`): minor version bump for features, and patch version bump for bug fixes.
7.  **Resilience & Forced Compilation Mandate:**
    *   A bad scaffold, failing test, or compiler error must never permanently stop development or leave broken code.
    *   In case of any persistent or complex error, agents MUST force a compiling and runnable solution (even if simplified, fallback, or imperfect). The codebase MUST compile cleanly and pass tests; leaving a partially broken build or stalling on retries is completely unacceptable.
8.  **Secrets Handling & Privacy Mandate:**
    *   It is **completely forbidden** for AI agents to directly open, view, read, print, or leak the contents of `secrets.yaml` files into context or user-facing logs.
    *   When instructed or required to perform an action using credentials stored in `secrets.yaml` (e.g., executing an HTTP request or testing an API endpoint), you **must write a script or file** that programmatically reads `secrets.yaml`, extracts the needed key, and executes the task. Under no circumstances should raw secrets be printed, echoed, or included in tool parameters/outputs.
9.  **Comprehensive Error Handling & Crash Prevention Mandate:**
    *   Error handling **MUST BE comprehensive**. Every error across all packages, layers, and goroutines must be explicitly checked, handled, or wrapped with contextual detail (`fmt.Errorf("...: %w", err)`).
    *   All potential error scenarios and edge cases must be dealt with defensively in the best way possible (including graceful degradation, exponential backoff with jitter on transient database/network contentions, fallback strategies, proper unblocking of waiting callers, and thorough channel/mutex cleanup on shutdown) so that the program never unexpectedly crashes, deadlocks, panics, or silently loses state.
10. **Documentation Synchronization Mandate:**
    *   Every time the configuration schema, CLI flags, architectural components, prompt templates, or functionality changes, before committing, AI agents **MUST** inspect and update the corresponding documentation files under `docs/` and (if applicable) `README.md` and `SPEC.md`.
    *   Never leave documentation out of sync with code modifications. Ensure all configuration options, examples, and architecture references accurately reflect the latest codebase state.
11. **Deterministic Checks & Anti-Hallucination Mandate:**
    *   **Deterministic Gates vs. Stochastic Generators:** LLMs are generative code engines, not truth oracles. LLMs are probabilistic token predictors prone to confirmation bias, sycophancy, and plausible-sounding hallucinations when evaluating their own code or reasoning about structured state.
    *   **Strict Code-Based Verification:** Any verification that can be executed via programming language constructs (AST parsing, compilers, test runners, exit codes, file system existence, JSON/YAML schemas, socket dials, regex patterns) **MUST** be performed deterministically in code.
    *   **Prohibition of Conversational Auditing:** Never prompt an LLM in conversational natural language to evaluate, audit, or speculate whether structured state satisfies a requirement, whether code compiles, whether a test passed, or whether a daemon is running. Deterministic checks executed by the OS and runtime environment are the primary and most reliable defense against LLM hallucinations.
12. **LLM-Generated Test Harnesses & Diagnostic Probe Scaffolding Mandate:**
    *   **Code Generation Over Conversational Speculation:** When diagnosing service, daemon, socket, protocol, or liveness failures (e.g. Redis on 6379, HTTP API on 8080, database connection drops), **NEVER** ask the LLM to explain why the system failed in conversational prose.
    *   **Mandate Executable Probe Creation:** Instruct the LLM to **author an executable diagnostic probe script or test scaffolding** in the target project's language (e.g. `tests/probe_liveness.<ext>`, `tests/test_socket.py`, `probe_daemon.go`, or curl/bash scripts) that connects to the socket/port, sends test payloads, asserts responses, and captures OS-level error details (`ECONNREFUSED`, `ETIMEDOUT`, exit codes).
    *   **OS Ground Truth:** Run the probe script deterministically and pass the empirical terminal output and exit code back to the LLM. Ground truth must always come from the operating system and runtime, never from LLM speculation.
    *   **Proactive Test Scaffolding for Features:** When planning user stories or implementing new functionality, instruct the LLM to scaffold automated tests, BDD scenarios, or mock server harnesses *first*, establishing a rigid, automated test barrier before implementing production logic.
13. **Exhaustive Preflight Checks Mandate (Pre-Execution Hygiene):**
    *   **Toolchain & Runtime Preflight:** Before dispatching prompts or executing tasks, deterministically inspect the host environment for required runtimes, compilers, package managers, and container engines (`go`, `cargo`, `python3`, `node`, `make`, `docker`). If a required host runtime is absent, automatically detect it and transition to a containerized sandbox environment without waiting for failed task executions.
    *   **Build Manifest & Target Preflight:** Before executing build, formatting, or linting commands (e.g., `make format`, `make lint`, `npm run lint`), inspect project build manifests (`Makefile`, `package.json`, `Cargo.toml`, `pyproject.toml`, `go.mod`). Verify that target recipes actually exist before invoking them. If an optional recipe is missing from the `Makefile`, automatically skip or disable that quality gate in the preflight phase, eliminating wasted 30-second command timeouts and useless LLM repair cycles.
    *   **Fast AST & Syntax Pre-Flight Gate:** Before invoking full compiler pipelines, expensive linters, or heavy integration test suites, execute fast, in-process syntax checks (`go/parser`, `python3 -m py_compile`, `json.Unmarshal`, language AST parsers) on all modified files. Syntax errors are caught in milliseconds, preventing wasted LLM context, execution tokens, and long tool timeouts.
    *   **Workspace & Process-Aware Lock Preflight:** Verify clean worktree state, confirm base branch alignment, and verify lock holder process liveness via OS signal checks (`kill -0 <pid>`) before clearing locks, enforcing safe fallback grace thresholds if the PID is unrecorded.
14. **Task & User Story Work & DoD Verification Mandate:**
    *   **Universal Work Verification (Zero-Mutation Guard):** Every completed task and user story must produce verifiable mutations (uncommitted working tree changes, git diffs, or commits). Tasks or stories claiming completion with zero file mutations relative to the integration or base branch must be rejected deterministically as false-positive hallucinations.
    *   **Definition of Done (DoD) Enforcement:** Every user story must strictly satisfy its Definition of Done through passing automated test suites and programmatically executing machine-readable `PublicContracts` (`allowed_executables`, `exit_codes`, `stdout_contains`, `stderr_prefixes`). Verification must execute the compiled binary or script against these exact inputs and outputs programmatically, failing any story whose binary is missing, exits with an unexpected status, or produces invalid output.
15. **Language-Agnostic Structured Test Parsing & Anti-Spoofing Protocol:**
    *   **Structured Parsing Over Substring Matches:** Never rely on naive substring matching (e.g. checking for `"PASS"` or `"OK"`), which can be spoofed by dummy print statements (`print("ALL TESTS PASSED")`) or empty test suites.
    *   **Structured Test Output Formats:** Parse test runner outputs using language-agnostic structured parsers (JUnit XML, TAP, streaming JSON events such as `go test -json`, pytest reports, or strict multi-line regex metrics).
    *   **Deterministic Metric Assertion:** The test parser must deterministically assert that `total_tests > 0` and `failed_tests == 0`. Any test run that executes 0 tests or exits with zero tests discovered must be treated as a failure, preventing skipped or deleted test suites from falsely passing quality gates.
16. **Script-First Execution & Programmatic Debug Loop Mandate:**
    *   **Executable Programs Over Conversational Speculation:** For every task, diagnosis, verification, data transformation, socket/HTTP probe, bug reproduction, or validation that can be performed by an executable program or script: **always author the program or script** in the target language (e.g. `tests/probe_<name>.<ext>`, `scripts/<tool>.<ext>`, or in-process tests) and **execute it**.
    *   **Structured Data Processing Reflex:** For any agent that does a task dealing with structured data (JSON, YAML, TOML, CSV, XML, ASTs, SQL seed/migrations, tabular data, schemas, or graph relationships), it **MUST ask itself whether creating and running an automated script (in Python, Go, or the project runtime) wouldn't be better, faster, and more reliable than manual text editing or conversational manipulation**. Programmatic parsing, validation, serialization, and transformation guarantee mathematical precision, schema conformance, and eliminate manual human-style editing bugs, typos, and token-truncation errors.
    *   **Iterative Programmatic Debug Loop:** If the script or program fails, crashes, or produces unexpected output on the first run, **never resort to conversational prose or hallucinated theories**. Run an iterative debug loop with the program: inspect the empirical error output, exit codes, and stack traces, surgically modify the program, and run it again until it behaves as expected and produces ground truth.
    *   **OS Reality as the Truth Oracle:** Ground truth comes exclusively from running executable code against the operating system, compilers, sockets, and runtimes. LLMs must act as code authors and programmatic debuggers, never as speculative interpreters.
17. **Anti-False Zero Exit Code & Crash Signal Mandate:**
    *   **Masked Fatal Crash Rejection:** Never assume a process passed simply because its exit code was reported as 0. Shell constructs (`|| true`), unhandled subshell exceptions, or broken signal traps often mask fatal crashes.
    *   **Deterministic Signal & Crash Scanning:** The test and execution engines must scan stdout and stderr for fatal process signals (`SIGSEGV`, `SIGBUS`, `SIGABRT`, `SIGILL`, Go/Rust panics, Python fatal errors, out-of-memory `OOMKilled`, AddressSanitizer deadly signals). If any fatal crash signal is detected in output, the run must be rejected immediately as a failed execution, preventing masked crashes from falsely passing quality gates.

---

## 3. The Stateless Rule for Agents

`noctifab` is designed around the principle of a **stateless agent** controlled by a **stateful orchestrator**:
*   The orchestrator loads, updates, validates, and saves the system state.
*   The LLM agent only operates on the state snapshot provided to it at each step.
*   **Do not rely on the LLM's conversation history** to track what has happened previously. Always inspect the current State struct representation (e.g., in JSON file databases, local configurations, or databases) to determine the next task.

---

## 4. Running Validation Projects (Local E2E Matrix)

For comprehensive instructions, project matrix overview, capability ladders, runner scripts, dynamic scale timeouts, and failure attribution, refer directly to [**`validation/README.md`**](validation/README.md).

**Core Directives for Agents:**
- **Execution Harness**: Run validation via `python3 validation/bin/matrix_runner.py <projects>`, `python3 validation/bin/runner_9projects.py`, or `make validate PROJECT=<project>`.
- **Spec-Driven Autonomy**: Never pre-create, hand-edit, or commit static user stories under `validation/projects/<project>/roadmap/`. Noctifab must autonomously decompose `SPEC.md`.
- **Configuration Immutability**: Never alter a project's `.noctifab/config.yaml` when running validation.
- **Monitoring & Token Hygiene**: Do not run periodic 60-second polling loops or schedule timers. Launch jobs asynchronously in the background and rely on completion notifications and atomic execution reports (`validation/projects/<project>/output/report/*.md`). Aggregate metrics via `make validate-summary`.


