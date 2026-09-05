# Architecture Overview

`noctifab` is designed around a decoupled, state-centric architecture designed to optimize context windows and guarantee predictable execution cycles.

---

## The Stateless Agent / Stateful Orchestrator Design

A core architectural principle of `noctifab` is the strict separation between the **Stateless LLM Agent** and the **Stateful Orchestrator Loop**:

- **Stateless LLM Agent**: The LLM agent has no persistent memory of previous loop cycles, actions, or terminal outputs. In each step, the orchestrator compiles the absolute latest system state, file indexes, and history logs into a structured prompt context.
- **Stateful Orchestrator**: The Go orchestrator manages state loading, updates, database transactions, lock acquisitions, policy checks, sandbox isolation, and VCS merges.

This pattern prevents context drift, keeps context usage highly token-efficient, and ensures that the system's runtime remains deterministic.

---

## Core Component Pipeline

The orchestrator operates in a continuous loop comprising five distinct phases:

```mermaid
flowchart TD
    subgraph Observe ["1. Observe Phase"]
        A[Load State from DB] --> B[Sync File Indices]
    end
    subgraph Schedule ["2. Schedule Phase"]
        B --> C[Compute Task DAG]
        C --> D[Retrieve Ready Tasks]
    end
    subgraph Dispatch ["3. Dispatch Phase"]
        D --> E[Spawn Worker Goroutines]
        E --> F[Checkout Worker Branches]
    end
    subgraph Execute ["4. Execute Phase"]
        F --> G[Construct Agent Prompt]
        G --> H[LLM Decides Action]
        H --> I{Policy Validator}
        I -- Blocked --> J[Mark Task Failed]
        I -- Allowed --> K[Execute Tool Sandbox]
        K --> L[Update State DB]
    end
    subgraph Evaluate ["5. Evaluate Phase"]
        L --> M[Run Test Validator]
        M -- Passed --> N[Push & Merge Pull Request]
        M -- Failed --> O[Retry / Keep In Progress]
    end
```

---

## Major Architecture Modules

### 1. The World Model (`pkg/domain/state.go`)
The `State` struct represents the entire system state, including tasks, files, clarification mailboxes, and action logs. It serves as the single source of truth and is stored in a relational database (SQLite for local setups, PostgreSQL for concurrent Level 4 production loops).

#### Incremental State Persistence & Append-Only Telemetry
To minimize database transaction duration, prevent SQLite write lock contention, and preserve full execution audit trails:
- **Targeted Incremental Upserts**: State saves avoid destructive `DELETE + INSERT` table rewrites. Tables such as `tasks`, `stories`, and `workspace_files` use targeted `ON CONFLICT DO UPDATE` upserts, selectively pruning only items removed from the domain model.
- **Append-Only Action Telemetry**: The `actions` table operates as an append-only log with unique `action_id` indexes. Saves execute `INSERT ... ON CONFLICT(action_id) DO NOTHING`, ensuring already persisted actions are skipped with zero overhead, row IDs remain monotonic and stable, and telemetry data does not incur repetitive transaction rewrites on each progress tick.
- **Bounded Window Loading**: While the database retains the complete append-only historical audit trail, `Load` queries bound in-memory action loading to the most recent window (`domain.MaxLastActions = 200`) in chronological order, keeping memory consumption and load latency strictly bounded ($O(1)$) across long-running project executions.
- **Graceful Shutdown & SQLite WAL Checkpoint Truncation**: When terminating on completion, error, or OS signals (`SIGTERM`, `SIGINT`), the engine invokes `SQLiteRepository.Close()`, which explicitly executes `PRAGMA wal_checkpoint(TRUNCATE);` before closing the database handle. This guarantees all WAL journal frames are fully written to the base database file and the journal is truncated to zero bytes, eliminating corrupted disk image risks across host-container volume mounts.

### 2. Topological Task Scheduler (`pkg/services/scheduler.go`) & Story DAG Scheduler (`pkg/services/story_dag_scheduler.go`)

Noctifab evaluates dependencies and schedules work concurrently at two synchronized layers: macro-level user stories (`StoryDAGScheduler`) and micro-level task execution (`Scheduler`).

#### Global Task DAG Scheduling (`scheduler.go`)
- **Fine-Grained Global Addressing**: Tasks across all stories are assigned globally unique identifiers formatted as `<STORY_ID>-TASK-<NUMBER>` (e.g., `US-001-TASK-001`, `US-002-TASK-001`).
- **Eliminating False Serialization**: In traditional story-level scheduling, entire stories block sequentially behind upstream stories until the upstream story completes all tasks, passes Definition of Done (DoD) reviews, and finishes acceptance testing. In Noctifab's Global Task DAG model, tasks declare fine-grained dependencies directly on prerequisite tasks across story boundaries.
- **Selective Milestone Barrier Bypass**: When evaluating candidate tasks in `Scheduler.GetReadyTasks`, the scheduler checks if candidate tasks declare explicit cross-story dependencies (`US-XXX-TASK-YYY`). If explicit dependencies exist, the scheduler bypasses the coarse story-level milestone barrier, immediately scheduling downstream tasks (e.g. `US-002-TASK-001`) the moment their upstream prerequisites (e.g. `US-001-TASK-001`) reach `TaskSuccess`. If a task declares no cross-story dependencies, the scheduler cleanly falls back to milestone ordering to prevent out-of-order execution.
- **Dynamic File Lock Arbitration**: Even when tasks across multiple stories are unblocked simultaneously, `FileLockRegistry` prevents race conditions by arbitrating task target files (`TargetFiles`). If two tasks touch overlapping files, only one is scheduled per tick, safely serializing file access without human intervention.
- **Failure Cascade & Pruning**: If an upstream foundation task fails (`TaskFailed`), dependent tasks across all stories are automatically blocked or pruned, while independent tasks in downstream stories continue execution without deadlocking.

#### Story DAG Scheduler & Cross-Story Pipelining (`story_dag_scheduler.go`)
- **Pipelined Execution Mode (`SetPipelined(true)`)**: In multi-story parallel runs (`agents.orchestrator.number > 1`), `StoryDAGScheduler` enables pipelined scheduling. Rather than waiting for parent stories to reach `SUCCESS`, child stories are dispatched to begin decomposition and task scheduling concurrently once parent stories reach `RUNNING`.
- **Global State Merging (`Orchestrator.PlanStory`)**: When multiple stories decompose their tasks concurrently, `PlanStory` merges newly generated tasks into `currentState.Tasks` without overwriting existing tasks from other stories.
- **Independent Story Task Isolation**: Story executors in `start_story_executor.go` track tasks per story via `getStoryTasks` and determine story completion independently via `allStoryTasksFinished`, allowing stories to complete their DoD sign-off asynchronously.

