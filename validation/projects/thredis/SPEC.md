# thredis Specification: Thread-Based Redis RESP Key-Value Store in .NET 9

## 1. Overview

`thredis` is an in-memory key-value store with a **native Redis wire-protocol (RESP2/RESP3) API exposed over TCP** (default port `6379`), written in modern **C# 13 / .NET 9** utilizing multi-threaded systems engineering primitives (`System.IO.Pipelines`, `System.Threading.Channels`, `ConcurrentDictionary<TKey, TValue>`, and fine-grained synchronization). It mirrors core Redis command semantics (`PING`, `ECHO`, `QUIT`, `SET`, `GET`, `DEL`, `EXISTS`, `INCR`, `DECR`, `EXPIRE`, `TTL`, `KEYS`, `FLUSHALL`) with deterministic RESP reply/error envelopes, and persists state to an append-only file (AOF) using absolute expiration timestamps so data and TTLs survive process restarts and `SIGKILL`. Standard Redis client utilities (`redis-cli`, `StackExchange.Redis`, `redis-py`) MUST be fully compatible with `thredis`.

The project exercises the multithreaded systems programming seam on Linux:
- **Zero-Allocation Socket Streaming:** High-performance TCP reading and frame slicing via `System.IO.Pipelines` (`PipeReader` / `PipeWriter`) operating over `ReadOnlySequence<byte>`.
- **Thread-Safe Concurrent Storage:** In-memory store built on `ConcurrentDictionary<string, CacheEntry>` with fine-grained per-key synchronization for atomic mutations (`INCR`, `DECR`, `SET NX/XX`) and reader-writer locking for multi-key queries.
- **Asynchronous AOF Persistence:** Producer-consumer pattern using bounded `System.Threading.Channels.Channel<AofMutation>` coupled with a dedicated background writer flushing via `FileStream.Flush(flushToDisk: true)`.
- **Dependency Injection (DI) & Testability:** Decoupled interfaces (`IStore`, `IClock`, `IAofLogger`, `IRespDecoder`, `ICommandDispatcher`) wired through `Microsoft.Extensions.DependencyInjection`.

---

## 2. Pinned Directory Layout

```
thredis/
├── Thredis.sln               # .NET Solution file
├── Makefile                  # build/run/test/lint targets (all REQUIRED)
├── README.md                 # Usage, command API, architecture, benchmark instructions
├── .gitignore                # Ignore bin/, obj/, data/, *.user
├── src/
│   └── Thredis/
│       ├── Thredis.csproj    # C# 13, net9.0, Nullable=enable, TreatWarningsAsErrors=true
│       ├── Program.cs        # Entrypoint (Generic Host / WebApplication), SIGINT/SIGTERM handlers
│       ├── Common/
│       │   ├── IClock.cs     # Injected clock interface for deterministic time
│       │   └── SystemClock.cs# Default DateTimeOffset.UtcNow implementation
│       ├── Protocol/
│       │   ├── RespType.cs   # RESP frame type enum (SimpleString, Error, Integer, Bulk, Array)
│       │   ├── RespFrame.cs  # Immutable RESP value representation
│       │   ├── RespDecoder.cs# Streaming parser over ReadOnlySequence<byte> (chunking & pipelining)
│       │   └── RespEncoder.cs# UTF-8 and binary serialization into IBufferWriter<byte>
│       ├── Store/
│       │   ├── IStore.cs     # Store contract for key-value operations and TTL queries
│       │   ├── CacheEntry.cs # Record tracking string value and absolute expiration timestamp
│       │   └── ConcurrentStore.cs # Thread-safe store implementation using ConcurrentDictionary
│       ├── Commands/
│       │   ├── ICommandDispatcher.cs # Command routing and arity validation
│       │   └── CommandDispatcher.cs  # Handler implementations for supported commands
│       ├── Persistence/
│       │   ├── IAofLogger.cs # AOF append contract with absolute expire_at timestamps
│       │   └── ChannelAofLogger.cs # Background Channel consumer with fsync support
│       └── Server/
│           ├── TcpServer.cs  # TcpListener accepting connections and dispatching Pipelines
│           └── ClientConnection.cs # Per-socket async PipeReader/Writer processing loop
├── tests/
│   └── Thredis.Tests/
│       ├── Thredis.Tests.csproj # xUnit, FluentAssertions / Microsoft.NET.Test.Sdk
│       ├── ProtocolTests.cs     # Frame decoding, chunking, and pipelining tests
│       ├── StoreTests.cs        # Set/Get/Incr/Ttl tests with mock IClock
│       ├── CommandTests.cs      # Arity, unknown commands, case-insensitivity, NX/XX flags
│       ├── PersistenceTests.cs  # AOF round-trip, absolute TTL replay, corrupt line recovery
│       └── IntegrationTests.cs  # Live in-process TCP socket tests
└── data/                     # Runtime AOF storage directory (created on start)
```

