# Validation Containers Execution Registry

This document tracks all validation containers executed in the Noctifab dark factory environment, recording execution timestamp, status, tool actions, user story completion, and detailed results.

---

## 1. Registry Table

| Project | Tech Stack | Status | Start Time (UTC) | Completion Time (UTC) | Lead Time | Stories Completed | Tasks Completed | Key Results / Observations |
| :--- | :--- | :---: | :--- | :--- | :--- | :---: | :---: | :--- |
| [`wc`](file:///Users/diegoj/repos/noctifab/validation/projects/wc) | Rust CLI | ✅ SUCCESS | 2026-08-14 11:17:45 | 2026-08-14 11:24:19 | 6m 33s | 4/4 | 12/12 | 100% task efficiency (23/23 attempts). Rust streaming counters, multi-file summary, UTF-8 char counting & max line length implemented. |
| [`pyedis`](file:///Users/diegoj/repos/noctifab/validation/projects/pyedis) | Python 3.14 + FastAPI | ✅ SUCCESS | 2026-08-14 12:19:20 | 2026-08-14 13:52:00 | 1h 32m 39s | 1/1 | 6/6 | 100% task efficiency (8/8 attempts). Typed Python Redis server (FastAPI, key storage, TTL clock, AOF persistence, uvicorn HTTP POST /commands). 32 files created/modified (+3,648 lines). |

---

## 2. Completed Run Details

### 🦀 2.1 `wc` (Rust `wc` CLI)

- **Execution Window:** 2026-08-14 11:17:45 – 11:24:19 UTC  
- **Lead Time:** 6m 33s 535ms  
- **Status:** **SUCCESS**  
- **Providers Used:** `gemini`, `openai`  
- **User Stories (4/4 Completed):**
  1. `story-0001`: Implementation of WC Utility (US-001)
  2. `story-0002`: Core wc counting (lines, words, bytes) for a single file or stdin (US-002)
  3. `story-0003`: Multi-file support with total counts summary (US-003)
  4. `story-0004`: UTF-8 characters count, maximum line length, and error handling (US-004)
- **Tasks (12/12 Completed):** Project initialization, streaming counter logic, multi-file orchestration, CLI integration, UTF-8 fallback & max line length logic.
- **Codebase Impact:** 5 files changed (+125 lines, -27 lines)
- **Execution Report:** [`validation/projects/wc/output/report/20260814_111745_wc.md`](file:///Users/diegoj/repos/noctifab/validation/projects/wc/output/report/20260814_111745_wc.md)

---

### 🐍 2.2 `pyedis` (Python 3.14 + FastAPI Redis Server)

- **Execution Window:** 2026-08-14 12:19:20 – 13:52:00 UTC  
- **Lead Time:** 1h 32m 39s 862ms  
- **Status:** **SUCCESS**  
- **Providers Used:** `anthropic`, `gemini`  
- **User Stories (1/1 Completed):**
  1. `story-0001`: US-001: pyedis Core Implementation & Legacy Stabilization
- **Tasks (6/6 Completed):**
  1. `task-f1bbe2ae`: Project scaffold, tooling config, and Dockerfile characterization tests
  2. `task-836f0dc6`: Implement `store.py`: keyspace, injected Clock, and TTL/expiry semantics
  3. `task-8fb5fa32`: Implement `commands.py`: Redis-flavored dispatcher with ERR/WRONGTYPE conventions
  4. `task-f00a751c`: Implement `persistence.py`: append-only AOF writer, fsync policy, and startup replay
  5. `task-48b13db0`: Implement `main.py` FastAPI app factory and HTTP POST `/commands` integration tests
  6. `task-830d2e16`: Wire Dockerized e2e suite into `make e2e` with coverage and lint gates
- **Codebase Impact:** 32 files changed (+3,648 lines, -40 lines)
- **Tokens Measured:** 3,026,722 tokens
- **Contract Verification:** `api.commands` (HTTP POST `/commands` interface via `uvicorn`) — **PASSED**
- **Execution Report:** [`validation/projects/pyedis/output/report/20260814_121920_pyedis.md`](file:///Users/diegoj/repos/noctifab/validation/projects/pyedis/output/report/20260814_121920_pyedis.md)
