# Noctifab E2E Autonomy Validation Matrix

This directory contains resources to run fully containerized, isolated, end-to-end (E2E) integration checks of `noctifab` implementing software systems autonomously inside target validation projects based strictly on their technical specifications (`SPEC.md`).

---

## 1. The Validation Matrix at a Glance

The matrix encompasses 17 distinct software projects covering diverse languages, paradigms, toolchains, and architectural boundaries:

| Project | Language / Stack | Architecture Seam | Strong Axis |
| :--- | :--- | :--- | :--- |
| `echo` | Go CLI | Single-process CLI | Baseline smoke & full loop integrity |
| `todo-cli` | Go CLI | CLI + JSON file persistence | Subcommand correctness & state handling |
| `wc` | Rust CLI | CLI, strict compiler, memory-efficient streaming | Strict compiler adherence & streaming I/O |
| `calculator` | Ruby CLI | CLI + REPL | Linter self-healing loop (RuboCop / RSpec) |
| `fortune` | C17 + SQLite | Native memory management + embedded DB C API | Low-level correctness & build discipline |
| `t4` | C17 | Network HTTP server, black-box public contract | Network server & HTTP protocol parsing |
| `pyedis` | Python 3.14 (asyncio + RESP2/3) | In-memory key-value store, typed API, AOF persistence | Type rigor (`mypy --strict`), concurrency, durability |
| `notebook` | TypeScript (React + Fastify + PostgreSQL) | Full-stack SPA + REST API + JWT Auth + WebSockets | Relational DB (SQL, migrations) + strict TypeScript |
| `frontpunch` | Python + Valkey | Distributed task broker & async background workers | Concurrency, broker queues, worker lifecycle |
| `djanban` | Python 3.12 + Django 5.x | Legacy codebase modernization & Kanban engine | Framework modernization & migration safety |
| `stricc` | Rust + LLVM 18 + C | Safe C compiler, GCC/Clang differential testing | Rigor, compiler correctness & memory safety |
| `searchthedocs` | Python 3.12 + FastAPI + Redis | Async scraping queues & RAG vector search engine | Async workers, search indexing & vector retrieval |
| `auth-vault` | Go 1.22+ | OAuth2/OIDC Zero-Trust Authorization Server + PKI Vault | Rigor, cryptographic identity & zero-trust auth |
| `buffonstream` | Go 1.22+ (gRPC / Protobuf) | Protobuf-native real-time bi-directional streaming storage | Protocol Buffers, gRPC & CDC streaming |
| `jpacioli` | Java 21 + Spring Boot 3.3+ + PostgreSQL | Full Event Sourcing (ES) + CQRS + Financial Ledger + JWT/RBAC | Enterprise rigor, event sourcing & CQRS |
| `ocalogue` | OCaml 5.x + Dune | Datalog deductive logic engine + Semi-Naive Fixpoint | Formal algorithmic logic & stratified negation |
| `ninline` | Python 3.14 (CLI + Game Engine) | Generalized (M,N,K)-Game, Ray-Casting, Minimax AI | Model-per-Agent routing & deterministic search |

---

## 2. Tier Classification & Diagnostic Capability Ladder

Validation projects are classified by **how much diagnostic signal each run yields per unit of execution time and LLM token consumption**.

### 2.1 Tier Classification

| Tier | Purpose | Projects |
| :--- | :--- | :--- |
| **Tier 0 — Baseline Smoke** | Fastest, cheapest full-loop proof (init → PM → plan → generate → test → merge). Always run first: if this fails, core orchestration is broken. | `echo` |
| **Tier 1 — Differentiating Seams** | Core capability coverage testing network/HTTP, typed concurrency, relational databases, distributed brokers, security vaults, and event sourcing. | `t4`, `pyedis`, `notebook`, `djanban`, `auth-vault`, `buffonstream`, `jpacioli`, `ninline` |
| **Tier 2 — Rigor Probes** | Deep quality and discipline validation under unforgiving compilers, typecheckers, and linters. | `calculator`, `wc`, `fortune`, `stricc`, `ocalogue` |
| **Tier 3 — Breadth & Heavy Integration** | Heavy runtime services, persistent state brokers, or multi-stage pipelines with high API rate-limit and duration footprint. | `todo-cli`, `frontpunch`, `searchthedocs` |

