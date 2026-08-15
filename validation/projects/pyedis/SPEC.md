# pyedis Specification: Native Redis RESP Key-Value Store in Python 3.14

## 1. Overview

`pyedis` is an in-memory key-value store with a **native Redis wire-protocol (RESP2/RESP3) API exposed over TCP** (default port `6379`), written in modern Python (3.14) with **Python 3.10+ type hints throughout** (using built-in generic collections `dict`, `list`, `tuple`, `set` per PEP 585 instead of `typing.Dict`, `typing.List`, `typing.Tuple`) and a strict `mypy --strict` gate. It mirrors Redis command semantics (`PING`, `ECHO`, `QUIT`, `SET`, `GET`, `DEL`, `EXISTS`, `INCR`, `DECR`, `EXPIRE`, `TTL`, `KEYS`, `FLUSHALL`) with deterministic RESP reply/error envelopes, and persists state to an append-only file (AOF) using absolute expiration timestamps so data and TTLs survive process restarts and `SIGKILL`. Standard Redis client utilities (`redis-cli`, `redis-py` v5+) MUST be fully compatible with `pyedis`.

The project deliberately exercises the validation host's Python-ecosystem seam: async TCP socket streams (`asyncio.start_server`), zero-copy stream parsing, dependency injection (clock + store + AOF logger), absolute timestamp durability, and lock-protected async concurrency.

## 2. Pinned Directory Layout

```
pyedis/
├── pyproject.toml            # PEP 621; deps + [tool.mypy], [tool.ruff]
├── requirements.txt          # pinned runtime + dev deps (see §3.1)
├── Makefile                  # install/run/test/lint/format/e2e targets (all REQUIRED)
├── README.md                 # usage, command API, persistence format, e2e instructions
├── .readthedocs.yaml         # Read the Docs configuration file (REQUIRED)
├── docs/                     # Read the Docs documentation directory (REQUIRED)
│   ├── index.md              # Documentation entry point
│   └── api.md                # Command API, RESP protocol, and architecture documentation
├── .gitignore                # ignore __pycache__/, .venv/, data/, .coverage, htmlcov/
├── docker-compose.yml        # e2e black-box harness (see §10)
├── src/
│   ├── __init__.py
│   ├── main.py               # asyncio TCP server entrypoint (run_server) + SIGINT/SIGTERM handlers
│   ├── resp.py               # RESP wire protocol encoder & streaming decoder (chunking + pipelining)
│   ├── store.py              # Store (in-memory dict + expiration index), DI clock, async lock
│   ├── commands.py           # Command dispatcher, arity & syntax validation, error envelopes
│   └── persistence.py        # AOF append logger (with absolute expire_at) + startup replay
├── tests/
│   ├── unit/
│   │   ├── test_store.py     # Set/get/del/incr/ttl with injected deterministic mock clock
│   │   ├── test_resp.py      # RESP frame encoding, streaming chunk reassembly, pipelined buffers
│   │   ├── test_commands.py  # Arity, unknown commands, case-insensitivity, syntax/type errors, NX/XX
│   │   └── test_persistence.py  # AOF round-trip: write, absolute TTL replay, corrupt line recovery
│   ├── integration/
│   │   └── test_server.py    # Live TCP socket tests via redis-py (commands, pipeline, concurrency, restart)
│   └── e2e/
│       ├── Dockerfile        # python:3.14-alpine + redis (redis-cli), copies run_tests.sh
│       └── run_tests.sh      # Black-box redis-cli assertions against ${REDIS_URL}
└── data/                     # runtime AOF (git-ignored, created on start)
```

## 3. Toolchain, Invocation, Exit Codes, and Configuration

### 3.1 Runtime / Dev Dependencies (pinned)
- **Runtime:** Standard Python 3.14 library (`asyncio`, `dataclass`, `os`, `sys`, `time`, `typing`). No external web frameworks (no FastAPI/Uvicorn).
- **Dev:** `redis>=5.0`, `ruff>=0.8`, `mypy>=1.13`, `coverage>=7.6`.
- **Testing Framework Rule (MANDATORY):** Tests MUST NOT use `pytest` or any third-party test framework under any circumstances. All unit and integration tests MUST be written using Python's standard library `unittest` framework (using `unittest.TestCase`, `unittest.IsolatedAsyncioTestCase` for async tests, and `unittest.mock`). All test modules must be executable via `python3 -m unittest discover -s tests`.

