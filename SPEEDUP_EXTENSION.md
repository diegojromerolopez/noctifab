# SPEEDUP_REVIEW.md: Extended Code Generation Acceleration Proposals for noctifab

This document reviews `SPEEDUP.md` (Proposals 1–9) and introduces **Proposals 10–18**, derived from a comprehensive audit of the `noctifab` codebase (`pkg/services/`, `pkg/infrastructure/`, `pkg/domain/`).

Combining all 18 proposals creates an ultra-optimized, continuous Dark Factory code generation engine capable of achieving up to **10x–20x throughput speedups**.

---

## 1. Executive Summary & New Proposals Matrix

While Proposals 1–9 in `SPEEDUP.md` focus on high-level concurrency (DAG pools), LLM model routing, majority-vote test parallelism, and container pre-baking, Proposals 10–18 target **LLM KV prompt cache maximization, token output minimization, test impact filtering, daemonized worker reuse, and stream-level speculative execution**.

| Proposal | Target Area | Estimated Speedup | Implementation Effort | Status | Codebase Target |
| :--- | :--- | :---: | :---: | :---: | :--- |
| **10. KV Prompt Cache Prefix Stabilization** | LLM Ingestion Latency | **2.0x–4.0x** (80% TTFT reduction) | Low | **[DONE] ✅** | `pkg/infrastructure/llm/`, `pkg/services/orchestrator_generator.go` |
| **11. Unified Diff Multi-File Patching Tool (`apply_patch`)** | Output Generation & Turn Count | **2.5x–3.5x** | Medium | **[DONE] ✅** | `pkg/services/apply_patch_tool.go`, `validator.go` |
| **12. Selective / Incremental Test Impact Analysis (TIA)** | Intermediate Test Validation | **3.0x–5.0x** | Medium | **[PLANNED] ⏳** | `pkg/services/test_validator.go`, `sandbox_python.go`, `sandbox_docker.go` |
| **13. Hot Daemonized Test Worker Pool & Process Reuse** | Test Runner Boot Latency | **1.5x–2.0x** | Medium | **[PLANNED] ⏳** | `pkg/services/sandbox.go`, `sandbox_docker.go` |
| **14. AST-Driven Interface Extraction for Dependency Files** | Prompt Token Ingestion | **1.5x–2.0x** | Low | **[PLANNED] ⏳** | `pkg/services/context_slicer.go`, `orchestrator_execute.go` |
| **15. Generator Fast-Path Exit on Passing `run_tests`** | Generator Turn Overhead | **1.3x–1.5x** (Saves 1 turn/task) | Low | **[DONE] ✅** | `pkg/services/orchestrator_generator.go` |
| **16. Real-Time Speculative Streaming Tool Execution** | Tool Execution Pipelining | **1.3x–1.4x** | High | **[PLANNED] ⏳** | `pkg/infrastructure/llm/parser.go`, `pkg/services/orchestrator_generator.go` |
| **17. Atomic Task Granularity & Entity Mandate (PM Agent)** | Task Complexity & Retries | **2.0x** | Low | **[DONE] ✅** | `pkg/infrastructure/llm/prompt_templates.go`, `roadmap_generator.go` |
| **18. Pooled Git Worktrees & Async State Flushing** | File System & Disk I/O | **1.2x–1.4x** | Medium | **[PLANNED] ⏳** | `pkg/services/orchestrator_execute.go`, `state_repository.go` |
| **19. Interpreter/Compiler Syntax Pre-Validation** | Turn Latency & Retry Cost | **1.5x** | Low | **[DONE] ✅** | `pkg/services/validator.go`, `production_tools.go` |
| **20. Adaptive Test Coverage Gates (Diff Coverage)** | Task Merge Timeout | **2.0x** | Low | **[DONE] ✅** | `pkg/services/test_validator.go`, `validator.go` |

---

## 2. Review & Validation of SPEEDUP.md (Proposals 1–9)

The foundational proposals in `SPEEDUP.md` address key structural bottlenecks:
* **Proposals 1 & 3** (Parallel DAG & Parallel 3x Tests): Remove sequential bottlenecks in task dispatch and test verification.
* **Proposal 2** (Tiered LLM Routing): Cuts latency by sending code editing to high-speed coding models.
* **Proposals 4, 5, 6** (Pre-baked Base Images, Mock Clocks, Native JSON): Reduce environment configuration retries and schema failures.
* **Proposals 7, 8, 9** (Implicit Verification, Prompt Pruning, Speculative Prefetch): Smooth transitions between turns and tasks.

Proposals 10–20 build directly on top of these foundations to maximize per-turn efficiency and eliminate micro-latencies across the entire agent loop.

---

## 3. Detailed Proposals (Proposals 10–20)

