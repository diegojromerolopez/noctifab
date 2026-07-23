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

### 9. Force Task Merge (Resolve Quarantine)
* **Method & Path**: `POST /api/v1/tasks/{id}/override-merge`
* **Description**: Manually overrides a task branch blocked by a merge conflict or quarantined status, forcing the orchestrator to merge it into the integration branch.
* **Response**: `202 Accepted`
  ```json
  {
    "status": "accepted"
  }
  ```
