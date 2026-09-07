# Fallback Agent (Omni-Agent)

The **Fallback Agent** (*Omni-Agent* / *Autonomous Recovery & Sovereign Repair Agent*) is Noctifab's unified self-healing and escalation system. It combines continuous passive pipeline health monitoring with active sovereign multi-file repair and compromise authority.

When standard specialized agents (Product Manager, Planner, Generator, Tester, QA, Resolver) encounter deadlocks, cyclic error loops, contradictory specifications, missing sandbox toolchains, or exhausted retry budgets, the Fallback Agent intervenes to guarantee that the pipeline unblocks itself and delivers a clean-compiling, test-passing program.

---

## 1. Architectural Overview & Operating Modes

In a dark factory autonomous harness, standard worker agents operate with strictly isolated boundaries:
* **The Product Manager** parses `SPEC.md` into User Stories without editing code.
* **The Planner** decomposes stories into task DAGs.
* **The Generator** writes implementation code against test contracts and cannot edit test files.
* **The Tester** writes test suites and cannot edit production code.
* **The QA Specialist** verifies black-box acceptance criteria in isolated review workspaces.
* **The Resolver** arbitrates three-way Git merge conflicts.

When subtle contradictions arise (e.g. a test asserting a signature that conflicts with the domain model, or an uninstalled dependency stalling compilation), specialized workers can enter an **infinite specialization deadlock**.

The **Fallback Agent** eliminates this trap by operating in two synchronized tiers:

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│                               FALLBACK AGENT (OMNI-AGENT)                               │
├─────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                         │
│  [MODE 1: PASSIVE WATCHDOG & SCOPE TRIAGE]                                              │
│  ├── Continuous Sentry Polling Loop (Every poll_interval: 30s)                          │
│  ├── Live Log Tailing & Credential Sanitization (SanitizeLog)                           │
│  ├── 0-Token Fast-Path Regex Classifier (< 5ms instant CLI unblocking)                  │
│  ├── Dynamic Scope Triage (Defers US-003+ on Budget/Timeout Cliffs or High Pressure)   │
│  └── Dispatches Corrective Directives to CommandMailbox                                 │
│                                                                                         │
│  [MODE 2: ACTIVE SOVEREIGN OMNI-BUILDER]                                                │
│  ├── Summoned on StallCount >= 2, Retries Exhaustion, or Cyclic Loops                   │
│  ├── Full Workspace Tool Authority (write_files, edit_file, run_tests, apply_patch)     │
│  ├── 4-Tier Compromise Hierarchy (Harmonization, Stdlib Stubs, Scope Pruning)          │
│  ├── Atomically Synchronizes Code + Tests + User Story Contracts                        │
│  └── Guarantees Forced Compilation & Clean Test Execution (Rule 7)                      │
│                                                                                         │
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Trigger Matrix: What Triggers the Fallback Agent

The Fallback Agent is invoked automatically under specific pipeline conditions across both operating modes:

### Comprehensive Trigger Reference

