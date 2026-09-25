# Frontpunch Specification: Valkey/Redis-Backed Async Jobs for Python

## 1. Overview
`frontpunch` is an asynchronous background job processing library for Python, heavily inspired by Ruby's Sidekiq. It uses Valkey (preferred for licensing reasons) or Redis as a message broker for high-throughput, low-latency job distribution, supports concurrent multi-threaded execution workers, schedules tasks for future runtimes, and implements robust error retries with exponential backoff.

### 1.1 Minimal Project Skeleton (Walking Skeleton Priority for US-001)
To ensure rapid verification, the first user story (`roadmap/US-001.md`) MUST establish a runnable walking skeleton delivering the minimal end-to-end flow:
1. `frontpunch/__init__.py`: Package entrypoint exposing `@task`, `.delay()`, and `enqueue()`.
2. `frontpunch/client.py`: Basic Redis/Valkey client pushing JSON job payloads (`LPUSH`).
3. `frontpunch/worker.py` & CLI `frontpunch worker`: Minimal worker loop pulling from queues (`BRPOP`) and executing functions.
4. Baseline smoke test in `tests/test_basic.py` verifying that enqueuing and worker execution works.
Subsequent stories then incrementally add exponential backoff retries, cron scheduling, and dashboard features.

---

## 2. Requirements

### 2.1. Client API
The client library must provide a Python API to define and enqueue jobs:

1.  **Job Definition Decorator:**
    ```python
    import frontpunch

    @frontpunch.task(queue="default", max_retries=5)
    def send_welcome_email(user_id, email_address):
        # Email sending logic here
        pass
    ```

2.  **Asynchronous Enqueueing:**
    *   **Immediate execution:** Enqueues the job into the designated Redis queue immediately.
        ```python
        send_welcome_email.delay(123, "user@example.com")
        # OR
        frontpunch.enqueue(send_welcome_email, 123, "user@example.com")
        ```
    *   **Scheduled execution (relative delay):** Schedules the job to run after a delay.
        ```python
        send_welcome_email.perform_in(300, 123, "user@example.com")  # 5 minutes delay
        ```
    *   **Scheduled execution (absolute time):** Schedules the job to run at a specific UTC timestamp.
        ```python
        from datetime import datetime, timezone
        run_at = datetime(2026, 6, 22, 10, 0, 0, tzinfo=timezone.utc)
        send_welcome_email.perform_at(run_at, 123, "user@example.com")
        ```

### 2.2. Worker Daemon
The library must include a command-line interface to run the background job processor:

```bash
frontpunch worker --queues default,critical --concurrency 5 --redis-url redis://localhost:6379/0
```

*   **Concurrency:** Spawns a pool of `N` worker threads (or processes) to handle incoming jobs concurrently.
*   **Polling:** Efficiently blocks or polls Redis for tasks matching the specified queues (prioritized from left to right).
*   **Graceful Shutdown:** On receipt of `SIGTERM` or `SIGINT`, stops accepting new tasks, allows active tasks to complete within a grace period (default 10 seconds, configurable via `--shutdown-timeout`), then moves any in-flight tasks still held in the per-thread `frontpunch:processing:<worker_id>:<thread_id>` lists back to their original `frontpunch:queue:{queue_name}` list via `LPUSH` before exiting. The `frontpunch:processing:*` list model is introduced by US-020 (Super Fetch); prior to US-020 the worker uses `BRPOP` directly on `frontpunch:queue:*` and shutdown simply discards in-flight tasks (or relies on US-020 recovery).

### 2.3. Queueing Protocol (Redis Data Structures)
*   **Active Queues:** Stored using Redis Lists. Clients `LPUSH` serialized JSON job payloads, and workers pull them using `BRPOP`.
*   **Scheduled Jobs / Retries:** Stored in a Redis Sorted Set (ZSET) named `frontpunch:scheduled`. The score is the UTC unix timestamp of when the job should run. A scheduler thread inside the worker periodically queries this ZSET, pops mature jobs, and moves them to their active lists.
*   **Job Payload Schema (JSON):**
    ```json
    {
      "jid": "job-uuid-12345",
      "class": "path.to.send_welcome_email",
      "args": [123, "user@example.com"],
      "queue": "default",
      "created_at": 1782086400,
      "enqueued_at": 1782086400,
      "retry_count": 0,
      "max_retries": 5
    }
    ```
    The `class` field MUST be a fully-qualified dotted import path resolvable by `importlib.import_module(class_path)` on the worker. Bare function names are not permitted.

