# Noctifab E2E Autonomy Validation

This directory contains resources to run fully containerized, isolated, end-to-end (E2E) integration checks of `noctifab` implementing features autonomously inside a target project based only on its user story roadmap and configs.

## Available Projects

1. **`frontpunch`**: Checks out the base `main` branch of the frontpunch spec and executes `roadmap/US-001.md`. Validates that the Python worker infrastructure is successfully planned, written, tested, and passing.
2. **`todo-cli`**: Checks out the base `main` branch of a clean Todo CLI project spec and executes `roadmap/US-001.md`. Validates that task addition and listing commands are planned, implemented in `cmd/todo/main.go` and `internal/task/`, tested, and validated.
3. **`wc`**: Checks out the base `main` branch of a Rust `wc` (word count) project spec and executes `roadmap/US-002.md`. Validates that a UNIX `wc`-compatible CLI is planned and implemented in Rust following SOLID/DDD principles and memory-efficient streaming, with `Cargo.toml` and `src/main.rs` present and tests passing.
4. **`echo`**: Checks out the base `main` branch of a Go `echo` project spec and executes `roadmap/US-001.md`. Validates that a minimal `echo`-compatible CLI is planned and implemented in Go following clean formatting and with `cmd/echo/main.go` present and tests passing.
5. **`calculator`**: Checks out the base `main` branch of a Ruby terminal calculator project spec and executes `SPEC.md`. Validates that a terminal-based calculator is planned and implemented in Ruby following SOLID/DDD principles and RuboCop lint rules, with `calculator.rb` and `lib/calculator/cli.rb` present and RSpec tests passing.
6. **`fortune`**: Checks out the base `main` branch of a C fortune-quote project spec and executes `SPEC.md`. Validates that a C17 CLI is planned and implemented against the SQLite C API with a seeded 100-quote database, with `main.c` and a `Makefile` present and tests passing.
7. **`t4`**: Checks out the base `main` branch of a simplified, bucket-less S3-style object store spec and executes `SPEC.md`. Validates that a C17 HTTP server with a pinned black-box contract (PUT/GET/HEAD/DELETE/list/`Range`, deterministic `ETag`) is planned and implemented, with `src/t4.c`, a `Makefile`, and a `docker-compose.yml` e2e harness present and tests passing.
8. **`pyedis`**: Checks out the base `main` branch of a native Redis wire-protocol (RESP2/RESP3) key-value store spec and executes `SPEC.md`. Validates that an in-memory key-value store with AOF persistence, injected clock, and TCP RESP protocol is planned and implemented in Python 3.14 with type hints throughout (`mypy --strict`), with `app/main.py` and `pyproject.toml` present and tests passing.
9. **`notebook`**: Checks out the base `main` branch of a notes REST API spec and executes `SPEC.md`. Validates that a TypeScript (strict) + Fastify service on PostgreSQL is planned and implemented, with `src/index.ts`, `package.json`, and a `docker-compose.yml` e2e harness present and tests passing.
10. **`stricc`**: Checks out the base `main` branch of a C compiler spec written in Rust and executes `SPEC.md`. Validates that a defined-behavior safe C compiler with an LLVM 18 backend is planned and implemented in safe Rust (`#![deny(unsafe_code)]`), validated against GCC and Clang differential test suites.
11. **`djanban`**: Checks out the base `main` branch of a legacy Django 5.x Kanban board spec and executes `SPEC.md`. Validates modernization of legacy models, views, and migrations.
12. **`searchthedocs`**: Checks out the base `main` branch of a documentation scraper and vector search spec and executes `SPEC.md`. Validates async scraping queues and RAG vector search API.
13. **`auth-vault`**: Checks out the base `main` branch of an OAuth2/OIDC zero-trust authorization server spec and executes `SPEC.md`. Validates PKI vault and zero-trust authentication.
14. **`buffonstream`**: Checks out the base `main` branch of a Protobuf-native real-time streaming storage engine spec and executes `SPEC.md`. Validates bi-directional gRPC streaming.
15. **`jpacioli`**: Checks out the base `main` branch of an enterprise double-entry financial accounting ledger and transaction engine spec and executes `SPEC.md`. Validates Java 21, Spring Boot 3.3+, Full Event Sourcing (ES), CQRS with materialized read projections, and Stateless JWT / RBAC permissions against PostgreSQL.

