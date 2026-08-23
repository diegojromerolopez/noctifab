# Last-Resort Agent (Omni-Unblocker & Sovereign Repair Agent)

The **Last-Resort Agent** (*Omni-Unblocker* / *Sovereign Repair Agent*) is Noctifab's ultimate escalation mechanism. When standard specialized agents (Generator, Tester, QA, Resolver) encounter intractable blockers—such as cyclic error loops, contradictory or shallow specifications, missing sandbox toolchains, or exhausted retry budgets—the orchestrator summons the Last-Resort Agent with **sovereign compromise authority**.

Unlike standard role-isolated workers, the Last-Resort Agent operates with cross-domain authority to refactor implementation code, rewrite test suites, adapt User Story contracts, and guarantee a clean-compiling, test-passing build.

---

## 1. Architectural Philosophy: The Specialization Trap vs. Sovereign Resolution

In an autonomous dark factory harness, standard worker agents operate within rigid boundaries:
* **The Generator** writes implementation code against test contracts and cannot modify test assertions.
* **The Tester** writes test suites against specifications and cannot modify production code.
* **The Resolver** arbitrates Git merge conflicts and cannot rewrite architectural patterns.

When specifications contain subtle contradictions (e.g., a spec requiring a function signature that violates a type system invariant, or a missing binary in a hermetic container sandbox), specialized agents enter an **infinite specialization deadlock**:
1. The Generator fails because the test assertion demands an impossible invariant.
2. The Tester refuses to relax the assertion because the User Story requires it.
3. The Surgical Repair turn fails because single-line patches cannot bridge multi-file structural dissonance.
4. Retries exhaust, and the dark factory stalls.

The **Last-Resort Agent** shatters this trap by assuming sovereign, unified control over the entire workspace.

---

## 2. Relationship with the Unblocker Agent

Noctifab separates stall detection from stall remediation into two distinct agents:

```
                      ┌────────────────────────────────────────┐
                      │             UNBLOCKER AGENT            │
                      │  (Continuous Sentry & Monitoring Loop) │
                      └───────────────────┬────────────────────┘
                                          │
                    Detects Livelock / Stalls / Loops
                                          │
                                          ▼
                      ┌────────────────────────────────────────┐
                      │           COMMAND MAILBOX              │
                      │      (ResetTaskCmd / Escalation)       │
                      └───────────────────┬────────────────────┘
                                          │
                         Stall Threshold Reached (StallCount >= 4)
                                          │
                                          ▼
                      ┌────────────────────────────────────────┐
                      │           LAST-RESORT AGENT            │
                      │ (Sovereign Chief Surgeon & Solver)     │
                      │   - Rewrites production code           │
                      │   - Realigns test assertions           │
                      │   - Replaces missing dependencies      │
                      │   - Prunes deadlocked spec scope       │
                      └────────────────────────────────────────┘
```

### Division of Labor

| Dimension | Unblocker Agent (`UNBLOCKER`) | Last-Resort Agent (`LAST_RESORT`) |
| :--- | :--- | :--- |
| **Role & Persona** | Observant Monitor & Process Sentry | Hands-on Chief Surgeon & Omni-Solver |
| **Lifecycle** | Runs continuously in a background polling loop | Summoned ephemerally on blocker thresholds |
| **Write Permissions** | Command Mailbox only (`ResetTaskCmd`, `FailTaskCmd`) | Full workspace (`src/`, `tests/`, `roadmap/`, `Makefile`) |
| **Code Modification** | **None** (never touches workspace files) | **Deep modification** across multiple files in a single turn |
| **Spec Mutation** | Cannot modify specifications | Can adapt User Stories and add compromise trailers |
| **Execution Cost** | Minimal (0-token fast-path regex + cheap diagnoses) | Focused (1–2 deep multi-file LLM turns) |

---

## 3. Sovereign Permissions & Authority

When active, the Last-Resort Agent possesses explicit authority to:
1. **Atomically Synchronize Code and Tests**: Modify both production source files and test suites in the same response to resolve contract mismatches.
2. **Prune Test Assertions**: Remove redundant or contradictory test cases that assert impossible constraints.
3. **Swap External Dependencies for Standard Library Equivalents**: Replace uninstalled third-party packages with pure language standard-library implementations when container sandboxes lack specific toolchains.
4. **Adapt User Story Contracts**: Mark unresolvable secondary requirements as `[SCOPE-REDUCED]` or `[FALLBACK-IMPLEMENTED]` in `roadmap/user-stories/US-xxx.md`.
5. **Enforce Forced Compilation (Rule 7)**: Ensure that broken scaffoldings or compiler errors are replaced with clean, compiling stubs that return structured errors (`ErrNotImplemented` / 501) rather than failing builds.