### 2.4. Error Handling and Automatic Retries
*   If a job raises an unhandled exception during execution, the worker must catch it.
*   If `retry_count < max_retries`, schedule the job for retry with exponential backoff:
    `delay_seconds = 15 + (retry_count * 10) + (retry_count ** 4)`
    Increment the `retry_count` and insert the job into the `frontpunch:scheduled` ZSET. Comparison is `retry_count >= max_retries` (i.e., once `retry_count` equals `max_retries`, the job goes to the dead list, not retried again).
*   If retries are exhausted, move the job to the dead-letter list (`frontpunch:dead`).

### 2.5. Web Dashboard (Monitoring API)
The daemon or package must expose a simple HTTP dashboard/API via **FastAPI** (sync routes via `@app.get`; no async handlers required) to inspect queues:
*   `GET /stats`: Returns total processed, failed, currently active, and dead job counts. Live counters are stored as Redis strings `frontpunch:stats:processed` and `frontpunch:stats:failed`, incremented via `INCR` on each success/failure event. `active` is the sum of all `frontpunch:processing:*` list lengths. `dead` is `LLEN frontpunch:dead`.
*   `GET /queues`: Returns active queues and their current lengths.
*   `POST /queues/dead/retry`: Retries a specific job currently in the dead list.
*   `GET /workers`: Returns all active worker processes and their current execution details.

### 2.6. Extensibility & Parity Features
To maintain complete feature-parity with the full Sidekiq ecosystem (including Pro and Enterprise features), the following advanced constructs must be implemented:

1.  **Middleware Pipeline:**
    *   The framework must support both client-side (triggered during enqueuing) and server-side (triggered around job execution) middleware chains.
    *   Middlewares are callables that wrap the target execution. They must accept the job payload, its queues, and a callable reference to continue execution along the chain (similar to the standard middleware pattern).
    *   Example registration:
        ```python
        frontpunch.server_middleware.add(MyCustomLoggerMiddleware())
        ```

2.  **Weighted Queue Polling:**
    *   The worker daemon must support queue weights rather than strictly prioritized polling.
    *   If queues are specified as `critical: 3` and `default: 1`, the worker must pool the queues such that, statistically, `critical` jobs are picked for processing 3 times more often than `default` jobs.

3.  **Process Registry & Heartbeats:**
    *   Every active worker process must write a periodic heartbeat hash to Redis/Valkey (e.g., `frontpunch:processes:<host>:<pid>`) with a TTL of 60 seconds. Hostnames containing characters not valid in Redis keys are sanitized (replace `_` and other invalid chars with `-`).
    *   The heartbeat hash fields are: `host` (string), `pid` (int), `started_at` (ISO timestamp), `updated_at` (ISO timestamp), `concurrency` (int), `active_jobs` (JSON array of `{jid, queue}` pairs — only jid + queue, never full payloads, to avoid leaking sensitive data and bloating the hash), and `quiet` (boolean, set by US-015 SIGTSTP handler).
    *   This registry must be queried by the HTTP dashboard API (`GET /workers`) to show live orchestration status.

4.  **Batches & Workflows (Pro feature):**
    *   Support grouping multiple jobs together and triggering a callback when all of them finish executing successfully.
    *   A `Batch` object must write metadata to Valkey/Redis containing the total count of jobs, pending count, and callback details.
    *   As each job inside the batch completes, it must atomically decrement the pending count. When the count reaches `0`, the specified `on_complete` callback job must be automatically enqueued.
    *   Example API:
        ```python
        batch = frontpunch.Batch(description="billing-batch")
        batch.on_complete = "path.to.billing_callback"
        with batch:
            for user_id in user_ids:
                charge_user.delay(user_id)
        ```

5.  **Unique Jobs (Enterprise feature):**
    *   Prevent enqueuing duplicate tasks using Redis/Valkey locks.
    *   Supported modes: `until_executing` (lock released when job starts running) and `until_executed` (lock released when job finishes executing).
    *   A lock key is derived dynamically from the task name and its arguments (e.g. `frontpunch:unique:<hash>`).
    *   Registration:
        ```python
        @frontpunch.task(unique="until_executed", unique_expiration=3600)
        def generate_report(report_id):
            pass
        ```

