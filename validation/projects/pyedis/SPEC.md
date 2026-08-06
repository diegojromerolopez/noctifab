# pyedis Specification: HTTP Key-Value Store in Python 3.14 + FastAPI

## 1. Overview

`pyedis` is a key-value store with a **Redis-flavored command API exposed
over HTTP** via FastAPI, written in modern Python (3.14) with **type hints
throughout** and a strict `mypy --strict` gate. It mirrors Redis command
semantics (`SET`, `GET`, `DEL`, `EXISTS`, `INCR`, `DECR`, `EXPIRE`, `TTL`,
`KEYS`, `FLUSHALL`) with deterministic reply/error envelopes, and persists state
to an append-only file so data survives process restarts and `SIGKILL`.

The project deliberately exercises the validation host's Python-ecosystem seam:
FastAPI + uvicorn, pydantic schemas, dependency injection (clock + store),
AOF-style durability, and an async-safe store.

## 2. Pinned Directory Layout

```
pyedis/
├── pyproject.toml            # PEP 621; deps + [tool.pytest.ini_options], [tool.mypy], [tool.ruff]
├── requirements.txt          # pinned runtime deps (see §3.1)
├── Makefile                  # install/run/test/lint/format/e2e targets (all REQUIRED)
├── README.md                 # usage, command API, persistence format, e2e instructions
├── .gitignore                # ignore __pycache__/, .venv/, data/, .coverage, htmlcov/
├── docker-compose.yml        # e2e black-box harness (see §8)
├── app/
│   ├── __init__.py
│   ├── main.py               # FastAPI app factory (create_app) + /commands + /healthz
│   ├── store.py              # Store (dict + TTL registry), DI clock, async lock
│   ├── commands.py           # command parser + dispatch (arity/name validation)
│   ├── persistence.py        # AOF append + startup replay
│   └── schemas.py            # pydantic request/reply models
├── tests/
│   ├── unit/
│   │   ├── test_store.py     # set/get/del/incr/ttl with injected fake clock
│   │   ├── test_commands.py  # arity, unknown command, WRONGTYPE, NX/XX
│   │   └── test_persistence.py  # AOF round-trip: write, replay, FLUSHALL truncate
│   ├── integration/
│   │   └── test_api.py       # httpx against the ASGI app (startup/durability across restarts)
│   └── e2e/
│       ├── Dockerfile        # python:3.14-alpine + curl, copies run_tests.sh
│       └── run_tests.sh      # black-box HTTP assertions against ${REDIS_URL}
└── data/                     # runtime AOF (git-ignored, created on start)
```

## 3. Toolchain, Invocation, Exit Codes

### 3.1 Runtime / dev dependencies (pinned)
- `fastapi>=0.115`, `pydantic>=2.10`, `uvicorn[standard]>=0.30`.
- Dev: `pytest>=8.0`, `httpx>=0.27`, `ruff>=0.8`, `mypy>=1.13`, `coverage>=7.6`.

### 3.2 Makefile (all targets REQUIRED, defined exactly once)
- `make install` → `python3 -m pip install -e ".[dev]"`.
- `make run` → `python3 -m uvicorn app.main:app --host 0.0.0.0 --port ${PORT:-8000}`.
- `make test` → `python3 -m pytest -q` (unit + integration); zero failures.
- `make lint` → `ruff check app tests` AND `mypy --strict app` — both must pass with zero findings.
- `make format` → `ruff format app tests` (idempotent).
- `make e2e` → `docker compose up --build --exit-code-from e2e` (host-run).

**Linter gate (REQUIRED):** the project MUST pass `make lint` with ZERO findings before
the test gate is considered green. Both `ruff check app tests` (all default rules) and
`mypy --strict app` are mandatory — no `# noqa` suppressions of style rules and no
`# type: ignore` that hides a real typing error.

