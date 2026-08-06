# notebook Specification: TypeScript + Fastify Notes API on PostgreSQL

## 1. Overview

`notebook` is a REST API for managing notes, written in **TypeScript (strict)** on
**Fastify (v5)**, persisting to **PostgreSQL**. It is the matrix's first
relational-DB service: the produced artifact owns DDL migrations, a connection
pool, repository-layer SQL, and a pinned JSON wire contract, with unit,
integration (real Postgres), and docker-compose e2e (black-box) test suites.

## 2. Pinned Directory Layout

```
notebook/
├── package.json              # scripts: start/build/test/lint/format; deps pinned (§3.1)
├── tsconfig.json             # strict:true, NodeNext, target ES2022
├── eslint.config.mjs         # typescript-eslint recommended, zero-error gate
├── .prettierrc.json
├── Makefile                  # install/start/build/test/lint/format/e2e (all REQUIRED)
├── README.md                 # usage, API contract, e2e instructions
├── .gitignore                # node_modules/, dist/, *.log
├── docker-compose.yml        # db + api + e2e black-box harness (see §8)
├── migrations/
│   └── 001_init.sql          # notes table + schema_migrations bookkeeping
├── src/
│   ├── index.ts              # bootstrap: buildApp, listen on PORT (default 3000)
│   ├── app.ts                # buildApp(db): Fastify instance factory (DI)
│   ├── db.ts                 # createPool(databaseUrl): pg Pool factory
│   ├── migrate.ts            # apply pending migrations from migrations/ at startup
│   ├── notes-repo.ts         # NoteRepository: SQL against the pool
│   ├── routes/notes.ts       # route registration + JSON-schema validation
│   └── types.ts              # Note, NewNote types
├── tests/
│   ├── unit/
│   │   └── notes-repo.test.ts   # repo logic against a stub pool (vitest)
│   ├── integration/
│   │   ├── helpers.ts           # ephemeral PG start/stop (see §7.2)
│   │   └── crud.test.ts         # real Postgres: migrate + full CRUD round trip
│   └── e2e/
│       ├── Dockerfile           # node:22-alpine + curl, copies run_tests.sh
│       └── run_tests.sh         # black-box HTTP assertions against ${API_URL}
└── dist/                      # tsc build output (git-ignored)
```

## 3. Toolchain, Invocation, Exit Codes

### 3.1 package.json (pinned ranges)
- Runtime: `fastify ^5`, `pg ^8`.
- Dev: `typescript ^5`, `tsx ^4`, `@types/node ^22`, `@types/pg ^8`,
  `vitest ^3`, `@vitest/coverage-v8 ^3`, `eslint ^9`, `typescript-eslint ^8`,
  `prettier ^3`.
- Scripts: `start` → `tsx src/index.ts`; `build` → `tsc -p tsconfig.json`;
  `test` → `vitest run`; `test:cov` → `vitest run --coverage`;
  `lint` → `eslint src tests`; `format` → `prettier --write .`.

### 3.2 Makefile (all targets REQUIRED, defined exactly once)
- `make install` → `npm install --no-audit --no-fund`.
- `make start` → `npm run start`.
- `make test` → `npm test` (zero failures).
- `make lint` → `npm run lint` AND `npx tsc --noEmit` — zero findings.
- `make format` → `npm run format` (idempotent).
- `make e2e` → `docker compose up --build --exit-code-from e2e` (host-run).

**Linter gate (REQUIRED):** the project MUST pass `make lint` with ZERO findings before
the test gate is considered green. Both `eslint src tests` (typescript-eslint
recommended) AND `npx tsc --noEmit` under `"strict": true` are mandatory — no
`// eslint-disable`, no `@ts-ignore`, and no `any` escapes.

### 3.3 Configuration (env vars, pinned)
- `PORT` (default `3000`) — HTTP listen port.
- `DATABASE_URL` (default `postgres://postgres:postgres@localhost:5432/notebook`).

### 3.4 Exit codes
- Clean `SIGINT`/`SIGTERM` shutdown → exit `0`.
- Startup failure (cannot connect to DB, migration error, invalid port) → log
  `notebook: <reason>` to stderr, exit `1`.

## 4. HTTP API (pinned, black-box)

All JSON bodies. Note object shape:
`{"id": <int>, "title": "<str>", "content": "<str>", "created_at": "<iso8601-utc>", "updated_at": "<iso8601-utc>"}`.
Timestamps are PostgreSQL `TIMESTAMPTZ` returned as ISO-8601 UTC strings with
millisecond precision (e.g. `2026-08-03T12:00:00.000Z`).

| Method | Path | Success | Failure |
| :--- | :--- | :--- | :--- |
| `GET` | `/notes` | `200` array of note objects | — |
| `GET` | `/notes?query=<q>` | `200` filtered array | — |
| `GET` | `/notes/:id` | `200` note object | `404` `{"error":"note not found"}` |
| `POST` | `/notes` | `201` created note | `400` `{"error":"validation failed"}` |
| `PUT` | `/notes/:id` | `200` updated note | `404` / `400` |
| `DELETE` | `/notes/:id` | `204` empty body | `404` |
| `GET` | `/healthz` | `200` `{"status":"ok"}` | — |

### 4.1 Request validation (pinned)
- `POST /notes` body: `{"title": string, "content": string}`.
  `title` required, non-empty, ≤ 200 chars; `content` required, ≤ 10000 chars.
  Violations → `400` `{"error":"validation failed"}` (via Fastify JSON schema +
  a custom error handler that normalizes the payload).
- `PUT /notes/:id` body: `{"title"?, "content"?}` — at least one field required,
  same field constraints. Validation failure → `400`.
- `:id` must be a positive integer; non-integer → `404`.