### 2.2 Recommended Diagnostic Order (Progressive Capability Ladder)

When validating changes to `noctifab` itself, run projects along this progressive diagnostic ladder so failures attribute cleanly to specific subsystems:

| # | Project | Capability Tested | Failure Attribution |
| :-: | :--- | :--- | :--- |
| **Wave 1 — The Core Loop** | | | |
| 1 | `echo` | Full loop integrity (init → PM → plan → generate → test → merge) with minimal tokens | Core orchestrator broken; stop here |
| 2 | `todo-cli` | Subcommand parsing + JSON file-based state persistence | File persistence / state handling |
| 3 | `calculator` | Linter self-healing loop (RSpec + RuboCop) | Linter retry/repair loop |
| **Wave 2 — Strict Discipline** | | | |
| 4 | `wc` | Strict compiler adherence (Rust + `clippy -D warnings`) | Architectural discipline / strict typing |
| 5 | `fortune` | Native memory safety + SQLite C API integration | Native toolchain & C memory management |
| **Wave 3 — Differentiating Seams** | | | |
| 6 | `t4` | Network server: black-box HTTP contract (status codes, headers, `Range`, binary) | HTTP parsing & socket daemon handling |
| 7 | `pyedis` | Typed Python (`mypy --strict`), DI clock/store, AOF persistence + async event loop | Concurrency & typing rigor |
| 8 | `notebook` | Relational database (migrations, SQL, ephemeral PostgreSQL) + strict TypeScript | Relational DB & JS ecosystem |
| **Wave 4 — Heavy Integration** | | | |
| 9 | `frontpunch` | Distributed broker seam (external Valkey, background workers, scheduling) | Distributed queue / worker concurrency |

**Fast Triage Check (4 Projects):** Running `echo`, `t4`, `pyedis`, and `notebook` de-risks the CLI loop, network server, typed concurrency, and relational DB seams in under 30 minutes.

---

## 3. Architecture & Project Layout

```
validation/
├── Dockerfile.validation          # Multi-stage base image (noctifab binary + projects)
├── README.md                      # This comprehensive guide
├── bin/                           # Harness execution scripts
│   ├── matrix_runner.py           # Multi-project matrix runner with dynamic scale timeouts
│   ├── runner_9projects.py        # Canonical 9-project sequential runner
│   ├── runner_all17.py            # Comprehensive 17-project sequential runner
│   ├── serial_runner.py           # Generic serial runner with dynamic timeouts
│   ├── run_one.sh                 # Single project runner (build, mount, run, trace)
│   ├── run_all.sh                 # Multi-project runner with parallel/serial flags
│   ├── validate.sh                # Container entrypoint script executed inside Docker
│   └── summarize_reports.sh       # Summary aggregator for execution reports
└── projects/                      # Project definitions (isolated templates)
    ├── auth-vault/{Dockerfile, SPEC.md, .noctifab/}
    ├── buffonstream/{Dockerfile, SPEC.md, .noctifab/}
    ├── calculator/{Dockerfile, SPEC.md, .noctifab/}
    ├── djanban/{Dockerfile, SPEC.md, .noctifab/}
    ├── echo/{Dockerfile, SPEC.md, .noctifab/}
    ├── fortune/{Dockerfile, SPEC.md, .noctifab/}
    ├── frontpunch/{Dockerfile, SPEC.md, .noctifab/}
    ├── jpacioli/{Dockerfile, SPEC.md, .noctifab/}
    ├── ninline/{Dockerfile, SPEC.md, .noctifab/}
    ├── notebook/{Dockerfile, SPEC.md, .noctifab/}
    ├── ocalogue/{Dockerfile, SPEC.md, test_suite/, .noctifab/}
    ├── pyedis/{Dockerfile, SPEC.md, .noctifab/}
    ├── searchthedocs/{Dockerfile, SPEC.md, .noctifab/}
    ├── stricc/{Dockerfile, SPEC.md, .noctifab/}
    ├── t4/{Dockerfile, SPEC.md, .noctifab/}
    ├── todo-cli/{Dockerfile, SPEC.md, .noctifab/}
    └── wc/{Dockerfile, SPEC.md, .noctifab/}
```

