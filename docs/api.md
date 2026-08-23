# REST and Server-Sent Events (SSE) API Reference

Noctifab exposes local HTTP APIs for headless orchestration, supervisor LLM control, and real-time visual web telemetry:
1. **Headless Daemon REST API**: default `127.0.0.1:18080` (started via `noctifab start` / `serve`).
2. **Visual Web Dashboard API**: default `127.0.0.1:8080` (started via `noctifab start -w` or `noctifab dashboard -w`).

> An OpenAPI 3.1.0 specification is available at [`api/openapi.yaml`](file:///Users/diegoj/repos/noctifab/api/openapi.yaml).

---

## Endpoint Reference

### 1. System State Snapshot
* **Method & Path**: `GET /api/v1/state`
* **Description**: Returns the full JSON snapshot of system state including user stories, topological task DAG, active agent worker telemetry cards, action logs, and token metrics.
* **Response**: `200 OK`

---

### 2. Live Telemetry Event Stream (SSE)
* **Method & Path**: `GET /api/v1/events`
* **Description**: Server-Sent Events (`text/event-stream`) streaming real-time timeline events (`tool_executed`, `task_started`, `test_run`, `task_failed`, `story_completed`) with a 100-event circular ring buffer replay on reconnection.
* **Response**: `200 OK` (`text/event-stream`)

---

### 3. Inject Mid-Flight Steering Directive
* **Method & Path**: `POST /api/v1/steer`
* **Description**: Injects a human-in-the-loop steering prompt into the target task or active running worker goroutines.
* **Request Payload**:
  ```json
  {
    "task_id": "task-0",
    "directive": "Use PostgreSQL connection pool instead of SQLite"
  }
  ```
* **Response**: `202 Accepted`

---

### 4. Submit Feature Prompt Order
* **Method & Path**: `POST /api/v1/orders`
* **Description**: Enqueues an ad-hoc feature requirement / prompt order directly into the autonomous story queue.
* **Request Payload**:
  ```json
  {
    "prompt": "Add rate limiting middleware with sliding window algorithm"
  }
  ```
* **Response**: `202 Accepted`

---

### 5. Enqueue User Story
* **Method & Path**: `POST /api/v1/stories`
* **Description**: Enqueues a single markdown user story file or a folder path containing stories to execute lexicographically.
* **Request Payload**:
  ```json
  {
    "path": "./roadmap/user-stories/US-001.md"
  }
  ```
* **Response**: `202 Accepted`

---

### 6. List Execution Statuses
* **Method & Path**: `GET /api/v1/status`
* **Description**: Returns the real-time status of all active and completed user story orchestrations.
* **Response**: `200 OK`

---

### 7. List Pending Clarifications
* **Method & Path**: `GET /api/v1/clarifications?pending=true`
* **Description**: Returns all unresolved clarification questions asked by the Planner Agent.
* **Response**: `200 OK`

---

### 8. Resolve Clarification
* **Method & Path**: `POST /api/v1/clarifications/{id}/resolve`
* **Description**: Answers a pending clarification question by ID, unblocking the orchestrator thread.
* **Request Payload**:
  ```json
  {
    "answer": "Redis"
  }
  ```
* **Response**: `200 OK`

---

### 9. Pause Execution
* **Method & Path**: `POST /api/v1/pause`
* **Description**: Temporarily suspends the active orchestration cycle loop.
* **Response**: `200 OK` (`{"status":"paused"}`)

---

### 10. Resume Execution
* **Method & Path**: `POST /api/v1/resume`
* **Description**: Resumes the paused story orchestration.
* **Response**: `200 OK` (`{"status":"resumed"}`)

---

### 11. Cancel Execution
* **Method & Path**: `POST /api/v1/cancel`
* **Description**: Gracefully halts the active task execution, reverts the current task branch, and releases directory locks.
* **Response**: `202 Accepted` (`{"status":"cancelled"}`)

---

### 12. Add Manual Task
* **Method & Path**: `POST /api/v1/tasks`
* **Description**: Inserts a custom manual task directly into the active scheduling pipeline.
* **Request Payload**:
  ```json
  {
    "title": "Migrate Legacy Users Table",
    "description": "Export old DB records and merge into the user profile table.",
    "depends_on": ["task-171923"]
  }
  ```
* **Response**: `202 Accepted`

---

### 13. Override Task Merge Quality Gate
* **Method & Path**: `POST /api/v1/tasks/{id}/override-merge`
* **Description**: Force-overrides the test validation gate for a specific task branch.
* **Response**: `202 Accepted`

---

### 14. Health & Diagnostics Probes
* **`GET /healthz`**: Liveness probe returning `{"status":"ok"}`.
* **`GET /readyz`**: Readiness probe returning `{"status":"ready"}`.
* **`GET /statusz`**: Lightweight stripped state diagnostic summary for daemon monitoring.

---

### 15. Visual Spec Studio & Specification Endpoints
* **`GET /api/v1/spec`**: Returns current `SPEC.md` content, revision versions, and active iteration state.
* **`POST /api/v1/spec`**: Initiates multi-agent specification drafting from a prompt payload (`{"prompt":"..."}`).
* **`POST /api/v1/spec/refine`**: Submits iterative refinement feedback (`{"feedback":"..."}`).
* **`POST /api/v1/spec/checkout`**: Time-travel rollbacks to a previous version (`{"version": 1}`).
* **`POST /api/v1/spec/approve`**: Approves specification and triggers automatic roadmap story decomposition.

---

### 16. Execution Report Endpoint
* **`GET /api/v1/report`**: Returns the latest synthesized markdown execution report (`<TIMESTAMP>_<PROJECT>.md`) and metadata.

---

### 17. Modified Files Content Inspector
* **`GET /api/v1/files/content?file=<path>`**: Returns raw source code, line counts, and diff metadata for workspace files modified during task runs.

---

### 18. Telemetry & Token Metrics Endpoint
* **`GET /api/v1/metrics`**: Returns token consumption broken down by agent role (`GENERATOR`, `TESTER`, `PLANNER`, `QA`) and tool action distributions.

---

### 19. Roadmap & User Story Status Endpoint
* **`GET /api/v1/roadmap`**: Returns parsed user story specifications, dependency DAGs, and task completion percentages.

---

### 20. State History Snapshots Endpoint
* **`GET /api/v1/states`**: Returns historical state transitions and timeline snapshots for visual timeline scrubbers.

---

### 21. Queued Feature Orders List Endpoint
* **`GET /api/v1/orders/list`**: Returns the queue of submitted prompt orders and their processing statuses.

---

## LLM Provider Infrastructure & API Key Authentication

The Go infrastructure client package (`pkg/infrastructure/llm`) instantiates provider clients based on `config.yaml` (`llm.provider`) or runtime configuration parameters.

### Environment Variable Fallback Resolution Order

If `llm.api_key` is not explicitly provided, `Client.Complete` resolves authentication tokens in the following order:

1. Provider specific environment variable (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `OPENCODE_API_KEY`, `KIMI_API_KEY`/`MOONSHOT_API_KEY`, `GROQ_API_KEY`, `OPENROUTER_API_KEY`, `DASHSCOPE_API_KEY`/`QWEN_API_KEY`, `TOGETHER_API_KEY`, `LLAMA_API_KEY`/`META_API_KEY`, `HUGGINGFACE_API_KEY`, `MISTRAL_API_KEY`, `DEEPSEEK_API_KEY`, `HERMES_API_KEY`, `XAI_API_KEY`/`GROK_API_KEY`, `PERPLEXITY_API_KEY`, `FIREWORKS_API_KEY`, `SAMBANOVA_API_KEY`, `COHERE_API_KEY`/`CO_API_KEY`).
2. Generic LLM environment override (`NOCTIFAB_LLM_API_KEY`).
3. `secrets.yaml` reference lookup (e.g. `secret:KIMI_API_KEY`).
