# Todo CLI Specification

## 1. Overview
The goal of this project is to implement a command-line interface (CLI) application for managing personal TODO tasks. The CLI should allow users to add tasks, list them, mark them as completed, and remove them. Tasks must be persisted locally in a JSON file.

---

## 2. Requirements

### 2.1. CLI Subcommands
The compiled binary/script should support the following commands and arguments:

1.  **`todo add "<task description>"`**
    *   **Description:** Adds a new task to the list.
    *   **Behavior:** Appends the task as pending and prints a success message including the task's new ID or index.
    *   **Output Example:** `Added task: "Buy milk" (ID: 1)`

2.  **`todo list`**
    *   **Description:** Lists all tasks, showing their ID/index, description, status, and creation date.
    *   **Behavior:** Prints a formatted list. Completed tasks should be visibly marked (e.g., with a checkmark or `[x]`).
    *   **Output Example:**
        ```
        ID  Status  Description  Created At
        1   [ ]     Buy milk     2026-06-21 23:15
        2   [x]     Clean room   2026-06-21 22:00
        ```

3.  **`todo done <id>`**
    *   **Description:** Marks a task with the given ID as completed.
    *   **Behavior:** Updates the task state and prints a confirmation message.
    *   **Output Example:** `Task 1 marked as completed.`
    *   **Error Case:** If the ID does not exist, print an error to `stderr` and exit with non-zero code.

4.  **`todo rm <id>`**
    *   **Description:** Removes a task with the given ID from the list.
    *   **Behavior:** Deletes the task and prints a confirmation message.
    *   **Output Example:** `Task 1 removed.`
    *   **Error Case:** If the ID does not exist, print an error to `stderr` and exit with non-zero code.

### 2.2. Persistence
*   Tasks must be stored in a file named `todo.json`.
*   The default path should be the current working directory, but the CLI should optionally accept a custom file path via an environment variable `TODO_FILE` or a `--file` flag.
*   The JSON schema should structure each task with at least:
    *   `id` (integer or string)
    *   `description` (string)
    *   `completed` (boolean)
    *   `created_at` (ISO timestamp string)
    *   `completed_at` (ISO timestamp string or null)

---

## 3. Technical Constraints
*   **Language:** Go, Python, or Node.js.
*   **Dependencies:** Standard library CLI parsing (e.g. `flag` in Go, `argparse` in Python) is preferred. Keep dependencies lightweight.
*   **Storage Format:** Raw JSON file.

---

## 4. Verification Criteria & Testing

### 4.1. Expected Tests
*   **Unit Tests:**
    *   Verify task addition, completion, and deletion functions in isolation.
    *   Verify correct JSON serialization and deserialization logic.
*   **Integration Tests:**
    *   Execute CLI commands as subprocesses and check exit codes and stdout/stderr outputs.
    *   Verify that tasks are successfully read from and written to the `todo.json` file.