### 3.2 Makefile Targets (all REQUIRED, defined exactly once)
- `make install` → `python3 -m pip install -e ".[dev]"`.
- `make run` → `python3 -m src.main`.
- `make test` → `python3 -m unittest discover -s tests -v` (runs unit + integration suites via standard library `unittest`; zero failures allowed).
- `make lint` → `ruff check src tests` AND `mypy --strict src` — both must pass with zero findings.
- `make format` → `ruff format src tests` (idempotent code formatting).
- `make e2e` → `docker compose up --build --exit-code-from e2e` (host-run black-box harness).

**Linter Gate (REQUIRED):** The project MUST pass `make lint` with ZERO findings before the test gate is considered green. Both `ruff check src tests` (all default rules) and `mypy --strict src` are mandatory — no `# noqa` suppressions of style rules and no `# type: ignore` that hides typing issues.

### 3.3 Configuration (environment variables, pinned)
- `PORT` (default `6379`) — TCP listen port.
- `PYEDIS_DATA_DIR` (default `./data`) — directory holding `dump.aof`.
- `PYEDIS_AOF_FSYNC` (`true` default) — call `os.fsync` on the AOF file after every mutation before transmitting the RESP reply.

### 3.4 Exit Codes and Logging
- Clean `SIGINT`/`SIGTERM` shutdown → flush open files, close server socket, exit `0`.
- Fatal startup error (e.g. data dir unwritable, port binding failure) → log `pyedis: <reason>` to stderr, exit `1`.
- All operational diagnostic logs on stdout/stderr MUST use the prefix `pyedis: `.

---

## 4. RESP Wire Protocol & Framing Engine

`pyedis` communicates exclusively via standard REdis Serialization Protocol (RESP2/RESP3) over TCP.

### 4.1 RESP Frame Types & Envelopes

| Frame Type | Byte Prefix | Wire Format | Example |
| :--- | :---: | :--- | :--- |
| **Simple String** | `+` | `+<string>\r\n` | `+OK\r\n`, `+PONG\r\n` |
| **Error** | `-` | `-<TYPE> <message>\r\n` | `-ERR syntax error\r\n`, `-ERR unknown command 'FOO'\r\n` |
| **Integer** | `:` | `:<signed_number>\r\n` | `:1\r\n`, `:0\r\n`, `:-1\r\n`, `:-2\r\n` |
| **Bulk String** | `$` | `$<length>\r\n<data>\r\n` | `$5\r\nhello\r\n`, `$0\r\n\r\n` |
| **Null Bulk String** | `$` | `$-1\r\n` | `$-1\r\n` (key absent / nil reply in RESP2) |
| **Array** | `*` | `*<count>\r\n<element_1>...` | `*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n` |
| **Empty Array** | `*` | `*0\r\n` | `*0\r\n` (no matching keys) |
| **Null Array** | `*` | `*-1\r\n` | `*-1\r\n` (nil array) |

### 4.2 Stream Parsing, Fragmentation, Pipelining, and Inline Commands
1. **TCP Stream Buffering & Chunking:** TCP streams provide no boundary guarantees. The RESP stream decoder must accumulate incoming chunks into a byte buffer and yield complete frames only when the entire payload (including matching byte counts and trailing `\r\n`) has arrived. Partial frames must not block or crash the reader.
2. **Command Pipelining:** Clients may transmit multiple RESP commands concatenated in a single TCP read buffer. The server must parse and execute every command sequentially in FIFO order, transmitting the corresponding RESP replies back-to-back in matching sequence without dropping or reordering frames.
3. **Multi-Bulk Arrays vs. Inline Commands:**
   - Standard client drivers (`redis-py`, `redis-cli`) send commands formatted as RESP Multi-Bulk Arrays (e.g. `*2\r\n$3\r\nGET\r\n$3\r\nkey\r\n`).
   - Telnet, netcat, and health checkers send plain text **Inline Commands** separated by whitespace and terminated by `\r\n` (e.g. `PING\r\n`, `PING hello\r\n`, `SET k v\r\n`). The decoder must support both formats seamlessly.
