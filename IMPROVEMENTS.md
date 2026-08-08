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

## 3. Terminal User Interface (TUI) & UX Enhancements

### 3.1 Interactive Failure Stack Inspector & Colorized Diagnostics

#### 🟡 Issue 3: Truncated Failure Visibility in Dashboard
* **Location:** [`cmd/noctifab/cli/dashboard_render.go`](cmd/noctifab/cli/dashboard_render.go#L123-L128)
* **Root Cause:** Failed tasks in the dashboard only display a single truncated tail line via `extractFailureTailReason()`.
* **Impact:** Users cannot diagnose compilation errors or test failures without leaving the dashboard to inspect raw log files on disk.
* **Proposed Solution:**
  1. Add an interactive **Log Inspector Modal** to the dashboard TUI.
  2. Pressing `<Enter>` or `d` on a failed task opens a scrollable, syntax-highlighted modal displaying the full error log, assertion diff, and stack trace.

---

### 3.2 Interactive Modal Overlays for Dashboard Prompts

#### 🟡 Issue 4: TUI UI Freeze During Input Prompts
* **Location:** [`cmd/noctifab/cli/dashboard.go`](cmd/noctifab/cli/dashboard.go#L149-L216)
* **Root Cause:** When a user triggers an interactive action (`p` for pause, `n` for new order, `c` for clarification), the background dashboard refresh loop is blocked by acquiring `mu.Lock()`.
* **Impact:** The dashboard stops updating agent status and progress while the user is typing an input response.
* **Proposed Solution:**
  1. Decouple input prompting into floating overlay regions.
  2. Maintain background status polling and render updates continuously around the active input prompt modal.

---

## 4. Architecture & Reliability Refinements

### 4.1 Automated Stack Autodetection & Pre-flight Bootstrap
* **Location:** [`pkg/services/sandbox.go`](pkg/services/sandbox.go#L1-L100) & [`pkg/services/test_validator.go`](pkg/services/test_validator.go#L1-L100)
* **Observation:** In projects without a pre-existing `Makefile` or test configuration, initial test validation invocations fail before the agent writes a characterization test suite.
* **Improvement:** Implement an automatic **Project Tech Stack Classifier** during PM roadmap generation. If standard test runners (e.g. `Cargo.toml`, `pyproject.toml`, `package.json`, `go.mod`) are detected without a test script, Noctifab can auto-synthesize default build/test wrappers before starting Task 1.

---

## 5. Prioritized Implementation Roadmap

| Priority | Area | Improvement Title | Target Files | Est. Impact |
| :---: | :---: | :--- | :--- | :--- |
| **P1** | Speed | **Git Subprocess Overhead Reduction & Worktree Prune Batching** | [`pkg/services/orchestrator_execute.go`](pkg/services/orchestrator_execute.go#L96-L175) | Reduces per-task overhead by 2–5 seconds |
| **P1** | Performance | **Workspace File Tree Caching & System Prompt Pre-population** | [`pkg/services/search_tools.go`](pkg/services/search_tools.go#L1-L100), [`orchestrator_generator.go`](pkg/services/orchestrator_generator.go#L1-L100) | Cuts redundant directory listing tool turns by 40% |
| **P2** | UX | **Interactive TUI Stack Trace / Log Inspector Modal** | [`cmd/noctifab/cli/dashboard.go`](cmd/noctifab/cli/dashboard.go#L1-L100), [`dashboard_render.go`](cmd/noctifab/cli/dashboard_render.go#L123-L128) | Allows immediate in-TUI debugging of failed tasks |
| **P2** | UX | **Decoupled Input Prompt Overlay in Dashboard TUI** | [`cmd/noctifab/cli/dashboard.go`](cmd/noctifab/cli/dashboard.go#L149-L216) | Prevents UI freeze during interactive prompts |