### 3.3 Configuration (env vars, pinned)
- `PORT` (default `8000`) — HTTP listen port.
- `PYEDIS_DATA_DIR` (default `./data`) — directory holding `dump.aof`.
- `PYEDIS_AOF_FSYNC` (`true` default) — fsync the AOF after each mutation
  before the HTTP reply is sent (durability across `SIGKILL`).

### 3.4 Exit codes
- Clean `SIGINT`/`SIGTERM` shutdown → exit `0`.
- Fatal startup error (data dir unwritable, invalid port) → log `pyedis: <reason>`
  to stderr, exit `1`.

## 4. HTTP API (pinned, black-box)

### 4.1 `GET /healthz`
- `200` with body `{"status":"ok"}`.

### 4.2 `POST /commands`
Request body: `{"command": "<NAME>", "args": ["<arg>", ...]}`. `command` is
case-insensitive and normalized to uppercase; `args` is a JSON array of strings.
Missing/invalid `command` → `400` `{"error":"ERR unknown command ''"}`.

Reply envelope (all replies are single JSON objects):
- String reply → `{"reply":"OK"}` / `{"reply":"<value>"}`.
- Integer reply → `{"reply": 1}` (JSON number).
- Nil reply (missing key, `NX`/`XX` no-op) → `{"reply": null}`.
- Errors → HTTP `400` with `{"error":"ERR <message>"}` or
  `{"error":"WRONGTYPE Operation against a key holding the wrong kind of value"}`.

Supported commands and exact semantics:

| Command | Behavior | Replies / Errors |
| :--- | :--- | :--- |
| `SET key value [EX seconds] [PX ms] [NX] [XX]` | Store string value. `NX` only if key absent; `XX` only if present. `EX`/`PX` set TTL. `EX`/`PX` values must be positive integers. | `OK` / `null` (NX/XX no-op) / `ERR syntax error` (bad options/arity) |
| `GET key` | Fetch value. | value / `null` / `ERR wrong number of arguments for 'get' command` |
| `DEL key [key ...]` | Delete; returns number deleted. | integer count |
| `EXISTS key [key ...]` | Count of existing keys (expired keys count as missing). | integer count |
| `INCR key` | +1; sets to 1 if absent; value must be a valid base-10 integer in `[-9223372036854775808, 9223372036854775807]`. | new integer / `WRONGTYPE ...` error |
| `DECR key` | −1; same rules as `INCR`. | new integer / `WRONGTYPE ...` error |
| `EXPIRE key seconds` | Set TTL (positive integer). | `1` (set) / `0` (key missing) |
| `TTL key` | Remaining seconds. | `-2` missing, `-1` no expiry, else integer seconds |
| `KEYS pattern` | Glob match over key names. `*` (any run), `?` (any single char), otherwise literal. | list of matching keys (empty list `{"reply":[]}`) |
| `FLUSHALL` | Remove all keys and truncate the AOF. | `OK` |

Unknown command → `400` `{"error":"ERR unknown command '<NAME>'"}`.

### 4.3 Error prefixes (pinned)
- `ERR ` — command/arity/syntax errors.
- `WRONGTYPE ` — type errors (e.g. `INCR` on a non-integer value).
- All error strings are lowercase except the fixed `WRONGTYPE` prefix and quoted
  command names.

## 5. Store Semantics

- Values are Python `str` (UTF-8); storage size is unlimited for validation.
- TTLs are integer seconds; keys expire lazily on access and are swept before
  `KEYS`/`FLUSHALL`/`EXISTS`.
- The store is injected with a `Clock` callable (default `time.time`, seconds) so
  tests can freeze time; TTL math uses only the injected clock.
- All mutations and reads happen under a single `asyncio.Lock` owned by the store
  (single worker `uvicorn` process; no multi-process mode required).

## 6. Persistence (AOF, pinned)

- On every successful mutation (`SET`, `DEL`, `INCR`, `DECR`, `EXPIRE`,
  `FLUSHALL`), append exactly one JSON line to `<data-dir>/dump.aof`:
  - `{"op":"SET","key":"<k>","value":"<v>","ttl_seconds":<n or null>}`
  - `{"op":"DEL","key":"<k>"}`
  - `{"op":"INCR","key":"<k>"}` / `{"op":"DECR","key":"<k>"}`
  - `{"op":"EXPIRE","key":"<k>","seconds":<n>}`
  - `{"op":"FLUSHALL"}`