#### Structured Roadmap Layout & Task Serialization
- User stories are discovered strictly in `roadmap/user-stories/`, formatted as `US-XXX-title-slug.md`.
- Task domain models are automatically serialized into markdown files in `roadmap/tasks/` (`US-XXX-TASK-YYY-slug.md`) during planning and tool additions for full git auditability.

### 3. Policy Validator (`pkg/services/validator.go`)
Acts as a security checkpoint before any LLM-proposed tool is executed. It matches tools and command patterns against role profiles defined in `.noctifab/profiles/` to prevent directory traversal attacks, illegal network requests, or host command escapes.

#### Tool Sandboxing & Hermetic Package Resolution
Noctifab restricts agent capabilities to guarantee deterministic execution and security:
- **No Direct Terminal Execution (`exec` Disabled)**: Neither `generator` nor `tester` agents are granted access to terminal execution tools (`exec`). Agents cannot directly invoke shell installation commands such as `pip install`, `npm install`, `cargo add`, or `go get`.
- **Manifest File Declarations & Batch Operations**: Generator agents modify project manifest files and source code using `write_file`, `write_files`, `edit_file`, or `apply_patch`. The `write_files` tool allows atomic multi-file creation in a single LLM turn, reducing scaffolding latency by over 70%.
- **Hermetic Offline Failures & Standard Library Preference**: In containerized, offline, or dark-factory validation environments, introducing un-cached third-party dependencies causes `run_tests` to fail with module import errors (e.g. `ModuleNotFoundError`). All agent prompts enforce a **Standard Library First Mandate**: if `run_tests` fails on an uninstalled package, the agent is instructed to immediately refactor the implementation to use built-in standard library primitives (e.g. `asyncio`, `net/http`, `node:fs`, `socket`) rather than burning retries on un-fetchable imports.

### 4. Sandboxed Runner (`pkg/services/sandbox.go`)
Executes shell commands and test suites. It supports two modes:
- **Host Sandbox**: Executes commands directly on the host using jail-like path boundary validations.
- **Docker Sandbox**: Spawns ephemeral, warm Docker containers to execute commands in complete isolation, preventing host resource pollution.

The **Watchdog Liveness Monitor** (`pkg/services/watchdog.go`) wraps command execution with two safeguards:
- **MaxDuration**: Absolute wall-clock timeout (default 5 min). The process group is killed via `SIGKILL` if execution exceeds this limit.
- **IdleTimeout**: Sliding window that resets on every byte of stdout/stderr output. If no output is produced for the configured duration, the process is killed, and `ErrWatchdogIdleTimeout` is returned. This prevents silent hangs from deadlocked threads or infinite loops without output.
- **`killProcessGroup`**: Both timeouts use `syscall.SysProcAttr{Setpgid: true}` to kill the entire process group, ensuring child processes and background threads are terminated.

Configured via `sandbox.idle_timeout_seconds` in `config.yaml` (default: 30s).

#### Declarative Pre-Flight Formatter Auto-Fix
To eliminate high-latency agent turns spent fixing trivial whitespace, indent, or import formatting errors, `RunTestsTool` executes the project's declarative `formatter_command` (e.g. `ruff format .`, `cargo fmt`, `rubocop -A`, `go fmt ./...`) before running test commands. This keeps the engine strictly **language-agnostic** while ensuring deterministic code cleanliness prior to verification.

#### Worktree Cache & Dependency Symlinking (`pkg/services/worktree_cache.go`)
Isolated Git worktrees created under `.noctifab/worktrees/` redirect heavy compiler caches (Cargo, Go, pip, ccache) to `.noctifab/cache/`. When `package.json` is present, root `node_modules` are automatically symlinked into the worktree directory, enabling Node/TypeScript test runners (`jest`, `vitest`, `ts-node`) to execute seamlessly without redundant per-worktree installations.

### 5. Rebase Queue (`pkg/services/rebase_queue.go`)
A thread-safe channel queue that manages Git rebases and branch merges. When multiple tasks complete in parallel, the rebase queue serializes merges into the target branch to avoid merge conflicts and race conditions.

### 6. Command Mailbox & Single-Writer Event Loop (`pkg/services/command_channel.go`)
The `CommandMailbox` serves as the centralized, serial event loop for all state mutations across the engine. Worker goroutines and external REST API handlers send mutation command payloads (`StateMutationCmd`, `ResolveClarificationCmd`, `AddTaskCmd`, `OverrideMergeCmd`, `ResetTaskCmd`, `FailTaskCmd`) to the mailbox.

- **`SendSync(ctx, cmd)`**: Synchronously submits a mutation to the single-writer FIFO queue and awaits execution on the dedicated repository writer goroutine. This completely eliminates SQLite transaction locking contentions and OCC version conflicts between concurrent agents.
- **OCC Fallback**: If the mailbox is not actively running (e.g. in isolated unit test harnesses), `updateStateWithRetry` seamlessly falls back to optimistic concurrency control with exponential reload-and-retry backoff.
- **Wakeup Interrupts**: The mailbox exposes a `Wakeup()` notification channel. The orchestrator's fallback OCC sleep (`SleepWithInterrupt`) selects on this channel, allowing operator commands (abort, steer, model switch) to interrupt backoff immediately without waiting for timers to expire.
- **Loopback REST Server**: Binds on `127.0.0.1:18080` to allow operators and dashboard interfaces to inspect runtime health (`/healthz`, `/statusz`) and dynamically steer execution. See [docs/api.md](api.md) for the complete endpoint reference.

