# Execution Reports and Logs

`noctifab` provides a structured execution reporting and telemetry subsystem that records fine-grained events during autonomous runs and synthesizes a deterministic, human-and-machine-readable Markdown **Execution Report** without parsing raw container logs.

---

## Architectural Overview

The reporting architecture distinguishes between two complementary concepts:

1. **`execution_log` (Event Stream & Telemetry)**:
   - A concurrency-safe stream of structured timeline events (`ExecutionEvent` / `ExecutionLog`) captured during orchestrator, planner, generator, tester, and unblocker agent activities.
   - Tracks event timestamps, agent roles, phase transitions, task attempts, duration measurements (in milliseconds), token usage, errors, and retries.

2. **`execution_report` (Synthesized Diagnostic Report)**:
   - The final and periodic Markdown report artifact (`execution_report.md`) generated from snapshot aggregation.
   - Documents executive summaries, live execution status tables, role active/waiting metrics, deterministic bottlenecks (`BN-*`), evidence-backed issues (`ISSUE-*`), actionable proposals (`PROP-*`), and read-only model hypotheses.

---

## Configuration

Enable execution reporting in your project's `.noctifab/config.yaml`:

```yaml
config_version: "2.0"
execution_report: ".noctifab/reports/execution_report.md"
```

### Path Resolution & Boundary Rules

When `execution_report` is specified:
- If given a standard filename (e.g. `.noctifab/reports/execution_report.md` or `execution_report.md`), Noctifab formats it with a UTC timestamp and workspace folder name:
  ```text
  .noctifab/reports/20260811_220000_myproject.md
  ```
- Path resolution strictly enforces workspace boundaries. Reports cannot be placed inside forbidden paths such as `.git/` or `secrets.yaml`.
- All report files are written atomically using exclusive temporary files with restrictive permissions (`0600`) and parent directory synchronization.

---

## Report Structure

The generated `execution_report.md` contains the following structured sections:

| Section | Description |
| :--- | :--- |
| **Title & Status Header** | Top-level title, overall execution outcome (`SUCCESS`, `FAILED`, `CANCELLED`), run ID, and checkpoint indicator. |
| **Executive Summary** | Concise, synthesized summary of the run outcome, total wall time, error count, and retries. |
| **Live Status** | Stable single-row summary table showing completion percentage, stories, tasks, errors, retries, token usage, elapsed time, and active provider. |
| **Run Metadata** | Execution command, project path, report path, start timestamp, and Noctifab version. |
| **Outcome & Time Spent** | Total wall time breakdown and reporting overhead measurements. |
| **Agent Performance** | Breakdown of agent invocations per role, story, task, active time, LLM time, tool time, turn count, and outcome. |
| **Phase Performance** | Union wall time measurements across pipeline phases (Planner, Generator, Tester, Unblocker). |
| **Bottlenecks** | Deterministic performance bottleneck rules derived by the reporting engine: <br>- `BN-PHASE-DOMINANT`: Phase consumes >40% total execution wall time. <br>- `BN-OP-DOMINANT`: Tool/operation consumes >35% active time. <br>- `BN-RETRY`: Excess task retries observed. <br>- `BN-TIMEOUT`: Operation timeouts encountered. <br>- `BN-TOKEN`: High token consumption rate. |
| **Issues Found** | Evidence-backed issue entries (`ISSUE-<HASH>`) classified by category, severity (`critical`, `high`, `medium`, `low`), scope, title, and impact. |
| **Proposals & Next Actions** | Actionable recommendations (`PROP-<HASH>`) mapping issue root causes to concrete verification steps. |
| **User Story and Task Results** | Summary of user stories and task attempt outcomes. |
| **LLM & Cost Usage** | Token counts and estimated LLM costs. |
| **Reliability & Concurrency** | Error count, retry count, and dropped event metrics. |

---

## LLM Analyzer Integration

When an LLM client is available, Noctifab invokes a read-only analysis step using the `submit_report_analysis` virtual action call:
- Input payloads are strictly bounded to 64 KB.
- Output JSON arguments populate synthesized summary narratives, issue priorities, hypotheses, and improvement proposals.
- If no LLM client is active or the analyzer call fails, Noctifab gracefully falls back to deterministic rule summaries without breaking the build or CLI execution.