### 3.1 Container Hierarchy & Toolchain Layering

The base image (`Dockerfile.validation`) is a multi-stage build: `golang:1.25-alpine` compiles the `noctifab` binary, which is copied into an `alpine:3.21` runtime alongside project templates and `validate.sh`. Each target project layers its required compilers, linters, and runtimes:

| Project | Base Layer (`FROM`) | Toolchains Installed | Target Artifacts Checked |
| :--- | :--- | :--- | :--- |
| `echo` | `noctifab-validation:base` | go, make | `cmd/echo/main.go` |
| `todo-cli` | `noctifab-validation:base` | go, make | `cmd/todo/main.go` |
| `wc` | `rust:1.84-alpine` (+ base) | rustc, cargo, rustfmt, clippy | `Cargo.toml`, `src/main.rs` |
| `calculator` | `ruby:3.2-alpine` (+ base) | ruby, rspec, rubocop | `calculator.rb`, `lib/calculator/cli.rb` |
| `fortune` | `noctifab-validation:base` | gcc, make, sqlite-dev | `main.c`, `Makefile` |
| `t4` | `alpine:3.21` (+ base) | gcc, make, clang-format, clang-tidy | `Makefile`, `docker-compose.yml`, `src/t4.c` |
| `pyedis` | `python:3.14-alpine` (+ base) | python3.14, redis, pytest, ruff, mypy | `app/main.py`, `pyproject.toml` |
| `notebook` | `node:22-alpine` (+ base) | node22, npm, typescript, eslint, vitest, postgresql | `src/index.ts`, `package.json`, `docker-compose.yml` |
| `frontpunch` | `noctifab-validation:base` | python3, pip, ruff, mypy, valkey | `frontpunch/worker.py` |
| `djanban` | `python:3.12-alpine` (+ base) | python3.12, django5, ruff | `manage.py`, `djanban/settings.py` |
| `stricc` | `rust:alpine` (+ base) | rustc, cargo, llvm18, gcc, clang | `Cargo.toml`, `stricc/src/main.rs` |
| `searchthedocs` | `python:3.12-alpine` (+ base) | python3.12, fastapi, redis | `app/main.py` |
| `auth-vault` | `golang:1.22-alpine` (+ base) | go, make | `cmd/server/main.go` |
| `buffonstream` | `golang:1.22-alpine` (+ base) | go, protoc, make | `cmd/server/main.go` |
| `jpacioli` | `eclipse-temurin:21-jdk-alpine` (+ base) | java21, gradle, postgresql | `build.gradle`, `src/**/*.java` |
| `ocalogue` | `ocaml/opam:alpine-ocaml-5.2` (+ base) | ocaml5.2, opam, dune, menhir | `dune-project`, `lib/*.ml` |
| `ninline` | `python:3.14-alpine` (+ base) | python3.14, pytest, ruff, mypy | `app/main.py` |

---

## 4. How It Works & Key Constraints

### 4.1 Execution Flow

1. **Isolation**: Containers execute with zero host filesystem mutation, except for mounted output paths (`output/src/`, `output/log/`, `output/report/`, `output/dist/`).
2. **Ephemeral Workspace Initialization** (`validate.sh`):
   - Copies the project template to `/tmp_verify_autonomy/`.
   - Initializes a fresh Git repository, creates a local bare `origin`, and commits baseline files.
   - Copies the securely mounted `secrets.yaml` into `.noctifab/secrets.yaml`.
   - Executes `noctifab start` using the root directory or `SPEC.md`.