### 7. Task Diagnostic Cache & SHA-256 Context Deduplication (`pkg/services/diagnostic_cache.go`)
`TaskDiagnosticCache` provides in-memory caching and cryptographic deduplication across agent turns:
- **SHA-256 Content Verification**: When pre-loaded files (e.g., `SPEC.md` or story contracts) are injected into the prompt context at task startup, their SHA-256 checksums are cached. If an agent calls `read_file` on an unmodified file, the cache returns a concise reference notice instead of re-injecting duplicate multi-kilobyte payloads into subsequent prompt turns, drastically cutting token consumption.
- **Dynamic Invalidation**: When any file-mutating tool (`write_file`, `write_files`, `edit_file`, `delete_file`, `apply_patch`) executes, cached diagnostic results (`run_tests`, `run_linter`) and inspection buffers are automatically invalidated.
- **Disk Integrity Checks**: On each `read_file` inspection, the file's on-disk checksum is compared against the cached hash; any external or command-induced mutation immediately bypasses the cache to fetch fresh content.

### 8. Walking Skeleton Slicing Mandate (`US-001`)
The autonomous dark factory architecture prioritizes a **Vertical Walking Skeleton** over horizontal abstraction layers:
- The Product Manager Agent formats the initial user story (`roadmap/user-stories/US-001.md`) to produce an end-to-end compiling binary with baseline test verification.
- Delivering a working entrypoint in the first 2 minutes establishes the compilation pipeline and baseline black-box test passes before downstream stories expand deeper domain schemas or auxiliary interfaces.

### 9. Two-Turn Surgical Repair Pipeline (`pkg/services/orchestrator_execute_turns.go`)
When a test failure occurs during code generation:
- **Diagnostic Pre-Reading**: The orchestrator parses the compiler or test failure log, extracts the offending file paths, and pre-reads their current disk content into the surgical generator's context buffer.
- **Two-Turn Budget**: Surgical repair runs with a dedicated 2-turn budget. In Turn 1, the agent applies targeted fixes directly via `write_file`/`edit_file`. In Turn 2, verification checks and final adjustments are completed, avoiding unbounded single-action inspection loops.

### 10. Persistent LLM Capability Cache & Upfront Model Resolution (`pkg/infrastructure/llm/`)
- **Preflight Model Discovery**: `PingAndResolveModel` queries `/models` at startup to validate configured model identifiers. Deprecated or 404 models are automatically upgraded to the highest-ranked available flagship model before orchestrator dispatch.
- **In-Memory Capability Memory (`globalCapabilityCache`)**: Memorizes unsupported parameters (`temperature`, `max_tokens`, `response_format`, `extra_body`) on the first API rejection, ensuring subsequent calls across all agent roles automatically omit incompatible options without repetitive retry churn.

### 11. Story-Level Parallelism & DAG Scheduling (`pkg/services/story_dag_scheduler.go`)
Noctifab supports two tiers of concurrent execution:
- **Story-Level Parallelism**: Independent feature user stories branch from the walking skeleton (`US-001`) with minimal inter-story dependencies (`depends_on: ["US-001"]`). When `agents.orchestrator.number > 1` or `vcs.use_worktrees: true`, the `StoryDAGScheduler` dispatches unblocked user stories concurrently across isolated worker branches.
- **Task-Level Parallelism**: Within each active user story, independent tasks are processed in parallel across Generator and Tester worker pools (`scheduler.max_parallel_workers > 1`), merging safely through the serialized `RebaseQueue`.
- **Multi-Loop Succeeded Caching**: When running remediation iteration loops (`runtime.loops > 1`), completed user stories are pre-marked as succeeded (`MarkStoryCompleted`), immediately unlocking dependent downstream stories without re-executing passing code.

---

## Multi-Agent Roles & Team Pipeline

Noctifab exposes the following implemented roles and retained experimental capability:

| Role Key | Agent Name | Domain Scope & Responsibility |
| :--- | :--- | :--- |
| **`orchestrator`** | Orchestrator Agent | Coordinates state persistence, VCS branch rebasing, task assignment, and PR creation. |
| **`product_manager`** | Product Manager Agent | Analyzes `SPEC.md` and existing user stories in `roadmap/user-stories/`. Employs **Progressive Roadmapping**: on Pass 1, immediately emits `US-001-walking-skeleton.md` with runnable contracts to allow code implementation to start in $<3$ minutes, deferring deeper decomposition to subsequent passes. Enriches stories with explicit Definitions of Done (DoD), language-agnostic interface contracts, I/O formatting invariants, error prefixes, exit codes, and comprehensive edge-case scenario matrices before task planning starts. |
| **`planner`** | Task Planner Agent | Decomposes User Stories into a Directed Acyclic Graph (DAG) of executable technical tasks, automatically serializing task entities into `roadmap/tasks/`. |
| **`generators`** | Generator Agent | Writes production source code and initial feature logic in task branches. |
| **`testers`** | Tester Agent | Independently writes black-box test suites (unit, integration, e2e) against public contracts. |
| **`resolver`** | Resolver Agent | Resolves complex 3-way Git merge and rebase conflicts across parallel worker branches using a 5-tier merge engine (including whole-file dual reimplementation). |
| **`qa`** | Experimental QA capability | Retained but disabled in Phase 0; no QA runtime executes. |
| **`auditor`** | Acceptance Auditor Agent | Evaluates whole-project compliance against root `SPEC.md` and story contracts prior to PR creation, halting release if critical command or interface omissions are found. |
| **`fallback`** | Fallback Agent (Omni-Agent) | Unified pipeline watchdog & sovereign chief surgeon (merging previous `unblocker` and `last_resort` roles). Operates in two modes: Passive Watchdog (0-token fast-paths, log escalation, scope triage) and Active Sovereign Omni-Builder (cross-domain repair under 4-Tier Compromise Hierarchy). |

Architecture, security, performance, documentation, and infrastructure work is represented by explicit planner tasks and deterministic validators, not specialist agents.

### Generator-Tester Oscillation Circuit Breaker (`pkg/services/circuit_breaker.go`)

During multi-turn task execution, Generator and Tester agents can enter an oscillation loop where the Tester makes non-essential micro-tweaks to docstrings or assertions after the implementation is already fully verified. The `OscillationCircuitBreaker` monitors turn progression and breaks test churn:
- **Trip Conditions**: Activates when (1) $\ge 2$ consecutive test runs pass with 0 failures, (2) $\ge 2$ consecutive turns have only modified test files with unchanged `src/` production code, and (3) task progress is $\ge 70\%$.
- **Action**: When tripped, the orchestrator halts further test refinement mutations and transitions the task forward to review and completion, saving 40%–60% of task token expenditure.

