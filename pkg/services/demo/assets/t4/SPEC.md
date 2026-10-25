# t4 Specification: Simplified S3-Style Object Store in C

## 1. Overview

`t4` is a self-contained C17 command-line HTTP server that implements a **simplified,
bucket-less S3-style object store** with a sharp black-box contract. Objects are
stored under a single flat namespace keyed by URL-encoded path segments. The
server is compiled from scratch with `gcc` (no third-party C libraries): it
implements a minimal HTTP/1.1 server over POSIX sockets, a file-backed object
store with atomic writes, and deterministic `ETag` metadata.

`t4` MUST NOT support buckets, SigV4 signing, multipart uploads, or versioning.

## 2. Pinned Directory Layout

```
t4/
├── Makefile                 # build/test/lint/format/e2e targets (all REQUIRED)
├── README.md                # usage, API contract, e2e instructions
├── .gitignore               # ignore bin/, data/, *.o
├── docker-compose.yml       # e2e black-box harness (see §8)
├── src/
│   ├── t4.c                 # composition root: CLI parsing, socket accept loop
│   ├── http.h               # public HTTP parse/write API
│   ├── http.c               # minimal HTTP/1.1 request parsing + response writing
│   ├── store.h              # public object store API
│   ├── store.c              # file-backed object store (atomic PUT, GET, DELETE, LIST)
│   ├── etag.h
│   ├── etag.c               # ETag computation (size + FNV-1a 64)
│   ├── route.h
│   └── route.c              # request routing/dispatch
├── tests/
│   ├── unit/
│   │   ├── test.h           # tiny assertion harness (no third-party libs)
│   │   ├── test_http.c      # URL decoding, request parsing, Range parsing
│   │   ├── test_store.c     # store semantics against a temp data dir
│   │   ├── test_etag.c      # ETag determinism
│   │   └── test_route.c     # method/key validation and 404/405 dispatch
│   ├── integration/
│   │   └── blackbox.sh      # starts server, asserts full HTTP contract via curl
│   └── e2e/
│       ├── Dockerfile       # alpine + curl + bash, copies run_tests.sh
│       └── run_tests.sh     # black-box HTTP assertions against ${T4_URL}
└── data/                    # runtime object store (git-ignored, created on start)
```

## 3. Build, Invocation, and Exit Codes

### 3.1 Makefile (all targets REQUIRED, each defined exactly once)
- `make build` → compiles `bin/t4` from `src/*.c` with:
  `gcc -Wall -Wextra -Werror -pedantic -std=c17 -O2 -Isrc -o bin/t4 src/*.c`
- `make test` → builds unit + integration tests and runs them; MUST exit zero with no failures.
- `make lint` → runs `clang-tidy` and a strict `gcc` compile, both with ZERO findings (see the linter gate below).
- `make format` → formats sources with `clang-format -i src/*.c src/*.h` (idempotent).
- `make e2e` → `docker compose up --build --exit-code-from e2e` (run from the host).
- `make clean` → removes `bin/`, `*.o`, `data/`.

**Linter gate (REQUIRED):** the project MUST pass `make lint` with ZERO findings before
the test gate is considered green. `make lint` MUST run exactly:
1. `clang-tidy -checks='clang-diagnostic-*,clang-analyzer-*' -Isrc src/*.c` — zero findings.
2. The strict compile `gcc -Wall -Wextra -Werror -pedantic -std=c17 -fsyntax-only -Isrc src/*.c` — zero warnings.

### 3.2 CLI Invocation (pinned)
```
t4 [--data-dir DIR] [--host HOST] [--port PORT]
```
- Defaults: `--data-dir ./data`, `--host 0.0.0.0`, `--port 8080`.
- `--port` must be an integer in `[1, 65535]`; invalid → print `t4: invalid port` to
  stderr and exit `1`.
- The data directory is created (recursively) on startup; if it cannot be created
  or the socket cannot bind, print `t4: <reason>` to stderr and exit `1`.
- On `SIGINT`/`SIGTERM` the server shuts down cleanly and exits `0`.
- Normal server logs go to stdout; all errors start with the prefix `t4: `.

## 4. HTTP Contract (pinned, black-box)