### Project Tiers (effectiveness classification)

Projects are classified by **how much validation signal each run returns per unit of
time/tokens** — the priority ramp to follow when reading results or running a subset.

| Tier | Purpose | Projects |
| :--- | :--- | :--- |
| **Tier 0 — Baseline smoke** | Cheapest full-loop proof (init → PM → plan → generate → test → merge). Run first, always: if this stalls, nothing else is worth reading. | `echo` |
| **Tier 1 — Differentiating seams** | New capability coverage the matrix previously lacked: network/black-box HTTP, typed-Python command API + durability, relational-DB + strict-TypeScript service, legacy Django modernization, zero-trust OAuth2/OIDC PKI, Protobuf real-time CDC streaming, and Java/Spring Boot Full Event Sourcing + CQRS double-entry accounting. The core set. | `t4`, `pyedis`, `notebook`, `djanban`, `auth-vault`, `buffonstream`, `jpacioli` |
| **Tier 2 — Rigor probes** | Deepen quality confidence under merciless toolchains and linter discipline (incl. compiler correctness and safety matrix). | `calculator`, `wc`, `fortune`, `stricc` |
| **Tier 3 — Breadth** | State persistence and distributed/broker seams; heaviest runtime and highest API rate-limit exposure — run last or when targeting those seams specifically. | `todo-cli`, `frontpunch`, `searchthedocs` |

**Quick triage run:** `echo t4 pyedis notebook` covers the four major seams; add
Tiers 2–3 when validating depth rather than as the default feedback loop.

---

## Layout

```
validation/
├── Dockerfile.validation          # Shared base image (noctifab binary + projects + validate.sh)
├── validate.sh                    # Per-project harness, runs inside the container
├── run_one.sh                     # Build + run one project, capture log and execution report
├── run_all.sh                     # Run every project in parallel, aggregate exit codes
├── README.md                      # This file
└── projects/
    ├── frontpunch/{Dockerfile, SPEC.md, roadmap/, .noctifab/}
    ├── todo-cli/{Dockerfile, SPEC.md, roadmap/, .noctifab/}
    ├── wc/{Dockerfile, SPEC.md, roadmap/, .noctifab/}
    ├── echo/{Dockerfile, SPEC.md, .noctifab/}
    ├── calculator/{Dockerfile, SPEC.md, .noctifab/}
    ├── fortune/{Dockerfile, SPEC.md, .noctifab/}
    ├── t4/{Dockerfile, SPEC.md, .noctifab/}
    ├── pyedis/{Dockerfile, SPEC.md, .noctifab/}
    ├── notebook/{Dockerfile, SPEC.md, .noctifab/}
    ├── stricc/{Dockerfile, SPEC.md, .noctifab/}
    ├── djanban/{Dockerfile, SPEC.md, .noctifab/}
    ├── searchthedocs/{Dockerfile, SPEC.md, .noctifab/}
    ├── auth-vault/{Dockerfile, SPEC.md, .noctifab/}
    ├── buffonstream/{Dockerfile, SPEC.md, .noctifab/}
    └── jpacioli/{Dockerfile, SPEC.md, .noctifab/}
```

Each project owns its own `Dockerfile` that layers the language toolchain it
needs on top of the shared `noctifab-validation:base` image:

| Project     | `FROM`                       | Toolchain added                  | Target artifacts checked by `validate.sh` |
|-------------|------------------------------|----------------------------------|-------------------------------------------|
| `frontpunch`| `noctifab-validation:base`   | python3, pip, black, ruff, mypy  | `frontpunch/worker.py`                    |
| `todo-cli`  | `noctifab-validation:base`   | go                               | `cmd/todo/main.go`                        |
| `wc`        | `rust:1.84-alpine` (+ base)  | rustc, cargo, rustfmt, clippy    | `Cargo.toml`, `src/main.rs`               |
| `echo`      | `noctifab-validation:base`   | go                               | `cmd/echo/main.go`                        |
| `calculator`| `ruby:3.2-alpine` (+ base)   | ruby, rspec, rubocop             | `calculator.rb`, `lib/calculator/cli.rb`  |
| `fortune`   | `noctifab-validation:base`   | gcc, make, sqlite-dev            | `main.c`, `Makefile`                      |
| `t4`        | `alpine:3.21` (+ base)       | gcc, make, clang-format, clang-tidy | `Makefile`, `docker-compose.yml`, `src/t4.c` |
| `pyedis`| `python:3.14-alpine` (+ base)| python3.14, redis, pytest, ruff, mypy | `app/main.py`, `pyproject.toml` |
| `notebook`   | `node:22-alpine` (+ base)    | node22/npm, typescript, eslint, prettier, vitest, postgresql | `src/index.ts`, `package.json`, `docker-compose.yml` |
| `stricc`     | `rust:alpine` (+ base)       | rustc, cargo, llvm18, gcc, clang | `Cargo.toml`, `stricc/src/main.rs`, `runtime/src/lib.rs` |
| `djanban`    | `python:3.12-alpine` (+ base)| python3.12, django5              | `manage.py`, `djanban/settings.py`        |
| `searchthedocs` | `python:3.12-alpine` (+ base) | python3.12, fastapi, redis      | `app/main.py`                             |
| `auth-vault` | `golang:1.22-alpine` (+ base)| go                               | `cmd/server/main.go`                      |
| `buffonstream` | `golang:1.22-alpine` (+ base)| go, protoc                       | `cmd/server/main.go`                      |
| `jpacioli`   | `eclipse-temurin:21-jdk-alpine` (+ base)| java21, gradle, postgresql  | `build.gradle`, `src/**/*.java`           |

The base image (`Dockerfile.validation`) is a multi-stage build: a
`golang:1.25-alpine` builder compiles the `noctifab` binary, which is then
copied into an `alpine:3.21` runtime alongside the validation projects and
`validate.sh`. No language toolchain is installed in the base image — each
project's `Dockerfile` is responsible for its own.

### Secret handling

Validation images are kept **credential-free**: `.dockerignore` excludes
`validation/projects/*/.noctifab/secrets.yaml` from the build context, so no
LLM key or VCS token is ever baked into a `noctifab-validation:*` image layer.

At runtime, `run_one.sh` bind-mounts the project's `secrets.yaml` read-only
into the container at `/run/secrets/noctifab-secrets.yaml`:

```
docker run -v <host>/validation/projects/<project>/.noctifab/secrets.yaml:/run/secrets/noctifab-secrets.yaml:ro ...
```

`validate.sh` copies that mounted file into the temporary workspace's
`.noctifab/secrets.yaml` right after `cp -R`, so noctifab's config loader
resolves `secret:OPENCODE_API_KEY` (and any other `secret:` references) from
it as usual (`pkg/infrastructure/config/secrets.go:38`). If the runtime
mount is missing, `validate.sh` falls back to a `secrets.yaml` baked into the
project tree (with a warning) and otherwise aborts with a clear error.

Override the host path to mount by setting `NOCTIFAB_SECRETS_FILE`.

---

## How It Works

1. **Base image** (`Dockerfile.validation`): multi-stage build compiles
   `noctifab` and stages it into an `alpine:3.21` runtime along with the
   validation projects and `validate.sh`.
2. **Per-project image** (`validation/projects/<project>/Dockerfile`): builds
   `FROM noctifab-validation:base` (or copies its artifacts) and installs the
   project's own toolchain (Rust, Go, Python, ...). Each project therefore owns
   its toolchain version pins.