4. **Binary Safety:** Bulk strings are binary-safe byte slices (supporting arbitrary UTF-8 characters, JSON strings, whitespace, embedded `\r\n`, and null bytes `\x00`).
5. **Command Case-Insensitivity:** Command names are strictly case-insensitive (`ping`, `PING`, `Ping`, `sEt`, `gEt` must all route to the same handler).

---

## 5. Supported Commands & Parity Semantics

Every command supported by `pyedis` must adhere strictly to official Redis specifications:

| Command | Signature & Description | Success RESP Reply | Error RESP Reply |
| :--- | :--- | :--- | :--- |
| `PING` | `PING [message]`<br>Tests connection liveness. | With 0 args: `+PONG\r\n`<br>With 1 arg: `$<len>\r\n<message>\r\n` | `>1` args: `-ERR wrong number of arguments for 'ping' command\r\n` |
| `ECHO` | `ECHO message`<br>Echoes the given string. | `$<len>\r\n<message>\r\n` | `!=1` arg: `-ERR wrong number of arguments for 'echo' command\r\n` |
| `QUIT` | `QUIT`<br>Asks the server to close the connection. | `+OK\r\n` (server closes TCP connection immediately after sending reply) | `!=0` args: `-ERR wrong number of arguments for 'quit' command\r\n` |
| `SET` | `SET key value [EX seconds] [PX ms] [NX\|XX]`<br>Sets string value with optional expiration and existence guards. | `+OK\r\n` (if set)<br>`$-1\r\n` (if condition `NX` or `XX` not met) | `<2` args: `-ERR wrong number of arguments for 'set' command\r\n`<br>Both `NX` & `XX`: `-ERR syntax error\r\n`<br>Invalid TTL: `-ERR value is not an integer or out of range\r\n` |
| `GET` | `GET key`<br>Gets the string value of a key. | `$<len>\r\n<val>\r\n`<br>`$-1\r\n` (if key missing or expired) | `!=1` arg: `-ERR wrong number of arguments for 'get' command\r\n` |
| `DEL` | `DEL key [key ...]`<br>Removes specified key(s). | `:<count>\r\n` (integer count of keys successfully removed) | `<1` arg: `-ERR wrong number of arguments for 'del' command\r\n` |
| `EXISTS` | `EXISTS key [key ...]`<br>Returns the count of existing keys. | `:<count>\r\n` (sum of existing keys; duplicates counted per instance per Redis 3.0+) | `<1` arg: `-ERR wrong number of arguments for 'exists' command\r\n` |
| `INCR` | `INCR key`<br>Increments the integer value of a key by 1. | `:<new_int_value>\r\n` | `!=1` arg: `-ERR wrong number of arguments for 'incr' command\r\n`<br>Non-integer value: `-ERR value is not an integer or out of range\r\n` |
| `DECR` | `DECR key`<br>Decrements the integer value of a key by 1. | `:<new_int_value>\r\n` | `!=1` arg: `-ERR wrong number of arguments for 'decr' command\r\n`<br>Non-integer value: `-ERR value is not an integer or out of range\r\n` |
| `EXPIRE` | `EXPIRE key seconds`<br>Sets a timeout on key in seconds. | `:1\r\n` (timeout set)<br>`:0\r\n` (key missing or expired) | `!=2` args: `-ERR wrong number of arguments for 'expire' command\r\n`<br>Non-integer: `-ERR value is not an integer or out of range\r\n` |
| `TTL` | `TTL key`<br>Returns remaining TTL in seconds. | `:-2\r\n` (key does not exist or expired)<br>`:-1\r\n` (key exists with no expiry)<br>`:<seconds>\r\n` (remaining positive seconds) | `!=1` arg: `-ERR wrong number of arguments for 'ttl' command\r\n` |
| `KEYS` | `KEYS pattern`<br>Returns all keys matching glob pattern (`*`, `?`, `[abc]`, `\*`). | `*<count>\r\n$<len>\r\n<key1>\r\n...` (returns `*0\r\n` if no matches) | `!=1` arg: `-ERR wrong number of arguments for 'keys' command\r\n` |
| `FLUSHALL` | `FLUSHALL`<br>Deletes all keys from the store and truncates AOF. | `+OK\r\n` | None |
| *Unknown* | Any unrecognized command `<NAME>` | None | `-ERR unknown command '<NAME>'\r\n` |