---

## 3. Toolchain, Invocation, Exit Codes, and Configuration

### 3.1 Runtime / Dev Toolchain (pinned)
- **SDK:** .NET 9.0 SDK (`mcr.microsoft.com/dotnet/sdk:9.0-alpine`).
- **Compiler Settings (`Thredis.csproj`):**
  ```xml
  <PropertyGroup>
    <TargetFramework>net9.0</TargetFramework>
    <Nullable>enable</Nullable>
    <ImplicitUsings>enable</ImplicitUsings>
    <TreatWarningsAsErrors>true</TreatWarningsAsErrors>
  </PropertyGroup>
  ```
- **Dependencies:** `System.IO.Pipelines`, `Microsoft.Extensions.Hosting`, `Microsoft.Extensions.DependencyInjection`.

### 3.2 Makefile Targets (all REQUIRED)
- `make build` → `dotnet build -c Release` (zero compiler warnings allowed).
- `make run` → `dotnet run --project src/Thredis -c Release`.
- `make test` → `dotnet test -c Release --verbosity normal` (runs all unit and integration tests; zero failures allowed).
- `make lint` → `dotnet format --verify-no-changes` (or `dotnet build -c Release` with `TreatWarningsAsErrors=true`).
- `make clean` → `dotnet clean && rm -rf src/Thredis/bin src/Thredis/obj tests/Thredis.Tests/bin tests/Thredis.Tests/obj`.

### 3.3 Configuration (environment variables, pinned)
- `PORT` (default `6379`) — TCP listen port.
- `THREDIS_DATA_DIR` (default `./data`) — directory holding `dump.aof`.
- `THREDIS_AOF_FSYNC` (`true` default) — call `FileStream.Flush(flushToDisk: true)` after every mutation before transmitting the RESP reply.

### 3.4 Exit Codes and Logging
- Clean `SIGINT`/`SIGTERM` shutdown → cancel background workers, flush AOF channel, close sockets, exit code `0`.
- Fatal startup error (e.g. port bind collision, unwritable directory) → write `thredis: <reason>` to stderr, exit code `1`.
- All operational diagnostic logs on stdout/stderr MUST use the prefix `thredis: `.

---

## 4. RESP Wire Protocol & Framing Engine

`thredis` communicates exclusively via standard REdis Serialization Protocol (RESP2/RESP3) over TCP.

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

### 4.2 Stream Parsing via `System.IO.Pipelines`
1. **Pipelines Chunking:** The `PipeReader` reads incoming byte sequences (`ReadOnlySequence<byte>`). `RespDecoder.TryParse` inspects the sequence. If a frame is incomplete, the reader marks consumed/examined positions and waits for additional bytes without allocating buffer arrays.
2. **Command Pipelining:** Multiple concatenated commands in a single TCP read buffer are parsed sequentially in FIFO order. Replies are written into `PipeWriter` and flushed in exact matching order.
3. **Inline Commands:** Plain text commands (e.g. `PING\r\n`, `SET k v\r\n`) without `*` prefix are split by whitespace and parsed transparently.
4. **Binary Safety:** Bulk Strings are stored as `byte[]` or `ReadOnlyMemory<byte>`, allowing arbitrary binary payloads, UTF-8 strings, and null bytes (`0x00`).
5. **Case-Insensitivity:** Command names are compared using `StringComparison.OrdinalIgnoreCase`.

