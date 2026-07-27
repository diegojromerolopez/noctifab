# 🚀 Dark Factory Speed, Latency & Reliability Benchmarks

This document defines the empirical performance standards, reliability metrics, and automated benchmark harness design for `noctifab`.

---

## 1. Benchmark Suites & Latency Targets

`noctifab` measures dark factory software generation performance across standardized specifications:

### 1.1. Micro Spec Benchmark (`todo-cli`)
* **Target Spec:** Standard 3-story CLI application ([`validation/projects/todo-cli/SPEC.md`](file:///Users/diegoj/repos/noctifab/validation/projects/todo-cli/SPEC.md)).
* **Target Stack:** Go (`go test -v ./...` & `golangci-lint`).
* **Latency Target:** **< 3 Minutes (180s)** total execution time from specification ingest to final auto-merged integration branch.
* **Target TTFC (Time To First Commit):** **< 30 seconds**.

### 1.2. Medium Spec Benchmark (REST HTTP API)
* **Target Spec:** 10-story HTTP REST API microservice with persistent storage and authentication.
* **Target Stack:** Go / Node.js / Python FastAPI.
* **Latency Target:** **< 10 Minutes (600s)** total execution time.
* **Target First-Pass Success Rate:** **> 90%** of tasks passing 3x consensus without watchdog repair retries.

---

## 2. Key Metrics & Telemetry Tracked

Every benchmark run records structured JSON execution metrics stored in `output/metrics/`:

| Metric Name | Symbol | Description | Target Standard |
|---|---|---|---|
| **Time To First Commit** | `TTFC` | Duration from spec parsing to initial Git commit | < 30 seconds |
| **Total Execution Time** | `T_exec` | Total wall-clock time for full spec completion | < 3 mins (micro), < 10 mins (medium) |
| **First-Pass Repair Rate** | `R_first` | Percentage of tasks passing validation on Retry = 0 | > 90% |
| **Consensus Pass Rate** | `P_consensus` | Percentage of tasks achieving ≥ 2/3 test pass consensus | 100% |
| **Token Efficiency** | `Tokens/Story` | Average total LLM tokens used per story | < 15,000 tokens / story |

---

## 3. Stalling & Failover Resiliency Benchmarks

To guarantee zero process hangs under adverse network conditions:

1. **Socket Silence Simulation:**
   * Mock LLM socket drops connection and stops emitting bytes mid-response.
   * **Target:** Orchestrator watchdog detects timeout within **45 seconds**, aborts context, and switches to fallback provider without goroutine leaks.
2. **Infinite JSON Repetition Loop:**
   * Mock LLM returns repeating JSON payload loop.
   * **Target:** Preprocess validator halts response parsing after max token threshold and triggers repair retry.
3. **500 Server Error Failover:**
   * Primary LLM returns HTTP 500 error.
   * **Target:** Seamless failover to secondary LLM client in `< 2 seconds`.

---

## 4. Multi-Worker Parallel Concurrency Benchmark

* **Configuration:** `concurrency = 4` worker goroutines executing an 8-task independent DAG.
* **Verification Criteria:**
  * Zero Optimistic Concurrency Control (OCC) state corruption in SQLite/PostgreSQL state tables.
  * All 4 workers execute in separate Git worktree directories without file contamination.
  * Final integration branch cleanly merges all task commits.

---

## 5. Automated Benchmark Harness Implementation Plan

### Phase 1: Benchmark CLI Command (`noctifab benchmark`)
* Add `noctifab benchmark --spec <spec> --target-time <seconds>` command in `cmd/noctifab/cli/benchmark.go`.
* Automatically run spec against target sandbox, time all execution phases, and dump `benchmark_results.json`.

### Phase 2: CI Benchmark Regression Gate
* Add GitHub Actions workflow `.github/workflows/benchmark.yml` triggered on pull requests.
* Enforce latency budget: fail CI if `todo-cli` benchmark execution exceeds 180 seconds.
