# Dynamic Prompt Enhancement via Unblocker Log Injection

## 1. Executive Summary & Concept Overview

When an autonomous software engineering dark factory operates without human supervision, tasks can occasionally **stall or block** due to hanging background processes, interactive prompts waiting for `stdin`, network socket timeouts, infinite build/test loops, or LLM response stalls.

This document proposes an architectural enhancement to **Noctifab**: **Dynamic Prompt Enhancement using Real-Time Execution Logs**. When a task is detected as stalled (e.g., no progress for >60s or past configured stall thresholds), Noctifab will dynamically extract the **last 60 seconds of standard output and standard error logs** for that task, attach them to the diagnostic prompt, and ask the LLM to identify the root cause and prescribe a corrective action.

### Key Takeaway
> **Yes, this is strictly the domain of the Unblocker Agent (`UnblockerAgent`).** 
> The Unblocker Agent is already Noctifab's dedicated daemon for detecting progress freezes, pipeline deadlocks, and orphaned tasks. Enhancing its prompt engine with live log tailing gives it the high-resolution runtime visibility needed to resolve complex stalls autonomously.

---

## 2. Architectural Role: Why the Unblocker Agent?

Noctifab separates responsibilities cleanly:
- **Orchestrator**: Manages state transitions, task scheduling, and state persistence.
- **Worker Agents**: Execute individual tasks (writing code, generating tests, running builds).
- **Unblocker Agent (`UnblockerAgent`)**: Periodically monitors the pipeline, detects anomalies/stalls, and executes recovery actions.

Currently, the `UnblockerAgent` (`pkg/services/unblocker.go` & `pkg/services/unblocker_prompt.go`) diagnoses stalls using **static metadata**:
- Task ID, Title, Status, Progress percentage
- Retry counter and MaxRetries
- Historical `FailureLog` (only available *after* a task explicitly fails)
- Stall duration and assigned Agent ID

### The Limitation
When a task **stalls in real time** (e.g., frozen at 40% progress for 3 minutes), `FailureLog` is empty because the task has not failed yet. The Unblocker currently sees *that* it is frozen, but has zero visibility into *what* the task was doing when it froze.

### The Solution
By piping the last 60 seconds of live execution output into `buildUnblockerPrompt()`, the Unblocker Agent transforms from a simple heuristic timeout resetter into a **runtime context-aware diagnostician**.

---

## 2.1 Critical Distinction: Explicit Error vs. Live Stall

It is important to differentiate between **two types of failures** in Noctifab:

| Dimension | Explicit Error (Command Finished) | Live Stall / Block (Process Hung) |
| :--- | :--- | :--- |
| **Process State** | Terminated (Exit code $\ne 0$) | Still Running / Frozen (IN_PROGRESS) |
| **Output State** | Complete `FailureLog` available | No exit code, no `FailureLog` written yet |
| **Detection** | Immediate via process exit code | Watchdog / `UnblockerAgent` timer (>60s stall) |
| **Log Source** | Standard command return buffer | Live streaming ring-buffer of running sub-process |
| **Handling Agent** | Direct Worker Agent (`Generator`/`Tester`) | **`UnblockerAgent`** (Intercepts, kills, re-prompts worker) |

1. **For Explicit Errors**: When a command finishes with an error (e.g. `go test` returns exit code 1), the executing Worker Agent (`Generator` or `Tester`) directly gets the stderr output in its tool result and is prompted to fix it immediately.
2. **For Live Stalls / Blocks**: The process is **stuck indefinitely** (e.g., waiting for `stdin` input, stuck in a `lock()` wait, or infinite loop). The Worker Agent is blocked and cannot continue. The **`UnblockerAgent`** must intervene by:
   - Extracting the **last 60s of live streaming output** before the freeze.
   - Terminating the stuck process.
   - Injecting the captured log tail into the Worker Agent's prompt for attempt $N+1$ with a targeted fix request.

---

## 3. Detailed Workflow & System Design