| Trigger ID | Trigger Scenario | Condition / Signal | Threshold | Operating Mode |
|---|---|---|---|---|
| **TR-01** | **Interactive CLI Stdin Hang** | Task log tail matches interactive regex pattern (`[y/n]`, overwrite prompt, port binding, test watcher) | Instant (Regex match) | **Mode 1** (Fast-Path) |
| **TR-02** | **Frozen Progress** | Task status `IN_PROGRESS` with no `UpdatedAt` change | `> stall_threshold` (default: `2m`) | **Mode 1** (Passive Sentry) |
| **TR-03** | **Orphaned Task** | Task status `IN_PROGRESS` but no agent in `State.ActiveAgents` is marked `WORKING` | `> stall_threshold / 2` (default: `1m`) | **Mode 1** (Passive Sentry) |
| **TR-04** | **Agent Inconsistency** | Agent marked `WORKING` on a task that is `SUCCESS`, `FAILED`, or `PENDING` | Immediate (`0s`) | **Mode 1** (Passive Sentry) |
| **TR-05** | **Unresolved Merge Conflict** | Task status `CONFLICT_BLOCKED` without resolution | `> conflict_threshold` (default: `5m`) | **Mode 1** (Passive Sentry) |
| **TR-06** | **Budget / Timeout Cliff** | Elapsed story execution time exceeds configured ratio of `runtime.max_duration` | `> budget_cliff_ratio` (default: `50%`) | **Mode 1** (Scope Triage) |
| **TR-07** | **High Backlog Stall Pressure** | Stalls detected while $\ge 3$ user stories remain pending in the backlog | Concurrent stalls $> 0$ | **Mode 1** (Scope Triage) |
| **TR-08** | **Persistent Task Stall / Livelock** | Task reaches or exceeds configured stall count threshold | `task.StallCount >= 2` (or `triggers.stall_count_threshold`) | **Mode 2** (Sovereign Omni-Builder) |
| **TR-09** | **Retry Budget Exhaustion** | Task attempts reach maximum configured retries | `task.Retries >= task.MaxRetries` | **Mode 2** (Sovereign Omni-Builder) |
| **TR-10** | **Cyclic Compilation / Test Loop** | Task failure envelope detects identical repetitive compiler error traces | `triggers.cyclic_loop_detection: true` | **Mode 2** (Sovereign Omni-Builder) |
| **TR-11** | **Missing Sandbox Toolchain** | Test runner or build binary missing from sandbox (`FailureSandbox`) | `triggers.missing_toolchain_fast_abort: true` | **Mode 2** (Sovereign Omni-Builder) |
| **TR-12** | **QA Review Deadlock** | QA review turn fails to reach consensus after repeated cycles | `triggers.qa_deadlock_turns` (default: `2`) | **Mode 2** (Sovereign Omni-Builder) |
| **TR-13** | **Watchdog Timeout Failure** | Task test execution hangs and exhausts watchdog repair turns | `triggers.watchdog_timeout_turns` (default: `2`) | **Mode 2** (Sovereign Omni-Builder) |
| **TR-14** | **Post-Merge Integration Collapse** | Global integration test suite fails on `integrationBranch` after story task completion | Global test failure after 2 repair turns | **Mode 2** (Sovereign Omni-Builder) |
| **TR-15** | **Hard-Stop Permanent Failure** | Task stalls repeatedly even after sovereign intervention attempts | `task.StallCount >= 5` | **Mode 1** (Hard-Stop Sentry) |

---

## 3. Detailed Scenarios: What the Fallback Agent Does

### Scenario 1: Routine CLI Hangs (Stdin Prompts, Port Collisions, Watch Spinners)
* **Trigger**: **TR-01** (Task log tail matches interactive regex).
* **What Happens**:
  1. The Fallback Agent tails the live task log (`.noctifab/logs/tasks/<task_id>.log`) and runs `FastPathClassify()`.
  2. If an interactive prompt pattern is matched (e.g. `(?i)(\\?.*do you want to|overwrite\\? \\[y/n\\])`), it matches in **< 5ms** with **0 LLM token overhead**.
  3. It dispatches a `ResetTaskCmd` with a targeted recovery directive:
     - *Interactive Stdin Prompt*: Injects directive to append non-interactive flags (`-y`, `--non-interactive`, `CI=true`).
     - *Port Binding Collision*: Injects directive to use ephemeral port `:0` or kill occupied test ports.
     - *Interactive Test Watcher*: Injects directive to pass `--watchAll=false` or `--ci`.
  4. The task is reset to `PENDING`, and the worker re-executes with the directive injected into its prompt.

---

### Scenario 2: Budget Cliff & High Backlog Stall Pressure (Scope Triage)
* **Trigger**: **TR-06** (Execution elapsed $> 50\%$ of `runtime.max_duration`) or **TR-07** (Stall pressure with $\ge 3$ pending backlog stories).
* **What Happens**:
  1. The Fallback Agent determines that continuing full roadmap feature development risks failing the entire execution timeout before delivering a functional program.
  2. It dispatches `ScopeTriageCmd{KeepStories: 2, Reason: "budget cliff reached"}` to the `CommandMailbox`.
  3. `ScopeTriageCmd.Execute()`:
     - Keeps foundational walking skeleton stories (`US-001` and `US-002`) active.
     - Automatically transitions downstream stories (`US-003`, `US-004`, ...) to `DEFERRED` status.
     - Automatically transitions pending tasks belonging to deferred stories to `DEFERRED`.
     - Logs triage actions to `State.LastActions`.
  4. 100% of remaining CPU, memory, and timeout budget is dedicated to finalizing, compiling, and verifying the core working program.

---