### Proposal 10: Provider-Native KV Prompt Cache Prefix Stabilization [DONE] ✅
* **Codebase Target**: `pkg/infrastructure/llm/client.go`, `prompt_templates.go`, `pkg/services/orchestrator_generator.go`
* **Current Bottleneck**: `orchestrator_generator.go` reconstructs `currentPrompt` on every turn by placing dynamic parameters (`turn` index, remaining turns, changing tool outputs) near the top or middle of the prompt. This continuously alters the prompt prefix, causing LLM provider Key-Value (KV) Prompt Caching (OpenAI Prompt Caching, Anthropic Context Caching, DeepSeek Context Caching, Gemini Context Caching) to miss on every turn.
* **Architecture Solution**:
  1. Re-architect prompts into a strict 3-tier structure:
     - **Static Prefix (Cached)**: System instructions, tool definitions, architecture guidelines, and immutable repository rules (`AGENTS.md`, `SPEC.md`).
     - **Semi-Static Block (Cached per Task)**: Task title, description, target files, and dependency signatures.
     - **Dynamic Tail (Uncached)**: Tool outputs from the current turn and turn counters appended strictly at the end of the user message.
  2. Maintain identical prompt prefixes across all turns in a task execution.
* **Impact**: **2.0x–4.0x TTFT reduction**; cuts cost by up to 90% on KV-cache enabled providers.

---

### Proposal 11: Fast-Path Direct Multi-File Patching (`apply_patch` / Unified Diff Tool) [DONE] ✅
* **Codebase Target**: `pkg/services/production_tools.go` (`WriteFileTool`, `EditFileTool`, `MultiReplaceFileContentTool`)
* **Current Bottleneck**: `WriteFileTool` requires the LLM to rewrite entire 300+ line files to modify 5 lines, generating thousands of slow output tokens. `EditFileTool` requires rigid JSON representations (`ReplacementChunk`) which frequently suffer formatting/escaping retries. Furthermore, modifying 3 separate files takes 3 sequential LLM turns.
* **Architecture Solution**:
  1. Add an `apply_patch` tool accepting standard unified diff format (`diff -u` / Git patch format) supporting multi-file edits in a single payload.
  2. Implement fuzzy hunk matching (`pkg/services/production_tools.go`) to gracefully handle slight line offset shifts.
* **Impact**: **2.5x–3.5x speedup** on code generation turns by reducing output token count from ~2,000 to ~50 tokens and multi-file turns from 3 to 1.

---

### Proposal 12: Selective / Incremental Test Impact Analysis (TIA) [PLANNED] ⏳
* **Codebase Target**: `pkg/services/test_validator.go`, `validator.go`, `sandbox_docker.go`, `sandbox_python.go`
* **Current Bottleneck**: Every call to `run_tests` during intermediate generator loops executes the entire test suite (`make test` or `go test ./...` or `pytest`), running tens or hundreds of unrelated tests for 15s–30s per turn.
* **Architecture Solution**:
  1. Trace modified source files against the test suite (e.g. mapping `pkg/services/orchestrator.go` -> `pkg/services/orchestrator_test.go`).
  2. During intermediate generator turns (turns 1 to N-1), execute **only affected unit tests**.
  3. Reserve full test suite execution for final task verification (Turn N / 3x majority-vote).
* **Impact**: Reduces turn test latency from **15s–30s down to ~1s** (**3.0x–5.0x speedup** on iteration loops).

---

### Proposal 13: Hot Daemonized Test Worker Pool & Process Reuse [PLANNED] ⏳
* **Codebase Target**: `pkg/services/sandbox.go`, `sandbox_docker.go`, `sandbox_python.go`
* **Current Bottleneck**: Calling `run_tests` or `run_linter` spawns new OS subprocesses or Docker containers from scratch every time, incurring process instantiation overhead (1s–5s per call).
* **Architecture Solution**:
  1. Maintain a persistent background worker process daemon (e.g., `pytest-xdist` socket server, pre-warmed `go test` build cache daemon, or a warm Docker container process with Unix socket IPC).
  2. Send test execution requests over fast local sockets instead of re-spawning process environments.
* **Impact**: **1.5x–2.0x speedup** on test execution overhead.

---

### Proposal 14: AST-Driven Interface Extraction for Dependency Files [PLANNED] ⏳
* **Codebase Target**: `pkg/services/context_slicer.go`, `orchestrator_execute.go` (`collectTargetFilesRecursively`)
* **Current Bottleneck**: `collectTargetFilesRecursively` pulls raw file contents of all upstream target files into `fileContexts`, adding thousands of implementation lines into the LLM prompt context window.
* **Architecture Solution**:
  1. For files that are **direct target files** of the current task, provide full raw content (or diff window).
  2. For files that are **inherited dependencies**, extract only AST symbol headers, struct definitions, exported function signatures, and interface contracts using Tree-Sitter / Go AST parser.
