# 🤖🌌 noctifab

[![CI Build Status](https://github.com/diegojromerolopez/noctifab/actions/workflows/ci.yml/badge.svg)](https://github.com/diegojromerolopez/noctifab/actions)
[![Go Version](https://img.shields.io/github/go-mod/go-version/diegojromerolopez/noctifab)](https://github.com/diegojromerolopez/noctifab)
[![Documentation Status](https://readthedocs.org/projects/noctifab/badge/?version=latest)](https://noctifab.readthedocs.io/en/latest/?badge=latest)
[![Autonomy Level](https://img.shields.io/badge/Autonomy-Level%203%20%2F%204-blueviolet)](https://noctifab.readthedocs.io)
[![License](https://img.shields.io/github/license/diegojromerolopez/noctifab)](/LICENSE)
[![Linter Status](https://img.shields.io/badge/Linter-Linting%20Clean-success)](https://github.com/diegojromerolopez/noctifab)

`noctifab` is an autonomous, long-running agentic harness that operates without human intervention to resolve issues, verify builds, run tests, and manage software project lifecycles. 

Designed as a **Dark Factory Platform** for GitHub and GitLab, it is compiled as a single Go binary and runs as a single-node autonomous loop engine to replace manual developer execution bottlenecks.

---

## ⚡ 1-Line Quickstart Installer

Install `noctifab` instantly on macOS or Linux:

```bash
curl -sSL https://raw.githubusercontent.com/diegojromerolopez/noctifab/main/scripts/install.sh | sh
```

Initialize a project workspace (creates `.noctifab/config.yaml`, `.noctifab/secrets.yaml`, and `SPEC.md` template):

```bash
noctifab init [my-project-dir]
```

Interactively draft, refine, and audit your software specification with multi-agent AI consensus:

```bash
noctifab spec "Build a REST API with JWT authentication and PostgreSQL"
```

Launch the dark factory loop in any project with a `SPEC.md` (add `-w` for live Visual Web Dashboard & Spec Studio):

```bash
noctifab start [my-project-dir] -w
```

---

## Autonomy Matrix

The platform classifies development automation into distinct levels. `noctifab` is built to run at **Level 3** and **Level 4** autonomy:

| Level | Name | Platform Behavior |
| :--- | :--- | :--- |
| **Level 1** | Autocomplete | AI suggests code inline. Human drives the editor and makes all decisions. |
| **Level 2** | Interactive Assistant | AI generates entire files/functions. Human reviews every single change in the editor. |
| **Level 3** | Spec-Driven (Gated) | AI generates code autonomously from specifications. Continuous test suites gate quality. Human clicks merge. |
| **Level 3.5** | Selective Auto-Merge | Same as Level 3, but low-risk modules merge automatically. Human can block. |
| **Level 4** | Full Dark Factory | Specs go in, tested code comes out fully merged. Human reviews only exceptions. |

### Configuring Autonomy Level

The autonomy level is controlled by the VCS `pull_request` settings in `.noctifab/config.yaml`:

| Level | `pull_request` settings | Description |
|---|---|---|
| **Level 3** | `auto_create: true`, `auto_merge: false` | Generates branches and PRs; human reviews and merges. |
| **Level 3.5** | `auto_create: true`, `auto_merge: true` | Validated tasks merge automatically upon consensus passing. |
| **Level 4** | `auto_create: true`, `auto_merge: true`, `auto_rebase: true` | Fully autonomous dark factory loop with automated rebase queuing. |

---

## Core Pillars

1. **Stateless Agent, Stateful Orchestrator**: The AI agents have no memory of previous runs or actions. Instead, the orchestrator compiles and tracks system state (tasks, file indices, action logs, and clarifications) in a local database (SQLite/PostgreSQL) and feeds it to the agent at each step.
2. **Topological Task Scheduling**: Decomposes complex feature specifications into a Directed Acyclic Graph (DAG) of task models, running independent tasks concurrently.
3. **Verification First, Validation Second**: Decouples execution into two distinct lifecycle stages: *Verification* (achieving a minimal working solution that compiles and passes basic functional checks) and *Validation* (leveraging black-box test safety rails to iteratively refactor, optimize, and harden code to full specification compliance).
4. **Test-Driven Quality Gates**: Employs a multi-stage sequential execution cycle between the generator and test-writer agents. The Test Validator executes the test suite 3 times, requiring a majority vote consensus (at least 2/3 passing runs) to approve changes, preventing regression and flaky builds.
5. **Sandboxed Action Isolation**: Safely edits files and runs test commands inside host path jails or isolated Docker containers, restricted by role-based authorization profiles.

---

## Architecture: The Software Dark Factory Loop

To understand how `noctifab` works as a "dark factory" (an automated software development environment operating without human intervention), it helps to view the system as a **stateful orchestrator** controlling **stateless, role-segregated agent workers**.

```mermaid
flowchart TD
    subgraph Inputs ["Input Specifications & Developer Steering"]
        Spec["SPEC.md / User Stories"] -->|Product Manager| Roadmap["Roadmap & Specs"]
        Developer["Developer Orders / Prompts"] -->|"noctifab steer / order"| CmdMailbox["Command Mailbox"]
        TUI["Terminal TUI (noctifab dashboard)"] <-->|Real-Time Telemetry| CmdMailbox
        WebUI["Web Dashboard (noctifab start -w / dashboard -w)"] <-->|"SSE Live Stream & Prompts"| CmdMailbox
    end

    Roadmap -->|Planner Agent| DAG["Topological Task DAG"]
    CmdMailbox -->|Inject Steering Directives| Orchestrator["Stateful Orchestrator"]
    DAG -->|Task Schedule| Orchestrator

    subgraph FactoryLoop ["Dark Factory Autonomous Execution Loop"]
        Orchestrator -->|Observe State| StateDB[("State DB (SQLite / Postgres)")]
        Orchestrator -->|Decide & Dispatch| Worktree["Git Worktree Sandbox"]

        Worktree -->|1. Verification| GenMinimal["Generator Agent (Minimal Functional Code)"]
        GenMinimal -->|Commit| Worktree

        Worktree -->|2. Black-Box Tests| TesterWrite["Tester Agent (Behavioral Tests)"]
        TesterWrite -->|Commit| Worktree

        Worktree -->|3. Validation| GenRefactor["Generator Agent (Refactor & Polish)"]
        GenRefactor -->|Commit| Worktree

        Worktree -->|4. Test Alignment| TesterRefactor["Tester Agent (Align Tests)"]
        TesterRefactor -->|Commit| Worktree

        Worktree -->|Validate Quality Gate| Consensus["3x Consensus Test Validator"]
        Consensus -->|Run Test Suite| Worktree
    end

    Consensus -->|"Majority Pass (>= 2/3)"| Merge["Rebase / Auto-Merge PR to main"]
    Consensus -->|Test Failures| Retry["Incremental Backoff / Unblocker Repair"]

    Merge -->|Update State| StateDB
    Retry -->|Update State| StateDB
```

### The Verification vs. Validation Principle

`noctifab` structures development around two complementary phases:
- **Verification Stage ("Make It Work First")**: The Generator Agent focuses on functional correctness. It builds the simplest working implementation that compiles, links, and satisfies basic sanity checks. The goal is to reach a green baseline quickly without getting stalled by over-engineering or premature optimization.
- **Validation Stage ("Make It Clean & Robust Under Test Safety Nets")**: Once tests are written (asserting public contracts, API signatures, and CLI outputs—*never* private implementation details), the agent leverages these tests as a safety net. It iteratively refactors, cleans up, and hardens the code against edge cases and specification requirements.

### The Orchestrator Loop (Observe -> Decide -> Validate -> Execute -> Save)
The core engine runs a continuous polling event loop that drives all development tasks:
1. **Observe (State Sync)**: The orchestrator scans the filesystem to index files, build metadata, and check the task database. It ensures a consistent, up-to-date representation of the workspace. During startup, it automatically executes database migrations inside transactions.
2. **Decide (Task Scheduling)**: It analyzes the Directed Acyclic Graph (DAG) of tasks. Ready tasks (those whose dependencies have succeeded) are selected and dispatched concurrently up to the configured limit.
3. **Execute (Agent Dispatch)**: For each ready task, the orchestrator sets up an ephemeral git worktree/sandbox environment and executes a multi-stage, sequential coordination flow:
   - **Initial Flow (Retries = 0)**:
     1. *Verification (Minimal Functional Code)*: Dispatches the **Generator Agent** to implement the bare-minimum logic required for the task to compile and run.
     2. *Black-Box Test Writing*: Dispatches the **Tester Agent** to write unit and integration tests verifying observable behaviors, return contracts, and CLI/API outputs.
     3. *Validation (Refactoring & Hardening)*: Dispatches the **Generator Agent** to refactor, optimize, and expand the implementation under the safety net of the passing tests.
     4. *Test Alignment*: Dispatches the **Tester Agent** to refine, clean, and align the test suite to match the final implementation structure.

   - **Retry Flow (Retries > 0)**:
     1. *Fix Implementation*: Dispatches the **Generator Agent** to address validation failures and refactor the code.
     2. *Fix Tests*: Dispatches the **Tester Agent** to fix or refactor tests to align with the updated code.
4. **Validate (Quality Gate Evaluation)**: Post-generation, the orchestrator runs the project's test suite inside the sandbox. To guard against flaky tests, the **Test Validator** runs the suite 3 times, requiring a majority vote consensus (e.g., at least 2/3 passing runs) to succeed.
5. **Save & Integrate (Rebase/Merge & State Update)**:
   - If tests pass, the branch is pushed, a Pull Request is created and automatically merged using the rebase queue, and the task is updated to `SUCCESS`.
   - If tests fail, the task is marked as `PENDING` to be retried (or `FAILED` if retry limit is reached).
   - In all cases, the ephemeral worktree is pruned to maintain a clean workspace.

---

## Self-Healing, Dynamic Prompts & Self-Correcting Resiliency

`noctifab` is designed with robust self-healing and dynamic prompt adaptation mechanisms at both the agent and orchestrator levels to maximize autonomous progress, self-correct errors, and prevent execution stalls:

1. **Dynamic Prompt Enhancement & Unblocker Log Injection**: When a task freezes or stalls, the `UnblockerAgent` extracts live execution logs, scrubs sensitive credentials (`log_tailer.go`), and diagnoses the stall:
   - **0-Token Fast-Path Regex Pre-Filter (`unblocker_fastpath.go`)**: Pre-filters routine CLI hangs (interactive `y/n` prompts, port binding collisions, test watch spinners) in **< 5ms** with **0 LLM token overhead**.
   - **10x Progressive Log Window Escalation**: Scales diagnostic scope dynamically based on stall count (Level 1: 50 lines $\rightarrow$ Level 2: 500 lines $\rightarrow$ Level 3: 5,000 lines, capped at 3 escalations before failing task).
   - **Stall Recovery Directives (`[STALL RECOVERY DIRECTIVE]`)**: Attaches recovery directives to task state upon reset and injects `[STALL RECOVERY DIRECTIVE]` into `Generator` and `Tester` prompts on retry attempts to prevent repeating the hanging command.
2. **Legacy Codebase Characterization & Stabilization Prompts**: When `noctifab` runs in a workspace containing existing code, `scanLegacyFiles` detects pre-existing source files and dynamically injects a `LEGACY CODEBASE STABILIZATION & REFACTORING MANDATE` into the Product Manager prompt. The PM automatically generates `roadmap/user-stories/US-001.md` titled `"Legacy Codebase Characterization & Stabilization"`, requiring unit/integration characterization tests before refactoring or feature additions. Planner, Generator, and Tester prompts dynamically adapt with characterization testing and surgical refactoring (`edit_file`, `apply_patch`) directives.
3. **Pre-Flight LLM Provider Capability Caching (`providerCapabilityCache`)**: Dynamically learns provider model parameter rejections (`temperature`, `max_tokens`, `response_format` for reasoning models like OpenAI O-series) upon the first HTTP 400 rejection. Caches capabilities per model in a thread-safe cache and automatically omits unsupported parameters on subsequent calls without error roundtrips.
4. **Intra-Turn Iterative Self-Healing**: Generator and Tester agents execute in a multi-turn feedback loop (up to **5 turns** per task). If verification tools like `run_tests` or `run_linter` fail, the orchestrator appends compiler, syntax, or test failure outputs directly back into the prompt context. The agent receives this output as direct feedback to repair the code dynamically in the next turn before finalizing its work.
5. **Watchdog Self-Repair (Inter-Turn)**: If a completed task fails the final verification gate, the orchestrator intercepts the failure and invokes a dedicated `WatchdogRepair` handler across three repair contexts:
   - **Timeout**: Fixes infinite loops, deadlock hangs, and thread leaks.
   - **Compile**: Solves syntax issues, missing imports, and compile failures.
   - **Test Logic**: Fixes assertion value mismatches and incorrect test expectations.
   The handler attempts up to **3 consecutive repairs** automatically.
6. **Dynamic Model Fallback Engine (Zero-Stall Resilience)**: If the configured LLM returns an error (rate limits HTTP 429, authentication/quota failure HTTP 401/402, or server error HTTP 5xx), `noctifab` automatically queries the provider's API endpoint (`GET /models` or `/v1/models`) **live** to discover accessible models. It applies custom provider-specific capacity ranking algorithms (`parse<Provider>Model`) to select and transparently fall back to the next highest-capacity model from that provider without interrupting dark factory execution.
7. **Parallel Prompt Compaction Engine (`context.compaction`)**: Compresses HTTP prompt payloads using `simple_english` (active voice, simplified vocabulary) or `caveman` (telegraphic Markdown compaction) modes. Parallelizes line block compaction across worker goroutines for inputs $> 20$ KB to reduce latency and token usage by 25%+ while preserving code blocks, JSON schemas, file paths, and technical invariants.
8. **Automatic Tool Formatting & Makefile Tab Normalization**: Dynamically converts space-indented recipe lines in `Makefile` and `*.mk` files into tab-indented (`\t`) lines during `write_file` and `edit_file` execution, maintaining build tool syntax invariants automatically.
9. **Safety Circuit Breakers & Token Ceilings**:
   - **`runtime.max_actions`**: Config value (default: `100`) that sets a ceiling on the total task execution loops. If the system exceeds this limit, the orchestrator aborts the story to protect the LLM token budget from infinite loops.
   - **`runtime.max_silent_stall_duration`**: Story-level livelock watchdog (default: `30m`). If no task makes progress or updates state within this window, the orchestrator fails remaining tasks and aborts the stalled story cleanly.
   - **`runtime.max_tokens_per_story` & `runtime.max_tokens_per_task`**: Hard budget token caps to guard against excessive token consumption per story or task.
   - **`runtime.max_tokens`**: Global token consumption ceiling across the entire execution run (default: `100000000` / 100M).
   - **`runtime.loops`**: Number of isolated execution loop passes per `noctifab start` run (default: `1`).
   - **`max_user_stories`**: Ceiling on Product Manager roadmap story generation (default: `5`).
   - **`runtime.max_duration`**: Story-level wall-clock timeout.
   - **`timeout_seconds`**: Configurable execution time limit for test runs (default: 5m), preventing premature timeouts on large project test suites.
10. **First-Class Generator Surgical Repair (`surgical_repair`)**: When task verification fails due to a compilation error or test assertion failure, the orchestrator immediately triggers a single-turn surgical repair pass (`surgical_repair` prompt template) without context-gathering overhead to apply minimal, localized fixes without rewriting working code.
11. **Zero-Token Pre-Commit Auto-Formatting**: Executes configured language formatters (`sandbox.formatter_command`, e.g. `go fmt ./...`, `npx prettier --write .`, `ruff format .`) automatically before staging Git commits, ensuring clean formatting without burning LLM turns.
12. **Anthropic Adaptive Parameter Retry**: Dynamically detects Anthropic HTTP 400 Bad Request parameter deprecations and constraints (`temperature` deprecations on newer models, excessive `max_tokens` clamped to 4096, and unsupported `cache_control` headers), self-correcting and retrying automatically. Fully supports Claude 5 series (`claude-sonnet-5`, `claude-opus-5`, `claude-haiku-5`).
13. **Structured Roadmap Directories & Task Serialization (`roadmap/tasks/`)**:
   - Organizes user story specifications into `roadmap/user-stories/` using title slug filenames (`US-XXX-title-slug.md`).
   - Automatically serializes task models into markdown files in `roadmap/tasks/` (`US-XXX-TASK-YYY-slug.md`) for full auditability.
14. **User Story DAG Scheduler (`depends_on` Cross-Story Parallelism)**:
   - Parses `depends_on` dependencies from User Story YAML frontmatter.
   - Concurrently executes all unblocked user stories across worker slots, dynamically unblocking dependent stories as prerequisites complete.
15. **Incremental Story Resume (`noctifab resume` & `noctifab start --resume`)**: Enables resuming interrupted or partially completed project executions, skipping completed stories (`StorySuccess`) and picking up execution at the first incomplete story.
16. **Zero-Stall Resilient Architecture (Optimistic Merging, 5-Tier Merge Engine, Tool Degradation & Post-Merge Repair)**:
   - **Dynamic Tool Degradation & Eviction**: Auto-detects missing binaries (e.g. exit code 127, `pytest: command not found`). Evicts missing tools and transitions validation to degraded mode (`[Validation Degraded]`), preventing endless retry stalls.
   - **Optimistic Task Merging**: Optimistically merges completed task code into `integrationBranch` with warnings (`MERGED_WITH_WARNINGS`) even when tests fail, unblocking dependent tasks.
   - **5-Tier Merge Engine**: Features non-interactive merge, deterministic conflict marker stripping, **Whole-File Dual Reimplementation by the Generator Agent** (prompting the LLM to rewrite the entire file combining all features from both branches), optimistic line union merge, and direct diff overlay.
   - **Stale Git Lock Sanitizer**: Automatically cleans stale `.git/index.lock` and worktree lock files older than 5 seconds.
   - **Post-Merge Integration Repair Agent**: Runs an automated repair phase on the consolidated `integrationBranch` after all tasks finish to fix cross-task discrepancies and broken tests with a strict 2-turn budget.
17. **Configurable Task Execution Order (`agents.task_execution_order`)**: Configurable verification sequence mode (`"generator_first"` default vs `"tester_first"` TDD mode). In `tester_first` mode, Noctifab automatically pre-seeds minimal compilation stub files (`ensureTargetStubFilesExist`) for missing target files so Turn 1 test compilation succeeds cleanly.
18. **Multi-Pass Product Manager Architecture (`agents.product_manager.passes`)**: Multi-pass specification decomposition (`passes: 1` Fast mode, `passes: 2` Standard mode, `passes: 3` Deep contract & dependency audit mode).
19. **Black-Box Contract Scenario Prompt Injection**: Machine-readable contract expectations parsed from story `noctifab-contract` JSON blocks are formatted into a prominent `### BLACK-BOX CONTRACT EXPECTATIONS (NON-NEGOTIABLE)` prompt context section and injected directly into Generator and Tester agent prompts.
20. **Pre-Flight Diagnostics & LLM Provider Ping**: Validates Git CLI availability, state database connectivity, LLM provider `/models` endpoint reachability, and sandbox mode before launching the orchestrator.

---

## ⚡ Dark Factory Acceleration Engine (5x–10x Speedup)

`noctifab` incorporates an end-to-end pipelined acceleration engine delivering **5x–10x faster dark factory throughput**:

1. **Story-Level Parallelism & DAG Scheduling**: Executes independent user stories concurrently (`agents.orchestrator.number > 1`), branching orthogonal tracks from the walking skeleton (`US-001`) with minimal inter-story dependencies to dramatically reduce greenfield project lead times.
2. **Parallel DAG Task Worker Pools**: Executes independent tasks concurrently (`scheduler.max_parallel_workers > 1`), assigning each task an isolated Git worktree (`.noctifab/worktrees/task-<id>`) and merging completed worker branches asynchronously via a serialized rebase queue (`pkg/services/rebase_queue.go`).
3. **Batched Multi-File Creation (`write_files`)**: Enables agents to atomically create or overwrite multiple workspace files in a single LLM turn (via `{"files": {"path/a": "...", "path/b": "..."}}`), eliminating 70% of single-file tool roundtrip latency during scaffolding.
4. **Per-Agent Adaptive Complexity Routing**: Dynamically routes agent tasks to optimal model tiers (`fast_tier` for scaffolding, `standard_tier` for domain features, `heavy_tier` for complex algorithms and repair turns) to minimize reasoning latency without sacrificing capability.
5. **Walking Skeleton Slicing Priority (`US-001`)**: Enforces that the initial user story delivers a minimal compiling and test-passing vertical slice, guaranteeing a working runnable baseline within the first 2 minutes.
6. **Parallel 3x Majority-Vote Test Validation**: Dispatches 3 test validation runs concurrently using Go goroutines, reducing verification latency from ~15s to ~3s.
7. **Unified Diff Multi-File Patching (`apply_patch`)**: Enables agents to apply multi-file unified diff patches (`diff -u` / Git format) in a single turn with fuzzy matching and sandbox security validation.
8. **Spec-Level Deterministic Mock Clocks**: Enforces mock clock invariants (`Store(clock=FakeClock())`) at the Product Manager specification layer (`US-xxx.md`), ensuring time-dependent tests pass deterministically on the first attempt.
9. **Aggressive Suffix-Only Prompt Pruning**: Truncates prompt history on retries to preserve LLM KV cache prefixes while providing exact failure tracebacks.
10. **Speculative Next-Task Prefetching**: Prefetches file contexts for candidate downstream tasks while current task verification executes in parallel.

### Autonomous Agent Roles & Relationship
To prevent "evaluation gaming" (where code generators approve their own buggy code) and break deadlock traps, `noctifab` partitions cognitive execution into specialized, cooperative agent roles:

1. **Product Manager Agent**: Decomposes high-level project specifications (`SPEC.md`) into granular, verifiable User Stories (`roadmap/user-stories/US-xxx.md`) with explicit public API contracts and mock clock mandates.
2. **Planner Agent**: Decomposes user stories into a topological Directed Acyclic Graph (DAG) of task models (`roadmap/tasks/`), identifying parallelization opportunities.
3. **Tester Agent**: Dedicated test-writing agent that writes and refactors unit, integration, and behavioral tests based on story specifications before and during code generation.
4. **Generator Agent**: Sandbox-restricted worker executing in task-specific Git worktrees. Writes and refactors production code to satisfy the test suites.
5. **Resolver Agent**: Dedicated agent for resolving complex three-way Git rebase and merge conflicts across parallel worker branches.
6. **Unblocker Agent (Sentry / Monitor)**: Independent background daemon that continuously scans pipeline state, captures live logs, applies 0-token regex fast-path fixes for interactive CLI hangs, and injects diagnostic recovery directives.
7. **Acceptance Auditor Agent**: Whole-project specification auditor that evaluates the completed codebase, CLI entrypoints, and wire protocol surface against the root `SPEC.md` prior to release. Halts PR creation if critical command, CLI, or interface omissions are detected.
8. **Last-Resort Agent (Chief Surgeon / Solver)**: Sovereign unblocker summoned when tasks encounter intractable blockers (exhausted retry budgets, contradictory or shallow specifications, missing sandbox toolchains, or post-merge integration collapses). Operates with sovereign authority across code, tests, and specs to deliver clean-compiling, test-passing builds under the 4-Tier Compromise Hierarchy.

**Inter-Agent Relationship & Dynamic Escalation**:
- **Generator $\leftrightarrow$ Tester (Separation of Concerns)**: Keeps production code and test assertions isolated so tests remain an objective quality gate. If the Generator detects a bug in the test definitions, it uses `request_test_fix` to coordinate changes.
- **Auditor Release Gate (Whole-Project Verification)**: Operates after task completion and before PR creation, cross-referencing all implemented commands and story contracts against `SPEC.md` to prevent partial feature releases.
- **Unblocker $\rightarrow$ Last-Resort (Two-Tier Deadlock Defense)**: The Unblocker acts as an observant, zero-risk sentry that resets tasks and injects guidance without modifying code. When a task reaches 4 stall cycles or exhausts its retry budget, the orchestrator escalates the issue to the Last-Resort Agent to perform deep, multi-file sovereign surgery.

---

## 📊 Structured Real-Time Execution Reports & Telemetry Logs

`noctifab` provides a native, structured execution reporting and telemetry subsystem that records fine-grained events during autonomous runs and synthesizes a deterministic, human-and-machine-readable Markdown **Execution Report** (`<TIMESTAMP>_<PROJECT>.md`) without requiring external tools to parse raw logs.

### Core Concepts & Real-Time Live Checkpointing

- **`execution_log` (Event Stream & Telemetry)**:
  A concurrency-safe stream of structured timeline events (`ExecutionEvent` / `ExecutionLog`) captured during orchestrator, planner, generator, tester, and unblocker agent activities (storing timestamps, agent roles, phase transitions, task attempts, millisecond duration measurements, token usage, errors, and retries).
- **`execution_report` (`<TIMESTAMP>_<PROJECT>.md`)**:
  The synthesized Markdown report artifact generated continuously during execution.
  * **Real-Time Live Updates**: Flushed atomically to disk every **5 seconds** (and instantly on phase/story transitions), allowing developers to watch live progress, active worker spans, token counts, and task attempt states in real time without polling.
  * **Structured Sections**: Includes executive summaries, live status tables, active agent performance spans (omitting zero units like `17s 116ms`), phase execution windows, human-readable bottleneck diagnoses, error breakdown tables, task titles linked to parent story IDs, and deliverables.

### Configuration

Enable execution reporting in your project's `.noctifab/config.yaml`:

```yaml
config_version: "2.0"
execution_report: ".noctifab/reports/execution_report.md"
```

Report paths are resolved strictly within workspace boundaries, timestamped with the canonical project folder name (`YYYYMMDD_HHMMSS_<project>.md`), and written atomically using exclusive temporary files with `0600` permissions. For detailed documentation, see [docs/execution_report.md](file:///Users/diegoj/repos/noctifab/docs/execution_report.md).

---

### 3-Tier Token Accountability System

`noctifab` tracks, persists, and reports LLM token metrics across three distinct hierarchical tiers:

1. **Tier 1 (Global State Metadata)**: Tracks total prompt (`TotalInputTokens`) and completion (`TotalOutputTokens`) tokens across the entire dark factory run lifetime.
2. **Tier 2 (User Story Level)**: Tracks accumulated input and output tokens for individual user story milestones (`US-001`, `US-002`, etc.) and renders them in execution reports as the `### Story Token Breakdown` table.
3. **Tier 3 (Task & Agent Worker Level)**: Tracks input/output tokens per task attempt and agent worker goroutine.

Token extraction is integrated natively across OpenAI (stream options `IncludeUsage`), Anthropic (`cache_read_input_tokens`, `input_tokens`, `output_tokens`), Gemini (`usageMetadata`), and OpenOTel GenAI telemetry attributes (`gen_ai.usage.input_tokens`, `gen_ai.usage.output_tokens`). For detailed documentation, see [docs/token_accountability.md](file:///Users/diegoj/repos/noctifab/docs/token_accountability.md).

---

### Agent Architecture Modes & Team Configuration (`agents:`)

`noctifab` supports unified configuration for its implemented roles under the **`agents:`** section in `.noctifab/config.yaml`. QA is retained as an experimental capability and is disabled by default.

```yaml
agents:
  architecture: "code_first" # Options: code_first (cfv), single_pass (spe), breadth_first (bfg)

  orchestrator:
    number: 1      # Task orchestration & state sync (default: 1)
    iterations: 2

  product_manager:
    number: 1      # Spec hardening & user story generation (default: 1)
    iterations: 2

  planner:
    number: 1      # Task DAG decomposition (default: 1)
    iterations: 2

  generators:
    number: 3      # Number of parallel Generator agents (default: 3)
    iterations: 5  # Maximum LLM repair turns per task (default: 5)

  testers:
    number: 2      # Number of parallel Tester agents (default: 2)
    iterations: 3  # Maximum LLM turns per task (default: 3)

  qa:
    enabled: false # Experimental; no QA runtime is active in Phase 0
    iterations: 1

  unblocker:
    number: 1      # Autonomous pipeline stall detection & task re-dispatch (default: 1)
    iterations: 2

  last_resort:
    enabled: true  # Sovereign unblocker for deadlocked tasks across code, tests, and specs
    temperature: 0.1
    max_turns: 2
    timeout: 180s
```

1. **`code_first` (`cfv`)** (Default): Generator implements code first, followed by independent Tester verification turns.
2. **`single_pass` (`spe`)**: Fast-path execution where a single Generator Agent pass co-generates implementation code and tests in one turn.
3. **`breadth_first` (`bfg`)**: Iterative ~80% happy-path generation across all user stories first, followed by benevolent judges refining edge cases and enforcing zero regressions.
4. **Explicit Quality Tasks**: Architecture, security, performance, documentation, and infrastructure concerns are ordinary planner tasks verified by deterministic validators. They are not separate agent roles.

---

## 🔁 Multi-Loop Dark Factory Orchestration & Quality Architecture

`noctifab` utilizes a multi-loop execution architecture to achieve high-resilience autonomous software delivery. In unattended dark factory runs, complex systems cannot always be fully implemented in a single linear pass. The multi-loop engine provides autonomous self-healing, progressive convergence, whole-workspace regression guarding, and strict quality verification.

```mermaid
flowchart TD
    A[Start Multi-Loop Run: Loop 1..N] --> B[Execute 100% Discovered Stories in Backlog]
    B --> C{Story Execution Complete?}
    C -->|Task Failed / Timeout| D[Record Diagnostics -> Continue Backlog Pass]
    C -->|All Tasks Succeeded| E[Stage 2: Definition of Done & Whole-Workspace Regression Audit]
    E --> F{DoD Satisfied & Zero Workspace Regressions?}
    F -->|No: Missing Features / Regressions| G{Remediations Remaining?}
    G -->|Yes| H[Refine Story MD & Queue qa-remediation Task]
    H --> B
    G -->|No| D
    F -->|Yes: 100% Verified| I[Mark StorySuccess & Finalize Story]
    D --> J{All Stories in Backlog Succeeded?}
    I --> J
    J -->|Yes| K[✨ Early Exit: 100% Backlog Converged]
    J -->|No: Incomplete Stories Remain| L{Loop Stagnation Detected?}
    L -->|Yes: 0 Diff & Identical Errors| M[⚠️ Stagnation Circuit Breaker: Early Terminate]
    L -->|No: Loops Remaining| N[Advance to Next Loop Pass: Loop k+1]
    N --> B
```

### Core Invariants

1. **Backlog Iteration Guarantee**: Every loop pass attempts 100% of discovered user stories in `roadmap/user-stories/`. A failure in an intermediate story records diagnostic telemetry but does not halt the loop, allowing downstream stories to proceed.
2. **Two-Stage Story Verification**: A user story is only marked `StorySuccess` when:
   - **Stage 1 (Task Integrity)**: 100% of planned tasks achieve `TaskSuccess`.
   - **Stage 2 (DoD & Behavioral Review)**: The `StoryQAAuditor` verifies that the generated codebase satisfies all Definition of Done (DoD) criteria and passes both E2E test suites and whole-workspace regression checks.
3. **Automated Story Refinement**: If a story is incomplete or missing DoD features, Noctifab automatically enriches `roadmap/user-stories/<story>.md` with a `## Refined Acceptance Criteria & Missing Requirements` section and queues a targeted remediation task for the worker pool.
4. **Whole-Workspace Regression Guarding**: In Loop $k \ge 2$, before finalizing any story, the test validator executes the entire repository's test suite (`go test ./...`, `pytest`, `cargo test`, `npm test`, or `make test`) to guarantee changes in shared packages didn't break earlier modules.
5. **Loop Stagnation Circuit Breaker**: If Loop $k+1$ generates 0 codebase mutations and repeats identical failure signatures as Loop $k$, the orchestrator detects stagnation and terminates early to prevent token waste.
6. **Early Convergence Exit**: If all user stories in the backlog achieve verified `StorySuccess` on Loop $k$, Noctifab completes immediately without burning tokens on remaining loops.

### Configuration & CLI Usage

In `.noctifab/config.yaml`:

```yaml
runtime:
  loop:
    count: 3                # Number of iteration loops (default: 1)
  max_tokens: 500000        # Global token consumption boundary
  max_duration: "10m"       # Total execution time limit

agents:
  product_manager:
    user_stories:
      max_count: 5          # Maximum user stories in roadmap backlog (default: 5)
      complexity:
        min: 15             # Minimum target complexity units per story
        max: 35             # Maximum target complexity units per story
    passes: 2               # PM multi-pass refinement passes
```

Override the loop count dynamically from the CLI:

```bash
# Execute with 3 iterative self-healing loops
noctifab start . --loops 3

# Resume from first incomplete story with 2 loops
noctifab resume . -L 2
```

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
./dist/noctifab init

# 2. Validate configurations
./dist/noctifab validate

# 3. Start planning and autonomous execution for a target directory
./dist/noctifab start ./my-project
```

---

## Interactive Mode

`noctifab` provides an interactive REPL shell allowing operators to issue commands, enqueue feature story specifications, monitor dark factory execution in real time, and resolve clarification prompts on the fly.

![Interactive Mode](assets/interactive-mode.png)

To launch the interactive session:

```bash
noctifab start
```

Key features of Interactive Mode:
- **Story Dispatching**: Enqueue individual user stories (`start roadmap/user-stories/US-001.md`) or an entire folder of specifications (`start roadmap/user-stories/`).
- **Real-Time Monitoring**: Observe autonomous DAG task progress, generator/tester execution turns, and quality gate results.
- **Clarification Resolution**: Answer disambiguation questions raised by Planner/Generator agents to unblock autonomous execution.

---

## Command Reference

- **`init`**: Initializes workspace folder structure (`.noctifab/`), SQLite DB, default config, and security permission profiles. Use `--profile <preset>` (`ollama-qwen`, `ollama-deepseek`, `vllm-local`, `openai-compat`) for 1-click local LLM configuration, `--spec <prompt>` to bootstrap interactive spec generation, or `-i` / `--interactive` for interactive wizard.
- **`spec`**: Interactively creates, refines, and audits software specifications (`SPEC.md`). Features multi-agent team consensus review (Architect, QA, Tester, PM), version snapshotting (`SPEC.v1.md`, `SPEC.v2.md`), and instant zero-token time-travel rollback (`undo`, `redo`, `history`, `checkout <v>`). Supports subcommands `noctifab spec [prompt]`, `noctifab spec new`, `noctifab spec refine`, and `noctifab spec audit`.
- **`demo`**: Runs an instant, 2-minute, zero-config autonomous sandbox using deterministic offline mock replay (supports `--project`, `--offline`, `--speed`, `--no-cleanup`).
- **`dashboard`**: Launches the real-time progress dashboard (terminal TUI by default, or visual web browser via `-w` / `--web`). Supports `--web-open` (auto-opens in default browser), `--port`, `--host`, and `--readonly`. Web Mission Control features:
  - **Task & Story DoD Inspector Modal**: Click any task or story card to view its full description, Definition of Done (DoD) verification checklist, target files, and failure stack traces with 1-click steering shortcuts.
  - **Failure Diagnostics & Quick-Steer Hints**: Automatically highlights root-cause compiler/test errors when health is failing and generates corrective steering directives with 1 click.
  - **Changed Files Explorer**: Browse and inspect syntax-highlighted code for all files modified by agent tasks.
  - **Multi-Story Roadmap Timeline**: Horizontal story scrubber to inspect task DAGs across past, active, and upcoming user stories.
  - **Audio Cues & Floating Toasts**: Native Web Audio chimes for story completion and alerts, paired with real-time floating toast notifications.
  - **Telemetry & Token Metrics Modal**: Detailed breakdown of token consumption by agent role (`GENERATOR`, `TESTER`, `PLANNER`, `QA`) and tool action distributions.
  - **Terminal Drawer Search & Filters**: Live search input and tag chips (`All`, `Errors`, `Tools`, `Tests`) for console output.
  - **Execution Report Viewer**: 1-click modal to inspect or download the latest Markdown execution report.
  - **Visual Spec Studio**: Side-by-side specification editor with time-travel revision scrubber, multi-model consensus review, and decomposed user story roadmap with completion meters.
- **`steer`**: Injects a mid-flight human-in-the-loop steering directive into the active task (`noctifab steer "Use PostgreSQL instead of SQLite"`).
- **`order`**: Enqueues an ad-hoc user story / feature prompt order into the autonomous execution queue (`noctifab order "Add JWT authentication middleware"`).
- **`validate`**: Checks configuration files, databases, and sandbox settings.
- **`start`**: Plans and executes a software specification end-to-end for a target directory (defaults to current directory `.`). Auto-generates user stories in `roadmap/user-stories/` from `SPEC.md` if missing, and executes stories concurrently via the Story DAG Scheduler. Pass `-w` / `--web` to launch the concurrent live Visual Web Dashboard, `--web-open` to auto-open in browser, `-i` for interactive TUI, `--standby` for persistent always-on dark factory mode, and `--resume` to skip completed stories.
- **`resume`**: Resumes execution of an interrupted or partially completed workspace, skipping already completed user stories (`StorySuccess`) and picking up execution at the first incomplete story (supports `-w` / `--web` and `--web-open` for concurrent web dashboard).
- **`serve`**: Runs the long-running headless orchestrator daemon loop, polling and executing tasks in the background with local loopback REST API endpoints.
- **`prompts`**: Inspects, customizes, initializes, and validates per-agent prompt templates (`list`, `show`, `init`, `validate`). Supports all 23 prompt templates across 7 agent roles.
- **`stop`**: Gracefully stops the background daemon process and saves state.
- **`clean`**: Resets all noctifab state (wipes the database, removes PID and log files). Use `--dry-run` to preview, `--yes` / `-y` to skip confirmation.
- **`maintenance`**: Cleans up completed branches, orphaned worktrees, and runs database schema migrations.
- **`version`**: Displays Noctifab release version, Git commit hash, and commit date. Supports `--short` / `-s`, `--verbose` / `-v`, and `--json`. Also accessible via `noctifab --version`.

---

## Secrets Management

Credentials such as API keys and VCS tokens must **not** be stored as literal values in `config.yaml`. Use the `secret:` reference syntax to load them from a gitignored `secrets.yaml` file instead:

- **Global Home Directory Secrets (`$HOME/.noctifab/secrets.yaml`)**: Stores default baseline API credentials for all Noctifab projects on your host machine.
- **Project-Level Overlay (`.noctifab/secrets.yaml`)**: Places project-specific credentials that overlay and take precedence over global home secrets.

```yaml
# $HOME/.noctifab/secrets.yaml or .noctifab/secrets.yaml  (gitignored — never commit)
GEMINI_API_KEY: "AIzaSy..."
GITHUB_TOKEN:   "github_pat_..."
```

```yaml
# .noctifab/config.yaml  (safe to commit)
llm:
  api_key: "secret:GEMINI_API_KEY"
vcs:
  token:   "secret:GITHUB_TOKEN"
```

`noctifab init` automatically adds `secrets.yaml` to `.noctifab/.gitignore`. For full details, supported fields, CI/CD patterns, and the security checklist see **[docs/secrets.md](docs/secrets.md)**.

### Supported LLM Providers & API Keys

`noctifab` supports all major cloud and open-weights LLM providers with automatic model hierarchy fallback. Provide your API key via `secrets.yaml` or environment variables:

| Provider | `provider` Key | Environment Variable(s) | Base URL |
|---|---|---|---|
| **OpenAI** | `openai` | `OPENAI_API_KEY` | `https://api.openai.com/v1` |
| **Anthropic** | `anthropic` | `ANTHROPIC_API_KEY` | `https://api.anthropic.com/v1` |
| **Gemini** | `gemini` | `GEMINI_API_KEY` | `https://generativelanguage.googleapis.com/v1beta` |
| **OpenCode** | `opencode` | `OPENCODE_API_KEY` | `https://opencode.ai/api/v1` |
| **Kimi (Moonshot AI)** | `kimi`, `moonshot` | `KIMI_API_KEY`, `MOONSHOT_API_KEY` | `https://api.moonshot.ai/v1` |
| **Groq** | `groq` | `GROQ_API_KEY` | `https://api.groq.com/openai/v1` |
| **OpenRouter** | `openrouter` | `OPENROUTER_API_KEY` | `https://openrouter.ai/api/v1` |
| **Qwen (DashScope)** | `qwen`, `dashscope` | `DASHSCOPE_API_KEY`, `QWEN_API_KEY` | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| **Together AI** | `together` | `TOGETHER_API_KEY` | `https://api.together.xyz/v1` |
| **Meta (Llama)** | `llama`, `meta` | `LLAMA_API_KEY`, `META_API_KEY` | `https://api.together.xyz/v1` |
| **HuggingFace** | `huggingface` | `HUGGINGFACE_API_KEY` | `https://api-inference.huggingface.co/v1` |
| **Mistral** | `mistral` | `MISTRAL_API_KEY` | `https://api.mistral.ai/v1` |
| **DeepSeek** | `deepseek` | `DEEPSEEK_API_KEY` | `https://api.deepseek.com/v1` |
| **Nous Hermes** | `hermes` | `HERMES_API_KEY` | `https://api.together.xyz/v1` |
| **Ollama (Local)** | `ollama` | `OLLAMA_API_KEY` *(optional)* | `https://ollama.com/v1` |
| **xAI (Grok)** | `xai`, `grok` | `XAI_API_KEY`, `GROK_API_KEY` | `https://api.x.ai/v1` |
| **Perplexity AI** | `perplexity` | `PERPLEXITY_API_KEY` | `https://api.perplexity.ai` |
| **Fireworks AI** | `fireworks` | `FIREWORKS_API_KEY` | `https://api.fireworks.ai/inference/v1` |
| **SambaNova** | `sambanova` | `SAMBANOVA_API_KEY` | `https://api.sambanova.ai/v1` |
| **Cohere** | `cohere` | `COHERE_API_KEY`, `CO_API_KEY` | `https://api.cohere.com/v2` |

---

## Security & Permission Profiles

To ensure secure and controlled agent execution, `noctifab` employs a profile-based Role-Based Access Control (RBAC) and security sandboxing system.

Every active agent role (such as `orchestrator`, `planner`, `generator`, or `tester`) is constrained by a security profile. These profiles are defined under the `profiles:` section inside `.noctifab/config.yaml`. If no profile is explicitly defined for a role, the orchestrator automatically uses its built-in default profile configuration.

### Security Sandbox Policies

1. **Tool Whitelisting (`allowed_tools`)**: Restricts the exact tools an agent is authorized to invoke (e.g., `read_file`, `write_file`, `edit_file`, `run_tests`, `run_linter`). By default, dangerous system commands and Git mutation actions (`git_checkout`, `git_commit`, `git_push`, `docker_action`) are strictly reserved for the privileged `orchestrator` profile.
2. **Command Whitelisting (`allowed_commands`)**: Restricts which shell execution binaries are allowed to run under sandbox execution. For example, `tester` and `generator` profiles are restricted to language-specific runtimes (e.g., `go`, `npm`, `pytest`, `make`, `python`), preventing command injection or host shell execution escapes.
3. **Path Jail Protection**: The validator dynamically enforces path checks preventing directory traversal attacks. Any file read or write tool parameters that resolve outside the workspace root path trigger an automatic sandbox boundary violation.
4. **Target Path Exclusion**: Agents are forbidden from reading, writing, or accessing sensitive testing framework directories (specifically `tests/holdout` and `holdout` directories) to prevent gaming the evaluation process.
5. **Branch Protection**: Direct git checkouts, commits, or pushes on protected base branches (like `main` or `master`) are rejected by the Policy Validator.

### Example Profiles Configuration in `.noctifab/config.yaml`

```yaml
profiles:
  generator:
    allowed_tools:
      - "read_file"
      - "write_file"
      - "edit_file"
      - "list_directory"
      - "find_files"
      - "grep_search"
      - "run_tests"
      - "run_linter"
      - "noop"
```

### Context Slicing & AST Indexing (`context.mode`)

Control how workspace source files are formatted into LLM prompt contexts to optimize speed and token consumption:

* **`full`** (default): Sends complete source file contents. Maximum context, best for small projects.
* **`diff_window`**: Extracts modified git diff lines and error stack traces (+/- 15 context lines), cutting token usage by ~80%.
* **`tree_sitter`**: Uses universal AST parsing to extract function signatures, struct/class definitions, and symbol maps.

```yaml
context:
  mode: "full"            # Options: "full" (default), "diff_window", "tree_sitter"
  diff_window_lines: 15   # Surrounding context lines for diff_window mode
```

### Workspace Inspection Caching (`workspace_cache.enabled`)

Optimize multi-turn agent turns by deduplicating read-only filesystem reads (`list_directory`, `read_file`, `find_files`, `grep_search`) and diagnostic test/linter runs during an agent's execution loop (top-level key `workspace_cache:`, with backward-compatible fallback for `agents.workspace_cache`):

```yaml
workspace_cache:
  enabled: true        # In-memory caching of workspace filesystem reads until a file write occurs (default: true)
```

---

---

## LLM Multi-Provider Prioritization & Per-Agent Routing

`noctifab` supports declaring a named registry of LLM providers (`llm.providers`), setting a global failover priority list (`llm.priority`), and overriding provider priority chains per agent role (`roles.<agent>.providers`).

### 🌟 Multi-Model Peer Review (Generate with Model A, Test with Model B, Audit with Model C)

Assigning specialized AI models to different execution phases is an essential best practice for autonomous development:
1. **Eliminate Confirmation Bias:** If the same model writes code, writes unit tests, and reviews its own PR, it will repeat its own logical blind spots. Multi-model routing creates an independent peer-review pipeline.
2. **Model Specialization:** Use fast syntax models for code generation, reasoning heavyweights for test design, and premier analytical models for code review and security audits.

```yaml
config_version: "2.0"

# 1. Named LLM Provider Registry & Global Failover
llm:
  priority:
    - "deepseek-coder"
    - "openai-primary"
    - "anthropic-reviewer"

  providers:
    - name: "deepseek-coder"
      provider: "deepseek"
      api_keys: "DEEPSEEK_API_KEY"
      model: "deepseek-coder"

    - name: "openai-primary"
      provider: "openai"
      api_keys: "OPENAI_API_KEY"
      model: "gpt-4o"

    - name: "anthropic-reviewer"
      provider: "anthropic"
      api_keys: "ANTHROPIC_API_KEY"
      model: "claude-3-5-sonnet-latest"

# 2. Assign Specialized Models per Agent Phase directly inside agents:
agents:
  generators:
    number: 4
    iterations: 5
    providers:
      - name: "deepseek-coder"
      - name: "openai-primary"

  testers:
    number: 2
    iterations: 3
    providers:
      - name: "openai-primary"
      - name: "anthropic-reviewer"

  qa:
    enabled: false
    iterations: 1
    providers:
      - name: "anthropic-reviewer"
      - name: "openai-primary"
```

> [!TIP]
> **Dynamic Version-Agnostic Mode:** You can omit specific model version strings (`model: ""`) from both providers and roles! `noctifab` will query each provider's `/models` API endpoint at runtime, automatically route to the highest-capacity flagship model for that provider (e.g. `openai` $\rightarrow$ flagship, `anthropic` $\rightarrow$ flagship, `deepseek` $\rightarrow$ flagship coder), and step down through lower model tiers if rate limits occur.

---

## LLM Providers

`noctifab` supports multiple LLM providers via a pluggable `llm.ProviderClient` interface. The active provider, model, and API key are set in `.noctifab/config.yaml`.

### Resilience Features

All providers benefit from the same resilience layer automatically:

* **Automatic retry with backoff** – transient errors (HTTP 5xx, network timeouts) are retried up to 3 times with exponential back-off.
* **Rate-limit awareness (HTTP 429)** – when a `429 Too Many Requests` response is received, `noctifab` warns the user, parses the provider's `retryDelay` field from the response body, and sleeps for exactly that duration before retrying.
* **Automatic model fallback** – if the chosen model is unavailable, `noctifab` first queries the provider for its live model list and falls back to the next smaller model in the static hierarchy below. The fallback continues down the chain until a working model is found or all options are exhausted.

### Provider Configuration Reference

#### Google Gemini

```yaml
# .noctifab/config.yaml
llm:
  provider: gemini
  model: gemini-3.6-pro          # fallback chain: → gemini-3.6-flash
  api_key: "secret:GEMINI_API_KEY"
  max_timeout: 60s               # Overall request hard timeout
  idle_timeout: 15s              # Socket stream inactivity timeout before failover
  streaming: true                # Enable HTTP SSE token streaming (default: true)
```

```yaml
# .noctifab/secrets.yaml
GEMINI_API_KEY: "AIzaSy..."
```

#### OpenAI

```yaml
llm:
  provider: openai
  model: gpt-4o                  # fallback chain: → gpt-4o-mini
  api_key: "secret:OPENAI_API_KEY"
```

```yaml
OPENAI_API_KEY: "sk-..."
```

#### Anthropic (Claude)

```yaml
llm:
  provider: anthropic
  model: claude-sonnet-5          # fallback chain: claude-sonnet-5 → claude-3-5-sonnet-latest → claude-3-5-haiku-latest
  api_key: "secret:ANTHROPIC_API_KEY"
```

```yaml
ANTHROPIC_API_KEY: "sk-ant-..."
```

#### Mistral AI

```yaml
llm:
  provider: mistral
  model: mistral-large-latest    # fallback chain: → mistral-medium-latest → mistral-small-latest → open-mistral-7b
  api_key: "secret:MISTRAL_API_KEY"
```

```yaml
MISTRAL_API_KEY: "..."
```

#### DeepSeek

```yaml
llm:
  provider: deepseek
  model: deepseek-coder          # fallback chain: → deepseek-chat
  api_key: "secret:DEEPSEEK_API_KEY"
```

```yaml
DEEPSEEK_API_KEY: "..."
```

#### Hermes (Nous Research via Hugging Face)

```yaml
llm:
  provider: hermes
  model: hermes-3-llama-3.1-405b  # fallback chain: → hermes-3-llama-3.1-70b → hermes-3-llama-3.1-8b
  api_key: "secret:HUGGINGFACE_API_KEY"
```

```yaml
HUGGINGFACE_API_KEY: "hf_..."
```

#### Ollama (local / self-hosted)

```yaml
llm:
  provider: ollama
  model: llama3.1                # any model pulled locally via `ollama pull`
  url: "http://localhost:11434"  # optional: override if running on a different host/port
  api_key: ""                    # not required for local Ollama instances
```

### Model Fallback Chains

| Provider | Model priority (high → low) |
|---|---|
| **Gemini** | `gemini-3.6-pro` → `gemini-3.6-flash` |
| **OpenAI** | `gpt-4o` → `gpt-4o-mini` |
| **Anthropic** | `claude-3-5-sonnet-latest` → `claude-3-5-haiku-latest` |
| **Mistral** | `mistral-large-latest` → `mistral-medium-latest` → `mistral-small-latest` → `open-mistral-7b` |
| **DeepSeek** | `deepseek-coder` → `deepseek-chat` |
| **Hermes** | `hermes-3-llama-3.1-405b` → `hermes-3-llama-3.1-70b` → `hermes-3-llama-3.1-8b` |
| **Ollama** | Queries the local `/api/tags` endpoint live; uses whatever models are pulled |




## Pull Request Configuration

In addition to the core LLM and VCS settings, `noctifab` supports automated PR management and branch integration:

```yaml
vcs:
  pull_request:
    auto_create: true    # Automatically create a PR from the task branch
    auto_merge: true     # Automatically merge the PR when CI checks pass
    auto_rebase: true    # Automatically rebase on base branch updates
    draft: false         # Create the PR as a draft
    assignees:           # GitHub usernames to auto-assign
      - "user1"
    labels:              # Labels to auto-apply to the PR
      - "autonomous"
```

> [!NOTE]
> **GitHub CLI (`gh`) Fallback**: If `GITHUB_TOKEN` is missing or fails authentication when creating or merging Pull Requests, `noctifab` automatically falls back to host `gh` CLI credentials (`gh auth token` or direct `gh pr create` / `gh pr merge` execution). If both fail, generated code is safely preserved in the local workspace.

For a full reference of all available settings and CLI flags, see the [SPEC.md](SPEC.md) and [docs/cli_usage.md](docs/cli_usage.md).

### Dependency Auto-Install

Set `sandbox.auto_install_deps: true` to automatically detect and install missing toolchain dependencies (e.g., `golangci-lint`, `pytest`, `cargo`). Configure supported package managers via `sandbox.package_managers`.

## Security & Self-Evolution

### SAST Security Gates

Static Application Security Testing (SAST) can be configured to run against generated code before PR creation:

```yaml
sast:
  enabled: true
  scanners: ["gosec"]       # "gosec" for Go, "bandit" for Python
  fail_on_severity: "high"  # Block on high, medium, or low severity
```

If SAST is enabled and a scanner finds issues meeting the severity threshold, the PR is blocked and the agent must fix them. See [SPEC.md](SPEC.md) for details.

### Hot-Reload

Noctifab can hot-reload its binary with zero downtime via a handoff file + health check protocol. See [SPEC.md §3.10](SPEC.md) for details.

### Intent Disambiguation

When the agent asks a clarification question, Noctifab can attempt to auto-answer it by analyzing git history, workspace files, and feature context — without blocking on human input. If the LLM's inferred answer is valid, the clarification is resolved automatically. Otherwise, the standard human clarification timeout applies.

## Target Scenarios & Examples

`noctifab` contains pre-configured example targets in the `examples/` folder to validate autonomous software implementation capabilities:
- **`url-shortener`**: An API server that generates short URLs, tracks analytics, and redirects clients.
- **`todo-cli`**: A command-line checklist manager with file persistence.
- **`weather-api`**: A service caching weather data and querying external providers.
- **`markdown-to-html`**: A utility that parses markdown files and generates styled HTML.
- **`task-scheduler`**: An in-memory scheduler executing functions at scheduled time intervals.
- **`frontpunch`**: A task worker demonstration featuring SOLID patterns and Sidekiq-compatible components.

---

## E2E Autonomy Validation

The `validation/` directory contains fully containerized, isolated end-to-end integration checks that run `noctifab` autonomously against real project specs — with **zero human intervention** — and verify that the correct source files are produced and all tests pass.

See [`validation/README.md`](validation/README.md) for the full project list, the tier-based effectiveness classification, setup, and credential details.

### Near-Instantaneous Iterations (Speedup Measures)
To optimize validation container runs for near-instantaneous development feedback loops, the platform includes:
- **Warm Compiler Caching:** Persistent mounts for Go modules/build caches and Cargo registries directly from the host.
- **Heuristic Context Preloading:** Bypasses context-gathering LLM calls for existing repository files, speeding up task initialization.
- **Zero-Delay Task Handoff:** Skips the polling delay sleep interval once a task completes, immediately scheduling the next task.


### Available Validation Projects

| Project | Language / Stack | Spec / User Story | What is Checked |
| :--- | :--- | :--- | :--- |
| **`echo`** | Go CLI | `SPEC.md` | `cmd/echo/main.go` created/modified and test suite passes |
| **`todo-cli`** | Go CLI | `roadmap/user-stories/US-001.md` | `cmd/todo/main.go` (or `main.go`) created/modified and test suite passes |
| **`wc`** | Rust CLI | `roadmap/user-stories/US-002.md` | `Cargo.toml` + `src/main.rs` created/modified and test suite passes |
| **`calculator`** | Ruby CLI | `SPEC.md` | `calculator.rb` (or under `lib/`) created/modified and test suite passes |
| **`fortune`** | C + SQLite | `SPEC.md` | `main.c` (or `Makefile`) created/modified and test suite passes |
| **`t4`** | C17 HTTP Server | `SPEC.md` | `Makefile` + `docker-compose.yml` + `src/t4.c` created/modified and test suite passes |
| **`pyedis`** | Python 3.14 (asyncio + RESP2/3) | `SPEC.md` | `src/main.py` (or `app/main.py`) + `pyproject.toml` created/modified and `unittest` passes |
| **`notebook`** | TypeScript + Fastify + PostgreSQL | `SPEC.md` | `src/index.ts` + `package.json` + `docker-compose.yml` created/modified and test suite passes |
| **`frontpunch`** | Python + Valkey | `roadmap/user-stories/US-001.md` | `frontpunch/worker.py` created/modified and test suite passes |
| **`djanban`** | Python 3.12 + Django 5.x | `SPEC.md` | Modernized Django models, views, and migrations created and test suite passes |
| **`stricc`** | Rust + LLVM 18 | `SPEC.md` | Safe C compiler with LLVM 18 backend created/modified and test suite passes |
| **`searchthedocs`** | Python 3.12 + FastAPI + Redis | `SPEC.md` | Documentation scraper + RAG vector search API created and test suite passes |
| **`auth-vault`** | Go 1.22+ | `SPEC.md` | OAuth2/OIDC Zero-Trust Authorization Server + PKI Vault created and test suite passes |
| **`buffonstream`** | Go 1.22+ (gRPC / Protobuf) | `SPEC.md` | Protobuf-Native Storage Engine & Real-Time Bi-Directional Streaming passes |
| **`jpacioli`** | Java 21 + Spring Boot 3.3+ + PostgreSQL | `SPEC.md` | Full Event Sourcing (ES) + CQRS Double-Entry Financial Ledger + JWT/RBAC passes |
| **`ocalogue`** | OCaml 5.x + Dune | `SPEC.md` | Datalog Deductive Logic Engine + Semi-Naive Fixpoint + Official Test Suite passes |

The `wc` project replicates the UNIX `wc` utility in Rust, enforcing SOLID/DDD architecture, `#![deny(unsafe_code)]`, and $O(1)$ streaming memory usage.

### Running Validation

Set your API key, then run via Make:

```bash
export GEMINI_API_KEY="your-actual-api-key"

# Run the default (frontpunch) validation
make validate

# Run a specific validation project
make validate PROJECT=todo-cli
make validate PROJECT=wc
make validate PROJECT=frontpunch
```

See [`validation/README.md`](validation/README.md) for full setup and credential details.

## Collaboration & Coding Standards

We welcome contributions! To maintain a highly clean and context-friendly repository, all code changes must adhere to the following directives:

1. **The 500-Line Limit**: No Go source code file (`.go`) may exceed **500 physical lines** (including comments and blank lines). Smaller, logically focused files prevent LLM context pollution.
2. **Dependency Injection**: Provide all clients, database connection objects, and configurations through struct constructors. Global state is strictly prohibited.
3. **100% Test Coverage**: Every package must be accompanied by unit tests (`_test.go` files). Ensure the test suite passes before submitting:
   ```bash
   go test -v ./pkg/... ./tests
   ```
4. **Code Quality and Lints**: Ensure that the code is formatted using `go fmt` and passes static analysis lints:
   ```bash
   docker run -t --rm -v $(pwd):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint run
   ```

---

## License

This project is licensed under the MIT License - see the [LICENSE](/LICENSE) file for details.
