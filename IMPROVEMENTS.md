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

#### 🟡 Issue 2: Redundant Workspace Directory Scanning
* **Location:** [`pkg/services/search_tools.go`](pkg/services/search_tools.go#L1-L100) & [`pkg/services/production_tools.go`](pkg/services/production_tools.go#L1-L100)
* **Root Cause:** Generator and Tester agents frequently execute 50+ repeated `list_directory` and `find_files` tool calls within the same task turn to inspect workspace layouts (e.g., `pyedis` executed 198 tool calls, including 97 list/find calls).
* **Impact:** Consumes unnecessary LLM turn budget and token bandwidth re-reading directory structures that have not changed.
* **Proposed Solution:**
  1. **Workspace Tree Cache:** Cache directory tree listings in memory during a task execution turn.
  2. **Initial Context Injection:** Pre-populate the initial agent system prompt with a lightweight representation of the project file tree, eliminating the need for exploratory directory listing calls in turn 1.

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
