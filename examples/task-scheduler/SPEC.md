# Task Scheduler Daemon Specification

## 1. Overview
The goal of this project is to implement a lightweight background Task Scheduler daemon. The daemon exposes an HTTP API to schedule, monitor, and cancel jobs that execute shell commands at designated intervals (in seconds). The scheduler stores jobs and their execution logs in an SQLite database.

---

## 2. Requirements

### 2.1. API Endpoints
The daemon must serve the following HTTP endpoints:

1.  **POST `/jobs`**
    *   **Description:** Schedules a new recurring command execution.
    *   **Request Body (JSON):**
        ```json
        {
          "id": "backup-logs",
          "command": "tar -czf logs.tar.gz /var/log/app",
          "interval_seconds": 60
        }
        ```
    *   **Response (Success, 201 Created):** Returns the scheduled job object.
    *   **Response (Error, 400 Bad Request):** If validation fails (e.g. interval < 1, command empty, or ID already exists).

2.  **GET `/jobs`**
    *   **Description:** Retrieves all active scheduled jobs.
    *   **Response (Success, 200 OK):** A list of jobs.

3.  **DELETE `/jobs/{id}`**
    *   **Description:** Cancels a scheduled job by ID.
    *   **Response (Success, 200 OK):** Confirmation of cancellation.
    *   **Response (Error, 404 Not Found):** If the job ID is not registered.

4.  **GET `/jobs/{id}/logs`**
    *   **Description:** Returns the execution history for the given job.
    *   **Response (Success, 200 OK):**
        ```json
        [
          {
            "job_id": "backup-logs",
            "executed_at": "2026-06-21T23:30:00Z",
            "duration_ms": 150,
            "exit_code": 0,
            "stdout": "tar: removing leading '/' from member names\n",
            "stderr": ""
          }
        ]
        ```

### 2.2. Background Scheduler Engine
*   A background execution loop or worker pool must monitor active jobs.
*   When a job's interval elapses, the daemon must execute the `command` asynchronously as a subprocess (shell execution).
*   The scheduler must not block the API server. Jobs must run concurrently.
*   Once a command finishes, the daemon must record an execution entry in the database:
    *   Execution timestamp
    *   Subprocess execution duration in milliseconds
    *   Command exit code
    *   First 1,000 characters of stdout and stderr

### 2.3. Persistence
*   Jobs and log history must be persisted in an SQLite database (e.g. `scheduler.db`).
*   On application startup, active jobs must be reloaded from the database and scheduled immediately.

---

## 3. Technical Constraints
*   **Language:** Go, Python, or Node.js.
*   **Concurrency:** Must safely handle concurrent API requests and background process executions without database/state locks.
*   **Database:** SQLite.

---

## 4. Verification Criteria & Testing

### 4.1. Expected Tests
*   **Unit Tests:**
    *   Verify interval calculation and execution triggers.
    *   Verify concurrent-safe state maps (preventing race conditions when adding/deleting tasks).
*   **Integration Tests:**
    *   Schedule a simple fast-running script (e.g. `echo "hello"`) with a low interval (e.g. 1 second).
    *   Verify that execution logs are written to SQLite within a few seconds.
    *   Verify that deleting the job stops further execution.
    *   Verify daemon reload reads previously saved jobs on startup.