### Scenario 3: Stalled, Orphaned, or Inconsistent Tasks
* **Trigger**: **TR-02** (`frozen_progress`), **TR-03** (`orphaned_task`), or **TR-04** (`agent_inconsistency`).
* **What Happens**:
  1. The Fallback Agent scans `detectStalls(state, now)` during its periodic loop.
  2. For `agent_inconsistency`, it immediately dispatches `ClearInconsistentAgentCmd` to free worker slot capacity.
  3. For `orphaned_task` or `frozen_progress` (< `stall_count_threshold`):
     - It scrubs sensitive credentials via `SanitizeLog()`.
     - Checks the LLM assessment cooldown (`5m` throttle per task/status).
     - If `llm_assessment: true`, queries the LLM with structured diagnostics (`unblocker_prompt.go`).
     - Dispatches `ResetTaskCmd{TaskID: t.ID, Reason: diagnosis, Directive: recoveryDirective}`.
  4. `ResetTaskCmd` increments `task.StallCount`, resets status to `PENDING`, and attaches `[STALL RECOVERY DIRECTIVE]` to the task state for the next worker attempt.

---

### Scenario 4: Persistent Stalls, Livelocks, and Retry Exhaustion (Sovereign Omni-Builder)
* **Trigger**: **TR-08** (`StallCount >= 2`), **TR-09** (`Retries >= MaxRetries`), **TR-10** (Cyclic Loops), **TR-12** (QA Deadlock), or **TR-13** (Watchdog Timeout).
* **What Happens**:
  1. The task is escalated to sovereign execution via `BypassToFallbackCmd` or direct invocation of `Orchestrator.RunFallbackAgent()`.
  2. **Prominent Log Alert**: Emits `🚨 [CRITICAL ALERT] Fallback Agent triggered for task <ID> (Reason: <reason>)!` to `stderr` and console.
  3. **Database Auditing**: Registers `fallback_agent_trigger` in `State.LastActions` and marks `task.FallbackUsed = true` and `task.LastResortUsed = true`.
  4. **Sovereign Omni-Execution Loop** (up to `agents.fallback.max_turns`, default: 2):
     - Constructs a unified, rich context block containing the root `SPEC.md`, User Story contract, full source file snapshot, test failure logs, and worktree diffs.
     - Uses the prompt template [`fallback/repair.tmpl`](../pkg/infrastructure/prompts/defaults/fallback/repair.tmpl).
     - Invokes the configured model with complete workspace tool authority (`write_files`, `edit_file`, `run_tests`, `install_package`, `apply_patch`).
  5. **4-Tier Compromise Application**:
     - **Tier 1 (Interface Harmonization)**: Fixes type mismatches, function signatures, and parameter discrepancies between code and tests.
     - **Tier 2 (Standard Library Fallback)**: Replaces uninstalled or missing external packages with pure language standard-library equivalents.
     - **Tier 3 (Scope Pruning)**: Prunes unreachable or secondary test constraints that block core functionality.
     - **Tier 4 (Forced Compiling Stub)**: Ensures broken scaffoldings compile cleanly, stubbing unimplemented edge cases with structured errors rather than breaking the build (Rule 7).
  6. **Compromise Documentation**: If scope was reduced, appends compromise trailers to the User Story (`roadmap/user-stories/US-xxx.md`):
     ```markdown
     > [!WARNING]
     > [SCOPE-REDUCED: Replaced uninstalled external library with standard library in-memory mock to pass hermetic container verification.]
     ```
  7. **Validation Consensus**: Executes `evaluator.ValidateTask()` on the repaired worktree.
  8. **Outcome Persistence**:
     - *If Tests Pass*: Appends `fallback_agent_success` to `State.LastActions`, commits the repaired code, and merges into the integration branch.
     - *If Tests Fail*: Appends `fallback_agent_failed` to `State.LastActions` and preserves detailed failure traces for human review.

---

### Scenario 5: Missing Toolchain / Sandbox Failure
* **Trigger**: **TR-11** (Missing build/test runner binary inside hermetic container sandbox).
* **What Happens**:
  1. Sandbox runner detects command failure with `executable file not found in $PATH` or exit code 127.
  2. The Fallback Agent fast-aborts repeated generator retry turns.
  3. It attempts `install_package` via available package managers (npm, pip, cargo, go install).
  4. If installation is impossible in the sandbox, it switches to in-process standard-library runners or marks the validation tool as degraded (`[Validation Degraded]`), preventing task retry loops while keeping generated code intact.

---

### Scenario 6: Hard-Stop Limit Reached (Max Stalls Exhausted)
* **Trigger**: **TR-15** (`task.StallCount >= 5`).
* **What Happens**:
  1. The Fallback Agent detects that a task has stalled 5 or more times despite automated resets and sovereign repair attempts.
  2. To prevent infinite loops and runaway LLM token expenditure, it dispatches `FailTaskCmd{TaskID: t.ID, Reason: "max stall escalations reached"}`.
  3. The task is permanently marked `FAILED`, the associated worker agent is marked `COMPLETED`, and `State.BuildStatus` is set to `BUILD_FAILING`.

