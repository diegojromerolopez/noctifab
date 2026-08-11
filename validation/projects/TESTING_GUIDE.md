# Testing noctifab through the Validation Projects

This guide explains **how to use the validation matrix to test `noctifab` itself** —
which projects to run, in what order, and what a failure in each one tells you about
the platform. It complements [`../README.md`](../README.md), which documents setup,
credentials, and the harness mechanics.

## 1. The Matrix at a Glance

| Project | Language / Stack | Architecture seam | Strong axis |
| :--- | :--- | :--- | :--- |
| `echo` | Go CLI | Single-process CLI | Effectiveness (smoke) |
| `todo-cli` | Go CLI | CLI + JSON file persistence | Usefulness |
| `wc` | Rust CLI | CLI, strict compiler, streaming | Effectiveness |
| `calculator` | Ruby CLI | CLI + REPL | Performance (linter loop) |
| `fortune` | C + SQLite | Native + embedded DB | Effectiveness |
| `t4` | C | Network HTTP server, black-box contract | Effectiveness |
| `pyedis` | Python 3.14 + FastAPI | Typed command API, AOF durability | Effectiveness |
| `notebook` | TypeScript (React + Fastify + PostgreSQL) | Full-stack SPA + REST API + JWT Auth + WebSockets | Usefulness |
| `frontpunch` | Python + Valkey | Async distributed workers | Usefulness |
| `djanban` | Python 3.12 + Django 5.x | Legacy codebase modernization | Effectiveness |
| `stricc` | Rust + LLVM 18 + C | Safe C compiler, GCC/Clang differential testing | Rigor / Safety |
| `searchthedocs` | Python 3.12 + FastAPI + Redis | Async Queue Scraper + RAG Vector Search Engine | Usefulness / AI |
| `auth-vault` | Go 1.22+ | OAuth2/OIDC Zero-Trust Authorization Server + PKI Vault | Rigor / Security |
| `sqlasm` | x86_64 Assembly (NASM) | Pure 64-bit Assembly B-Tree DBMS & SQL-92 Engine | Ultimate Low-Level Rigor |
| `buffonstream` | Go 1.22+ (gRPC / Protobuf) | Protobuf-Native Storage Engine & Real-Time Bi-Directional Streaming | Usefulness / Streaming |

## 2. Tier Classification (effectiveness)

Projects are classified by how much validation signal each run returns per unit of
time/tokens — the priority ramp to follow when reading results or running a subset.

| Tier | Purpose | Projects |
| :--- | :--- | :--- |
| **Tier 0 — Baseline smoke** | Cheapest full-loop proof (init → PM → plan → generate → test → merge). Run first, always: if this stalls, nothing else is worth reading. | `echo` |
| **Tier 1 — Differentiating seams** | New capability coverage the matrix previously lacked: network/black-box HTTP, typed-Python command API + durability, relational-DB + strict-TypeScript service, legacy Django modernization, zero-trust OAuth2/OIDC PKI, Protobuf real-time CDC streaming. The core set. | `t4`, `pyedis`, `notebook`, `djanban`, `auth-vault`, `buffonstream` |
| **Tier 2 — Rigor probes** | Deepen quality confidence under merciless toolchains and linter discipline (incl. compiler correctness, assembly, and safety matrix). | `calculator`, `wc`, `fortune`, `stricc`, `sqlasm` |
| **Tier 3 — Breadth** | State persistence and distributed/broker seams; heaviest runtime and highest API rate-limit exposure — run last or when targeting those seams specifically. | `todo-cli`, `frontpunch` |

## 3. Recommended Order to Test noctifab (diagnostic ramp)

To test the platform itself, run the projects as a **progressive capability ladder**:
each project de-risks one capability before you spend budget on the next, so a
failure attributes cleanly to a specific stage.

| # | Project | Capability it tests | If it fails, the problem is… |
| :--: | :--- | :--- | :--- |
| **Wave 1 — the loop** | | | |
| 1 | `echo` | Full loop integrity (init → PM → plan → generate → test → merge) at minimum cost | Core orchestrator broken; stop here |
| 2 | `todo-cli` | Subcommand correctness + file-based state persistence | Generation quality / state handling |
| 3 | `calculator` | Linter self-healing loop (RSpec + RuboCop — the known stall probe) | Retry/repair logic, linter iteration |
| **Wave 2 — strict discipline** | | | |
| 4 | `wc` | Architecture discipline under a merciless compiler (Rust + clippy `-D warnings`) | SOLID/DDD adherence in generated code |
| 5 | `fortune` | Native memory management + embedded SQLite C API | Low-level correctness, build discipline |
| **Wave 3 — the differentiating seams** | | | |
| 6 | `t4` | First network seam: black-box HTTP contract (status codes, headers, `Range`, binary) | Server/routing capability (highest-value new probe) |
| 7 | `pyedis` | Typed Python (mypy strict), DI clock/store, AOF durability + async concurrency | Ecosystem/typing rigor, durability |
| 8 | `notebook` | Relational DB (migrations, SQL, ephemeral PG) + strict TypeScript | SQL/DB seam, JS ecosystem |
| **Wave 4 — heavy integration** | | | |
| 9 | `frontpunch` | Distributed/broker seam (external Valkey, scheduling, concurrency) | Concurrency/distributed capability |