---

## 5. Supported Commands & Parity Semantics

| Command | Signature & Description | Success RESP Reply | Error RESP Reply |
| :--- | :--- | :--- | :--- |
| `PING` | `PING [message]`<br>Tests connection liveness. | With 0 args: `+PONG\r\n`<br>With 1 arg: `$<len>\r\n<message>\r\n` | `>1` args: `-ERR wrong number of arguments for 'ping' command\r\n` |
| `ECHO` | `ECHO message`<br>Echoes the given message. | `$<len>\r\n<message>\r\n` | `!=1` arg: `-ERR wrong number of arguments for 'echo' command\r\n` |
| `QUIT` | `QUIT`<br>Asks server to close connection. | `+OK\r\n` (closes socket after reply) | `!=0` args: `-ERR wrong number of arguments for 'quit' command\r\n` |
| `SET` | `SET key value [EX s] [PX ms] [NX\|XX]`<br>Sets string value with optional expiration and existence guards. | `+OK\r\n` (if set)<br>`$-1\r\n` (if condition `NX`/`XX` not met) | `<2` args: `-ERR wrong number of arguments for 'set' command\r\n`<br>Both `NX` & `XX`: `-ERR syntax error\r\n`<br>Invalid TTL: `-ERR value is not an integer or out of range\r\n` |
| `GET` | `GET key`<br>Gets string value of a key. | `$<len>\r\n<val>\r\n`<br>`$-1\r\n` (if missing or expired) | `!=1` arg: `-ERR wrong number of arguments for 'get' command\r\n` |
| `DEL` | `DEL key [key ...]`<br>Removes specified key(s). | `:<count>\r\n` (count of keys removed) | `<1` arg: `-ERR wrong number of arguments for 'del' command\r\n` |
| `EXISTS` | `EXISTS key [key ...]`<br>Returns count of existing keys. | `:<count>\r\n` (duplicates counted per Redis 3.0+) | `<1` arg: `-ERR wrong number of arguments for 'exists' command\r\n` |
| `INCR` | `INCR key`<br>Increments integer value by 1. | `:<new_int>\r\n` | `!=1` arg: `-ERR wrong number of arguments for 'incr' command\r\n`<br>Non-integer: `-ERR value is not an integer or out of range\r\n` |
| `DECR` | `DECR key`<br>Decrements integer value by 1. | `:<new_int>\r\n` | `!=1` arg: `-ERR wrong number of arguments for 'decr' command\r\n`<br>Non-integer: `-ERR value is not an integer or out of range\r\n` |
| `EXPIRE` | `EXPIRE key seconds`<br>Sets timeout on key in seconds. | `:1\r\n` (timeout set)<br>`:0\r\n` (key missing or expired) | `!=2` args: `-ERR wrong number of arguments for 'expire' command\r\n`<br>Non-integer: `-ERR value is not an integer or out of range\r\n` |
| `TTL` | `TTL key`<br>Returns remaining TTL in seconds. | `:-2\r\n` (missing or expired)<br>`:-1\r\n` (no expiry)<br>`:<seconds>\r\n` (remaining TTL) | `!=1` arg: `-ERR wrong number of arguments for 'ttl' command\r\n` |
| `KEYS` | `KEYS pattern`<br>Returns keys matching glob pattern (`*`, `?`). | `*<count>\r\n$<len>\r\n<key1>...` (`*0\r\n` if none) | `!=1` arg: `-ERR wrong number of arguments for 'keys' command\r\n` |
| `FLUSHALL` | `FLUSHALL`<br>Purges all keys and truncates AOF. | `+OK\r\n` | None |
| *Unknown* | Any unrecognized command `<NAME>` | None | `-ERR unknown command '<NAME>'\r\n` |

---

## 6. Store Architecture & Multi-Threaded Concurrency

1. **Storage Structure:**
   * Keys are indexed in a `ConcurrentDictionary<string, CacheEntry>`.
   * `CacheEntry` stores `byte[] Value` and nullable `DateTimeOffset? ExpiresAtUtc`.