### Sovereign Invariants (What the Agent Cannot Do)
* **Never Leak Secrets**: Strictly forbidden from inspecting or echoing `secrets.yaml` (Rule 8).
* **Never Break Project Agnosticism**: No validation-project-specific heuristics or language hardcodes.
* **Never Bypass Security / SAST**: Security policies (e.g. SQL injection, hardcoded credentials) remain 100% strict and non-negotiable.

---

## 4. Operating Mandates: No Cutting Corners

> [!IMPORTANT]
> **The Solution Consistency Mandate**:
> The Last-Resort Agent is authorized to compromise scope, but it **cannot cut corners**. Any simplified architecture, in-memory fallback, or standard-library replacement must be **robust, internally consistent, fully wired, and self-contained**.
>
> Generated code must strictly comply with all **`SPEC.md` Quality Invariants**:
> - **Dependency Injection (DI) & SOLID**: Dependencies must be passed explicitly via constructors.
> - **Domain-Driven Design (DDD)**: Clean domain models, value objects, and repository interfaces.
> - **File Size Limits**: No Go source file may exceed 500 lines of code.
> - **Error Handling**: Explicit wrapping (`fmt.Errorf("...: %w", err)`) and zero unhandled errors.
> - **Formatting & Linting**: Code must pass `go fmt` and linter checks cleanly.

---

## 5. The 4-Tier Compromise Hierarchy

When resolving a stalled task, the Last-Resort Agent evaluates fixes in strict hierarchical order:

```
┌─────────────────────────────────────────────────────────────┐
│ Tier 1: Interface Harmonization                             │
│ Align production signatures and test assertions without    │
│ altering business logic or user-facing contracts.          │
└──────────────────────────────┬──────────────────────────────┘
                               │ (if blocked)
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ Tier 2: Standard Library Fallback                           │
│ Replace missing external packages/tools with standard       │
│ library equivalents (e.g. pure net/http, in-memory map DB). │
└──────────────────────────────┬──────────────────────────────┘
                               │ (if blocked)
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ Tier 3: Feature Scope Pruning                               │
│ Prune deadlocked secondary requirements; annotate User      │
│ Story with [SCOPE-REDUCED] and commit working baseline.     │
└──────────────────────────────┬──────────────────────────────┘
                               │ (if blocked)
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ Tier 4: Safe Compiling Stub (Forced Compilation)            │
│ Guarantee a clean-compiling build returning structured     │
│ error (ErrNotImplemented / 501) to satisfy Rule 7.          │
└─────────────────────────────────────────────────────────────┘
```

---

## 6. Trigger Taxonomy

The Last-Resort Agent is summoned deterministically by the orchestrator:

| Trigger Code | Condition | Trigger Description |
| :--- | :--- | :--- |
| **`T1_RETRIES_EXHAUSTED`** | `task.Retries >= maxRetries - 1` | Standard Generator/Tester turns have exhausted their retry budget. |
| **`T2_UNBLOCKER_ESCALATION`** | `task.StallCount >= 4` | Unblocker sentry detects 4 consecutive stalls on the same task. |
| **`T3_MISSING_TOOLCHAIN`** | `category == FailureSandbox` | Missing compiler, package manager, or binary (Exit 127). |
| **`T4_POST_MERGE_COLLAPSE`** | Post-merge tests fail | Multi-task integration test suite fails on the integration branch. |
| **`T5_QA_GATE_DEADLOCK`** | `qaBlocked != ""` | QA acceptance review encounters persistent contract mismatch. |

---

## 7. Configuration Schema (`.noctifab/config.yaml`)

The Last-Resort Agent is enabled by default with zero configuration required.

