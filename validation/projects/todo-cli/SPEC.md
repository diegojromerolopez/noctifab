# Todo CLI Specification

## 1. Overview
The goal of this project is to implement a command-line interface (CLI) application for managing personal TODO tasks. The CLI should allow users to add tasks, list them, mark them as completed, and remove them. Tasks must be persisted locally in a JSON file.

---

## 2. Requirements

### 2.1. CLI Subcommands
The compiled binary (`todo`) should support the following commands and arguments:

1.  **`todo add "<task description>"`**
    *   **Description:** Adds a new task to the list.
    *   **Behavior:** Appends the task as pending and prints a success message including the task's new ID.
    *   **Output Example:** `Added task: "Buy milk" (ID: 1)`

2.  **`todo list`**
    *   **Description:** Lists all tasks, showing their ID, description, status, and creation date.
    *   **Behavior:** Prints a formatted list. Completed tasks are visibly marked with `[x]`; pending with `[ ]`.
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
    *   **Error Case:** If the ID does not exist, print `todo: task not found: <id>` to `stderr` and exit with code 1.

4.  **`todo rm <id>`**
    *   **Description:** Removes a task with the given ID from the list.
    *   **Behavior:** Deletes the task and prints a confirmation message.
    *   **Output Example:** `Task 1 removed.`
    *   **Error Case:** If the ID does not exist, print `todo: task not found: <id>` to `stderr` and exit with code 1.

### 2.2. Persistence
*   Tasks must be stored in a file named `todo.json`.
*   The default path is `./todo.json` in the current working directory. Path resolution precedence (highest first): `--file <path>` flag > `TODO_FILE` env var > `./todo.json`. If `--file`'s parent directory does not exist, exit code 1 with stderr `todo: directory does not exist: <dir>`. An initially absent file is treated as empty (no error) for `add` and `list`; `done`/`rm` on an absent file behave as "ID not found" (exit code 1). Malformed JSON yields exit code 1 with stderr `todo: corrupt file: <path>`.
*   The JSON schema structures each task with:
    *   `id` (integer)
    *   `description` (string)
    *   `completed` (boolean)
    *   `created_at` (ISO 8601 timestamp string, RFC3339 with `Z` suffix — UTC)
    *   `completed_at` (ISO timestamp string or null)

### 2.3. Architecture (DDD + DI)
Directory layout (DDD + DI):
- `cmd/todo/main.go` — composition root; wires injectors; no business logic.
- `internal/task/entity.go` — `Task` struct (ID int, Description string, Completed bool, CreatedAt time.Time, CompletedAt *time.Time).
- `internal/task/repository.go` — `Repository` interface (Add, List, GetByID, MarkCompleted, Delete, Save, Load).
- `internal/storage/jsonrepo.go` — JSON-file implementation of `Repository`; uses an injected `Clock` interface and `FileSystem` interface for testability.
- `internal/cli/` — one file per subcommand (`add.go`, `list.go`, `done.go`, `rm.go`), each exporting `Run(args []string, repo task.Repository, out io.Writer, errOut io.Writer) int`.

No file may exceed 500 lines (AGENTS.md §2.1). All collaborators (Repository, Clock, FS, writers) must be constructor-injected.

### 2.4. ID & Timestamp Semantics
IDs are integers, assigned by the repository as `max(existing IDs) + 1` (or `1` when the store is empty). `rm` does NOT reindex or reuse IDs; the next `add` uses `max(remaining IDs) + 1`. `created_at` is recorded by an injected `Clock` as RFC3339-UTC (Z suffix). `completed_at` is set on first `done`; subsequent `done` calls are idempotent (no-op, exit code 0, same confirmation message, `completed_at` is NOT re-stamped).

### 2.5. List Output Format (byte-level)
- Header row exactly: `ID  Status  Description  Created At\n` (two-space separators).
- Data row: `<id>  [< >|<x>]  <description>  <YYYY-MM-DD HH:MM>\n`, `Created At` formatted in **local time** as `2006-01-02 15:04`. Descriptions printed verbatim (no truncation).
- Empty case: prints `No tasks found.\n` and exits 0.

### 2.6. Exit Codes & Error Message Format
- `done`/`rm` missing id: `todo: task not found: <id>` to stderr, exit 1.
- `done`/`rm`/`add` non-integer `<id>` or missing arg: `todo: missing task id` (or `todo: invalid id: <input>` for non-integer), exit 2.
- `add` with no args: `todo: missing task description` to stderr, exit 2.
- File/dir errors: per §2.2.

---

## 3. Technical Constraints
*   **Language:** Go (module path `github.com/noctifab/todo-cli`, Go 1.22+). This is mandatory to satisfy AGENTS.md tooling (go.mod, Makefile, golangci-lint, BDD).
*   **Dependencies:** Use only the Go standard library for CLI parsing and JSON I/O. A subcommand parser may be implemented by inspecting `os.Args[1]`; do not introduce third-party CLI libraries. Test-only dependencies (e.g. `github.com/cucumber/godog` for BDD) are permitted in `*_test.go` files only.
*   **Storage Format:** Raw JSON file.

---

## 4. Verification Criteria & Testing

### 4.1. Expected Tests
*   **Unit Tests** (under `internal/.../*_test.go`): verify task addition, completion, and deletion functions in isolation with fake repositories; verify correct JSON serialization and deserialization logic with an in-memory `FileSystem` and a fake `Clock` returning a fixed time.
*   **Integration Tests** (under `tests/`): execute CLI commands as subprocesses (built via `go build -o /tmp/todo ./cmd/todo`) in a tempdir and check exit codes and exact stdout/stderr outputs; verify that tasks are successfully read from and written to the `todo.json` file.
*   **BDD acceptance tests** (under `tests/acceptance/`) using godog with `when`/`it` style per AGENTS.md §2.3.

### 4.2. Out of Scope
The AGENTS.md `docker compose` Postgres stack is NOT used by this project (state so explicitly to avoid an agent spinning it up).


## 5. Definition of Done (DoD)

To consider `todo-cli` fully implemented, the Go CLI application must satisfy:
1. **Public Binary:** `todo-cli` executable correctly parses subcommands (`add`, `list`, `complete`, `delete`), persists items to a JSON file store, and returns exit code `0` on success.
2. **Linting Invariant:** Zero warnings under `go vet ./...` and cleanly formatted via `go fmt ./...`.
3. **Verification Criteria:** 100% test pass rate executing `go test ./...`.
