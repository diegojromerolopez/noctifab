# Noctifab Engineering Review: Performance & UX Improvement Proposals

**Author:** Staff Software Engineer  
**Date:** August 7, 2026  
**Target Repository:** `noctifab` (`github.com/diegojromerolopez/noctifab`)  
**Scope:** Architecture, LLM Infrastructure, Orchestration Engine, Database & Storage, Terminal User Experience (TUI/UX), Sandboxing, and Developer Experience (DX).

---

## 1. Executive Summary

`noctifab` is a dark factory autonomous coding engine designed around a stateless agent / stateful orchestrator architecture. The system features multi-provider LLM routing, DAG-based task execution, automated test validation, self-healing code generation, and terminal-based monitoring.

Following a thorough codebase audit across `pkg/domain`, `pkg/infrastructure` (LLM, Storage, Config), `pkg/services` (Orchestrator, Unblocker, Sandbox, Tools), and `cmd/noctifab/cli` (TUI Dashboard), this document outlines concrete, high-impact recommendations to significantly boost **execution speed**, **provider resilience**, and **user experience (UX)**.

---

## 2. Performance & System Throughput Optimizations

### 2.1 LLM Infrastructure & Provider Circuit-Breaking

#### 🔴 Issue 1: Provider Fallback Bypass on Depleted API Keys (HTTP 401/402)
* **Location:** [`pkg/infrastructure/llm/client.go`](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/client.go#L258-L278) & [`pkg/infrastructure/llm/router.go`](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/router.go#L473-L508)
* **Root Cause:** When an LLM provider returns `HTTP 401 Unauthorized` (e.g. OpenCode `CreditsError: Insufficient balance`) or `HTTP 402 Payment Required`, `Client.Complete()` skips internal retries and model fallbacks. However, `ResilientLLMRouter` does **not** classify 401/402 errors as transient, so the candidate provider is **not** placed in `cooldowns`. On the subsequent task or planning turn, `ResilientLLMRouter` selects Candidate #1 again, re-executing an HTTP request to the depleted provider and stalling the entire story execution loop.
* **Impact:** Multi-provider priority configurations completely freeze when provider #1 runs out of funds, despite healthy fallback providers (`openai`, `qwencloud`, `openrouter`) being configured.
* **Proposed Solution:**
  1. Implement a **Session Provider Eviction Circuit-Breaker** (Negative Key Cache).
  2. When any client returns HTTP 401 (depleted/invalid API key) or HTTP 402 (payment required), flag that provider instance as `EVICTED` in the router for the remainder of the daemon session (or a TTL of 30 minutes).
  3. Evicted providers are skipped instantly during candidate resolution without attempting network calls.

#### 🟡 Issue 2: Synchronous Catalog Discovery Latency
* **Location:** [`pkg/infrastructure/llm/client.go`](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/client.go#L226-L230)
* **Root Cause:** `resolveLatestModel()` performs a synchronous network request to discover available models when encountering aliases like `latest` or `auto`. While catalog results are cached with a short TTL, cache misses block prompt completion.
* **Impact:** Incurs an extra 300ms–1200ms latency penalty on initial prompt dispatches.
* **Proposed Solution:**
  1. Implement **Asynchronous Background Catalog Refresh**.
  2. Seed model catalogs during CLI startup / daemon pre-flight checks.
  3. When catalog TTL is within 20% of expiration, trigger an asynchronous background goroutine to refresh the catalog while serving immediate calls from existing cache.

#### 🟡 Issue 3: Synchronous String-Regex Prompt Compaction Overhead
* **Location:** [`pkg/infrastructure/llm/client.go`](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/client.go#L175-L188)
* **Root Cause:** Prompt compaction modes (`caveman`, `simple_english`) run synchronously via heavy regular expression replacements on multi-megabyte concatenated string prompts inside `Complete()`.
* **Impact:** High memory allocation rates and garbage collection pauses during rapid multi-agent loops.
* **Proposed Solution:**
  1. Compact individual file snippets at context collection time (`context_slicer.go`) before constructing the final prompt string.
  2. Use byte-slice buffer operations instead of string allocations for text normalization.

---

### 2.2 Orchestration Engine & Git Subprocess Overhead

#### 🔴 Issue 4: Subprocess Fork Overhead for Git Task Lifecycle
* **Location:** [`pkg/services/orchestrator_execute.go`](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator_execute.go#L96-L175) & [`pkg/services/rebase_queue.go`](file:///Users/diegoj/repos/noctifab/pkg/services/rebase_queue.go#L1-L100)
* **Root Cause:** `executeTask()` executes up to 12–15 synchronous Git CLI subprocesses (`git show-ref`, `git checkout`, `git branch`, `git worktree add`, `git status`, `git diff`, `git add`, `git commit`, `git worktree remove`, `git worktree prune`) for every single task.
* **Impact:** In a 10-task story, Git subprocess invocation overhead accounts for 2–5 seconds per task (~20–50 seconds total per story) spent strictly in OS process fork/exec cycles.
* **Proposed Solution:**
  1. **Batch Git Worktree Cleanup:** Defer `git worktree prune` to periodic background routines (e.g. every 60 seconds) rather than executing it synchronously on every task completion.
  2. **In-Memory Index Checking:** Use direct file system checks (`os.Stat`) for local git ref files (`.git/refs/heads/<branch>`) instead of spawning `git show-ref` processes.

---

### 2.3 State Management & Storage Lock Contention

#### 🟡 Issue 5: High-Frequency SQLite Transaction Contention
* **Location:** [`pkg/infrastructure/storage/sqlite_repository.go`](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/storage/sqlite_repository.go#L69-L98) & [`pkg/services/orchestrator_execute.go`](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator_execute.go#L61-L75)
* **Root Cause:** `updateStateWithRetry()` triggers full Optimistic Concurrency Control (OCC) version checks and full transaction saves (`saveTx`) to SQLite on minor progress updates (e.g. progress 10% -> 25% -> 50%).
* **Impact:** Under parallel execution (e.g. breadth-first mode or concurrent agents), SQLite single-writer lock contention causes transaction rollbacks and write retries.
* **Proposed Solution:**
  1. **Debounced Progress Synchronization:** Buffer task progress percentage updates (10%, 25%, 50%, 75%) in memory and flush them to SQLite in 500ms debounced batches.
  2. Restrict synchronous transactional writes strictly to major task state transitions (`TaskPending` -> `TaskInProgress` -> `TaskSuccess` / `TaskFailed`).

---

### 2.4 Tool Execution & Workspace Context Caching

#### 🟡 Issue 6: Redundant Workspace Directory Scanning
* **Location:** [`pkg/services/search_tools.go`](file:///Users/diegoj/repos/noctifab/pkg/services/search_tools.go#L1-L100) & [`pkg/services/production_tools.go`](file:///Users/diegoj/repos/noctifab/pkg/services/production_tools.go#L1-L100)
* **Root Cause:** Generator and Tester agents frequently execute 50+ repeated `list_directory` and `find_files` tool calls within the same task turn to inspect workspace layouts (e.g., `pyedis` executed 198 tool calls, including 97 list/find calls).
* **Impact:** Consumes unnecessary LLM turn budget and token bandwidth re-reading directory structures that have not changed.
* **Proposed Solution:**
  1. **Workspace Tree Cache:** Cache directory tree listings in memory during a task execution turn.
  2. **Initial Context Injection:** Pre-populate the initial agent system prompt with a lightweight representation of the project file tree, eliminating the need for exploratory directory listing calls in turn 1.

---

## 3. Terminal User Interface (TUI) & UX Enhancements

### 3.1 Terminal Render Double-Buffering & Flicker Elimination

#### 🔴 Issue 7: Terminal Redraw Screen Flicker
* **Location:** [`cmd/noctifab/cli/dashboard_render.go`](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/dashboard_render.go#L24-L27)
* **Root Cause:** The dashboard renders updates by outputting clear-screen ANSI escape codes (`\033[H\033[J`) followed by streaming the whole rendered string to stdout every 1 second.
* **Impact:** Screen flickering occurs on fast refresh rates, especially over SSH connections or inside terminal multiplexers (`tmux`, `zellij`).
* **Proposed Solution:**
  1. Use cursor hiding (`\033[?25l` on start, `\033[?25h` on exit) and overwrite only modified lines (differential line rendering).
  2. Alternatively, migrate dashboard rendering to a structured TUI component model (such as `charmbracelet/bubbletea` or `gdamore/tcell`).

---

### 3.2 Pre-Flight LLM & Sandbox Credential Diagnostics

#### 🔴 Issue 8: Late Failure Discovery on CLI Launch
* **Location:** [`cmd/noctifab/cli/start.go`](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/start.go#L1-L100) & [`cmd/noctifab/cli/init.go`](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/init.go#L1-L100)
* **Root Cause:** `noctifab start` launches daemon tasks without first validating that configured LLM API keys are active/funded or that target build tools (`cargo`, `pytest`, `go`, `docker`) are available in the sandbox.
* **Impact:** Users wait 5–10 minutes into execution before discovering a missing dependency or an invalid API key.
* **Proposed Solution:**
  1. Add a **Pre-flight Diagnostic Check** step to `noctifab start` and `noctifab validate`.
  2. Perform lightweight API key pings via [`pkg/infrastructure/llm/ping.go`](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/ping.go#L1-L50) and test tool availability prior to launching orchestrator threads.
  3. Render a clean pre-flight summary table in the terminal:
     ```
     [✓] OpenAI API Key (gpt-5.6-luna) - Connected
     [✗] OpenCode API Key (glm-5.2) - HTTP 401 (Insufficient Balance) -> Evicted
     [✓] Sandbox Runtime (Python 3.12, Pytest) - Ready
     ```

---

### 3.3 Interactive Failure Stack Inspector & Colorized Diagnostics

#### 🟡 Issue 9: Truncated Failure Visibility in Dashboard
* **Location:** [`cmd/noctifab/cli/dashboard_render.go`](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/dashboard_render.go#L123-L128)
* **Root Cause:** Failed tasks in the dashboard only display a single truncated tail line via `extractFailureTailReason()`.
* **Impact:** Users cannot diagnose compilation errors or test failures without leaving the dashboard to inspect raw log files on disk.
* **Proposed Solution:**
  1. Add an interactive **Log Inspector Modal** to the dashboard TUI.
  2. Pressing `<Enter>` or `d` on a failed task opens a scrollable, syntax-highlighted modal displaying the full error log, assertion diff, and stack trace.

---

### 3.4 Interactive Modal Overlays for Dashboard Prompts

#### 🟡 Issue 10: TUI UI Freeze During Input Prompts
* **Location:** [`cmd/noctifab/cli/dashboard.go`](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/dashboard.go#L149-L216)
* **Root Cause:** When a user triggers an interactive action (`p` for pause, `n` for new order, `c` for clarification), the background dashboard refresh loop is blocked by acquiring `mu.Lock()`.
* **Impact:** The dashboard stops updating agent status and progress while the user is typing an input response.
* **Proposed Solution:**
  1. Decouple input prompting into floating overlay regions.
  2. Maintain background status polling and render updates continuously around the active input prompt modal.

---

## 4. Architecture & Reliability Refinements

### 4.1 Automated Stack Autodetection & Pre-flight Bootstrap
* **Location:** [`pkg/services/sandbox.go`](file:///Users/diegoj/repos/noctifab/pkg/services/sandbox.go#L1-L100) & [`pkg/services/test_validator.go`](file:///Users/diegoj/repos/noctifab/pkg/services/test_validator.go#L1-L100)
* **Observation:** In projects without a pre-existing `Makefile` or test configuration, initial test validation invocations fail before the agent writes a characterization test suite.
* **Improvement:** Implement an automatic **Project Tech Stack Classifier** during PM roadmap generation. If standard test runners (e.g. `Cargo.toml`, `pyproject.toml`, `package.json`, `go.mod`) are detected without a test script, Noctifab can auto-synthesize default build/test wrappers before starting Task 1.

---

## 5. Prioritized Implementation Roadmap

| Priority | Area | Improvement Title | Target Files | Est. Impact |
| :---: | :---: | :--- | :--- | :--- |
| **P0** | Resilience | **Depleted API Key Circuit Breaker (HTTP 401/402 Eviction)** | [`pkg/infrastructure/llm/router.go`](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/router.go#L473-L508), [`client.go`](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/client.go#L258-L278) | Eliminates execution freezes on multi-provider setups |
| **P0** | UX / DX | **CLI Pre-Flight Health & Credential Diagnostic** | [`cmd/noctifab/cli/start.go`](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/start.go#L1-L100), [`validate.go`](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/validate.go#L1-L50) | Prevents starting jobs with dead API keys or missing tools |
| **P1** | UX | **TUI Double-Buffering & Cursor Hide (Flicker Fix)** | [`cmd/noctifab/cli/dashboard_render.go`](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/dashboard_render.go#L24-L27) | Delivers smooth, flicker-free terminal monitoring |
| **P1** | Speed | **Git Subprocess Overhead Reduction & Worktree Prune Batching** | [`pkg/services/orchestrator_execute.go`](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator_execute.go#L96-L175) | Reduces per-task overhead by 2–5 seconds |
| **P1** | Performance | **Workspace File Tree Caching & System Prompt Pre-population** | [`pkg/services/search_tools.go`](file:///Users/diegoj/repos/noctifab/pkg/services/search_tools.go#L1-L100), [`orchestrator_generator.go`](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator_generator.go#L1-L100) | Cuts redundant directory listing tool turns by 40% |
| **P2** | Performance | **Async Model Catalog Discovery Refresh** | [`pkg/infrastructure/llm/client.go`](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/llm/client.go#L226-L230) | Saves 300ms–1200ms on first prompt turn |
| **P2** | UX | **Interactive TUI Stack Trace / Log Inspector Modal** | [`cmd/noctifab/cli/dashboard.go`](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/dashboard.go#L1-L100), [`dashboard_render.go`](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/dashboard_render.go#L123-L128) | Allows immediate in-TUI debugging of failed tasks |
| **P2** | Storage | **Debounced Progress Synchronization in SQLite** | [`pkg/infrastructure/storage/sqlite_repository.go`](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/storage/sqlite_repository.go#L69-L98) | Reduces DB lock contention during parallel execution |

---