```yaml
agents:
  last_resort:
    # Enabled activates the sovereign repair agent (default: true)
    enabled: true
    # Model override for deep reasoning (empty uses default LLM model)
    model: ""
    # Low temperature for deterministic, disciplined code edits (default: 0.1)
    temperature: 0.1
    # Prioritized provider fallback chain (identical to generator/planner)
    providers:
      - name: anthropic-deep
        models: ["claude-3-7-sonnet", "claude-3-5-sonnet-latest"]
      - name: openai-deep
        model: o3-mini
      - name: deepseek-local
        model: deepseek-reasoner
    # Maximum sovereign repair turns per task (default: 2)
    max_turns: 2
    # Execution timeout per turn (default: 180s)
    timeout: 180s
    # Allow adapting roadmap/user-stories/US-xxx.md (default: true)
    allow_spec_mutation: true
    # Allow pruning deadlocked secondary requirements (default: true)
    allow_scope_reduction: true
    # Enforce SPEC.md quality and linter checks on compromises (default: true)
    enforce_spec_quality: true

unblocker:
  last_resort_triggers:
    # Trigger on retry budget exhaustion (default: true)
    retries_exhaustion: true
    # Trigger on cyclic error/diff loops (default: true)
    cyclic_loop_detection: true
    # Fast abort on missing sandbox toolchains (default: true)
    missing_toolchain_fast_abort: true
    # QA gate deadlock turn threshold (default: 2)
    qa_deadlock_turns: 2
    # Watchdog timeout threshold (default: 2)
    watchdog_timeout_turns: 2
    # Stall count escalation threshold (default: 4)
    stall_count_threshold: 4
```

---

## 8. Critical Trigger Alerting & Dual Logging (Database & Logs)

> [!WARNING]
> **Triggering the Last-Resort Agent is a Worrying Event**:
> Summoning the Last-Resort Agent means standard specialized workflows (Generator, Tester, QA, Resolver) were unable to converge or encountered an intractable deadlock. It represents an exceptional event in the Dark Factory lifecycle.

To ensure full transparency and auditability, every triggering of the Last-Resort Agent is prominently recorded across all observability layers:

### 1. Dual Log Alerting (`stdout` & `stderr`)
* **Loud Alert Header**: When summoned, Noctifab immediately logs a prominent critical alert to both standard output and standard error:
  ```text
  🚨 [CRITICAL ALERT] Last-Resort Agent triggered for task <TASK_ID> (Reason: <TRIGGER_REASON>)!
  ```
* **Turn Progress & Diagnosis**: Logs every sovereign repair turn, the active compromise tier evaluated, and the diagnostic failure summary.
* **Terminal Outcome Alert**: Logs a final success or critical failure alert (`🚨 [CRITICAL ALERT] Last-Resort Agent completed N turns without resolving task...`).

### 2. State Database Persistence (SQLite & PostgreSQL)
* **`State.LastActions` Audit Trail**: The trigger event is immediately appended to the state database as an action record:
  * `tool`: `"last_resort_agent_trigger"`
  * `reasoning`: `"CRITICAL: Summoned Last-Resort Agent for task <ID> due to <TRIGGER_REASON>"`
  * `result`: Error trace snippet that triggered the escalation.
  * `success`: `false`
* **Outcome Recording**: On completion, either `last_resort_agent_success` or `last_resort_agent_failed` is appended to `State.LastActions` and saved to disk/database.
* **Active Agent State**: The agent's invocation is registered in `State.ActiveAgents` under role `LAST_RESORT` (`domain.AgentRoleLastResort`) with precise start and completion timestamps.

### 3. Execution Report & Telemetry Stream
* **Finding Event**: Emits a `domain.EventFindingRecorded` event with category `CRITICAL_LAST_RESORT_TRIGGERED`.
* **Markdown Report**: The execution report (`.noctifab/reports/<TIMESTAMP>_<PROJECT>.md`) surfaces the Last-Resort trigger in its executive summary and issue breakdown tables.

---

## 9. Post-Completion Handoff & Downstream Lifecycle

When the Last-Resort Agent successfully resolves a blocked task:
1. **Standardized Git Tagging**: Commits use the prefix `fix(lra): sovereign unblock for task <ID> - <Title> [turn X/Y]`.
2. **State Record Update**: The task is marked as `COMPLETED`, its `StallCount` is reset to 0, and the unblock trail is captured.
3. **Rebase Queue & Merge**: The repaired worktree branch is submitted to the Rebase Queue for merge into the integration branch.