Protocol: HTTP/1.0 or HTTP/1.1. Requests are case-sensitive; methods are uppercase.
Every response MUST include `Server: t4`, `Content-Length`, and `Connection: close`.
Error and listing bodies are JSON, newline-terminated.

### 4.1 Endpoints

| Method | Path | Success | Failure |
| :--- | :--- | :--- | :--- |
| `PUT` | `/{key}` | `200` (overwrite) / `201` (first create) + `ETag` header | `400` invalid key / `413` body too large |
| `GET` | `/{key}` | `200` + `ETag` + body bytes | `404` no such key |
| `GET` | `/{key}` with `Range` | `206` + `Content-Range` + slice | `416` unsatisfiable / `400` malformed |
| `HEAD` | `/{key}` | `200` + headers, no body | `404` |
| `DELETE` | `/{key}` | `204` empty body | `404` |
| `GET` | `/` | `200` + `{"keys":[...]}` JSON list | — |
| `*` | any other | `405` `{"error":"method not allowed"}` | — |

### 4.2 JSON body shapes (pinned)
- Errors: `{"error":"<reason>"}` where `<reason>` is one of exactly:
  `no such key`, `invalid key`, `invalid range`, `range not satisfiable`,
  `body too large`, `bad request`, `method not allowed`.
- Listing: `{"keys":["<key1>","<key2>",...]}` — keys sorted byte-wise
  lexicographically ascending, each percent-decoded, JSON-string-escaped.

### 4.3 Key rules
- A key is the path after the leading `/`, percent-decoded (`%XX` → byte; `+` is a
  literal `+`, never a space).
- After decoding, the key MUST be non-empty and MUST NOT contain `/` or `\0`.
  Any attempt to traverse (`.`, `..`, or any segment containing `/`) → `400 invalid key`.
- Data is stored at `<data-dir>/<key>` (keys never collide with internal names).

### 4.4 PUT semantics
- Body is read byte-for-byte per `Content-Length` (binary-safe). Missing/absent
  `Content-Length` is treated as an empty body (store 0 bytes).
- Body larger than `1048576` bytes (1 MiB) → `413 body too large` (connection closed).
- First create of a key returns `201`; subsequent overwrites return `200`.
- Must be atomic: write to a temp file, `fsync`, then `rename` over the target.
- Response header: `ETag: "<decimal-size>-<fnv1a64-hex>"` (see §5).

### 4.5 GET / HEAD / DELETE semantics
- `GET` returns the exact stored bytes as body; `Content-Type` is `application/octet-stream`.
- `HEAD` returns the same headers as `GET` (including `ETag`, `Content-Length`) with no body.
- `DELETE` unlinks the object and returns `204` with an empty body; missing key → `404`.

### 4.6 Range semantics (pinned)
- `Range: bytes=start-end` and `Range: bytes=start-` are supported (inclusive,
  zero-indexed, `end` optional). One range only; no multi-range.
- Valid, satisfiable → `206 Partial Content` with
  `Content-Range: bytes <start>-<end>/<total>` and the sliced body.
- `start >= total` → `416` with `Content-Range: bytes */<total>` and
  `{"error":"range not satisfiable"}`.
- Malformed syntax, `start > end`, or non-numeric → `400` `{"error":"invalid range"}`.

## 5. ETag Format (pinned)

`ETag` value is exactly `"<S>-<H>"` (strong ETag, quoted) where:
- `<S>` is the decimal byte length of the object body, and
- `<H>` is the 16-digit lowercase hex of the FNV-1a 64-bit hash of the body bytes
  (offset basis `14695981039346656037`, prime `1099511628211`, over the raw bytes).
- The same object content ALWAYS yields the identical `ETag`; empty body →
  `ETag: "0-cbf29ce484222325"`.

## 6. Implementation Constraints

- Pure C17, POSIX sockets only (no threads required): a single-threaded
  sequential accept loop reading one request per connection is acceptable.
- Domain logic (store, etag, route) MUST be free of I/O coupling and unit-testable;
  sockets and CLI parsing live only in `t4.c` / `http.c`.
- No global mutable state; the store is passed to the router via a struct (DI-style).
- Every source file stays under 500 lines.