3. **Execution**: the container starts with no volume mounts (100% isolated
   from your host filesystem).
4. **Workspace initialization** (`validate.sh`):
   - Copies the selected project template into a temporary runtime directory
     (`tmp_verify_autonomy/`).
   - Runs `git init`, checks out branch `main`, commits the initial config,
     and sets up a local bare `origin` so `git push` works without a remote.
   - Runs `noctifab start` using the target directory or specification (`SPEC.md`).
     
   > [!IMPORTANT]
   > **Spec-Driven Validation Rule:** Checking in pre-written roadmap user stories (e.g. under `roadmap/`) for new validation projects is **strictly forbidden**. Validation projects must be defined and run solely based on `SPEC.md` to verify that `noctifab` is capable of autonomously decomposing specifications into user stories on the fly using its Product Manager Agent.
5. **Validation check**: `validate.sh` asserts the target source file(s)
   were created/modified and that the test suite executed; exits non-zero
   otherwise.
6. **Feedback report**: `gen_feedback.py` parses the captured container log
   into a structured `<PROJECT>_FEEDBACK.md` (verdict, phase activity, build
   failures, test failures, policy violations, parser/runtime errors, spec
   ambiguity, raw tail) at the repository root.

---

## How to Run

### 1. Credentials Setup (git-ignored, never baked into images)
Create `validation/projects/<project>/.noctifab/secrets.yaml` on the host:
```yaml
# validation/projects/frontpunch/.noctifab/secrets.yaml
OPENCODE_API_KEY: "your-actual-api-key"
GITHUB_TOKEN: "your-optional-github-token"
```
`run_one.sh` bind-mounts this file read-only into the container at
`/run/secrets/noctifab-secrets.yaml`; `validate.sh` copies it into the
project workspace so noctifab resolves `secret:OPENCODE_API_KEY` from it.
No `OPENCODE_API_KEY` env var or `-e` flag is required.
Override the path with `NOCTIFAB_SECRETS_FILE=<path>`.

### 2. Run projects (Parallel or Serial)
Run all projects in parallel:
```bash
make validate-all
```

Run projects sequentially (serial mode, one after the other):
```bash
make validate-all SERIAL=1
./validation/run_all.sh --serial
```

Run a specific subset of validation projects (parallel or serial):
```bash
make validate-all PROJECT=echo,wc,t4
make validate-all SERIAL=1 PROJECT=echo,t4
./validation/run_all.sh --serial --projects echo,wc,t4
```

Skip the image build step (reuse existing `noctifab-validation:*` images):
```bash
make validate-all SKIP_BUILD=1
```

### 3. Run a single project
```bash
make validate PROJECT=todo-cli
./validation/run_one.sh wc
```

### Execution Timeout Limit (10-Minute Mandate)
A maximum execution time limit of **10 minutes** (unless another time limit is explicitly specified by the user or task request) MUST be set for each execution of each validation project. If a validation execution reaches 10 minutes (or the specified custom limit), agents must terminate the container run cleanly and record the results.

### 4. Output artifacts
- `validation/projects/<project>/output/src/` — live generated source codebase, updated in real time as Noctifab creates and modifies files.
- `validation/projects/<project>/output/report/*.md` — live atomic execution report updated during execution.
- `validation/projects/<project>/output/log/<project>.log` — full combined stdout/stderr of the container.
- `validation/projects/<project>/output/log/<project>.wrap.log` — `run_one.sh` build/launch/exit trace.
- `validation/projects/<project>/output/dist/` — compiled binaries and build outputs produced by the run.
- `validation/projects/<project>/output/feedback/<PROJECT>_FEEDBACK.md` — structured review of the run.
- `.validation-logs/run_all.<timestamp>.log` — `run_all.sh` global aggregate log.
  These feedback, report, source, dist, and log files are git-ignored (see `.gitignore`) as they are local analysis artifacts, not source.