3. **Verification & Assertions**:
   - `validate.sh` verifies that expected source code artifacts were created.
   - Executes test suites (`make test`, `cargo test`, `pytest`, `rspec`, etc.) and linters.
   - Emits structured exit codes and syncs final outputs.

### 4.2 Spec-Driven Validation Rule (Host Side)

> [!IMPORTANT]
> **Strictly Prohibited:** AI agents developing `noctifab` must **never** modify, pre-create, hand-edit, or check in static user stories under any `validation/projects/<project>/roadmap/` directory.
> Validation projects exist specifically to test whether Noctifab autonomously decomposes a raw `SPEC.md` into roadmaps and stories on the fly via its Product Manager Agent. Having existing fixture stories in legacy projects is acceptable, and Noctifab generating stories autonomously at runtime is expected; what is forbidden is manually writing roadmap files as a shortcut.

### 4.3 Configuration Immutability Mandate

> [!CAUTION]
> AI agents are strictly forbidden from modifying any validation project's `.noctifab/config.yaml` configuration file before or during validation execution. The project configuration must remain exactly as defined in the target project.

### 4.4 What the SPECs Demand of Generated Code

Every `SPEC.md` across all 17 projects enforces consistent engineering standards:
- **SOLID & Domain-Driven Design (DDD)**: Single-responsibility modules, domain entities decoupled from framework/transport layers.
- **Dependency Injection (DI)**: Injected stores, clocks, database connection pools, and brokers; zero global state singletons.
- **Realistic Unit Testing**: Testing with real collaborator objects wired through DI (e.g. real store with fake clock) rather than blanket mock objects.
- **Hermetic Integration Testing**: In-memory or temporary file doubles (`:memory:` SQLite, temp AOF file, ephemeral containers).
- **Zero-Finding Linter Passes**: Generated code must pass all configured linters (`ruff`, `mypy --strict`, `clippy -D warnings`, `rubocop`, `clang-tidy`, `eslint`).

---

## 5. Execution Guide & Harness Usage

### 5.1 Secret Handling & Credentials Setup

Validation images are strictly **credential-free**. Secret files are excluded from image builds via `.dockerignore`.

Create `validation/projects/<project>/.noctifab/secrets.yaml` on the host:
```yaml
OPENCODE_API_KEY: "your-actual-api-key"
GITHUB_TOKEN: "your-optional-github-token"
```

At runtime, `run_one.sh` bind-mounts this file read-only into `/run/secrets/noctifab-secrets.yaml`, and `validate.sh` copies it into the ephemeral workspace. You can override the host secrets path by setting `NOCTIFAB_SECRETS_FILE=<path>`.

### 5.2 Execution Commands

#### 1. Flexible Matrix Runner (Recommended for targeted testing)
Runs any subset of projects with automatic scale-based dynamic timeouts:
```bash
python3 validation/bin/matrix_runner.py echo t4 pyedis
# Or with comma-separated flag:
python3 validation/bin/matrix_runner.py --projects=wc,notebook,djanban
# With fixed timeout override:
python3 validation/bin/matrix_runner.py echo --timeout=1200
```

#### 2. Canonical 9-Project Sequential Runner
Runs the 9 core diagnostic projects in sequence and writes `PROJECT_FEEDBACK.md` and `VAL_PROJECT_FEEDBACK.md`:
```bash
python3 validation/bin/runner_9projects.py
# With custom timeout:
python3 validation/bin/runner_9projects.py --timeout=1500
```

#### 3. Full 17-Project Comprehensive Runner
Executes the full suite across all 17 validation projects sequentially:
```bash
python3 validation/bin/runner_all17.py
```

