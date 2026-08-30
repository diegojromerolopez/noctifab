# Architectural Proposal: Noctifab Multi-Loop Orchestration & Backlog Lifecycle

**Status**: Revised / Approved Design  
**Author**: Principal Software Engineer  
**Target Subsystems**: `pkg/infrastructure/config`, `cmd/noctifab/cli`, `pkg/services`, `pkg/domain`

---

## 1. Executive Summary & Architectural Critique

### 1.1 Critique of the Previous Proposal Draft
The previous draft of this proposal suffered from three fundamental architectural flaws:

1. **Conflation of Product Manager Scope with Loop Sizing:**
   The previous draft attempted to introduce `runtime.loop.max_user_stories` to slice the backlog into batches executed across distinct loops (e.g. Loop 1 handles stories 1–5, Loop 2 handles stories 6–10). This broke the fundamental domain model of Noctifab.
   - **`agents.product_manager.max_user_stories`** defines the **global maximum number of user stories the project can have in total** when the Product Manager agent decomposes `SPEC.md` into the roadmap.
   - A **loop** is not a batch slice; a loop is a **full iteration pass across all user stories in the project backlog**.

2. **Misunderstanding the Purpose of Multi-Loop Orchestration:**
   In complex specifications, initial passes (Loop 1) may result in incomplete features, missing edge-case implementations, or broken characterization assertions. **This is precisely why execution loops exist.**
   - Every loop must iterate through and attempt **all user stories**.
   - If Story $i$ is incomplete or fails in Loop 1, its state, failure logs, and workspace artifacts are preserved.
   - Loop 2 and subsequent loops revisit incomplete/failed stories, refining and repairing the implementation using accumulated state and test validator feedback until convergence or loop exhaustion.
   - Prematurely terminating a loop or aborting the entire process upon the first incomplete story defeats the iterative convergence guarantee of the factory.

3. **Configuration Ambiguity & Drift:**
   Introducing disjoint, unvalidated loop settings creates configuration conflicts. All loop settings must be strictly validated, normalized, and protected against contradictions.

---

## 2. Definitive Domain Concepts & Lifecycle

### 2.1 The Two Orthogonal Dimensions
To maintain a clean separation of concerns, the system distinguishes between **Backlog Sizing** and **Execution Iteration**:

| Dimension | Configuration Path | Domain Scope | Responsibility |
| :--- | :--- | :--- | :--- |
| **Backlog Capacity** | `agents.product_manager.max_user_stories` | **Project-Wide** | Caps the total number of user stories generated from `SPEC.md` by the Product Manager (0 = unlimited / CU-driven default). |
| **Iteration Passes** | `runtime.loops` (or `runtime.loop.count`) | **Execution Lifecycle** | The number of full passes executed over the entire story backlog to achieve complete, hardened implementations. |

```
                       ┌────────────────────────────────────────┐
                       │               SPEC.md                  │
                       └───────────────────┬────────────────────┘
                                           │
                                           ▼
                      ┌──────────────────────────────────────────┐
                      │          Product Manager Agent           │
                      │  Decomposes SPEC into Total Stories <= N │
                      │  (agents.product_manager.max_user_stories)│
                      └────────────────────┬─────────────────────┘
                                           │
                                           ▼
                      ┌──────────────────────────────────────────┐
                      │    Complete Story Backlog (US-001..US-N) │
                      └────────────────────┬─────────────────────┘
                                           │
          ┌────────────────────────────────┴────────────────────────────────┐
          │                                                                 │
          ▼                                                                 ▼
 ┌─────────────────────────────────┐                       ┌─────────────────────────────────┐
 │             LOOP 1              │                       │             LOOP 2+             │
 │  Execute Story US-001           │                       │  Re-evaluate & Refine US-001    │
 │  Execute Story US-002           │                       │  Re-evaluate & Refine US-002    │
 │  ...                            │                       │  ...                            │
 │  Execute Story US-N             │                       │  Re-evaluate & Refine US-N      │
 │  (All stories visited in pass)  │                       │  (Fixes stubs & test failures)  │
 └─────────────────────────────────┘                       └─────────────────────────────────┘
```

---

## 3. Configuration Schema & Validation Engine

### 3.1 YAML Schema Definition
The configuration schema in `pkg/infrastructure/config` supports both the top-level `runtime.loops` integer and the structured `runtime.loop` block for backward and forward compatibility:

```yaml
runtime:
  max_actions: 100
  max_duration: 2h
  max_silent_stall_duration: 30m
  max_tokens_per_story: 2000000
  max_tokens_per_task: 500000
  max_tokens: 100000000
  # Direct loop count setting
  loops: 1
  # Structured loop block (alternative)
  loop:
    count: 1

agents:
  product_manager:
    number: 1
    iterations: 2
    max_user_stories: 5   # Max user stories generated for the entire project
    passes: 3
```

### 3.2 Strict Validation Invariants (`cfg.Validate()`)
To ensure configuration consistency, the following invariants are enforced during startup:

1. **Loop Count Invariant:**
   - `runtime.loops >= 1` (or `runtime.loop.count >= 1`).
   - If both `runtime.loops` and `runtime.loop.count` are set in the YAML configuration, their values **must be equal**. Any mismatch returns a descriptive configuration error:
     ```
     invalid configuration: conflicting loop count settings: runtime.loops (2) != runtime.loop.count (3)
     ```
   - Negative or zero loop counts are rejected:
     ```
     invalid configuration: runtime.loops must be >= 1, got %d
     ```

2. **Product Manager `max_user_stories` Invariant:**
   - `agents.product_manager.max_user_stories >= 0`.
   - A value of `0` denotes **unbounded / natural CU-sizing** (the Product Manager creates as many stories as the complexity units of `SPEC.md` warrant).
   - Any value $> 0$ strictly caps the number of feature user stories created.
   - Negative values are rejected:
     ```
     invalid configuration: agents.product_manager.max_user_stories must be non-negative, got %d
     ```

3. **No Per-Loop Story Partitioning:**
   - Settings attempting to partition stories per loop (e.g. `runtime.loop.max_user_stories`) are strictly disallowed from the schema.

---

## 4. Orchestrator Multi-Loop Execution Lifecycle

### 4.1 Complete Backlog Iteration Guarantee
In `cmd/noctifab/cli/start_runner.go` and `cmd/noctifab/cli/serve.go`, every loop pass must process **100% of the discovered user stories**:

1. **Non-Fatal Story Failure Handling:**
   - If a user story fails verification or encounters an incomplete task during Loop $k$, the story status is recorded in state as `FAILED` or `INCOMPLETE`.
   - **The runner does not abort the entire loop.** It continues to execute the remaining stories in the backlog ($US_{i+1} \dots US_N$) so that all independent modules and vertical slices get implemented.

2. **Iterative Refinement in Loops $2 \dots M$:**
   - When Loop $k+1$ starts, it inspects the backlog state.
   - Already successful stories whose dependencies and contracts remain clean are verified for regression.
   - Incomplete, failed, or stubbed stories are re-opened for planning and execution. The Planner and Generators receive previous failure logs, characterization test outputs, and compiler diagnostics to implement missing logic.

3. **Exit Code Semantics:**
   - The CLI exits with code `0` (Success) if and only if **all user stories** reach `SUCCESS` (all tasks passed test validation and QA audit) by the end of the final loop.
   - If any user story remains `FAILED` or `INCOMPLETE` after all configured loops have completed, `noctifab start` exits with a non-zero code.

---

## 5. Summary of Key Configuration Invariants

| Setting | Default | Valid Range | Semantic Purpose |
| :--- | :--- | :--- | :--- |
| `runtime.loops` | `1` | `[1, 100]` | Total full-pass iteration loops across the entire story backlog. |
| `runtime.loop.count` | `1` | `[1, 100]` | Alias for `runtime.loops`. Must match `runtime.loops` if both are specified. |
| `agents.product_manager.max_user_stories` | `5` | `[0, 1000]` | Total user stories cap for the entire project ($0 = \text{unlimited}$). |
| `agents.product_manager.passes` | `2` | `[1, 10]` | Number of PM audit/refinement passes over the roadmap during generation. |

---

## 6. Implementation Strategy

1. **`pkg/infrastructure/config`**:
   - Update `RuntimeConfig` and `LoopConfig` structs with bidirectional normalization.
   - Implement `validateLoopConfig` and `validateProductManagerConfig` in `config.go`.
   - Add comprehensive unit tests in `config_validation_test.go` and `defaults_test.go`.

2. **`cmd/noctifab/cli/start_runner.go`**:
   - Ensure the story iteration loop visits all stories in scope without premature termination.
   - Record story outcome without short-circuiting subsequent backlog items in the same loop.
   - Support multi-loop story re-planning and iterative remediation.

3. **`pkg/services/roadmap_generator.go`**:
   - Pass `agents.product_manager.max_user_stories` into prompt data and enforce story count capping during roadmap creation.