### 5.1 Special Command Semantics & Edge Cases
1. **`SET` Overwrite Behavior:** Overwriting a key with `SET key new_val` (without `EX`/`PX`) MUST remove any existing expiration associated with that key, making the key persistent (`TTL` becomes `-1`).
2. **`SET` Expiration Flags:** `EX` accepts positive integer seconds; `PX` accepts positive integer milliseconds. If the specified duration is `<= 0`, return `-ERR value is not an integer or out of range\r\n`.
3. **`INCR`/`DECR` Semantics:** If the key does not exist, it is initialized to `"0"` prior to performing the increment/decrement (resulting in `1` or `-1`). If the key already has an expiration, the expiration MUST be preserved after `INCR`/`DECR`.
4. **`EXPIRE` Non-Positive Durations:** If `EXPIRE key seconds` is called with `seconds <= 0`, the key MUST be immediately deleted. The command returns `:1\r\n` if the key existed, or `:0\r\n` if it did not.
5. **Client Discovery Commands (`COMMAND`, `INFO`, `CLIENT`):** When modern Redis drivers (`redis-py`) connect, they may issue handshake commands. If unhandled, returning `-ERR unknown command '<NAME>'\r\n` allows standard fallback, or returning an empty array `*0\r\n` / simple string `+OK\r\n` satisfies connection negotiation.

---

## 6. Store Semantics & Expiration Engine

1. **Storage Structure:** In-memory dictionary mapping UTF-8 string keys to values, paired with an expiration map tracking absolute expiration timestamps (epoch seconds as float/int).
2. **Deterministic Time Injection:** The `Store` MUST accept a `Clock` callable (`Callable[[], float]`, default `time.time`) via dependency injection. All TTL calculations, expiration comparisons, and AOF timestamp generations MUST query this injected clock, enabling unit tests to freeze or fast-forward time deterministically.
3. **Dual-Mode Expiration Strategy:**
   - **Lazy Eviction:** On any key access (`GET`, `SET`, `DEL`, `EXISTS`, `INCR`, `DECR`, `TTL`, `EXPIRE`), if current time $\ge$ key expiration timestamp, the key is purged immediately before completing the command.
   - **Active Sweep:** Before executing `KEYS` or `FLUSHALL`, the store performs an active scan to evict all expired keys so stale keys never appear in query results.
4. **Thread & Async Concurrency Safety:** All mutations and queries against the store execute under a shared `asyncio.Lock` owned by the store, ensuring serialized atomic execution across all concurrent client socket connections.

---

## 7. Persistence Architecture (AOF Engine)

`pyedis` implements an Append-Only File (AOF) durability engine to ensure zero data loss across restarts.

### 7.1 AOF Record Schema
Every successful state-modifying mutation (`SET`, `DEL`, `INCR`, `DECR`, `EXPIRE`, `FLUSHALL`) appends exactly one valid JSON line to `<PYEDIS_DATA_DIR>/dump.aof`:
- `{"op":"SET","key":"<k>","value":"<v>","expire_at":<timestamp_float_or_null>}`
- `{"op":"DEL","key":"<k>"}`
- `{"op":"INCR","key":"<k>"}` / `{"op":"DECR","key":"<k>"}`
- `{"op":"EXPIRE","key":"<k>","expire_at":<timestamp_float>}`
- `{"op":"FLUSHALL"}`

