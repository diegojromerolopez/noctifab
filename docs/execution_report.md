# Execution Reports and Logs

`noctifab` provides a structured execution reporting and telemetry subsystem that records fine-grained events during autonomous runs and synthesizes a deterministic, human-and-machine-readable Markdown **Execution Report** without parsing raw container logs.

---

## Architectural Overview

The reporting architecture distinguishes between two complementary concepts:

1. **`execution_log` (Event Stream & Telemetry)**:
   - A concurrency-safe stream of structured timeline events (`ExecutionEvent` / `ExecutionLog`) captured during orchestrator, planner, generator, tester, and unblocker agent activities.
   - Tracks event timestamps, agent roles, phase transitions, task attempts, duration measurements (in milliseconds), token usage, errors, and retries.

2. **`execution_report` (Synthesized Diagnostic Report)**:
   - The Markdown report artifact (`<TIMESTAMP>_<PROJECT>.md`) generated continuously during execution and finalized at completion.
   - **Real-Time Live Checkpointing**: The report is flushed atomically in real time every **5 seconds** (and immediately on major lifecycle events like story start/end), allowing developers to inspect live progress, current activity, active spans, and token consumption without token-heavy polling or container log parsing.
   - Documents executive summaries, live status tables, active/waiting metrics, human-readable bottlenecks, error breakdown tables, actionable recommendations, and deliverables.

---

## Configuration

Enable execution reporting in your project's `.noctifab/config.yaml`:

```yaml
config_version: "2.0"
execution_report: ".noctifab/reports/execution_report.md"
```

### Path Resolution & Boundary Rules

When `execution_report` is specified:
- Noctifab formats the report path with a UTC timestamp and canonical workspace folder name:
  ```text
  validation/projects/wc/output/report/20260813_094612_wc.md
  ```
- Path resolution strictly enforces workspace boundaries. Reports cannot be placed inside forbidden paths such as `.git/` or `secrets.yaml`.
- All report files are written atomically using exclusive temporary files with restrictive permissions (`0600`) and parent directory synchronization.

---

## Report Structure

The generated `<TIMESTAMP>_<PROJECT>.md` contains the following structured sections:

| Section | Description |
| :--- | :--- |
| **Title & Status Header** | Top-level title, overall execution outcome (`RUNNING`, `SUCCESS`, `FAILED`, `CANCELLED`), run ID, and live status indicator. |
| **Executive Summary** | Concise, synthesized summary of run outcome, elapsed physical time, error count, and retries. |
| **Live Status** | Real-time status table showing stories completed, tasks passed, errors, token usage, elapsed time, and active LLM provider. |
| **Run Metadata** | Execution command, project path, canonical report path, start timestamp, and Noctifab version. |
| **Time Spent** | **Lead Time** (total physical clock time elapsed from start to completion) and reporting overhead. |
| **Agent Performance** | Breakdown of agent invocations per role, story, task, active execution span (omitting zero units, e.g. `17s 116ms`), and outcome. |
| **Phase Performance** | **Phase Cycle Time** (de-duplicated phase clock time) and **Execution Spans** across pipeline phases (Roadmap Generation, Story Execution). |
| **Codebase Changes & Workspace Impact** | Files modified, lines added/deleted, and net line delta. |
| **Self-Correction & Turn Efficiency** | Retries recorded, unblocker interventions, watchdog interventions, and task pass efficiency rate. |
| **Bottlenecks** | Human-readable bottleneck diagnoses detailing scope, measurement, and resolution impact. |
| **Developer Recommendations & Next Actions** | Actionable recommendations mapping target scopes to concrete verification steps. |
| **User Story & Task Results** | Summary of user stories (including spent time per story) and tasks (displaying Task ID, human-readable Task Title, parent Story ID, attempts, and elapsed time). |
| **Reliability & Concurrency** | Error counts, retries, dropped events, and a detailed **Execution Errors** breakdown table. |
| **Deliverables & Documentation** | Workspace implementation root and canonical execution report path. |
| **Verification & Testing Strategy** | Verification layers (automated unit tests, isolation worktree compilation, black-box contracts) and testing strategy notes. |
| **LLM & Cost Usage** | Total measured token counts and active provider failover chains. |

---

## LLM Analyzer Integration

When an LLM client is available, Noctifab invokes a read-only analysis step using the `submit_report_analysis` virtual action call:
- Input payloads are strictly bounded to 64 KB.
- Output JSON arguments populate synthesized summary narratives, issue priorities, hypotheses, and improvement proposals.
- If no LLM client is active or the analyzer call fails, Noctifab gracefully falls back to deterministic rule summaries without breaking the build or CLI execution.