2. **Atomic Per-Key Mutations:**
   * Atomic operations like `INCR`, `DECR`, and conditional `SET (NX/XX)` use atomic factory methods (`AddOrUpdate`, `TryAdd`) or per-key locking stripes to guarantee serialization across concurrent client threads.
3. **Dual Expiration Strategy:**
   * **Lazy Eviction:** On any key lookup, if `ExpiresAtUtc <= clock.UtcNow`, the key is removed and considered absent.
   * **Active Sweep:** A background `IHostedService` executing a `PeriodicTimer` (every 100ms) sweeps an active sample of keys to reclaim expired memory.
4. **Deterministic Clock Injection:**
   * All time queries invoke `IClock.UtcNow`. Unit tests inject a mock `IClock` allowing deterministic time advancing and freeze testing.

---

## 7. Append-Only File (AOF) Durability Engine

1. **Producer-Consumer Channel Architecture:**
   * Mutations are written to a bounded `System.Threading.Channels.Channel<AofMutation>` (`SingleReader = true`).
   * A dedicated background task reads from the channel and executes sequential writes to `dump.aof`.
   * When `THREDIS_AOF_FSYNC` is `true`, `FileStream.Flush(flushToDisk: true)` is awaited before returning acknowledgment.
   * Mutations with expirations are logged using absolute epoch timestamps (`EXPIREAT key <unix_timestamp>`).
2. **Replay & Startup Integrity:**
   * Before `TcpListener.Start()` is invoked, `AofLogger.ReplayAsync()` reads `dump.aof` from start to finish.
   * Expired keys (`ExpiresAt <= startupTime`) are pruned immediately during replay.
   * Truncated trailing lines (resulting from abrupt `SIGKILL`) are detected and cleanly skipped.

---

## 8. Comprehensive Test Strategy & Verification Specifications

All verification test suites must be executable via `make test` (`dotnet test -c Release`).

### 8.1 RESP Framing & Protocol Suite (`tests/Thredis.Tests/ProtocolTests.cs`)
- **Encoding:** Verifies byte-level wire formatting for all RESP types: Simple Strings (`+OK\r\n`), Errors (`-ERR ...\r\n`), Integers (`:100\r\n`), Bulk Strings (`$5\r\nhello\r\n`), Null Bulk (`$-1\r\n`), Arrays (`*2\r\n...`), and Null Arrays (`*-1\r\n`).
- **Binary Safety:** Encodes and decodes binary payloads containing embedded `\r\n`, null bytes (`0x00`), and non-ASCII UTF-8 sequences.
- **Chunked Stream Reassembly:** Simulates fragmented TCP packets. Feeds byte slices into `PipeReader` one byte at a time and asserts `RespDecoder.TryParse` returns true only when the full frame and trailing `\r\n` have arrived.
- **Command Pipelining:** Concatenates multiple RESP commands (`*1\r\n$4\r\nPING\r\n*2\r\n$4\r\nECHO\r\n$2\r\nhi\r\n`) into a single byte sequence; asserts all commands are parsed in FIFO order.
- **Inline Parsing:** Verifies plain text commands (`PING\r\n`, `PING hello\r\n`, `SET k v\r\n`) decode correctly.

### 8.2 In-Memory Store & Expiration Suite (`tests/Thredis.Tests/StoreTests.cs`)
- **Deterministic Mock Clock:** Tests initialize `ConcurrentStore` with a mock `IClock` instance.
- **Basic CRUD:** Sets, gets, deletes, and checks existence of string keys.
- **TTL Expiration Life Cycle:**
  - Sets key with 5-second TTL at $T_0$.
  - Advances mock clock to $T_0 + 3s$; asserts `TTL` returns `2`.
  - Advances mock clock to $T_0 + 5.1s$; asserts `GET` returns `null` and `TTL` returns `-2` (lazy eviction).
- **TTL Overwrite:** Setting an existing expiring key with a plain `SET` clears expiration; `TTL` returns `-1`.
- **Active Sweep before `KEYS`:** Expired keys are purged and not returned by `KEYS *`.