> [!IMPORTANT]
> **Absolute Expiration Invariant:** Expirations stored in `dump.aof` MUST use absolute Unix epoch timestamps (`expire_at`), NOT relative durations. This guarantees that when replaying the AOF on startup, keys that expired during downtime are immediately evicted and not revived with renewed lifetimes.

### 7.2 Fsync & Startup Replay
1. **Fsync Guarantee:** When `PYEDIS_AOF_FSYNC=true` (default), the server calls `os.fsync()` on the open AOF file descriptor after writing each mutation before transmitting the RESP reply to the client socket.
2. **Startup Replay:** On server initialization, if `dump.aof` exists in `PYEDIS_DATA_DIR`, lines are replayed sequentially into the store.
3. **Crash Recovery & Corrupt Trailing Line Tolerance:** If the server was terminated abruptly (`SIGKILL`, power failure) during an append, the final line in `dump.aof` may be truncated/corrupted. The replay engine MUST log a warning `pyedis: ignoring corrupt trailing AOF line` and successfully load all preceding valid lines.
4. **`FLUSHALL` Truncation:** Executing `FLUSHALL` immediately truncates `dump.aof` to 0 bytes.

---

## 8. Software Engineering & Architecture Constraints

- **Python 3.10+ Type Hints:** Every function, method, dataclass, and module must be strictly annotated using built-in generic collections (`dict`, `list`, `tuple`, `set`). Do NOT import `Dict`, `List`, `Tuple`, `Set` from `typing`. `mypy --strict src` must pass with zero errors.
- **SOLID & Domain-Driven Design (DDD):**
  - `src/resp.py`: Pure protocol encoding and streaming decoding without networking or storage logic.
  - `src/store.py`: In-memory storage, expiration logic, and clock abstractions without socket or AOF dependencies.
  - `src/commands.py`: Command routing, syntax parsing, and RESP envelope generation.
  - `src/persistence.py`: AOF append logger and replay engine.
  - `src/main.py`: Composition root, `asyncio.start_server` TCP loop, and signal handling.
- **Dependency Injection (DI):** The store, clock, and AOF logger are injected into dispatchers and server factories via constructors. No global mutable state or unmockable singletons.
- **File Size Limit:** No source file may exceed 500 lines of code.

---

## 9. The Exhaustive Redis Parity & Conformance Testset Matrix

To ensure feature-parity between `pyedis` and official Redis, the automated test suite must execute the following 8 comprehensive verification layers:

### 9.1 RESP Wire Protocol & Framing Suite (`tests/unit/test_resp.py`)
- **Encoding Verification:** Unit tests verifying exact wire bytes for simple strings (`+OK\r\n`), errors (`-ERR message\r\n`), integers (`:42\r\n`), bulk strings (`$5\r\nhello\r\n`), null bulk (`$-1\r\n`), arrays (`*2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n`), and empty arrays (`*0\r\n`).
- **Streaming Chunk Reassembly:** Feeds partial bytes across multiple chunks (e.g. `b"*2\r\n$3\r\n"`, followed by `b"GET\r\n$3\r\nkey\r\n"`) to the decoder and asserts correct reassembly into a single command.
- **Pipelined Buffer Decoding:** Feeds a single byte buffer containing 3 concatenated RESP commands and asserts the decoder yields all 3 commands in exact FIFO order.
- **Inline Command Parsing:** Asserts `b"PING\r\n"`, `b"PING hello\r\n"`, and `b"SET mykey myval\r\n"` parse into expected argument lists.
- **Binary Safety:** Asserts bulk strings containing embedded `\r\n`, null bytes `\x00`, spaces, tabs, and UTF-8 multi-byte characters are parsed without truncation or corruption.

### 9.2 In-Memory Store & Clock Conformance Suite (`tests/unit/test_store.py`)
- **Deterministic Time Advancement:** Tests inject a controllable fake clock:
  - Sets key with TTL = 5 seconds at $T_0$.
  - Asserts `GET` succeeds and `TTL` returns `5` at $T_0$.
  - Advances clock to $T_0 + 3s$; asserts `TTL` returns `2`.
  - Advances clock to $T_0 + 5.1s$; asserts `GET` returns `None` and `TTL` returns `-2` (lazy eviction).