```
+-----------------------------------------------------------------------------------+
|                                 STALL DETECTED                                    |
| Task "US-002" frozen in IN_PROGRESS for > 3 minutes (StallReasonFrozenProgress)   |
+-----------------------------------------------------------------------------------+
                                          |
                                          v
+-----------------------------------------------------------------------------------+
|                               LOG EXTRACTION SERVICE                              |
| Reads last 60s / 100 lines of streaming stdout/stderr from task log file          |
| (.noctifab/logs/tasks/<task_id>.log or live process buffer)                       |
+-----------------------------------------------------------------------------------+
                                          |
                                          v
+-----------------------------------------------------------------------------------+
|                            DYNAMIC PROMPT ENHANCEMENT                             |
| Injects log snippet + stall context into buildUnblockerPrompt()                   |
+-----------------------------------------------------------------------------------+
                                          |
                                          v
+-----------------------------------------------------------------------------------+
|                              UNBLOCKER LLM DIAGNOSIS                              |
| Analyzes log snippet -> Identifies cause (e.g., "waiting for sudo password")       |
+-----------------------------------------------------------------------------------+
                                          |
                                          v
+-----------------------------------------------------------------------------------+
|                            CORRECTIVE RECOVERY ACTION                             |
| Option A: Reset task + Attach Dynamic Advisory to worker's retry prompt           |
| Option B: Fail task with explicit diagnostic reason                               |
+-----------------------------------------------------------------------------------+
```

### 3.1 Step-by-Step Execution Plan

#### 1. Live Log Tailing & Ring Buffering
- During task execution, standard output and standard error are written to disk at `.noctifab/logs/tasks/<task_id>.log` or maintained in a ring buffer in memory (`bounded_buffer.go`).
- When `detectStalledTasks()` flags a task, a `LogTailer` utility fetches log lines timestamped within `[now - 60s, now]` (or the last N KB / N lines if timestamps are unavailable).

#### 2. Secrets & Token Scrubbing
- Per Noctifab's **Secrets Handling & Privacy Mandate**, any log snippet parsed for prompt injection must pass through a sanitization filter to strip API keys, bearer tokens, or password strings prior to sending to the LLM context.

#### 3. Enhanced Unblocker Prompt Construction (`pkg/services/unblocker_prompt.go`)
The `stalledTaskSummary` struct is updated to include `RecentLogs`:

```json
{
  "task_id": "task-003",
  "task_title": "Run Integration Test Suite",
  "task_status": "IN_PROGRESS",
  "progress": 45,
  "stalled_for": "3m12s",
  "recent_logs_last_60s": [
    "[23:41:02] Running command: go test -v ./tests/e2e/...",
    "[23:41:05] ? Do you want to download package dependencies? (Y/n)",
    "[23:41:05] (no output for 180 seconds)"
  ]
}
```

The system prompt for `UnblockerAgent` is updated to instruct the model:
> *"Inspect `recent_logs_last_60s` to determine why the process stopped producing output. Common causes include interactive CLI prompts, deadlocked mutexes, infinite build loops, or network timeouts."*

#### 4. Dual-Stage Recovery: Unblocker Diagnosis -> Worker Guidance
Instead of just resetting a task blindly, the Unblocker can issue an enhanced action: `reset_task_with_directive`.

When the worker agent picks up the re-queued task, its system prompt is dynamically injected with the Unblocker's advice:

```markdown
### ⚠️ PREVIOUS ATTEMPT STALL RECOVERY DIRECTIVE
Your previous execution attempt stalled for 3m12s with the following last log output:
> `? Do you want to download package dependencies? (Y/n)`

**Diagnostic Guidance from Unblocker Agent:**
The command froze because it was waiting for interactive stdin input. 
**Required Action:** Execute all commands non-interactively (e.g., pass `-y`, `--non-interactive`, or `CI=true`).
```

### 3.2 Progressive Log Window Escalation (The 10x Expansion Rule)

Not all stalls can be diagnosed from just the final 50 lines. For example, if a process hangs because an environment variable was misconfigured 5 minutes prior during initialization, the tail end of the log will only show silence.

To balance **token efficiency** with **diagnostic depth**, we implement a **2-Step 10x Progressive Escalation Strategy**:

| Stall Attempt | Log Window Size | Approx. Byte / Token Budget | Target Diagnostic Coverage |
| :---: | :---: | :---: | :--- |
| **Level 1 (1st Stall)** | **50 lines** (~60s) | ~2 KB (~500 tokens) | **90% of stalls**: Stdin prompts (`[Y/n]`), port binding errors, spinners, test watch mode. |
| **Level 2 (2nd Stall)** | **500 lines** (10x) | ~20 KB (~5,000 tokens) | **Complex stalls**: Stack trace roots, missing environment variables, build compilation errors. |
| **Level 3 (3rd Stall)** | **5,000 lines** (100x / Cap) | ~200 KB (~40,000 tokens) | **Systemic stalls**: Full execution history from process start. |

#### Escalation Rules & Hard Cap Guardrails
1. **Start Small (Level 1 - 50 lines)**: 90% of CLI stalls occur at the very end of the log output. Starting at 50 lines keeps 90% of unblock checks fast and cheap.
2. **Escalate on Recurring Stall (10x Factor)**: If the unblock action from Level 1 is attempted but the task freezes again, the Unblocker **expands the log window 10x to 500 lines**.
3. **Final Escalation (100x / Full Context Cap)**: If it stalls a 3rd time, expand 10x again to **5,000 lines** (or maximum context budget).
4. **Hard Stop After 2 Escalations**: The escalation is performed **at most twice** (Level 1 $\rightarrow$ Level 2 $\rightarrow$ Level 3). If a task stalls a 4th time after Level 3, the `UnblockerAgent` marks `task_status = FAILED` with `StallReasonUnrecoverable`. This strictly prevents infinite unblock/retry loops.

---

## 4. Real-World Stall Examples & Easy Win Scenarios

Below are common dark factory stall scenarios where **Dynamic Log Prompt Enhancement** provides instant, automated resolution:

### Example 1: Interactive CLI Prompt Wait (Easy Win ⭐⭐⭐⭐⭐)
- **What Happens**: A worker agent runs `npm install package-name` or `docker system prune` without non-interactive flags.
- **Captured Last 60s Logs**:
  ```text
  [23:41:00] npm WARN deprecated library@1.0.0
  [23:41:02] ? Do you want to proceed with installing optional dependencies? (Y/n)
  [23:41:02] (no further output for 180 seconds)
  ```
- **Unblocker Action**:
  - LLM identifies interactive prompt waiting on `stdin`.
  - Action: `reset_task_with_directive`.
- **Injected Retry Directive**:
  > *"Previous execution froze waiting for stdin input at `? Do you want to proceed?`. Re-run command with non-interactive flags (`--yes` or `DEBIAN_FRONTEND=noninteractive`)."*

### Example 2: Repetitive Progress Spinner / Watch Mode Loop (Easy Win ⭐⭐⭐⭐⭐)
- **What Happens**: Worker runs a test runner like Jest or Vitest without disabling watch mode (`jest` instead of `jest --watchAll=false`), causing the process to hang forever waiting for file changes.
- **Captured Last 60s Logs**:
  ```text
  [23:41:00] PASS tests/unit/calculator.test.js
  [23:41:00] Watch Usage: Press f to run failed tests, p to filter by filename.
  [23:41:00] (no further output for 180 seconds)
  ```
- **Unblocker Action**:
  - LLM recognizes interactive test watch mode.
  - Action: Kills process, resets task.
- **Injected Retry Directive**:
  > *"Previous execution entered interactive watch mode. Run tests non-interactively using `--watchAll=false` or `--ci`."*

### Example 3: Port Binding Collision / Socket Hang (Easy Win ⭐⭐⭐⭐)
- **What Happens**: A test command tries to launch a local server on port 8080, but a orphaned process from a previous test run is still occupying the port.
- **Captured Last 60s Logs**:
  ```text
  [23:41:01] Starting server on 127.0.0.1:8080...
  [23:41:01] Error: listen tcp 127.0.0.1:8080: bind: address already in use
  [23:41:01] (process enters retry loop / hangs indefinitely waiting for port release)
  ```
- **Unblocker Action**:
  - LLM detects `bind: address already in use`.
  - Action: Directs worker agent to pick an ephemeral port (`port 0` / dynamic port) or kill process on port 8080 before running tests.

