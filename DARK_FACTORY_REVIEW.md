# Dark Factory Code Generation Review: Speed, Architectural Completeness & Reliability

## 1. Executive Summary

This document reviews the codebase of `noctifab` with a single focus: **transforming `noctifab` into an ultra-fast, autonomous dark factory agent capable of ingesting software specifications or user stories and generating high-quality, fully tested code as fast as possible.**

While `noctifab` provides a solid foundation with state management, DAG-based task scheduling, git worktree isolation, multi-provider LLM failover, and watchdog repair, achieving maximum generation speed and reliability requires addressing critical architectural missing pieces, throughput bottlenecks, and validation harnesses.

---

## 2. Missing Pieces

To achieve maximum execution speed and end-to-end autonomy when generating code from specs/user stories, the following core components are missing:

### 2.1. Warm Container & Build Cache Persistence
* **Current State:** Sandbox execution ([`pkg/services/sandbox.go`](file:///Users/diegoj/repos/noctifab/pkg/services/sandbox.go)) creates isolated CLI environments, but test commands (`go test`, `npm test`, `cargo test`, `gcc`) recompile dependencies from scratch on every retry cycle.
* **Proposal:** Mount persistent host build cache volumes (`/go/pkg/mod`, `~/.cache/go-build`, `~/.cargo/registry`, `~/.npm`) directly into Docker container sandboxes in `pkg/services/sandbox.go` so incremental builds complete in milliseconds.

---

## 3. Improvements & Bottleneck Solutions

### 3.1. Task Decomposition Cohesion (Planner Prompt Optimization)
* **Identified Bottleneck:** The Planner Agent frequently splits tasks too granularly (e.g., Task 1: interface definition, Task 2: JSON repository implementation). When Tester runs on Task 1, it writes unit tests for an un-implemented interface, causing test compilation failures or redundant mock generation.
* **Improvement:**
  * Update Planner Agent prompt guidelines to mandate **Task Cohesion**: Interface definitions and their primary persistence implementation (or functional memory mock) MUST be grouped in the same task.
  * Add a pre-execution DAG validation rule that checks if interface-only tasks exist without implementation targets.

### 3.2. Fine-Grained Thread-Safe Locking & Parallel Worktree Synchronization
* **Identified Bottleneck:** When running multiple parallel workers (`concurrency > 1`), workers attempt to update state simultaneously in SQLite/Postgres. Global state table locking causes high Optimistic Concurrency Control (OCC) retries and backoff sleeps ([`pkg/services/orchestrator.go`](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator.go)).
* **Safety & Locking Rule:** Go mutexes and database locks are **absolutely essential** in concurrent environments to prevent data races, corrupted state, and memory corruption. Not using locks in Go is dangerous.
* **Proposal & Improvement:**
  * Maintain strict `sync.Mutex` and `sync.RWMutex` protection on all shared Go structures (such as `RebaseQueue`, `CommandMailbox`, and `FileLockRegistry`).
  * Replace global state table locking with **fine-grained per-task row locks** (`UPDATE tasks SET status = ... WHERE id = ...`), allowing parallel worker goroutines to update distinct tasks concurrently without blocking each other.
  * Maintain persistent per-worker Git worktrees instead of creating and pruning worktrees on every retry attempt.

### 3.3. Reader Phase Visibility & Granular Audit Logs
* **Identified Bottleneck:** During `RunReaderPhase` ([`pkg/services/orchestrator_helper.go`](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator_helper.go)), log outputs only show `Orchestrator: [Reader] phase ok for role tester: actions=7` without indicating which files or tools were accessed.
* **Improvement:** Log each individual reader action (e.g. `[Reader] executing read_file path=internal/task/repository.go`).

### 3.4. Smart Error Feedback & Instructive Linter Directives
* **Identified Bottleneck:** Generator retries after failing build/test runs currently dump raw error output into prompts.
* **Improvement:** Filter and format test/build errors with structured diffs and instructive suggestions (e.g. extract exact line numbers and failing assertion lines) to raise the LLM's first-try fix rate to >90%.

---

## 4. Validations

To guarantee system reliability, speed targets, and regression prevention, the following validation suites and testing harnesses must be implemented:

### 4.1. Speed & Latency Benchmark Suite
* **Objective:** Verify dark factory generation speed under realistic specifications.
* **Validation Standard:**
  * **Micro Spec Benchmark:** A standard 3-story CLI specification (`todo-cli`) must complete full execution (planning, test generation, code generation, validation, git merge) in **< 3 minutes**.
  * **Medium Spec Benchmark:** A 10-story HTTP API service must complete full execution in **< 10 minutes**.
* **Metric Tracked:** Time To First Commit (TTFC), total execution time, total LLM API calls, total retry rate.

### 4.2. Stalling & Failover Resiliency Test Harness
* **Objective:** Ensure the orchestrator never hangs when an LLM provider drops connection, times out, or produces invalid output loops.
* **Validation Test:** A mock LLM provider ([`tests/e2e/mock_llm`](file:///Users/diegoj/repos/noctifab/tests/e2e/mock_llm)) simulates:
  1. Complete socket silence (hanging connection).
  2. Infinite JSON repetition loop.
  3. 500 Server Errors.
* **Expected Result:** The orchestrator times out within 45 seconds, switches seamlessly to failover provider, and completes the task without process stalls or goroutine leaks.

### 4.3. Multi-Worker Parallel Worktree Stress Test
* **Objective:** Validate concurrent execution safety and Git worktree isolation.
* **Validation Test:** Run `concurrency = 4` on a DAG with 8 independent tasks.
* **Expected Result:**
  * All 4 workers execute tasks simultaneously in separate worktree directories without workspace contamination.
  * Zero OCC state corruption or deadlocks in SQLite/Postgres persistence.
  * Final integration branch contains clean commits from all tasks.

### 4.4. Task Cohesion & Mock Bleed Verification
* **Objective:** Prevent orphan test mocks and fragmented task scheduling.
* **Validation Test:** Unit test `planner.go` output against standard project specifications to assert that every task definition pairs domain types/interfaces with their corresponding implementation files in `target_files`.

### 4.5. Polyglot Target Project Matrix
* **Objective:** Ensure `noctifab` dark factory capabilities remain language-agnostic across compiled, interpreted, and low-level stacks.
* **Validation Matrix:** E2E containerized validation runs across 5 target stacks:
  1. **Go:** Standard CLI app (`todo-cli`, `echo`) with `go test` & `golangci-lint`.
  2. **TypeScript / Node.js:** Express API (`frontpunch`) with `vitest` / `jest` & `eslint`.
  3. **Python:** FastAPI microservice with `pytest` & `ruff`.
  4. **Rust:** Utility binary (`wc`) with `cargo test` & `clippy`.
  5. **C (ISO ANSI C17):** Fortune quote generator binary ([`validation/projects/fortune/SPEC.md`](file:///Users/diegoj/repos/noctifab/validation/projects/fortune/SPEC.md)) with GCC strict rules (`gcc -Wall -Wextra -Werror -pedantic -std=c17`), SQLite C API, zero memory leaks, and `make test`.

---

## 5. Summary Table of Priority Recommendations

| Category | Item | Impact on Speed / Reliability | Priority |
|---|---|---|---|
| **Improvement** | Task Cohesion Enforcement in Planner | Prevents orphan mocks & failing test cycles due to split tasks | **P1 (High)** |
| **Missing Piece** | Persistent Warm Build Cache | Reduces test execution times from tens of seconds to milliseconds | **P1 (High)** |
| **Validation** | Speed & Latency Benchmark Suite | Empirical verification of generation speed targets (< 3 min target) | **P1 (High)** |
| **Validation** | Multi-Worker Parallel Worktree Stress Test | Ensures safe concurrent execution with concurrency > 1 | **P2 (Medium)** |
