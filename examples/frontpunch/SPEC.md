# Frontpunch Specification: Valkey/Redis-Backed Async Jobs for Python

## 1. Overview
`frontpunch` is an asynchronous background job processing library for Python, heavily inspired by Ruby's Sidekiq. It uses Valkey (preferred for licensing reasons) or Redis as a message broker for high-throughput, low-latency job distribution, supports concurrent multi-threaded execution workers, schedules tasks for future runtimes, and implements robust error retries with exponential backoff.

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
*   **Graceful Shutdown:** On receipt of `SIGTERM` or `SIGINT`, stops accepting new tasks, allows active tasks to complete within a grace period (e.g. 10 seconds), and pushes uncompleted tasks back onto the Redis queue before exiting.

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
    ```

### 2.4. Error Handling and Automatic Retries
*   If a job raises an unhandled exception during execution, the worker must catch it.
*   If `retry_count < max_retries`, schedule the job for retry with exponential backoff:
    $$\text{delay} = 15 + (\text{retry\_count} \times 10) + (\text{retry\_count}^4) \text{ seconds}$$
    Increment the `retry_count` and insert the job into the `frontpunch:scheduled` ZSET.
*   If retries are exhausted, move the job to the dead-letter list (`frontpunch:dead`).

### 2.5. Web Dashboard (Monitoring API)
The daemon or package must expose a simple HTTP dashboard/API (e.g. via Flask or FastAPI) to inspect queues:
*   `GET /stats`: Returns total processed, failed, currently active, and dead job counts.
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
    *   Every active worker process must write a periodic heartbeat hash to Redis/Valkey (e.g., `frontpunch:processes:<host>:<pid>`) with a TTL of 60 seconds.
    *   The heartbeat must include the worker's host name, process ID (PID), startup timestamp, concurrency limit, and the set of currently executing job payloads.
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

---

## 3. Technical Constraints
*   **Language:** Python 3.10+.
*   **Dependencies:** `redis-py` (for Valkey/Redis connection), lightweight web library (e.g. `FastAPI` or `Flask` for the dashboard).
*   **Storage Backend:** Valkey (preferred for licensing reasons) or Redis (Valkey is a compatible drop-in replacement).
*   **Coding Standards:**
    *   All code must comply with `black` (configured with a line length of 120 characters), `mypy` (static type checking), and `ruff` (linting).
    *   All functions, methods, class attributes, and module variables must have explicit type hints (type annotations) to ensure static type safety.
*   **Architectural Guidelines:**
    *   Code must strictly adhere to the SOLID design principles.
    *   Special emphasis must be placed on **Dependency Injection (DI)**: dependencies (such as the Valkey/Redis client, clock provider, task runner, and HTTP client) must be explicitly provided through class/function constructors instead of being instantiated inline.
    *   Follow the Domain-Driven Design (DDD) approach.
*   **Implementation Rules:**
    *   Use Static Single-Assignment (SSA) form (assign variables exactly once) unless inside a loop.
    *   All HTTP requests must have a maximum timeout specified.
    *   Do not catch `Exception` broadly. Only catch particular, specific exceptions.

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
    *   100% test coverage is required for all source files.
    *   All unit tests must assert mock calls via `self.assertEquals` with `mock.call_args_list` compared to a list of expected `call` objects.
*   **Integration Tests:**
    *   Only external, third-party I/O dependencies (like Redis connection sockets) may be mocked.
*   **End-to-End (E2E) Tests:**
    *   Nothing in the codebase is allowed to be mocked.
    *   Docker-based container services must be spun up to respond to actual Redis and network/HTTP requests of the project.

### 4.3. Expected Tests
*   **Unit Tests:**
    *   Verify job payload generation and deserialization.
    *   Verify retry delay calculation matches the exponential formula.
*   **Integration Tests:**
    *   Using a real/mocked Redis server interface, enqueue a test task, run the worker program, and verify that the task executes successfully.
    *   Test that raising an exception in a task correctly retries the job and increments the retry count in Redis.
    *   Test scheduled execution by scheduling a job, verifying it is not run immediately, and verifying it runs after its scheduled timestamp has passed.