### 6.1 Software Engineering Guidelines

- **SOLID:** every source file owns exactly one responsibility — `http` parses/renders
  HTTP, `store` persists objects, `etag` computes metadata, `route` dispatches. Modules
  depend on their header-declared interfaces, never on file-local symbols.
- **DDD:** the domain layer (`store`, `etag`, `route`) is pure and I/O-free. Sockets, CLI
  parsing, and raw filesystem access live only in the infrastructure layer (`t4.c`,
  `http.c`, and the file I/O inside `store.c` behind its public interface).
- **Dependency injection:** the store is a struct instance created by `t4.c` and passed
  into the router via struct/function parameters. No globals, no singletons, and no
  inline store construction inside routing logic.
- **Unit tests must not mock everything:** unit tests exercise the REAL parser, store,
  ETag, and route functions against real inputs — a real store rooted at a temp data
  directory, real byte buffers. Only the OS boundary (sockets/files) is pointed at
  test-local locations; nothing under test is stubbed or re-implemented in the test.
- **Integration tests mock only I/O:** `blackbox.sh` starts the REAL server binary and
  exercises it over real sockets; the only substitutions are an ephemeral port and a
  temp data dir. No in-process stubs of server internals are allowed.
- **E2E black box:** `docker-compose.yml` builds the real server plus a separate
  test-runner container that touches ONLY the HTTP contract (see §8). The e2e suite must
  never compile against or import server internals.

## 7. Testing Requirements (zero-failure gates)

1. **Unit tests** (`make test` part 1): cover URL decoding, request-line parsing,
   `Range` parsing (satisfiable, suffix-open, unsatisfiable, malformed), ETag
   determinism (incl. empty body), store PUT/GET/DELETE/LIST/atomicity against a
   temp data dir, and route dispatch (404 vs 405, invalid key → 400). Zero failures.
2. **Integration black-box** (`tests/integration/blackbox.sh`, `make test` part 2):
   starts `bin/t4` on an ephemeral free port with a temp data dir, then asserts the
   FULL contract with `curl` — every endpoint, every status code, exact headers
   (`ETag`, `Content-Range`), exact bodies (including a binary payload with `\0`
   and non-ASCII bytes), and range slicing. Zero failures; nonzero exit otherwise.
3. **E2E (docker-compose, host-run)** — see §8. `make e2e` exits zero only if every
   assertion in `tests/e2e/run_tests.sh` passes.

## 8. E2E Black-Box Harness (docker-compose)

`docker-compose.yml` MUST define two services and be runnable from the host with
`make e2e` (or `docker compose up --build --exit-code-from e2e`):

```yaml
services:
  t4:
    build: .
    command: ["./bin/t4", "--data-dir", "/data", "--host", "0.0.0.0", "--port", "8080"]
    volumes:
      - t4-data:/data
    ports:
      - "8080:8080"
  e2e:
    build: ./tests/e2e
    depends_on:
      - t4
    environment:
      T4_URL: "http://t4:8080"
volumes:
  t4-data:
```

- `tests/e2e/Dockerfile`: `FROM alpine`, installs `curl bash`, copies
  `run_tests.sh`, `CMD ["/tests/run_tests.sh"]`.
- `tests/e2e/run_tests.sh`: treats `${T4_URL}` purely as a black box over HTTP —
  PUT/GET/HEAD/DELETE/list/range assertions, exact status codes, `ETag` presence,
  and byte-exact round-trips. Exits nonzero on the first failure.

## 9. README

`README.md` must document: build (`make build`), run (`./bin/t4` and flags),
the full API contract table, the ETag format, and how to run unit/integration
(`make test`) and e2e (`make e2e`) suites.


## 5. Definition of Done (DoD)

To consider `t4` fully implemented, the C HTTP server project must satisfy:
1. **Public HTTP Server:** `t4` binary runs an HTTP server supporting standard status codes, headers, `Range` byte requests, and binary downloads, exiting code `0` on clean shutdown.
2. **Linting Invariant:** Zero compiler or static analyzer warnings under `clang-tidy` and strict `gcc` (`-Wall -Wextra -Werror`).
3. **Verification Criteria:** 100% test pass rate executing `make test`.