6.  **Rate Limiting (Enterprise feature):**
    *   Global rate-limiting checks inside jobs using Valkey/Redis atomic operations.
    *   Support `Concurrent` limiting (e.g., max 5 concurrent connections to Stripe API) and `Window` limiting (e.g., max 100 API requests per 60 seconds).
    *   Example API:
        ```python
        with frontpunch.limiter.concurrent("stripe-api", limit=5):
            # perform API call
        ```

7.  **Cron / Periodic Jobs:**
    *   Support scheduling recurring jobs using standard cron syntax.
    *   A cron runner component inside the leader process must periodically check registered cron definitions, calculate next execution times, and enqueue matching jobs when due.
    *   Example API:
        ```python
        frontpunch.cron.add("hourly-cleanup", "0 * * * *", run_cleanup)
        ```

8.  **Encrypted Jobs (Enterprise feature):**
    *   Protect sensitive parameters in-transit by encrypting arguments in-memory before enqueuing to Redis/Valkey, and decrypting them inside the worker right before running.
    *   Encryption keys must be configured in-memory and not stored in Redis/Valkey.
    *   Example API:
        ```python
        @frontpunch.task(encrypt=["social_security_number"])
        def process_payroll(user_id, social_security_number):
            pass
        ```

9.  **Rolling Restarts (Quiet Mode / TSTP):**
    *   Support safe rolling upgrades of worker processes.
    *   Upon receiving a `SIGTSTP` signal, the worker daemon must enter "Quiet Mode"—it stops fetching any new jobs from Redis/Valkey queues but continues executing active threads.
    *   When the active threads drop to `0`, the process can be safely restarted or terminated via `SIGTERM` without requiring job recovery steps.

### 2.7. Global Configuration & Exception Hierarchy

**`frontpunch.configure(...)`:**
- Signature: `frontpunch.configure(redis_url: str | None = None, encryption_key: str | None = None, dashboard_token: str | None = None) -> None`.
- Initializes the process-wide singleton client used by `.delay()` / `frontpunch.enqueue`.
- Calling `.delay()` or any enqueue API before `configure()` raises `frontpunch.exceptions.NotConfigured`.
- Decorators may be applied before `configure`; the client is only resolved at first enqueue (not at decoration time).

**`frontpunch.exceptions` module** (`frontpunch/exceptions.py`, owned by US-000):
- `FrontpunchError` (base, subclasses `Exception`)
- `ConnectionError` (Valkey/Redis connection failures)
- `NotConfigured` (calling enqueue APIs before `frontpunch.configure`)
- `RateLimitExceeded` (US-012)
- `DecryptionError` (US-014)
- `SerializationError` (subclass of `TypeError`; non-serializable args)
- `PauseError` (US-017)

---

## 3. Technical Constraints
*   **Language:** Python 3.10+.
*   **Dependencies:**
    *   `redis>=5.0,<6.0` (supports Valkey and Redis)
    *   `fastapi>=0.110,<0.120` and `uvicorn[standard]>=0.27` for the dashboard (web framework is **FastAPI** — see §2.5)
    *   `cryptography>=42.0` for US-014 (encryption)
    *   `croniter>=2.0` for US-013 (cron)
    *   `click>=8.1` for the CLI
*   **Storage Backend:** Valkey (preferred for licensing reasons) or Redis (Valkey is a compatible drop-in replacement).
*   **Coding Standards:**
    *   All code must comply with `black` (configured with a line length of 120 characters), `mypy` (static type checking), and `ruff` (linting).
    *   All functions, methods, class attributes, and module variables must have explicit type hints (type annotations) to ensure static type safety.
*   **Architectural Guidelines:**
    *   Code must strictly adhere to the SOLID design principles.
    *   Special emphasis must be placed on **Dependency Injection (DI)**: dependencies (such as the Valkey/Redis client, clock provider, task runner, and HTTP client) must be explicitly provided through class/function constructors instead of being instantiated inline.
    *   Follow the Domain-Driven Design (DDD) approach. Project layout (see §2.7).