### Why this order

- **Cheapest, highest-signal first.** `echo` proves the entire loop for pennies;
  `calculator` surfaces the linter-loop bottleneck early (it already stalled once)
  before you spend budget on bigger projects.
- **Capability de-risking before cost.** Network (`t4`) comes before typed-app
  (`pyedis`) before relational (`notebook`); if noctifab cannot run a server yet,
  `pyedis`/`notebook` would fail confusingly anyway.
- **Most expensive last.** `frontpunch` is the heaviest and most rate-limit-exposed;
  it should fail only for its own reasons, never because an earlier stage was broken.

## 4. Practical Execution

- **Fast triage (a 4-run health check):** `echo`, `t4`, `pyedis`, `notebook` — covers
  loop + network + typed + DB, the four major seams.
- **Parallelism:** the harness runs everything concurrently by default
  (`make validate-all`). For *testing noctifab*, run the waves sequentially and
  parallelize *within* a wave (e.g. `wc` + `fortune` together). This avoids all nine
  containers failing on a wave-1 breakage and limits API 429 contention (parallel
  runs sharing one key have saturated quotas in the past — watch the `Errors` column).
- **Maximum Execution Time Limit (10 Minutes)**: A maximum execution time limit of **10 minutes** (unless another time limit is explicitly specified for the run) MUST be set for each execution of each validation project.
- **Reading results:** for each run, inspect the container log, the generated source
  in `output/src/`, and the `<PROJECT>_FEEDBACK.md` report — focusing on story count
  vs. planned, linter-loop iterations, test pass rate at hand-off, and wall-clock time.

## 5. Monitoring & Execution Report Inspection

While validation projects run in parallel or in the background (e.g. `make validate-all`),
agents **must rely on the generated execution report (`validation/projects/<project>/output/report/*.md`)** and background task completion notifications instead of running periodic 60-second polling loops or schedule timers.

### 5.1 Data sources
- Execution Report: `validation/projects/<project>/output/report/*.md` (live atomic report written during execution).
- Container console log: `validation/projects/<project>/output/log/<project>.log`
  (also mirrored at `.validation-logs/<project>.log`).
- Wrapper trace: `<project>.wrap.log` (build/launch/exit).
- Feedback: `<PROJECT>_FEEDBACK.md` (verdict, phase activity, failures, spec ambiguity)
  is written when the run finishes.
- Source/binary artifacts: `output/src/` and `output/dist/`.
| `frontpunch` | Running | No | 41% (11/27 stories) | 11/27 | 0/0 | Product Manager decomposing SPEC (over-decomposition risk) | 18m 30s | 21s ago | `glm-5.2` | — |

### 5.5 Known failure signatures to watch for
- **Linter self-healing stall:** repeated identical linter-fix edits (`calculator`/
  RuboCop is the canonical example) — intervene or cap retries.
- **API quota saturation:** HTTP `429` with `retryDelay` in the log when many
  projects run in parallel — spread the waves (§4) or rotate keys.
- **Roadmap over-decomposition:** story count far above `max_user_stories` (e.g. 27
  for `frontpunch`) — consolidation pass needed.
- **Non-JSON / schema retries:** repeated format reminders for envelope parsing.
- **Model failover:** `403`/`404` on a resolved model with fallback to a backup —
  expected, but frequent failovers add wall-clock time.

## 6. What the SPECs Demand of the Generated Code

Each project `SPEC.md` pins the same engineering contract, so a green run proves
noctifab produces code that satisfies:

- **SOLID + DDD:** single-responsibility modules; a pure domain layer with I/O and
  framework concerns at the edges.
- **Dependency injection:** all collaborators (store, clock, DB pool, repository)
  supplied through constructors/factories; no global singletons.
- **Unit tests that do NOT mock everything:** real collaborator objects wired via DI
  (e.g. a real store with an injected fake clock) instead of blanket mocks.
- **Integration tests that mock only I/O:** the real app against a real AOF temp file,
  real sockets, or a real ephemeral PostgreSQL.
- **E2E black-box:** a `docker-compose.yml` that builds the real service and a separate
  test-runner container exercising only the public HTTP contract.
- **Mandatory linters:** zero-finding `make lint` — `clang-tidy` + strict `gcc` for C,
  `ruff` + `mypy --strict` for Python, `eslint` + `tsc --noEmit` for TypeScript.