### Unified Fallback Architecture (Passive Watchdog & Active Sovereign Omni-Builder)

To prevent execution livelocks, deadlock stalls, and broken builds, `noctifab` provides a unified **Fallback Agent** (`fallback`, [docs/fallback_agent.md](fallback_agent.md)) combining continuous health monitoring with sovereign repair authority:
1. **Passive Watchdog Mode**: Non-invasive background daemon that polls pipeline state every 30s. Fixes routine CLI hangs via 0-token regex fast-paths, manages stall cooldowns, evaluates budget/timeout cliffs, and resets stalled tasks with injected recovery directives without altering code.
2. **Active Sovereign Omni-Builder Mode**: Directly summoned when a task accumulates repeated stalls, exhausts retry budgets, or encounters toolchain deadlocks. Operates with sovereign cross-domain authority across code, tests, and specification contracts under the 4-Tier Compromise Hierarchy to force a clean compiling, test-passing release.

---

## Specification-Based Complexity Units ($CU$)

To prevent micro-tasks and oversized monolithic user stories during spec decomposition, `noctifab` uses a **Unified Composite Complexity Unit ($CU$)** metric.

The $CU$ metric synthesizes three specification-driven dimensions directly from natural language text (`SPEC.md`):

$$\text{CU} = \underbrace{\text{Data Movements}}_{\text{COSMIC (Entry, Exit, Read, Write)}} + \underbrace{\text{Domain Concepts}}_{\text{DDD (Structs, Entities, State)}} + \underbrace{\text{Contract Invariants}}_{\text{RPA (Flags, Exit Codes, Output Rules)}}$$

### Theoretical Foundations & Academic References

The Unified Composite $CU$ Metric synthesizes three established specification-driven standards:

1. **COSMIC Function Points (ISO/IEC 19761:2011):**
   - Measures **Data Movements** directly from natural language text: **Entry (E)** (arguments, stdin, requests), **Exit (X)** (stdout, status codes, responses), **Read (R)** (filesystem/db reads), and **Write (W)** (file/db writes).
   - *Reference:* [ISO/IEC 19761:2011 — COSMIC Measurement Method](https://www.iso.org/standard/55222.html) and [COSMIC Measurement Manual v5.0](https://cosmic-sizing.org/publications/measurement-manual-v5-0/).

2. **Domain & Object Model Complexity (DDD & Chidamber-Kemerer):**
   - Evaluates domain structural complexity from requirement prose: core **Entities**, **Value Objects**, **Domain Services**, and **State Machines**.
   - *Reference:* [Chidamber & Kemerer (IEEE TSE, 1994) — A Metrics Suite for Object Oriented Design](https://doi.org/10.1109/32.295895) and [MIT Sloan Working Paper 3233-90](https://dspace.mit.edu/handle/1721.1/48493).

3. **Requirement Invariants & Imperatives (NASA ARM & RFC 2119):**
   - Measures contract invariants and interface surface area: CLI options, error conditions, exit code specifications, and imperative rules (`MUST`, `SHALL`).
   - *Reference:* [NASA Technical Memorandum 104640 (ARM Tool)](https://ntrs.nasa.gov/citations/19970024095) and [IETF RFC 2119 Requirement Levels](https://datatracker.ietf.org/doc/html/rfc2119).

### Sizing Boundaries & Decomposition Rules
1. **Product Manager Sizing (`generate.tmpl`):**
   - For concise specifications ($CU_{\text{total}} < 25$, such as `wc`, `echo`, `calculator`), the Product Manager Agent is mandated to create **exactly 1 User Story**.
   - For larger specifications, User Stories are sized to target windows ($15 \le CU_{\text{story}} \le 30$).
2. **Planner Task Sizing (`decompose.tmpl`):**
   - User stories are decomposed into tasks bounded by $CU_{\text{task}} \in [4, 8]$.
3. **Task Cohesion Validation (`pkg/services/task_cohesion.go`):**
   - Programmatically enforces task cohesion: rejecting micro-tasks ($CU < 4$) and splitting oversized tasks ($CU > 8$).

---

## Self-Healing & Anti-Stalling Resiliency

To prevent execution stalls and guarantee progress under validation failures, `noctifab` implements self-healing at two distinct layers:

### 1. Multi-Turn Agent Loop (Intra-Turn Healing)
During the **Execute Phase**, Generator and Tester agents are not restricted to a single-turn completion. If they execute verification tools like `run_tests` or `run_linter` and encounter failures (compilation errors, test assertion failures, or policy violations), the orchestrator automatically captures the error output, appends it to the prompt context, and completes the LLM again in a loop of up to **5 turns**. This allows agents to fix formatting, syntax, and logic bugs immediately within a single run.

### 2. General Watchdog Repair (Inter-Turn Healing)
If a task completes its execution phase but fails the final test suite evaluation, the orchestrator intercepts the failure logs and invokes the `WatchdogRepair` handler. It supports three failure categories:
- **Timeout**: Triggered when a test run hangs and is terminated by the liveness watchdog. The prompt focuses on resolving infinite loops, unjoined threads, and deadlocks.
- **Compile**: Triggered when compilation fails. The prompt focuses on resolving syntax errors, type mismatches, and import problems.
- **Test Logic**: Triggered when assertions fail. The prompt focuses on resolving incorrect test expectations or fixing logic implementations.

The repair handler makes up to **3 repair attempts** autonomously to self-heal the codebase.

### 3. Dynamic Model Fallback Engine (Provider-Specific Capacity Ranking)
When an LLM call fails due to rate limits (HTTP 429), quota limits (HTTP 401/402), or server errors (HTTP 5xx), the orchestrator triggers the dynamic fallback pipeline (`getNextLowerModel`):

1. **Live Endpoint Model Discovery**: Queries `GET /models` or `/v1/models` live from the provider API to fetch all currently active models. No static model lists are used.
2. **Provider Capacity Ranking**: Invokes the provider's `ParseModelFunc` (registered in `ProviderSpec`) to compute a numerical `Rank` for each model, combining tier keywords, version numbers, and parameter size suffixes.
3. **Descending Sort**: All models are sorted by `Rank` descending (highest capacity first).
4. **Transparent Model Switch**: The model immediately below the failing one in the sorted list is selected and `Client.Model` is updated in-place. The task resumes without losing context.

If the failing model is already the lowest-ranked model available, no fallback is selected and the error is surfaced to the caller.

#### Fallback Chain Examples

Each provider has its own capacity ranking formula. The following chains are verified by `TestModelFallbackChains` in [fallback_test.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/fallback_test.go).

**Anthropic (claude-* via `parseAnthropicModel` — tier keyword ranking: opus > sonnet > haiku)**

```
claude-3-opus-20240229   [Rank: 430]  ← primary
    ↓ fails
claude-3-5-sonnet-latest [Rank: 335]  ← fallback 1
    ↓ fails
claude-3-5-haiku-latest  [Rank: 235]  ← fallback 2
    ↓ fails
(no fallback — surface error)
```

**OpenAI (gpt-* via `parseOpenAIModel` — tier keyword + version multiplier ranking)**

```
gpt-4o        [Rank: flagship tier + version×5]  ← primary
    ↓ fails
gpt-4o-mini   [Rank: compact tier + version×5]   ← fallback 1
    ↓ fails
gpt-3.5-turbo [Rank: lite tier + version×5]      ← fallback 2
    ↓ fails
(no fallback — surface error)
```

**Mistral (via `parseMistralModel` — large/codestral > medium > small)**

```
mistral-large-latest  [Rank: 40]  ← primary
    ↓ fails
mistral-small-latest  [Rank: 20]  ← fallback 1
    ↓ fails
(no fallback — surface error)
```

**Meta Llama / Together AI (via `parseLlamaModel` — parameter size ranking using `StandardSizeWeights`)**

```
Llama-3.1-405B-Instruct  [Rank: 531]  ← primary
    ↓ fails
Llama-3.3-70B-Instruct   [Rank: 433]  ← fallback 1
    ↓ fails
Llama-3.1-8B-Instruct    [Rank: 231]  ← fallback 2
    ↓ fails
(no fallback — surface error)
```

**DeepSeek (via `parseDeepSeekModel` — r1/v3/coder > chat)**

```
deepseek-coder  [Rank: 30]  ← primary
    ↓ fails
deepseek-chat   [Rank: 20]  ← fallback 1
    ↓ fails
(no fallback — surface error)
```

**xAI / Grok (via `parseXAIModel` — grok-3 > grok-2 > grok-3-mini > mini)**

```
grok-3       [Rank: 75]  ← primary
    ↓ fails
grok-2       [Rank: 50]  ← fallback 1
    ↓ fails
grok-3-mini  [Rank: 45]  ← fallback 2 (if available)
    ↓ fails
(no fallback — surface error)
```

**Perplexity (via `parsePerplexityModel` — deep-research > reasoning-pro > reasoning > pro)**

```
sonar-deep-research  [Rank: 50]  ← primary
    ↓ fails
sonar-reasoning-pro  [Rank: 40]  ← fallback 1
    ↓ fails
sonar-reasoning      [Rank: 30]  ← fallback 2
    ↓ fails
sonar-pro            [Rank: 20]  ← fallback 3
    ↓ fails
(no fallback — surface error)
```

**Cohere (via `parseCohereModel` — command-r-plus > command-r > command-light)**

```
command-r-plus  [Rank: 40]  ← primary
    ↓ fails
command-r       [Rank: 30]  ← fallback 1
    ↓ fails
command-light   [Rank: 20]  ← fallback 2
    ↓ fails
(no fallback — surface error)
```

**Kimi / Moonshot (via `parseKimiModel` — k3 > k2.7 > k2.6 > k2.5 > k2)**

```
kimi-k3    [Rank: 50]  ← primary
    ↓ fails
kimi-k2.7  [Rank: 40]  ← fallback 1
    ↓ fails
kimi-k2.5  [Rank: 20]  ← fallback 2
    ↓ fails
(no fallback — surface error)
```

**Qwen / DashScope (via `parseQwenModel` — max > plus > turbo)**

```
qwen-max    [Rank: 65]  ← primary
    ↓ fails
qwen-plus   [Rank: 55]  ← fallback 1
    ↓ fails
qwen-turbo  [Rank: 45]  ← fallback 2
    ↓ fails
(no fallback — surface error)
```

**Ollama / HuggingFace (via `parseOllamaModel` / `parseHuggingFaceModel` — parameter size using `StandardSizeWeights`)**

```
llama3.1:70b  [Rank: 431]  ← primary
    ↓ fails
llama3.1:8b   [Rank: 231]  ← fallback 1
    ↓ fails
(no fallback — surface error)
```

> **Implementation**: The complete `TestModelFallbackChains` test in [fallback_test.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/fallback_test.go) verifies all the above chains end-to-end, using real `ParseModelFunc` parsers and `selectLowerModelFromParsed` to simulate exactly what `getNextLowerModel` in [client.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/client.go) does at runtime.

#### Fault Tolerance: New Model Resilience

The fallback engine is designed to remain operational even when a provider releases models whose names are not yet recognised by the capacity parser. Three edge cases are explicitly handled and verified by `TestFallbackFaultTolerance` in [fallback_test.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/fallback_test.go):

| Scenario | Behaviour |
|---|---|
| **Current model unrecognised by parser** (e.g. new tier keyword like `"claude-4-nova"`) | Safety valve selects the **lowest-ranked** known model from the provider's live `/models` list instead of failing |
| **Current model fails `RequiredPrefix` filter** (e.g. provider renames a model family) | Same safety valve — falls back to the lowest-ranked successfully parsed model |
| **Current model is the only available model** | Returns `""` — error is surfaced correctly; no infinite retry |
| **Current model is already the lowest-ranked** | Returns `""` — error is surfaced correctly; no infinite retry |
| **No models available from provider** | Returns `""` — emits a warning to stderr and surfaces error |

The core guarantee is: **as long as the provider's `/models` endpoint returns at least one model that the parser recognises, the fallback engine will never fail with an empty candidate list due to an unrecognised current model name.**

When a provider adopts a completely new naming convention (e.g., changing from `"claude-3-*"` to a scheme without the `"claude"` prefix), the `RequiredPrefix` in the parser should be updated or removed in the corresponding provider file (e.g. [anthropic.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/anthropic.go)). No other files need to be changed.

### 4. Safety Circuit Breakers & Stall Protection
- **`runtime.max_actions`**: Specifies a limit on the number of task execution cycles. If the total number of actions across all tasks reaches this ceiling, the story is aborted to prevent infinite repair loops and LLM budget exhaustion.
- **`runtime.max_silent_stall_duration`**: Story-level watchdog (default `30m`). If no task makes state updates or progress within this duration, the orchestrator fails remaining tasks and aborts the story cleanly.
- **`runtime.max_tokens_per_story` & `runtime.max_tokens_per_task`**: Hard token budget ceilings to terminate runaway tasks/stories.
- **`runtime.max_duration`**: Specifies a story-level wall-clock timeout.
- **`sandbox.timeout_seconds`**: Specifies a configurable command execution timeout for individual test and linter runs, preventing premature truncation on large test suites.

#### First-Class Generator Surgical Repair (`surgical_repair`)
When a task fails test validation due to a compilation error or test assertion failure, the orchestrator triggers a single-turn **Surgical Repair Pass** (`surgical_repair` prompt template). It bypasses the reader context collection phase to minimize token latency, passing the exact compiler/test failure stack trace directly to the Generator Agent to perform minimal, targeted edits in `edit_file` without rewriting working code.

#### Zero-Token Pre-Commit Auto-Formatting
Before Git commits are staged and committed (`stageAndCommit`), the orchestrator automatically executes the project's configured formatter (`sandbox.formatter_command`, e.g. `go fmt ./...`) directly within the active task worktree, ensuring syntax and style compliance with zero LLM turns.

#### Host QA Sandbox Runner (`pkg/services/qa_sandbox_host.go`)
The QA subsystem provides `HostQABuildSandbox` and `HostQASandboxRunner` to execute QA build and validation commands within host runtime directories (`tmp`, `home`, `cache`), safely isolating review workspaces.

### 5. Self-Correcting & Dynamic Prompts Framework

Noctifab employs a dynamic, context-aware prompt adaptation and self-correcting engine across all agent roles:

#### A. Fallback Agent Dynamic Prompt Injection & Fast-Path Engine (`pkg/services/fallback_agent.go`)
The **Fallback Agent** runs as an autonomous background watchdog on an independent timer (default `30s` poll interval). When a pipeline stall is detected (`frozen_progress`, `orphaned_task`, `agent_inconsistency`, `conflict_blocked`):
1. **Live Log Tailing & Secret Scrubbing (`log_tailer.go`)**: Tails standard output logs of stalled tasks and passes snippets through `SanitizeLog` to scrub sensitive API keys and tokens before prompt injection.
2. **0-Token Fast-Path Regex Classifier (`unblocker_fastpath.go`)**: Matches log snippets against static regex patterns for routine CLI hangs (stdin interactive `y/n` prompts, port binding collisions, test watch mode spinners), unblocking tasks in **< 5ms** with **0 LLM token overhead**.
3. **10x Progressive Log Window Escalation**: Scales diagnostic log depth based on `task.StallCount` (Level 1: 50 lines $\rightarrow$ Level 2: 500 lines $\rightarrow$ Level 3: 5,000 lines).
4. **Task Stall Recovery Directives (`[STALL RECOVERY DIRECTIVE]`)**: Attaches `RecoveryDirective` to task state upon reset, injecting instructions into Generator and Tester worker prompts on re-queued attempts to prevent repeating stalling actions.

See [docs/fallback_agent.md](fallback_agent.md) for full developer reference.

#### B. Git-Aware & Language-Agnostic Workspace Discovery (`pkg/services/workspace_discovery.go`)
1. **Unified Workspace Scanning (`ListWorkspaceSourceFiles`, `CollectWorkspaceSourceSnapshot`)**: Replaces language-specific directory lists with Git-aware discovery (`git ls-files -c -o --exclude-standard` / `git check-ignore`), automatically ignoring project `.gitignore` patterns, container targets (e.g. `target_container/`), and compiler caches across any language toolchain.
2. **Binary Content & Size Gating (`IsTextFile`)**: Evaluates initial file bytes for null characters (0x00) and enforces a 1MB file size ceiling to completely prevent object files (`.o`), static/dynamic libraries (`.a`, `.so`, `.rlib`), executables, and compiler metadata from leaking into LLM prompts.
3. **Workspace Legacy Scanning & Greenfield Classification (`ScanLegacyFiles`, `IsGreenfieldWorkspace`)**: Distinguishes true legacy codebases from greenfield starter repositories by ignoring package manifests, lockfiles, and stubs (< 5 lines), requiring $\ge 50$ lines of candidate code before triggering legacy stabilization mode.
4. **Product Manager Legacy Directive (`prompt_templates.go`)**: Injects `LEGACY CODEBASE STABILIZATION & REFACTORING MANDATE` into the PM prompt when legacy code is detected. The PM automatically generates `roadmap/user-stories/US-001.md` titled `"Legacy Codebase Characterization & Stabilization"`, requiring unit/integration characterization tests before refactoring or new feature work.

5. **Dynamic Role Prompt Adaptation**: Planner, Generator, and Tester prompts dynamically adapt with characterization testing requirements and surgical refactoring directives (`edit_file`, `apply_patch`).

#### C. Shared Dependency Worktree Caches (`pkg/services/worktree_cache.go`)
1. **Symlink Dependency Projection (`SymlinkSharedDependencies`)**: Automatically projects existing dependency directories (`node_modules`, `.venv`, `venv`, `vendor`, `.bundle`, `deps`, `_opam`, `gradle/` wrapper, `.mvn/` wrapper) and executable wrapper scripts (`gradlew`, `mvnw`) into new worktrees in **< 1ms**, eliminating redundant package downloads.
2. **Compiler & Package Cache Redirection (`BuildSharedCacheEnv`)**: Directs compiler and package manager caches across 11 ecosystems to `.noctifab/cache/` via environment variables (`CARGO_TARGET_DIR`, `GOCACHE`, `GRADLE_USER_HOME`, `MAVEN_OPTS`, `PIP_CACHE_DIR`, `npm_config_cache`, `CCACHE_DIR`, `BUNDLE_PATH`, `NUGET_PACKAGES`, `COMPOSER_CACHE_DIR`, `DUNE_CACHE`, `HEX_HOME`, `MIX_HOME`).
3. **Build Cache Acceleration (`ConfigureToolchainWorktreeCaches`)**: Generates `.cargo/config.toml` targeting the shared cache and enables Gradle task build caching (`org.gradle.caching=true`).
4. **Safe Worktree Cleanup**: Prunes worktree symlinks on task completion without modifying or recursing into shared root dependency structures.

#### D. Pre-Flight LLM Provider Capability Caching (`openai_adapt.go` & `openai.go`)

1. **Parameter Rejection Detection**: Tracks provider parameter rejections (e.g. `temperature`, `max_tokens`, `response_format` for reasoning models) upon receiving an initial HTTP 400 error.
2. **Thread-Safe Capability Cache (`providerCapabilityCache`)**: Records model parameter limitations in RAM across worker goroutines.
3. **Self-Correcting API Payloads**: Automatically omits unsupported parameters on subsequent API requests without requiring failing roundtrips.

#### D. Parallel Context Compaction Engine (`pkg/usecase/prompt_templates.go`)
For context payloads $> 20$ KB (`20,000` bytes):
1. **Parallel Worker Compaction**: Parallelizes line block compaction across worker goroutines in `CompactSimpleEnglish` and `CompactCaveman` modes (`context.compaction`).
2. **Invariant Preservation**: Cuts token volume by 25%+ while strictly preserving code blocks, JSON schemas, file paths, and technical invariants.

---

## Dark Factory Acceleration Engine (5x–10x Speedup)

To support near-instantaneous development feedback loops, `noctifab` implements an end-to-end pipelined acceleration engine:

1. **Parallel DAG Task Worker Pools**: Executes independent tasks concurrently (`scheduler.max_parallel_workers > 1`), allocating an isolated Git worktree (`.noctifab/worktrees/task-<id>`) to each worker and merging completed branches asynchronously via a serialized rebase queue (`pkg/usecase/rebase_queue.go`).
2. **Tiered LLM Provider Routing**: Routes deep reasoning models to PM/Planner agents while directing Generator/Tester implementation workers to high-throughput coding models.
3. **Parallel 3x Majority-Vote Test Validation**: Dispatches 3 test validation runs concurrently using Go goroutines, reducing verification latency from ~15s to ~3s.
4. **Unified Diff Multi-File Patching (`apply_patch`)**: Enables agents to apply multi-file unified diff patches (`diff -u` / Git format) in a single turn with fuzzy line matching and security validation (`pkg/services/apply_patch_tool.go`).
5. **Spec-Level Deterministic Mock Clocks**: Enforces mock clock patterns at the spec layer (`US-xxx.md`) to guarantee zero assertion flakiness on time-dependent code.
6. **Aggressive Prompt History Pruning**: Suffix-only pruning preserves LLM KV cache prefixes on retry turns.
7. **Warm Compiler & Sandbox Caching**: Mounts host package caches (`/go/pkg/mod`, `~/.cache/go-build`, `.cargo/registry`) directly into validation containers for rapid incremental builds.
8. **Zero-Delay Task Handoff**: Whenever a scheduler loop run makes progress (i.e., a task completes or state is updated), the orchestrator immediately invokes the next schedule check without sleeping for `poll_interval`. Tasks are chained sequentially with no idle latency.
9. **In-Memory Diagnostic Result Caching (`TaskDiagnosticCache`)**: During intra-turn multi-turn agent loops (`RunTesterAgent` and `RunGeneratorAgent`), the orchestrator instantiates an in-memory `TaskDiagnosticCache` that caches the execution results of read-only verification tools (`run_tests` and `run_linter`). Whenever an agent executes file-mutating actions (`write_file`, `edit_file`, `multi_replace_file_content`, `delete_file`), an internal `isDirty` boolean flag stored in RAM inside the cache struct is set to `true`, invalidating the cache. If an agent calls `run_tests` or `run_linter` again without modifying workspace files (`isDirty == false`), the orchestrator returns the cached result instantly (`0ms`), eliminating redundant subprocess executions.

---

## Orchestrator Execution Architecture Modes

The orchestrator supports three distinct execution architecture modes configured via `agents.architecture` in `.noctifab/config.yaml`:

### 1. `code_first` (Default, legacy alias `code_first_verification_loop`)
The **Code-First Verification Loop** separates implementation from test verification:
1. **Minimal Implementation Pass (Generator Agent)**: Scaffolds core types, function signatures, and minimal implementation logic first.
2. **Test Characterization Pass (Tester Agent)**: Inspects the created code signatures and writes comprehensive unit and integration tests against them.
3. **Refactor & Fulfill Pass (Generator Agent)**: Refactors and expands the implementation to satisfy all tests.

### 2. `single_pass` (Aliases: `single_pass_co_synthesis`, `co_synthesis`, `single_pass_execution`, `spe`, `spcs`)
The **Single-Pass Co-Synthesis** mode optimizes for maximum generation speed and minimum token latency:
* **Unified Generation Pass**: A single Generator agent pass co-synthesizes both the source code implementation and corresponding tests together in one turn (`generator/single_pass` prompt template).
* **Zero-Token Auto-Formatting**: Executes the configured project formatter (e.g., `cargo fmt`, `go fmt`, `black`) via `stageAndCommit` before staging and committing changes.
* **Pre-Test Anti-Stub Quality Gate**: Audits generator output with `auditGeneratorFunctionalOutput` before committing, automatically rejecting hollow stub code (e.g., `todo!()`, `pass`, `NotImplementedError`) and triggering an immediate remediation turn (`single_pass_fix`) if stubs are detected.
* **Fast-Path Verification & QA Merging**: When test validation succeeds on Turn 1, the orchestrator immediately proceeds to QA gating and worker branch merge-back, eliminating multi-turn delays for straightforward user stories and micro-specifications.

### 3. `breadth_first` (Legacy alias `breadth_first_generation`, `bfg`)
The **Breadth-First Generation** mode optimizes for rapid end-to-end prototype delivery across all user stories:
* **Pass 1 (Broad Foundation / ~80% Feature Coverage)**: Generator and Tester implement core happy-path functionality across all tasks first, explicitly deferring cosmetic formatting, linter nitpicks, and obscure corner cases.
* **Deterministic Validation**: Evaluates candidates based on functional happy paths and enforces the non-negotiable **Zero Regressions** rule.
* **Iterative Refinement (Passes 2..N)**: Progressive passes expand edge-case coverage, error handling, linter compliance, and performance hardening.

### 4. Task Execution Ordering (`agents.task_execution_order`)

Configured via `agents.task_execution_order` in `.noctifab/config.yaml`:
* **`generator_first` (Default)**: Generator Agent implements feature code on Turn 1; Tester Agent writes QA tests on Turn 2. Prevents Turn 1 compilation errors and guarantees 0 wasted turns.
* **`tester_first` (TDD Mode)**: Tester Agent writes tests on Turn 1; Generator Agent implements feature code on Turn 2. When `tester_first` is enabled, Noctifab automatically pre-seeds minimal compilation stub files (`ensureTargetStubFilesExist`) for missing target files so Turn 1 `run_tests` compiles cleanly.

```yaml
agents:
  architecture: code_first
  task_execution_order: generator_first # "generator_first" (default) or "tester_first"
  product_manager:
    passes: 2 # 1 = Fast, 2 = Standard 2-Pass Refinement (default), 3 = Deep Contract Audit
```

### 5. Multi-Pass Product Manager Architecture (`agents.product_manager.passes`)
The Product Manager Agent executes a multi-pass specification decomposition and audit loop:
* **Pass 1 (Decomposition & Drafting)**: Renders `generate` prompt, creating initial user stories in `roadmap/user-stories/US-XXX-slug.md`.
* **Pass 2+ (Cross-Story Audit & Contract Alignment)**: Renders `audit` prompt with existing generated stories as context, verifying cross-story dependencies, contract IDs, and `SPEC.md` requirement coverage.

### 6. Black-Box Contract Scenario Prompt Injection
Machine-readable contract expectations parsed from story `noctifab-contract` JSON blocks (`AllowedExecutables`, `ExitCodes`, `StderrPrefixes`, `StdoutContains`) are formatted into a prominent `### BLACK-BOX CONTRACT EXPECTATIONS (NON-NEGOTIABLE)` prompt context section and injected directly into Generator and Tester agent prompts.

### 8. Interactive Command Mailbox & Steering Architecture (`pkg/services/command_channel.go`, `command_channel_steer.go`)
Noctifab implements an asynchronous `CommandMailbox` channel allowing operators to dynamically steer in-flight agent workers and queue prompt orders while the orchestrator runs:
- **Thread-Safe Mutation Queueing**: Incoming commands (`SteerCmd`, `OrderCmd`, `PauseCmd`, `ResumeCmd`) from CLI or Web UI are queued and processed sequentially within the daemon's state loop, eliminating SQLite/PostgreSQL write lock contention.
- **Human-in-the-Loop Directive Injections**: Steering directives are attached directly to target task models (`task.UserDirectives`) and injected into Generator and Tester prompt templates under `[USER HUMAN-IN-THE-LOOP STEERING DIRECTIVES]`.
- **Zero-Latency Story Enqueueing**: Ad-hoc prompt orders (`noctifab order "..."`) dynamically create markdown specifications in `.noctifab/stories/` and forward them to the story dispatcher channel.

### 9. Embedded Real-Time Web Server & SSE Telemetry (`pkg/interfaces/web/`)
- **Self-Contained Single Binary**: Embedded web assets (`//go:embed static/*`) compile directly into the single binary with zero external CDN or npm dependencies.
- **Ring-Buffered Event Replay (`SSEBroadcaster`)**: Keeps a 100-event circular memory buffer to replay missed events upon browser reconnects, accompanied by 15-second keepalive frames.
- **Mission Control Observability**: Exposes state snapshots (`GET /api/v1/state`), event streams (`GET /api/v1/events`), steering injection (`POST /api/v1/steer`), order creation (`POST /api/v1/orders`), and flow controls (`POST /api/v1/pause`, `POST /api/v1/resume`).

### 10. Pre-Flight .gitignore Guardrails & Build Artifact Isolation (`pkg/services/gitignore_guardrail.go`)
Noctifab automatically safeguards workspaces against build artifact explosion, dependency indexing bloat, and Git repository pollution:
- **Pre-Flight Synthesis (`EnsureProjectGitignore`)**: Before starting the execution loop, Noctifab inspects the project root for `.gitignore`. If missing, it writes a comprehensive, language-agnostic `.gitignore`. If already present, it non-destructively appends missing critical rules (`target/`, `node_modules/`, `dist/`, `bin/`, `build/`, `__pycache__/`, `*.py[cod]`, `.venv/`, `venv/`, `.bundle/`, `*.class`, `*.o`, `*.so`, `*.dylib`, `*.log`, `.noctifab/`).
- **Defensive Workspace Discovery (`IsPathExcluded`)**: System-level path evaluation defensively excludes standard build output directories and compiled binary extensions even in edge cases where a `.gitignore` has not yet been processed, preventing build artifacts (such as hundreds of `target/debug/` files in Rust or `node_modules/` in Node.js) from being indexed into SQLite file snapshots or staged in Git.

### 11. String-Literal Aware Code Fence Parser (`pkg/infrastructure/llm/parser.go`)
- **Lexical State Machine**: The parser tracks string literal boundaries (`inString`), backslash escape sequences, and JSON nesting depth (`jsonDepth`) during markdown code fence stripping.
- **Embedded Fence Preservation**: Markdown code blocks (` ```bash `, ` ```rust `, ` ``` `) appearing inside quoted JSON string payloads (such as file write `content` args) are preserved intact rather than being mistakenly stripped as outer markdown fences.
- **Elimination of Parse Retries**: Prevents premature JSON envelope truncation and `"no valid JSON object detected"` parsing retries when agents author markdown guides, documentation, or code snippets containing embedded fences.

### 12. Process-Aware Git Lock Management & Rebase Resilience (`pkg/services/rebase_queue.go`)
- **Process Liveness Verification (`isProcessAlive`)**: Reads PID recorded in Git lock files (`.git/index.lock`, `.git/worktrees/*/*.lock`) and queries process status via signal 0. If the PID is dead, the stale lock is removed immediately regardless of age.
- **Race Condition Elimination**: If no PID is recorded in the lock file, `CleanStaleLocks` enforces a safe 60-second fallback age threshold (`defaultStaleLockThreshold`), eliminating the race condition where concurrent checkouts and long compilations had active index locks deleted prematurely.

Architecture, security, performance, documentation, and infrastructure concerns are explicit planner tasks implemented by generators and checked by deterministic validators. They are not independently routed agent phases.