### 4.2 List semantics (pinned)
- `GET /notes` returns notes sorted by `created_at` DESC, then `id` DESC.
- `?query=<q>` filters (case-insensitive) to notes where `title` OR `content`
  ILIKE `%<q>%`; `q` empty or absent → no filter. `q` is URL-decoded.

## 5. Database Schema (pinned)

`migrations/001_init.sql`:
```sql
CREATE TABLE IF NOT EXISTS notes (
    id         BIGSERIAL PRIMARY KEY,
    title      TEXT NOT NULL,
    content    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```
- A `schema_migrations(version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ)`
  bookkeeping table is created and used so migrations apply exactly once.
- `updated_at` is bumped to `now()` on every `UPDATE`.
- Applies pending migrations automatically at startup (`src/migrate.ts`).

## 6. Implementation Constraints

- `tsconfig.json` uses `"strict": true`; `npx tsc --noEmit` passes with zero errors.
- `buildApp(db: NoteRepository)` is a factory — the repository (and its `pg.Pool`)
  are injected; no global singletons.
- Route handlers only depend on the repository interface; `notes-repo.ts` owns all SQL.
- Every source file stays under 500 lines.

### 6.1 Software Engineering Guidelines

- **SOLID:** one responsibility per module (`notes-repo` owns SQL, `routes` owns HTTP,
  `db` owns the pool, `migrate` owns DDL). Routes depend on the repository interface,
  never on `pg` concrete types.
- **DDD:** the domain types (`Note`, `NewNote`) are pure; SQL and HTTP are
  infrastructure at the edges. The repository interface is the domain boundary.
- **Dependency injection:** `createPool(DATABASE_URL)` and `buildApp(repo)` are the only
  construction paths; the pool/repository are injected through constructors/factories.
  No global singletons and no `new Pool()` inside route handlers.
- **Unit tests must not mock everything:** unit tests exercise pure helpers and route
  behavior with a real in-memory `NoteRepository` implementation injected via the
  interface (pure DI) — not a blanket mock of every dependency.
- **Integration tests mock only I/O:** integration tests run the REAL repository against
  a REAL ephemeral PostgreSQL (see §7.2). Nothing internal is stubbed — only the
  external DB/network boundary is exercised for real.
- **E2E black box:** `docker-compose.yml` runs real `db` + `api` services and a separate
  test-runner container that talks only HTTP to `${API_URL}` (see §8). The e2e suite
  must not import `src/*`.

## 7. Testing Requirements (zero-failure gates)

1. **Unit** (`tests/unit/`): pure helpers and route-handler behavior with a real
   in-memory `NoteRepository` injected via the interface (DI, not blanket mocks).
   Zero failures.
2. **Integration** (`tests/integration/`): against a REAL ephemeral PostgreSQL
   started inside the test process (see §7.2). Covers migration idempotency and
   full CRUD + filtering + `updated_at` bump. Zero failures.
3. **E2E** (`tests/e2e/`, host-run): `make e2e` via docker-compose (see §8) exits
   zero only if every black-box assertion passes.
4. Coverage of `src/` ≥ 90% lines (`vitest run --coverage`).

### 7.2 Ephemeral PostgreSQL inside the validation container
The validation container runs as `root`; PostgreSQL refuses to run as root, so
integration tests MUST start the DB as a dedicated OS user. The container image
installs the `postgres` and `postgresql-client` apk packages. Test helpers
(`tests/integration/helpers.ts`) must, before the suite:

1. Create/own a temp data dir writable by the `postgres` OS user
   (e.g. `/tmp/notebook-pg`, `chown -R postgres:postgres`).
2. Start a cluster as that user on a fixed port with a Unix socket dir, e.g.:
   ```
   su -s /bin/sh postgres -c 'initdb -D /tmp/notebook-pg/data --no-locale -E UTF8'
   su -s /bin/sh postgres -c 'pg_ctl -D /tmp/notebook-pg/data -l /tmp/notebook-pg/log \
       -o "-p 55432 -k /tmp/notebook-pg" start'
   ```
3. Create the database: `createdb -h /tmp/notebook-pg -p 55432 notebook`.
4. Export `DATABASE_URL=postgres://postgres@localhost:55432/notebook` and run the
   suite; stop the cluster in a `afterAll` hook
   (`su -s /bin/sh postgres -c 'pg_ctl -D ... stop'`).

Local (trust) auth is sufficient; the socket dir is passed explicitly so tests
work regardless of hostname.

## 8. E2E Black-Box Harness (docker-compose)

`docker-compose.yml` MUST define three services, runnable from the host with
`make e2e`:

```yaml
services:
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: notebook
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d notebook"]
      interval: 2s
      timeout: 2s
      retries: 20
    ports:
      - "5432:5432"
  api:
    build: .
    command: ["sh", "-c", "npm install --no-audit --no-fund && npm run start"]
    environment:
      PORT: "3000"
      DATABASE_URL: "postgres://postgres:postgres@db:5432/notebook"
    depends_on:
      db:
        condition: service_healthy
    ports:
      - "3000:3000"
  e2e:
    build: ./tests/e2e
    depends_on:
      - api
    environment:
      API_URL: "http://api:3000"
```

- `tests/e2e/Dockerfile`: `FROM node:22-alpine`, installs `curl bash`, copies
  `run_tests.sh`, `CMD ["/tests/run_tests.sh"]`.
- `tests/e2e/run_tests.sh`: black-box assertions against `${API_URL}` — health,
  CRUD lifecycle, validation failures, filtering, `404`s, and `204` on delete.
  Exits nonzero on the first failure.

## 9. README

`README.md` must document: install (`make install`), run (`make start` +
`DATABASE_URL`), the API contract table, the schema, `make test`/`lint`/`e2e`,
and the e2e compose usage.
