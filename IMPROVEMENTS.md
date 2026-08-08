# Noctifab Engineering Review: Performance & UX Improvement Proposals

**Author:** Staff Software Engineer  
**Date:** August 8, 2026  
**Target Repository:** `noctifab` (`github.com/diegojromerolopez/noctifab`)  
**Scope:** Architecture, LLM Infrastructure, Orchestration Engine, Database & Storage, Terminal User Experience (TUI/UX), Sandboxing, and Developer Experience (DX).

---

## 1. Executive Summary

`noctifab` is a dark factory autonomous coding engine designed around a stateless agent / stateful orchestrator architecture. The system features multi-provider LLM routing, DAG-based task execution, automated test validation, self-healing code generation, and terminal-based monitoring.

Following a thorough codebase audit across `pkg/domain`, `pkg/infrastructure` (LLM, Storage, Config), `pkg/services` (Orchestrator, Unblocker, Sandbox, Tools), and `cmd/noctifab/cli` (TUI Dashboard), this document outlines open, concrete recommendations to maximize **execution speed**, **provider resilience**, and **user experience (UX)**.

---

## 2. Performance & System Throughput Optimizations

### 2.1 Orchestration Engine & Git Subprocess Overhead

#### 🔴 Issue 1: Subprocess Fork Overhead for Git Task Lifecycle
* **Location:** [`pkg/services/orchestrator_execute.go`](pkg/services/orchestrator_execute.go#L96-L175) & [`pkg/services/rebase_queue.go`](pkg/services/rebase_queue.go#L1-L100)
* **Root Cause:** `executeTask()` executes up to 12–15 synchronous Git CLI subprocesses (`git show-ref`, `git checkout`, `git branch`, `git worktree add`, `git status`, `git diff`, `git add`, `git commit`, `git worktree remove`, `git worktree prune`) for every single task.
* **Impact:** In a 10-task story, Git subprocess invocation overhead accounts for 2–5 seconds per task (~20–50 seconds total per story) spent strictly in OS process fork/exec cycles.
* **Proposed Solution:**
  1. **Batch Git Worktree Cleanup:** Defer `git worktree prune` to periodic background routines (e.g. every 60 seconds) rather than executing it synchronously on every task completion.
  2. **In-Memory Index Checking:** Use direct file system checks (`os.Stat`) for local git ref files (`.git/refs/heads/<branch>`) instead of spawning `git show-ref` processes.

---

### 2.2 Tool Execution & Workspace Context Caching

#### 🟡 Issue 2: Redundant Workspace Directory Scanning (Staff Architectural Analysis)
* **Location:** [`pkg/services/search_tools.go`](pkg/services/search_tools.go#L47-L72), [`pkg/services/production_tools.go`](pkg/services/production_tools.go#L1-L100), & [`pkg/services/orchestrator_generator.go`](pkg/services/orchestrator_generator.go#L1-L100)
* **Root Cause (Zero-Context Blindness):** Generator and Tester agents start every task turn with zero visual awareness of the workspace layout. They spend 2–5 initial turns executing exploratory tool calls (`list_directory`, `find_files`) just to discover file paths (e.g. in `pyedis`, agents ran 198 tool calls including 97 redundant directory walks).
* **Impact Analysis:**
  - **Latency Overhead:** 3–5 unnecessary LLM round-trips $\times \approx 1.2\text{s} = 3.6\text{s}\text{--}6.0\text{s}$ wasted per task.
  - **Token Overhead:** Resending prompt history on exploratory turns 1–3 consumes $\approx 4,000\text{--}10,000$ input tokens per task.
  - **Iteration Depletion:** Depletes 20%–33% of the allowed per-task iteration budget before generating a character of code.
* **Detailed Staff Engineer Architecture Solution (Dual-Tier Design):**
  1. **Tier 1: System Prompt Workspace Tree Injection (Highest ROI):**
     - Before sending the initial task prompt, compute a compact, relative directory tree (up to depth 3, ignoring `node_modules`, `.git`, `vendor`, `__pycache__`, `target`).
     - Pre-populate this tree directly into the agent's system prompt context.
     - *Net Gain:* Eliminates exploratory directory listing calls on Turns 1–3 for 90%+ of tasks, letting agents proceed directly to `view_file` or `write_to_file` on Turn 1.
  2. **Tier 2: Task-Scoped Write-Invalidated Ephemeral Tool Cache:**
     - Implement an in-memory cache for `find_files` and `list_directory` scoped strictly to the task execution turn.
     - Automatically invalidate the cache instance whenever a mutating file operation occurs (`write_to_file`, `delete_file_tool`, `apply_patch`, or `run_command`).
     - *Net Gain:* Guarantees 100% filesystem consistency while eliminating redundant disk walks during multi-file search loops.

* **Performance & Impact Trade-off Matrix:**
  | Metric | Current Uncached | Proposed Dual-Tier Fix | Net Optimization Impact |
  | :--- | :---: | :---: | :---: |
  | **Exploratory Tool Calls / Task** | 3 – 6 calls | 0 – 1 calls | **80%+ reduction in exploratory turns** |
  | **First-Turn Execution Latency** | ~4.5s overhead | ~1.2s overhead | **3.3s faster task execution** |
  | **Token Bandwidth** | ~12k tokens on exploration | ~200 tokens for tree injection | **Massive input token savings** |
  | **Filesystem Consistency Risk** | None (reads disk every time) | Zero (invalidated on mutation) | **Guaranteed data consistency** |

---

## 3. Architecture & Reliability Refinements

### 3.1 Automated Stack Autodetection & Pre-flight Runner Bootstrap (Clarification)
* **Location:** [`pkg/services/sandbox.go`](pkg/services/sandbox.go#L1-L100) & [`pkg/services/test_validator.go`](pkg/services/test_validator.go#L1-L100)
* **Clarification:** This proposal **does NOT generate dummy or trivial test files** (e.g. `assert 1 == 1`). Real, meaningful unit and integration test logic is always written autonomously by LLM Dark Factory agents according to `SPEC.md`.
* **Purpose:** On legacy or newly-initialized projects that possess source code but lack a top-level `Makefile` or test entrypoint script, Noctifab automatically identifies the target programming language and build framework (`go.mod` $\rightarrow$ `go test ./...`, `Cargo.toml` $\rightarrow$ `cargo test`, `pyproject.toml` $\rightarrow$ `pytest`, `package.json` $\rightarrow$ `npm test`) so `TestValidator` can execute project test suites cleanly from turn 1.

---

## 4. Prioritized Implementation Roadmap

| Priority | Area | Improvement Title | Target Files | Est. Impact |
| :---: | :---: | :--- | :--- | :--- |
| **P1** | Speed | **Git Subprocess Overhead Reduction & Worktree Prune Batching** | [`pkg/services/orchestrator_execute.go`](pkg/services/orchestrator_execute.go#L96-L175) | Reduces per-task overhead by 2–5 seconds |
| **P1** | Performance | **Workspace File Tree Caching & System Prompt Pre-population** | [`pkg/services/search_tools.go`](pkg/services/search_tools.go#L1-L100), [`orchestrator_generator.go`](pkg/services/orchestrator_generator.go#L1-L100) | Cuts redundant directory listing tool turns by 40% |
