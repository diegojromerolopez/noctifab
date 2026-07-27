# Headless Daemon REST API

When running the headless orchestrator daemon (`noctifab serve`), a loopback HTTP REST API is exposed locally on `127.0.0.1:18080` to manage user stories, task lifecycles, clarifications, and active execution states.

---

## Endpoint Reference

### 1. Enqueue User Story
* **Method & Path**: `POST /api/v1/stories`
* **Description**: Enqueues a single markdown user story file or a folder path containing stories to execute lexicographically.
* **Request Payload**:
  ```json
  {
    "path": "./roadmap/US-001.md"
  }
  ```
* **Response**: `202 Accepted`

---

### 2. List Execution Statuses
* **Method & Path**: `GET /api/v1/status`
* **Description**: Returns the real-time status of all active and completed user story orchestrations.
* **Response**: `200 OK`
  ```json
  [
    {
      "id": "story-1719234857",
      "project_path": "/Users/user/repos/myproject",
      "story_status": "running",
      "build_status": "success",
      "metadata": {
        "feature_name": "Login Form Validation",
        "total_cost_usd": "0.1420"
      }
    }
  ]
  ```

---

### 3. List Pending Clarifications
* **Method & Path**: `GET /api/v1/clarifications?pending=true`
* **Description**: Returns all unresolved clarification questions asked by the Planner Agent.
* **Response**: `200 OK`
  ```json
  [
    {
      "id": "clar-923847293",
      "question": "Which database type is preferred for session storage?",
      "resolved": false,
      "options": ["Redis", "PostgreSQL", "SQLite"]
    }
  ]
  ```

---

### 4. Resolve Clarification
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

### 5. Pause Execution
* **Method & Path**: `POST /api/v1/pause`
* **Description**: Temporarily suspends the active orchestration cycle loop.
* **Response**: `202 Accepted`
  ```json
  {
    "status": "paused"
  }
  ```

---

### 6. Resume Execution
* **Method & Path**: `POST /api/v1/resume`
* **Description**: Resumes the paused story orchestration.
* **Response**: `202 Accepted`
  ```json
  {
    "status": "running"
  }
  ```

---

### 7. Cancel Execution
* **Method & Path**: `POST /api/v1/cancel`
* **Description**: Gracefully halts the active task execution, reverts the current task branch, and releases directory locks.
* **Response**: `202 Accepted`
  ```json
  {
    "status": "cancelled"
  }
  ```

---

### 8. Add Manual Task
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

---

## LLM Provider Infrastructure & API Key Authentication

The Go infrastructure client package (`pkg/infrastructure/llm`) instantiates provider clients based on `config.yaml` (`llm.provider`) or runtime configuration parameters.

### Environment Variable Fallback Resolution Order

If `llm.api_key` is not explicitly provided, `Client.Complete` resolves authentication tokens in the following order:

1. Provider specific environment variable (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GEMINI_API_KEY`, `OPENCODE_API_KEY`, `KIMI_API_KEY`/`MOONSHOT_API_KEY`, `GROQ_API_KEY`, `OPENROUTER_API_KEY`, `DASHSCOPE_API_KEY`/`QWEN_API_KEY`, `TOGETHER_API_KEY`, `LLAMA_API_KEY`/`META_API_KEY`, `HUGGINGFACE_API_KEY`/`HF_TOKEN`, `MISTRAL_API_KEY`, `DEEPSEEK_API_KEY`, `HERMES_API_KEY`, `XAI_API_KEY`/`GROK_API_KEY`, `PERPLEXITY_API_KEY`, `FIREWORKS_API_KEY`, `SAMBANOVA_API_KEY`, `COHERE_API_KEY`/`CO_API_KEY`).
2. Generic LLM environment override (`NOCTIFAB_LLM_API_KEY`).
3. `secrets.yaml` reference lookup (e.g. `secret:KIMI_API_KEY`).