*   **Implementation Rules:**
    *   Variables must not be re-assigned after their first definition unless inside a loop or `try/except` recovery path. Verify with `ruff` rule `RET504` and code review.
    *   All HTTP requests must have a maximum timeout specified.
    *   Do not catch `Exception` broadly **at module boundaries**. The worker's execution envelope (US-004) is the single allowed exception: it catches `Exception` to drive retry logic but re-raises `KeyboardInterrupt` and `SystemExit`.

### 3.1. Project Layout
The repository root MUST contain `pyproject.toml` (PEP 621, hatchling backend), `frontpunch/` (package), `tests/unit/`, `tests/integration/`, `tests/e2e/`, and `tests/e2e/docker-compose.yml` (E2E compose file exposing Valkey on `localhost:6379/0`; E2E tests connect via the `VALKEY_URL` env var). Package layout follows DDD:
- `frontpunch/domain/` — entities (`Job`, `Batch`, `CronEntry`), value objects (`JobPayload`, `BackoffStrategy`), repository ports (`JobRepository`, `ProcessRegistry`, `StatsRepository`).
- `frontpunch/application/` — use cases (`EnqueueJob`, `ExecuteJob`, `RetryJob`, `RecoverOrphans`).
- `frontpunch/infrastructure/valkey/` — `ValkeyJobRepository`, `ValkeyProcessRegistry`, `LuaScripts`, `ValkeyConnection`.
- `frontpunch/interfaces/` — `cli/` (`worker`, `swarm` commands), `http/` (`dashboard`), `client/` (`task`, `enqueue`, `Batch`).
All file paths in user stories (e.g. `frontpunch/client.py`) MUST be interpreted as facade modules re-exporting from the DDD tree above.

---

## 4. Verification Criteria & Testing

### 4.1. Test Suite Structure
Tests must be organized into the following three directories:
*   `tests/unit`: For fast, isolated validation.
*   `tests/integration`: For verifying collaboration between multiple classes and I/O wrappers.
*   `tests/e2e`: For full end-to-end flow validation.

### 4.2. Testing Guidelines
*   **Test Runner:** Use Python's standard `unittest` library. **Do not use pytest.**
*   **Unit Tests:**
    *   100% test coverage is required for all source files, enforced via `coverage run --source=frontpunch -m unittest discover -s tests && coverage report --fail-under=100`. The sandbox `test_command` in `.noctifab/config.yaml` MUST be set to this command.
    *   All unit tests must assert mock calls via `self.assertEqual` (note: not the deprecated `assertEquals`) with `mock.call_args_list` compared to a list of expected `call` objects.
*   **Integration Tests:**
    *   Only external, third-party I/O dependencies (like Redis connection sockets) may be mocked.
*   **End-to-End (E2E) Tests:**
    *   Nothing in the codebase is allowed to be mocked.
    *   Docker-based container services must be spun up to respond to actual Redis and network/HTTP requests of the project. E2E services are defined in `tests/e2e/docker-compose.yml` exposing Valkey on `localhost:6379/0`; E2E tests connect to `redis://valkey:6379/0` via the `VALKEY_URL` environment variable.
    *   E2E tests live in `tests/e2e/` and are excluded from the `--fail-under=100` gate via `[tool.coverage.run] source = ['frontpunch']` and `omit = ['tests/*']`.

### 4.3. Expected Tests
*   **Unit Tests:**
    *   Verify job payload generation and deserialization.
    *   Verify retry delay calculation matches the exponential formula.
*   **Integration Tests:**
    *   Using a real/mocked Redis server interface, enqueue a test task, run the worker program, and verify that the task executes successfully.
    *   Test that raising an exception in a task correctly retries the job and increments the retry count in Redis.
    *   Test scheduled execution by scheduling a job, verifying it is not run immediately, and verifying it runs after its scheduled timestamp has passed.


## 5. Definition of Done (DoD)

To consider `frontpunch` fully implemented, the generated Python framework must satisfy:
1. **Public API & CLI:** `frontpunch.task`, `.delay()`, `.perform_in()`, `.perform_at()` client APIs work seamlessly, and `frontpunch worker` daemon runs concurrent background processing with graceful shutdown.
2. **Linting Invariant:** Zero linter findings under `ruff check . && mypy .`.
3. **Verification Criteria:** 100% test pass rate and 100% unit coverage under `coverage run --source=frontpunch -m unittest discover -s tests && coverage report --fail-under=100`.
