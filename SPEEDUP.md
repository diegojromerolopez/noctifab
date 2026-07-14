# Proposals for Near-Instantaneous Development in Noctifab

This document outlines key latency bottlenecks in `noctifab` based on performance logs, E2E validation runs, and codebase structure, and proposes actionable solutions to achieve a near-instantaneous development loop.

---

## 1. Sandbox Caching: Mount Go Build & Module Caches
### The Bottleneck
When running `make validate`, the E2E validation container (`validate-todo-cli-*`) is launched from scratch. Inside the container, the Go build cache and module cache are completely empty. Every validation run:
1. Re-downloads all Go packages (e.g., `pgx`, `uuid`, `otel`, `sqlite`, etc.) which takes **20–30 seconds**.
2. Performs full, uncached compilations of the test suites on every `run_tests` and `run_linter` call.

### The Proposal
Mount the host's Go caches into the Docker validation containers at runtime. 
Modify [run_one.sh](file:///Users/diegoj/repos/noctifab/validation/run_one.sh) and the Docker setups to mount:
*   **Go Module Cache:** `-v "${HOME}/go/pkg/mod:/go/pkg/mod"`
*   **Go Build Cache:** `-v "${HOME}/.cache/go-build:/root/.cache/go-build"`

This will reduce container startup and subsequent task compilations from **~30s to <2s** (a **15x speedup**).

---

## 2. Eliminate LLM Call Overhead in Context Gathering
### The Bottleneck
For every task in the DAG, before the Tester and Generator Agents execute their turns, Noctifab runs `RunReaderPhase` to gather context. This phase makes a full LLM call just to ask: *"What files do you want to read?"*
*   **Tester Agent:** 1 context-gathering LLM call + 1 tool execution turn + turn loop.
*   **Generator Agent:** 1 context-gathering LLM call + 1 tool execution turn + turn loop.
This adds **two mandatory sequential LLM roundtrips** (taking **5–10 seconds**) per task cycle, even if the target files are obvious.

### The Proposal
**Implement Heuristic Context Loading:**
Instead of using an LLM to decide what to read, the orchestrator should automatically read the contents of all files listed in `task.TargetFiles` (which the Planner Agent already generates) and inject them directly into the initial prompt. 
*   Skip the `RunReaderPhase` LLM step entirely for the happy path.
*   Only fall back to an LLM reader phase if `task.TargetFiles` is empty or if the agent explicitly requests more files using a new `read_more_files` action.
This saves **2 LLM calls per task** (a **~10-second reduction** per task).

---

## 3. Turn-Level Concurrency & Single-Turn execution
### The Bottleneck
Agents currently run in a turn-by-turn fashion. An agent writes a file, waits for the orchestrator to return "File written", then in the next turn calls `run_tests`, waits for the test output, then in the next turn does something else. Each turn incurs a 2-4 second LLM generation latency.

### The Proposal
*   **Batch Actions:** Promote multi-action generation in prompts. Encourage agents to write files and call `run_tests` in the **same turn prompt output** rather than doing them sequentially.
*   **Parallel Tool Execution:** Modify the orchestrator loop to run independent tool executions (e.g., reading multiple files, running linter and tests) concurrently using Go routines.

---

## 4. Faster Orchestrator Polling & Wakeups
### The Bottleneck
The orchestrator relies on a `PollInterval` loop to check if tasks are ready or state has changed. If set too high (e.g., 5 seconds), this introduces dead time between task handoffs.

### The Proposal
*   Use file system watchers (`fsnotify`) or database channel notifications to trigger execution cycles instantly when state updates or task completions happen, rather than polling on a timer.

---

## 5. Parallel Task Execution
### The Bottleneck
Although the orchestrator supports task concurrency (`Concurrency` setting), the validation harness runs one user story task sequential sequence. 

### The Proposal
Enable full task concurrency on non-dependent tasks in the DAG by allocating multiple parallel worker sandboxes (either multiple Docker containers or isolated working directories on the host).