#### 4. Make Targets
```bash
# Run a single project
make validate PROJECT=echo

# Reuse existing docker images (skipping the rebuild phase)
make validate PROJECT=echo SKIP_BUILD=1

# Run all projects in parallel
make validate-all

# Run all projects in serial mode
make validate-all SERIAL=1
```

#### 5. Direct Shell Scripts
```bash
# Run a single project directly
./validation/bin/run_one.sh wc

# Run a subset sequentially
./validation/bin/run_all.sh --serial --projects echo,t4,pyedis
```

### 5.3 Scale-Based Dynamic Timeouts

Validation runs apply execution envelopes scaled by architectural complexity (Complexity Units, $CU$):

| Scale Class | Complexity Units | Max Runtime Envelope | Target Projects |
| :--- | :---: | :---: | :--- |
| **Tier 0 / Small CLI Utilities** | $CU < 35$ | 15–20 minutes | `echo`, `calculator`, `wc`, `todo-cli`, `fortune` |
| **Tier 1 / Medium Systems** | $35 \le CU \le 75$ | 30 minutes | `t4`, `frontpunch`, `ocalogue`, `ninline`, `pyedis`, `stricc` |
| **Tier 2 / Large Multi-Subsystem** | $CU > 75$ | 35–40 minutes | `notebook`, `djanban`, `auth-vault`, `buffonstream`, `searchthedocs`, `jpacioli` |

If a project exceeds its dynamic scale envelope (or explicit `--timeout` limit), the container execution is cleanly terminated and recorded.

---

## 6. Monitoring, Telemetry & Artifact Inspection

### 6.1 Monitoring Rule for AI Agents

> [!IMPORTANT]
> When executing validation runs in the background, **AI agents MUST NOT execute periodic 60-second polling loops or schedule recurring timers**.
> Launch the runner as a background task and wait for completion notifications or inspect the live execution reports on demand.

### 6.2 Output Artifact Locations

All artifacts are persisted directly under the project's output directory:

| Artifact | Path | Description |
| :--- | :--- | :--- |
| **Live Source Code** | `validation/projects/<project>/output/src/` | Real-time generated codebase as Noctifab writes/edits files |
| **Execution Report** | `validation/projects/<project>/output/report/*.md` | Live atomic execution report emitted by Noctifab |
| **Console Output** | `validation/projects/<project>/output/log/<project>.log` | Combined stdout/stderr of the container run |
| **Wrapper Trace** | `validation/projects/<project>/output/log/<project>.wrap.log` | Container launch, image build, and lifecycle trace |
| **Compiled Distributables** | `validation/projects/<project>/output/dist/` | Compiled binaries and executables produced by the run |
| **Run Feedback** | `validation/projects/<project>/output/feedback/*.md` | Individual project evaluation report |
| **Global Summary** | `VAL_PROJECT_FEEDBACK.md` | Aggregated cross-project validation analysis |

To aggregate metrics, durations, and token consumption across all finished runs, run:
```bash
make validate-summary
# Or directly:
./validation/bin/summarize_reports.sh
```

### 6.3 Known Failure Signatures & Diagnostic Indicators

- **Linter Self-Healing Stall**: Repeated identical modifications attempting to satisfy a linter (e.g. RuboCop in `calculator`) — indicates retry ceiling reached or conflicting rules.
- **API Quota Saturation**: HTTP `429` errors with backoff retry indications — caused by running too many heavy parallel containers on a single LLM API key; resolve by running in serial mode or wave batches.
- **Roadmap Over-Decomposition**: Product Manager Agent generates excessive stories ($> 15$ for a small utility) without completing the foundation — verify PM prompt constraints for minimum working entrypoints.
- **Provider Eviction / Cooldown**: Model eviction triggered by transient compiler errors — ensure error classification distinguishes model non-existence from code errors.
- **Missing Build Dependencies**: Exit code 127 in `validate.sh` — indicates a toolchain package (`make`, `gcc`) was omitted from the project's `Dockerfile`.