- If `PYEDIS_AOF_FSYNC=true`, `os.fsync` the file before replying.
- On startup, if `dump.aof` exists, replay lines in order to rebuild state; a
  corrupt final line is ignored (logged), earlier lines still apply.
- `FLUSHALL` truncates the AOF to zero bytes.

## 7. Implementation Constraints

- Every public function, method, and dataclass is fully type-annotated; `mypy
  --strict app` passes with zero errors.
- FastAPI app is built via a `create_app(*, store=None)` factory (DI): the store
  and its clock are injected, never global singletons.
- Domain logic (`store`, `commands`, `persistence`) has no FastAPI imports.
- Every source file stays under 500 lines.

### 7.1 Software Engineering Guidelines

- **SOLID:** one responsibility per module (`store`, `commands`, `persistence`,
  `schemas`, `main`). Domain modules depend on protocols/abstract types, never on
  concrete FastAPI types.
- **DDD:** `store`, `commands`, and `persistence` form a pure domain/application layer
  with no FastAPI/uvicorn imports; HTTP concerns live only in `main.py` and `schemas.py`.
- **Dependency injection:** the store, its `Clock`, and the AOF writer are supplied
  through `create_app(...)` or constructor parameters. No module-level singletons, and
  no instantiation inside command handlers.
- **Unit tests must not mock everything:** unit tests construct the REAL `Store` with an
  injected fake `Clock` (pure DI), exercise the REAL command router, and use a REAL AOF
  on a temp directory. No monkeypatching of the module under test and no blanket mocks.
- **Integration tests mock only I/O:** ASGI tests run the real app with a real AOF on a
  temp path. Only the external I/O boundary (filesystem, network) is pointed at
  test-local locations — never the app's internals.
- **E2E black box:** `docker-compose.yml` runs the real uvicorn service and a separate
  test-runner container that speaks only HTTP to `${REDIS_URL}` (see §8). The e2e suite
  must not import `app.*`.

## 8. E2E Black-Box Harness (docker-compose)

`docker-compose.yml` MUST define two services, runnable from the host with
`make e2e`:

```yaml
services:
  api:
    build: .
    command: ["python3", "-m", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
    environment:
      PYEDIS_DATA_DIR: "/data"
    volumes:
      - data:/data
    ports:
      - "8000:8000"
  e2e:
    build: ./tests/e2e
    depends_on:
      - api
    environment:
      REDIS_URL: "http://api:8000"
volumes:
  data:
```

- `tests/e2e/Dockerfile`: `FROM python:3.14-alpine`, installs `curl bash`, copies
  `run_tests.sh`, `CMD ["/tests/run_tests.sh"]`.
- `tests/e2e/run_tests.sh`: black-box assertions against `${REDIS_URL}` for every
  command, reply envelope, TTL behavior, `WRONGTYPE`/`ERR` errors, and
  `FLUSHALL`. Exits nonzero on the first failure.

## 9. Testing Requirements (zero-failure gates)

1. **Unit** — store semantics + TTL with a fake clock, command arity/unknown/type
   errors, AOF write/replay/truncate. Zero failures.
2. **Integration** — ASGI-level tests with `httpx` covering the full command
   surface AND durability across an app restart (write → new store → data
   present) and `SIGKILL`-style durability (replay from an AOF written with
   `fsync=true`). Zero failures.
3. **E2E** — `make e2e` (docker-compose black box) exits zero only if every
   assertion passes.
4. Coverage of `app/` must be ≥ 95% lines (`pytest --cov=app --cov-report=term-missing`).

## 10. README

`README.md` must document: install/run, the command API table, reply/error
envelopes, the AOF format, `make test`/`lint`/`e2e`, and the e2e compose usage.