### 8.3 Command Semantics & Dispatch Suite (`tests/Thredis.Tests/CommandTests.cs`)
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

### 8.4 AOF Persistence & Recovery Suite (`tests/Thredis.Tests/PersistenceTests.cs`)
- **Mutation Logging:** Verifies every mutation writes a valid line with command, key, and absolute `expire_at`.
- **Replay Accuracy:** Appends records to a temporary AOF file, re-initializes store from the file, and asserts full state equality.
- **Time-Travel Resilience:** Writes a key expiring at $T_0 + 10s$. Sets clock to $T_0 + 20s$ and replays AOF into fresh store; asserts the key is expired and absent.
- **Truncated Last Line Recovery:** Simulates crash by writing a partial line at the end of `dump.aof`. Replay successfully restores all valid preceding entries.
- **`FLUSHALL` Truncation:** Asserts `FLUSHALL` truncates `dump.aof` to zero bytes on disk.

### 8.5 Socket & Protocol Integration Suite (`tests/Thredis.Tests/IntegrationTests.cs`)
- Runs a live `thredis` TCP server on `127.0.0.1` with an ephemeral port.
- **Raw Socket Verification:** Tests raw TCP socket connections sending inline strings and multi-bulk arrays.
- **Client Interop (via `redis-cli` or `StackExchange.Redis`):**
  - Executes full command suite (`ping`, `set`, `get`, `del`, `exists`, `incr`, `decr`, `expire`, `ttl`, `keys`, `flushall`).
  - Executes command pipelines; asserts replies return in strict matching sequence.
- **Concurrent Client Load:** 50 concurrent `Task` workers hammering `INCR counter` simultaneously. Asserts final value is exactly `50` with zero lost updates.
- **Server Restart Durability:** Writes data, terminates server, restarts new server on same data directory, asserts data persists.

### 8.6 Black-Box E2E Conformance Suite (`tests/e2e/run_tests.sh`)
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

### 8.7 Differential Conformance Oracle
The integration test suite and E2E script MUST yield the exact same test outputs and return code `0` whether executed against `thredis` or standard `redis:7-alpine`.

---

## 9. E2E Black-Box Harness (docker-compose)

`docker-compose.yml` MUST define two services, runnable from the host with `make e2e`:

```yaml
services:
  api:
    build: .
    command: ["dotnet", "run", "--project", "src/Thredis", "-c", "Release"]
    environment:
      THREDIS_DATA_DIR: "/data"
      PORT: "6379"
      THREDIS_AOF_FSYNC: "true"
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

- `tests/e2e/Dockerfile`: `FROM alpine:3.21`, installs `redis` (`redis-cli` tool), bash, copies `run_tests.sh`, `CMD ["/tests/run_tests.sh"]`.
- `tests/e2e/run_tests.sh`: Black-box `redis-cli` assertions against `${REDIS_URL}`. Exits nonzero on the first failure.

---

## 10. Documentation Requirements (README & Architecture Guide)

1. **`README.md`**: Must exist at root level documenting: installation, running the server, the full command API table, RESP wire format envelopes, AOF format, `make test`/`lint`/`e2e`, and `redis-cli` usage examples.
2. **`docs/` Folder**: Must contain markdown documentation covering Multithreaded Architecture, Pipelines & Zero-Allocation Streaming, Lock-Free Storage, and AOF Persistence.

---

## 11. Definition of Done (DoD)

To consider `thredis` fully implemented, the project must satisfy:
1. **Public RESP API & CLI Compatibility:** Native Redis RESP2/RESP3 wire-protocol TCP server operating on port 6379, passing all command assertions via official `redis-cli`.
2. **Persistence & Durability Invariant:** AOF persistence engine logs mutations with absolute `expire_at` timestamps, tolerates corrupt trailing lines, and successfully restores state on restart.
3. **Compiler Invariant:** Clean compilation with zero compiler warnings under `<TreatWarningsAsErrors>true</TreatWarningsAsErrors>`.
4. **Verification Criteria:** 100% test pass rate on `make test` (`dotnet test -c Release`) and `make e2e`.