- **TTL Overwrite:** Setting an existing expiring key with a plain `SET` clears expiration; `TTL` returns `-1`.
- **Active Sweep before `KEYS`:** Expired keys are purged and not returned by `KEYS *`.

### 9.3 Command Semantics & Dispatch Suite (`tests/unit/test_commands.py`)
- **Arity Errors:** Verifies exact error string `-ERR wrong number of arguments for '<cmd>' command\r\n` when commands are invoked with too few or too many arguments (`PING`, `GET`, `SET`, `DEL`, `EXISTS`, `INCR`, `DECR`, `EXPIRE`, `TTL`, `KEYS`, `QUIT`).
- **Case-Insensitivity:** Verifies `set`, `SET`, `Set`, `sEt` execute identically.
- **`SET` Flag Permutations:**
  - `SET k v NX`: returns `+OK\r\n` on first call; returns `$-1\r\n` on second call.
  - `SET k v XX`: returns `$-1\r\n` when key is absent; returns `+OK\r\n` after key is created.
  - `SET k v NX XX`: returns `-ERR syntax error\r\n`.
  - `SET k v EX 10`: sets expiration to 10 seconds.
  - `SET k v EX 0` / `SET k v EX -5`: returns `-ERR value is not an integer or out of range\r\n`.
- **`INCR`/`DECR` Operations:**
  - Missing key initialized to `0` and incremented to `1` / decremented to `-1`.
  - String representing an integer `"100"` incremented to `101`.
  - Non-integer string `"hello"` returns `-ERR value is not an integer or out of range\r\n`.
  - Preserves existing TTL on the key after increment.
- **`EXISTS` Multi-Key Count:** `EXISTS k1 k2 k3` returns integer count of existing keys. Supplying the same existing key twice (`EXISTS k1 k1`) returns `:2\r\n`.
- **`DEL` Multi-Key Count:** `DEL k1 k2 missing_k` returns count of deleted keys (`:2\r\n`).
- **`KEYS` Glob Patterns:** Tests glob matching against `h*llo`, `h?llo`, `h[ae]llo`, `*`, and `test\*key`.
- **Unknown Command:** `FOOBAR` returns `-ERR unknown command 'FOOBAR'\r\n`.

### 9.4 AOF Persistence & Recovery Suite (`tests/unit/test_persistence.py`)
- **Mutation Logging:** Verifies every mutation writes a valid JSON line with `op`, `key`, and absolute `expire_at`.
- **Replay Accuracy:** Appends records to a temporary AOF file, re-initializes a fresh `Store` from the file, and asserts full state equality.
- **Time-Travel Resilience:** Writes a key expiring at $T_0 + 10s$. Sets clock to $T_0 + 20s$ and replays AOF into fresh store; asserts the key is expired and absent.
- **Truncated Last Line Recovery:** Simulates crash by writing a partial JSON string `{"op":"SET","key":"k` at the end of `dump.aof`. Replay successfully restores all valid preceding entries and logs a warning.
- **`FLUSHALL` Truncation:** Asserts `FLUSHALL` truncates `dump.aof` to zero bytes on disk.

### 9.5 Socket & Protocol Integration Suite (`tests/integration/test_server.py`)
- Runs a live `pyedis` TCP server on `127.0.0.1` with an ephemeral port.
- **Raw Socket Verification:** Tests raw TCP socket connections sending inline strings and multi-bulk arrays.
- **Official Driver Interop (`redis-py`):**
  - Connects using `r = redis.Redis(host="127.0.0.1", port=PORT)`.
  - Executes full command suite (`ping`, `set`, `get`, `delete`, `exists`, `incr`, `decr`, `expire`, `ttl`, `keys`, `flushall`).
  - Executes `redis-py` Pipelines (`pipe = r.pipeline(); pipe.set('a', '1'); pipe.incr('a'); pipe.get('a'); res = pipe.execute()`; asserts `res == [True, 2, b'2']`).