* **Impact**: **1.5x–2.0x speedup** on prompt ingestion by stripping 80% of irrelevant implementation details from non-edited dependency files.

---

### Proposal 15: Generator Fast-Path Exit on Passing `run_tests` [DONE] ✅
* **Codebase Target**: `pkg/services/orchestrator_generator.go` (lines 71–193)
* **Current Bottleneck**: When `run_tests` completes successfully in a generator turn, the orchestrator appends the passing output to the prompt and queries the LLM for another turn just so the model can return `noop`.
* **Architecture Solution**:
  1. In `orchestrator_generator.go`, if `run_tests` executes successfully (0 test failures, 100% pass) and no further file modifications are requested in the same turn, automatically mark the generator phase as complete and exit the turn loop immediately.
* **Impact**: Eliminates 1 trailing LLM turn per task (**1.3x–1.5x turn reduction**).

---

### Proposal 16: Real-Time Speculative Streaming Tool Execution [PLANNED] ⏳
* **Codebase Target**: `pkg/infrastructure/llm/parser.go`, `pkg/services/orchestrator_generator.go`
* **Current Bottleneck**: The orchestrator waits for the complete LLM HTTP response stream to finish before parsing actions and executing tools.
* **Architecture Solution**:
  1. Stream JSON response tokens into a streaming JSON parser (`pkg/infrastructure/llm/parser.go`).
  2. As soon as a read-only or inspection tool call (e.g., `read_file`, `search_code`) is parsed, launch its execution asynchronously before the model finishes generating trailing reasoning text.
* **Impact**: **1.3x–1.4x turn latency speedup** by overlapping tool execution with LLM token streaming.

---

### Proposal 17: Atomic Task Granularity & Entity Mandate (PM Agent) [DONE] ✅
* **Codebase Target**: `pkg/infrastructure/llm/prompt_templates.go`, `pkg/services/roadmap_generator.go`
* **Current Bottleneck**: When the Product Manager agent creates broad multi-responsibility tasks or separate "test-only" tasks, generator agents require 8–15 turns with high failure and retry rates.
* **Architecture Solution**:
  1. Update Product Manager system prompts to enforce strict task atomicity and entity: **NO test-only tasks are allowed**. Every task MUST have concrete functionality entity alongside its co-located unit tests.
  2. Enforce explicit DoD (Definition of Done) invariants so tasks have zero ambiguity.
* **Impact**: **2.0x overall project speedup** due to higher DAG concurrency, zero test-only task clutter, and near-zero task retries.

---

### Proposal 18: Pooled Git Worktrees & Asynchronous State Flushing [PLANNED] ⏳
* **Codebase Target**: `pkg/services/orchestrator_execute.go` (lines 123–144), `pkg/domain/state_repository.go`
* **Current Bottleneck**: Creating and destroying Git worktrees via shell commands on every task and synchronously persisting state JSON files on every status update introduces disk I/O bottlenecks.
* **Architecture Solution**:
  1. Maintain a pool of pre-created Git worktrees under `.noctifab/worktrees/pool-*`.
  2. Implement an in-memory state repository with asynchronous write-behind disk flushing for state transitions.
* **Impact**: **1.2x–1.4x speedup** on file system operations during concurrent DAG execution.

---

### Proposal 19: Interpreter/Compiler In-Process Syntax Pre-Validation [DONE] ✅
* **Codebase Target**: `pkg/services/validator.go`, `pkg/services/production_tools.go`
* **Current Bottleneck**: When an agent introduces a syntax error, launching the full test suite container incurs a 15s–30s process penalty per attempt.
* **Architecture Solution**:
  1. Run the target language's interpreter/compiler syntax check (e.g. `python -m py_compile` / `go vet`) in-process or via lightweight CLI check prior to invoking full test runners.
  2. Fail invalid syntax immediately (0.01s) before wasting test sandbox execution turns.
* **Impact**: Saves 15–30 seconds per syntax error turn.

---

### Proposal 20: Adaptive Test Coverage Gates (Diff Coverage vs. Absolute Gate) [DONE] ✅
* **Codebase Target**: `pkg/services/test_validator.go`, `validator.go`
* **Current Bottleneck**: Forcing a strict 95.0% absolute project-wide coverage gate causes agents to spend 20+ minutes in 10-attempt retry loops chasing 1.7% fractional coverage gains.
* **Architecture Solution**:
  1. Measure coverage adaptively using Diff Coverage (coverage on modified lines for intermediate task merging).
  2. Reserve full project-wide coverage gates for final project verification.
* **Impact**: Eliminates 10-attempt retry timeouts on intermediate tasks.