---

## 4. Sovereign Permissions vs. Standard Worker Roles

The Fallback Agent possesses elevated workspace permissions compared to standard isolated worker roles:

| Action / Capability | Generator Agent | Tester Agent | QA Agent | **Fallback Agent (Omni-Agent)** |
|---|:---:|:---:|:---:|:---:|
| **Edit Production Code (`src/`)** | ✅ Yes | ❌ Forbidden | ❌ Forbidden | ✅ **Full Authority** |
| **Edit Test Code (`tests/`)** | ❌ Forbidden | ✅ Yes | ❌ Forbidden | ✅ **Full Authority** |
| **Modify Code & Tests in Same Turn** | ❌ Forbidden | ❌ Forbidden | ❌ Forbidden | ✅ **Atomic Synchronization** |
| **Adapt User Stories (`roadmap/`)** | ❌ Forbidden | ❌ Forbidden | ❌ Forbidden | ✅ **Compromise Trailers** |
| **Dispatch Mailbox Directives** | ❌ Forbidden | ❌ Forbidden | ❌ Forbidden | ✅ **Full Authority** |
| **Execute Multi-File Batch Writes** | ✅ `write_files` | ✅ `write_files` | ❌ Forbidden | ✅ `write_files` |
| **Install Sandbox Packages** | ⚠️ Restricted | ⚠️ Restricted | ❌ Forbidden | ✅ `install_package` |

---

## 5. Strict Quality Invariants (No Cutting Corners)

> [!IMPORTANT]
> **The Solution Consistency Mandate**:
> Although the Fallback Agent is authorized to compromise scope when deadlocked, it **cannot produce sloppy or broken code**. Any simplified architecture, in-memory mock, or standard-library fallback must be **robust, internally consistent, fully wired, and self-contained**.
>
> Generated code must strictly comply with all **Repository Coding Rules**:
> 1. **File Size Constraint**: No Go source file may exceed 500 lines of code.
> 2. **Dependency Injection (DI)**: Pass all configurations, objects, and clients through constructors.
> 3. **SOLID & DDD**: Adhere to single responsibility and domain-driven packaging boundaries.
> 4. **Comprehensive Error Handling**: Explicitly check, handle, and wrap all errors (`fmt.Errorf("...: %w", err)`).
> 5. **Formatting & Linting**: Code must pass `go fmt ./...` and `golangci-lint` with 0 issues.
> 6. **Secrets Safety (Rule 8)**: Never inspect, read, print, or leak `secrets.yaml` files.

---

## 6. Configuration Reference

Configure the Fallback Agent in `.noctifab/config.yaml`:

```yaml
fallback:
  # Enable passive watchdog monitoring and scope triage
  enabled: true
  
  # Frequency of passive pipeline scans (default: 30s)
  poll_interval: 30s
  
  # Maximum task resets before permanent failure (default: 5)
  max_retries: 5
  
  # How long a task can remain frozen IN_PROGRESS before stall detection (default: 2m)
  stall_threshold: 2m
  
  # How long an unresolved merge conflict waits before intervention (default: 5m)
  conflict_threshold: 5m
  
  # Enable LLM-based structured diagnostics for non-fast-path stalls (default: true)
  llm_assessment: true
  
  # Ratio of max_duration elapsed before triggering ScopeTriageCmd (default: 0.50)
  budget_cliff_ratio: 0.50
  
  # Threshold triggers that summon Mode 2 sovereign repairs
  triggers:
    retries_exhaustion: true
    cyclic_loop_detection: true
    missing_toolchain_fast_abort: true
    qa_deadlock_turns: 2
    watchdog_timeout_turns: 2
    stall_count_threshold: 2

agents:
  fallback:
    # Enable active sovereign omni-builder repairs
    enabled: true
    
    # Model to use for sovereign omni-builder turns
    model: "claude-3-7-sonnet"
    temperature: 0.1
    profile: "fallback"
    
    # Multi-turn budget for sovereign repair (default: 2)
    max_turns: 2
    
    # Timeout per sovereign repair turn (default: 180s)
    timeout: 180s
    
    # Allow updating User Story compromise trailers
    allow_spec_mutation: true
    allow_scope_reduction: true
    enforce_spec_quality: true
```

### Backwards Compatibility

Existing configuration files with `unblocker:` and `agents.last_resort:` blocks continue to function transparently via type aliases and schema accessors (`GetFallback()`).