- **Concurrent Client Load:** 50 concurrent `asyncio` tasks or threads hammering `INCR counter` simultaneously. Asserts final value is exactly `50` with zero dropped updates.
- **Server Restart Durability:** Writes data, terminates server, starts new server instance on same data directory, asserts data persists.

### 9.6 Black-Box E2E Conformance Suite (`tests/e2e/run_tests.sh`)
- Executed inside the `e2e` container against `${REDIS_URL}` using official `redis-cli`:
  1. `redis-cli ping` → `PONG`
  2. `redis-cli ping "hello world"` → `"hello world"`
  3. `redis-cli set k1 v1` → `OK`, `redis-cli get k1` → `v1`
  4. `redis-cli set k2 v2 NX` → `OK`, `redis-cli set k2 v2 NX` → `(nil)`
  5. `redis-cli set k3 v3 EX 1` → `OK`, `sleep 2`, `redis-cli get k3` → `(nil)`, `redis-cli ttl k3` → `-2`
  6. `redis-cli exists k1 k2 missing` → `2`
  7. `redis-cli incr counter` → `1`, `redis-cli incr counter` → `2`, `redis-cli decr counter` → `1`
  8. `redis-cli keys "*"` → list of keys
  9. `redis-cli del k1 k2 counter` → `3`
  10. `redis-cli flushall` → `OK`
- Exits code `0` only if all assertions pass.

### 9.7 Differential Conformance Oracle
The integration test suite (`tests/integration/test_server.py`) and E2E script (`tests/e2e/run_tests.sh`) MUST yield the exact same test outputs and return code `0` whether executed against `pyedis` or standard `redis:7-alpine`.

### 9.8 Code Coverage Gate
Total test coverage of `src/` must be $\ge 95\%$ lines (`coverage run -m unittest discover -s tests && coverage report`).

---

## 10. E2E Black-Box Harness (docker-compose)

`docker-compose.yml` MUST define two services, runnable from the host with `make e2e`:

```yaml
services:
  api:
    build: .
    command: ["python3", "-m", "src.main"]
    environment:
      PYEDIS_DATA_DIR: "/data"
      PORT: "6379"
      PYEDIS_AOF_FSYNC: "true"
    volumes:
      - data:/data
    ports:
      - "6379:6379"
  e2e:
    build: ./tests/e2e
    depends_on:
      - api
    environment:
      REDIS_URL: "redis://api:6379"
volumes:
  data:
```

- `tests/e2e/Dockerfile`: `FROM python:3.14-alpine`, installs `redis` (`redis-cli` tool), copies `run_tests.sh`, `CMD ["/tests/run_tests.sh"]`.
- `tests/e2e/run_tests.sh`: Black-box `redis-cli` assertions against `${REDIS_URL}`. Exits nonzero on the first failure.

---

## 11. Documentation Requirements (README & Read the Docs)

1. **`README.md`**: Must exist at root level documenting: installation, running the server, the full command API table, RESP wire format envelopes, AOF format, `make test`/`lint`/`e2e`, and `redis-cli` usage examples.
2. **`docs/` Folder**: Must contain documentation in Read the Docs format (MkDocs/Sphinx markdown files) covering Architecture, Protocol Reference, Store & Expiration Engine, Persistence, and Deployment.
3. **`.readthedocs.yaml`**: Must exist at root level configured to build the `docs/` documentation bundle.

---

## 12. Definition of Done (DoD)

To consider `pyedis` fully implemented, the project must satisfy:
1. **Public RESP API & CLI Compatibility:** Native Redis RESP2/RESP3 wire-protocol TCP server operating on port 6379, passing all command assertions via official `redis-cli` and `redis-py` (v5+).
2. **Persistence & Durability Invariant:** AOF persistence engine logs mutations with absolute `expire_at` timestamps, tolerates corrupt trailing lines, and successfully restores state on restart.
3. **Linting Invariant:** Zero findings under `ruff check src tests` AND `mypy --strict src`.
4. **Verification Criteria:** 100% test pass rate on `make test` (unit + integration) and `make e2e`, with $\ge 95\%$ test coverage across `src/`.