---

## 4. Master Acceleration Architecture (Proposals 1–18 Synergy)

When Proposals 1–9 and Proposals 10–18 are combined, `noctifab` becomes a fully pipelined, zero-latency dark factory:

```text
                                [SPEC.md]
                                   │
                                   ▼ (Proposal 17: Atomic PM Decomposition)
                        [Topological DAG Dispatcher]
                                   │
                ┌──────────────────┴──────────────────┐
                ▼ (Proposal 1: Parallel Pools)       ▼ (Proposal 18: Pooled Worktrees)
     [Worker 1: Worktree A]                     [Worker 2: Worktree B]
                │                                     │
                ▼ (Proposal 2: Tiered LLM)            ▼ (Proposal 10: KV Prompt Cache)
     [Fast Coding Model]                         [Fast Coding Model]
                │                                     │
                ▼ (Proposal 11: `apply_patch`)        ▼ (Proposal 14: AST Interfaces)
     [Multi-File Patching]                      [Multi-File Patching]
                │                                     │
                ▼ (Proposal 12: Incremental TIA)      ▼ (Proposal 13: Hot Test Worker)
     [Fast Unit Test Run]                       [Fast Unit Test Run]
                │                                     │
                ▼ (Proposal 15: Fast Exit on Pass)    ▼ (Proposal 16: Speculative Streaming)
     [Parallel 3x Final Vote] (Proposal 3)      [Parallel 3x Final Vote] (Proposal 3)
                │                                     │
                └──────────────────┬──────────────────┘
                                   ▼ (Proposal 9 & 18: Async Rebase Queue)
                          [Main Branch Commit]
```

### Combined Performance Gain Summary:
* **LLM Ingestion & Processing**: 80% lower TTFT via KV Prompt Cache stabilization (Prop 10) + AST interface extraction (Prop 14).
* **LLM Output Generation**: 5x–10x faster edits via unified diff patching (`apply_patch`) (Prop 11).
* **Agent Turn Efficiency**: 1-turn savings per task via fast-path exit (Prop 15) + real-time speculative streaming (Prop 16).
* **Verification Overhead**: 10x faster intermediate test loops via Test Impact Analysis (Prop 12) + daemonized worker pool (Prop 13).
* **Overall Dark Factory Speedup**: **10x–20x continuous throughput improvement**.

---

## 5. Compatibility, Conflict Resolution & Extensibility Enhancements

While Proposals 1–9 and Proposals 10–18 are designed to be complementary, naive implementations of Proposals 1–9 create **3 subtle architectural friction points**. Modifying Proposals 1, 7, and 8 in `SPEEDUP.md` ensures full forward-compatibility and extensibility:

### 1. Proposal 1 Friction: Worktree Lifecycle vs. Daemon & Build Cache Retention (Props 13 & 18)
* **Conflict**: Proposal 1 suggests creating and destroying transient Git worktrees (`.noctifab/worktrees/task-<id>`) on-the-fly via `git worktree add` and `git worktree remove --force`. Deleting worktrees destroys hot build caches (`.venv`, `node_modules`, `go build` cache) and background worker daemon sockets required by Proposals 13 & 18.
* **Extensibility Fix for Proposal 1**: Change Proposal 1 to use **Pooled Persistent Worktree Slots** (`.noctifab/worktrees/pool-worker-1`, `pool-worker-2`). Between tasks, workers reset their branch state (`git checkout -B integrationBranch`) rather than deleting the worktree directory, preserving hot build artifacts and test daemon connections.

### 2. Proposal 8 Friction: Retry History Pruning vs. KV Prompt Cache Invariance (Prop 10)
* **Conflict**: Proposal 8 suggests "discarding intermediate tool history and passing only target source code + latest test traceback" on retries. Completely altering the prompt structure on turn 2 changes the prompt prefix and **invalidates the LLM Provider KV Prompt Cache** established in turn 1.
* **Extensibility Fix for Proposal 8**: Enforce **Suffix-Only Pruning**. Keep the static system instructions, tool schema, and task context prefix 100% identical across all turns, and perform history truncation strictly within the dynamic tail of the user turn message.

### 3. Proposal 7 Friction: Implicit Test Verification vs. Selective Test Impact Analysis (Prop 12)
* **Conflict**: Proposal 7 automatically triggers `run_tests` implicitly upon file modification. If `run_tests` always runs the full repository test suite, implicit execution creates heavy CPU latency per turn.
* **Extensibility Fix for Proposal 7**: Explicitly delineate **Intermediate Implicit Testing** from **Final Verification Testing**. Intermediate implicit checks use **Selective Test Impact Analysis (TIA)** (Prop 12) to test only modified files, while final task completion triggers the full 3x Parallel Majority Vote suite (Prop 3).

