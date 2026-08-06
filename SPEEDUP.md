# SPEEDUP.md: Acceleration Strategies & Proposals for noctifab

This document outlines concrete architectural and configuration proposals to accelerate the autonomous development throughput of `noctifab` by **5x–10x**, derived from live analysis of dark factory validation runs.

---

## 1. Overview & Acceleration Matrix

| Proposal | Target Area | Estimated Speedup | Implementation Effort | Status |
| :--- | :--- | :---: | :---: | :---: |
| **1. Parallel DAG Worker Pools** | Task Execution | **2.5x–3.5x** | Medium | **[DONE] ✅** |
| **2. Tiered LLM Provider Routing** | LLM Latency | **2.0x–3.0x** | Low | **[DONE] ✅** |
| **3. Parallel 3x Majority-Vote Validation** | Test Verification | **3.0x** | Low | **[DONE] ✅** |
| **4. Pre-baked Validation Base Images** | Container Setup | **1.5x** | Low | **[DONE] ✅** |
| **5. Spec-Level Deterministic Mock Clocks** | Retry Avoidance | **2.0x** | Low | **[DONE] ✅** |
| **6. Native JSON Schema & Parameter Sanitization** | LLM Transport | **1.5x** | Low | **[DONE] ✅** |
| **7. Implicit Orchestrator Verification** | Pipeline Flow | **1.3x** | Low | **[DONE] ✅** |
| **8. Aggressive Prompt History Pruning** | Prompt Tokens | **1.4x** | Medium | **[DONE] ✅** |
| **9. Speculative Next-Task Prefetching on `noop`** | Transition Delay | **1.5x** | Medium | **[PLANNED] ⏳** |

> [!NOTE]
> **Non-Exclusivity Principle**: None of these proposals are mutually exclusive. They are **additive and complementary**. Combining them forms a continuous **End-to-End Pipelined Architecture**.

---

## 2. Detailed Proposals

### Proposal 1: Parallel DAG Task Worker Pools [DONE] ✅
* **Current Bottleneck**: `noctifab` executes tasks sequentially (1 worker thread at a time), even when DAG tasks have no mutual dependencies.
* **Architecture Solution**:
  * Enable multi-worker topological dispatch (`scheduler.max_parallel_workers > 1`).
  * Each ready DAG node is allocated an isolated Git worktree under `.noctifab/worktrees/task-<id>` and runs concurrently.
  * Passed worker branches merge asynchronously back into `main` using serialized rebase queue (`pkg/usecase/rebase_queue.go`).

### Proposal 2: Tiered LLM Provider Model Routing [DONE] ✅
* **Current Bottleneck**: All agent operations (including minor syntax fixes, status checks, and linter resolution) route through high-latency deep reasoning models (`~15s–30s` per call).
* **Architecture Solution**:
  * **Planning & Spec Decomposition**: Route to deep reasoning models (e.g., `gpt-4o`, `claude-3-5-sonnet`).
  * **Code Generators & Fixers**: Route to high-throughput, low-latency coding models (e.g., `qwen-2.5-coder-32b`, `claude-3-5-haiku`, `gpt-4o-mini`).
  * Expected latency reduction per turn: **30s ➔ <3s**.

### Proposal 3: Parallel 3x Majority-Vote Test Validation [DONE] ✅
* **Current Bottleneck**: To eliminate flaky AI tests, `test_validator` executes `make test` 3 times sequentially, taking ~15 seconds per task verification pass.
* **Architecture Solution**:
  * Dispatch the 3 test runs concurrently in parallel goroutines (`go testRunWorker(...)`).
  * Collect majority-vote pass/fail results via Go channels.
  * Reduces validation pass latency from **~15s down to ~3s**.

### Proposal 4: Pre-bake Common Test Dependencies in Base Images [DONE] ✅
* **Current Bottleneck**: Validation containers frequently hit test runner retries when required libraries (e.g. `pytest-asyncio`, `httpx`, `coverage`) are missing from base python images, forcing on-the-fly file creation or pip installs.
* **Architecture Solution**:
  * Pre-install language test runners in `Dockerfile.validation` and per-language base images (`python:3.14-alpine`, `golang:1.25-alpine`).
  * Eliminates dynamic dependency installation and configuration loops inside validation runs.

### Proposal 5: Spec Invariants for Deterministic Mock Clocks [DONE] ✅
* **Current Bottleneck**: Tasks with time, TTL, or expiration logic frequently experience 1-second wall-clock boundary assertion failures (`assert 19 == 20`), triggering multi-turn retry loops.
* **Architecture Solution**:
  * Enforce mock clock patterns (`Store(clock=FakeClock())`) at the Product Manager spec decomposition layer (`US-xxx.md`).
  * Ensures tests written on attempt 1 are 100% deterministic and pass verification immediately.

### Proposal 6: Native JSON Schema Enforcement & Parameter Sanitization [DONE] ✅
* **Runtime Issue**: LLM endpoints return unformatted markdown or fail on invalid parameters (`max_tokens`, `temperature`), causing 1-shot retry loops.
* **Solution**: Enforce native `response_format: {"type": "json_object"}` in `pkg/infrastructure/llm/` and sanitize request parameters upfront.

### Proposal 7: Implicit Orchestrator Verification [DONE] ✅
* **Runtime Issue**: Generators returning `noop` without calling `run_tests` trigger extra roundtrips.
* **Solution**: Automatically trigger `run_tests` implicitly upon file modification, accepting passes immediately.

### Proposal 8: Aggressive Prompt History Pruning on Retries [DONE] ✅
* **Runtime Issue**: Prompt history grows to `>49KB` on retries, slowing LLM inference.
* **Solution**: On retry turns, pass only target source code + latest test error traceback, discarding intermediate tool history via Suffix-Only pruning.

### Proposal 9: Speculative Next-Task Prefetching on `noop` [PLANNED] ⏳
* **Runtime Issue**: Serial handoff between task completion and next task dispatch causes idle waiting.
* **Solution**: When Task N issues `noop`, pre-fetch context and prepare Task N+1 in parallel while Task N's 3x verification runs.

---

## 3. The Fully Pipelined Dark Factory Model

By combining all 9 proposals, `noctifab` functions as a **continuous, assembly-line pipeline**:

```text
[SPEC.md]
   │
   ▼ (Tiered LLM: Deep Model)
[Product Manager Story Decomposition]
   │
   ▼
[Topological Task DAG Dispatcher] ──┐
   │                                │ (Parallel Workers)
   ▼                                ▼
[Worker 1: Worktree A]         [Worker 2: Worktree B]
   │                                │
   ▼ (Tiered LLM: Fast Coder)       ▼ (Tiered LLM: Fast Coder)
[Generator: Code Generation]   [Generator: Code Generation]
   │                                │
   ▼ (Implicit Verification)        ▼ (Implicit Verification)
[Parallel 3x Test Runner]      [Parallel 3x Test Runner]
   │ (Speculative Prefetch)         │ (Speculative Prefetch)
   ▼                                ▼
[Async Rebase Merge Queue]     [Async Rebase Merge Queue]
   │                                │
   └────────────────┬───────────────┘
                    ▼
           [Main Clean Branch]
```

### Key Benefits of Full Pipelining:
1. **Zero Idle CPU/GPU Time**: LLM inference, test runner verification, context prefetching, and Git branch rebasing happen concurrently across worker goroutines.
2. **Pipelined Handoff**: Warm file contexts are passed directly to downstream tasks, eliminating redundant inspection steps.
3. **Compound Speedup**: Combining all 9 proposals yields an overall throughput increase of **5x–10x**, completing full multi-story projects in minutes rather than hours.