### Example 4: External Network / API Socket Timeout (Medium Win ⭐⭐⭐)
- **What Happens**: LLM API request or package download hangs on socket read without setting a read timeout.
- **Captured Last 60s Logs**:
  ```text
  [23:41:00] Fetching package index from https://registry.npmjs.org/...
  [23:41:00] (no output for 180 seconds, socket open but unresponsive)
  ```
- **Unblocker Action**:
  - LLM identifies socket read timeout / slow network endpoint.
  - Action: Advises worker to set CLI timeout flags (`--connect-timeout 30`) or use local cache / fallback registry mirror.

---

---

## 5. Implementation Complexity & Staff Engineering Extensions

### 5.1 Why Core Implementation Is "Low-Hanging Fruit"
Implementing this feature in Noctifab is straightforward because **80% of the required infrastructure already exists in the codebase**:
1. **Pre-existing Log Capture**: Noctifab already writes task execution output to disk under `.noctifab/logs/tasks/<task_id>.log` and maintains output buffers in `pkg/services/bounded_buffer.go`.
2. **Pre-existing Unblocker Loop**: `UnblockerAgent.loop()` in `pkg/services/unblocker.go` already periodically scans for stalled tasks (`detectStalledTasks`).
3. **Simple File Tail Helper**: Extracting the last 60 seconds of logs requires only a simple ~20 line Go helper.
4. **Zero State Changes**: Injecting `recent_logs` into `buildUnblockerPrompt()` requires zero database schema modifications and zero changes to happy-path performance.

---

### 5.2 Staff Engineer Architectural Extensions (Missing Advanced Features)

To make this system truly production-grade and exhaustive, four key advanced extensions should be incorporated:

#### Extension A: Process Telemetry Injection (Handling "Silent Stalls")
- **The Problem**: What if a process enters an infinite `for{}` loop or a `sync.Mutex` deadlock and emits **0 log lines** for 3 minutes? Log tailing alone returns empty silence.
- **The Extension**: When log tailing yields 0 lines, inspect basic OS process metrics for the worker pid:
  - `CPU %`: 99–100% $\rightarrow$ **Infinite CPU Loop** (e.g. `while(true)` without sleep).
  - `CPU %`: 0.0% $\rightarrow$ **Deadlocked Wait / Stdin Wait** (e.g. channel deadlock or waiting on input).
- **Prompt Enrichment**:
  ```json
  "process_telemetry": {
    "log_lines_last_60s": 0,
    "cpu_usage_pct": 99.8,
    "memory_mb": 142,
    "diagnosis_hint": "High CPU with 0 log activity indicates an unbuffered infinite computation loop."
  }
  ```

#### Extension B: Fast-Path Deterministic Regex Pre-Filter (0-Token Unblocking)
- **The Problem**: Invoking an LLM for every stall adds 5-10s API latency and token cost.
- **The Extension**: Before calling the Unblocker LLM, run the 60s log tail through a lightweight static regex engine:
  - `(?i)(\\?.*do you want to|overwrite\\? \\[y/n\\])` $\rightarrow$ Immediate non-interactive fix.
  - `(?i)(bind: address already in use)` $\rightarrow$ Immediate port release fix.
  - `(?i)(watch usage: press f to run)` $\rightarrow$ Immediate `--watchAll=false` fix.
- **Benefit**: 80% of routine CLI stalls are resolved deterministically in **< 5ms** with **0 LLM token cost**. The Unblocker LLM is only invoked when regex filters return no match.

#### Extension C: Secret & Token Sanitization Filter (Security Compliance)
- **The Problem**: Commands running verbose output or `env` dumps might accidentally print secrets (`GITHUB_TOKEN`, `OPENAI_API_KEY`) into the log stream.
- **The Extension**: Pipeline all extracted log snippets through a mandatory `SanitizeLog()` sanitizer before injecting them into the LLM prompt:
  ```go
  var secretRegexes = []*regexp.Regexp{
      regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret)\s*[:=]\s*["']?([^\s"']+)`),
      regexp.MustCompile(`(ghp_[A-Za-z0-9_]{36}|sk-[A-Za-z0-9_]{32,})`),
  }
  ```

#### Extension D: Cross-Task Stall Pattern Memory (Continuous Learning)
- **The Problem**: If Task 1 stalls on a missing dependency flag (e.g. `npm install --legacy-peer-deps`), Task 2 in a later story might hit the exact same stall.
- **The Extension**: When the Unblocker successfully resolves a stall pattern, save the `(StallRegex -> CorrectiveDirective)` mapping to `.noctifab/stall_rules.json`. When future tasks run, pre-inject these learned directives into worker prompts *before* stalls even occur!

---

## 6. Key Benefits & Trade-offs

### Benefits
1. **Prevents Repeating Stalls**: Instead of blindly retrying a task 3 times only for it to hang on the exact same interactive prompt or timeout, the retry attempt receives explicit corrective directives.
2. **Context Window Efficiency**: By sending only the last 60 seconds (or last ~50-100 lines), token consumption remains negligible (~500–1000 tokens per stall check) while diagnostic resolution remains extremely high.
3. **Zero Impact on Happy Path**: This logic only fires when a stall threshold is crossed, causing zero overhead during normal, fast task execution.

### Trade-offs & Guardrails
1. **Log Truncation & Capping**: Must enforce a strict byte cap (e.g. max 4KB / 100 lines) to prevent runaway token usage if a noisy loop emits megabytes of logs in 60s.
2. **Log Timestamp Tracking**: Standard output from child commands must be buffered with timestamps or line-received times to reliably isolate the "last 60s".

---

## 7. Applicability Across Other Noctifab Agents

Beyond the **`UnblockerAgent`**, several other agent roles in Noctifab's multi-agent architecture could directly benefit from **Dynamic Log Prompt Enhancement**:

### 7.1 AgentRoleTester (QA / Test Verification Agent)
- **Current Behavior**: When test execution fails or times out, the `Tester` agent often receives only a binary pass/fail or an exit code.
- **Dynamic Enhancement**: Injecting the last 60 seconds of `stdout`/`stderr` from the test runner (e.g. `go test`, `pytest`, `npm test`) provides the `Tester` agent with exact panic tracebacks, failing assertion line numbers, or unhandled exceptions. This enables the `Tester` to generate high-resolution bug reports for the `Generator` agent.

### 7.2 AgentRoleGenerator (Worker / Implementation Agent)
- **Current Behavior**: During complex multi-step build/compilation loops, the worker agent must either wait for a command to finish completely or re-run failed commands blindly.
- **Dynamic Enhancement**:
  - **Inter-step Feedback**: If a tool command (like `go build` or `make`) fails mid-execution, injecting the tail of the build log directly into the `Generator`'s next turn prompt allows immediate, surgical fix attempts (e.g., resolving a single missing import line) without flooding the LLM context with full repository files.
  - **On Retry**: When assigned a re-queued task, receiving the last 60s logs from attempt $N-1$ ensures the `Generator` does not repeat identical broken commands.

### 7.3 AgentRoleResolver (Merge Conflict & Repair Agent)
- **Current Behavior**: The `Resolver` agent attempts to reconcile conflicting task changes or systemic build failures across parallel branches.
- **Dynamic Enhancement**: Equipping the `Resolver` with the 60-second execution tail of the breaking build gives it immediate context on which specific symbol collision or type mismatch broke the integration, allowing faster resolution.

### 7.4 AgentRolePlanner (Product Manager / Roadmap Agent)
- **Current Behavior**: During initial spec decomposition or project scaffolding, commands like `create-react-app`, `cargo init`, or `go mod init` may stall on dependency fetching.
- **Dynamic Enhancement**: If a scaffolding step stalls, `Planner` can inspect the 60s log tail to determine if external package repositories are down or if flags need modification.

---

## 8. Summary & Next Steps for Discussion

This proposal fits naturally into Noctifab's existing architecture:
- **Location**: `pkg/services/unblocker.go` & `pkg/services/unblocker_prompt.go`
- **Dependencies**: Integrates existing `UnblockerAgent` with task log files (`pkg/services/runner.go` / `.noctifab/logs/`).
- **No breaking changes**: Enhances `buildUnblockerPrompt` without changing the Orchestrator's core execution loop.

We can discuss:
1. Should the logs be injected **only to the Unblocker Agent**, or should the worker agent also receive the log snippet upon task re-dispatch?
2. What time window (60s vs 120s vs line count limit) makes the most sense for typical dark factory CLI workloads?


