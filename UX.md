# Plan: Interactive Terminal Dashboard & Multi-Story Progress UX

This document details the plan to implement a dynamic, real-time terminal user interface (TUI) for tracking the progress of multiple user stories and their nested tasks, updated every second.

---

## 1. UX Design & Terminal Layout

We propose introducing an interactive command: `noctifab dashboard` (or extending `noctifab status --watch`). This command will render a full-screen, real-time updated view using standard ANSI terminal escape sequences.

### The 4-Part TUI Layout Structure

The terminal screen is divided into 4 logical, visual sections:

```text
+--------------------------------------------------------------------------------+
| PART 1: HEADER PANEL (System state, PID, Cost, Active workers, and Budget)     |
+--------------------------------------------------------------------------------+
|                                                                                |
| PART 2: MAIN PROGRESS BOARD (User stories, Progress bars, and Nested tasks)   |
|                                                                                |
+--------------------------------------------------------------------------------+
| PART 3: LIVE LOG STREAM (Last 4-5 micro-actions and commands executed)        |
+--------------------------------------------------------------------------------+
| PART 4: KEYBOARD STATUS BAR (Interactive hotkeys, status alerts, refresh time) |
+--------------------------------------------------------------------------------+
```

### Complete TUI Layout Specification

```text
[PART 1]------------------------------------------------------------------------
noctifab Dark Factory Dashboard (PID: 12345) | Active Agents: 3 | Cost: $0.1245
Build: PASSING | Budget Limit: $10.00 | Total Tokens: 42,350
================================================================================
[PART 2]------------------------------------------------------------------------
📋 User Story: US-0001-auth-middleware.md [RUNNING]
Progress: [████████████████░░░░░░░░░░] 60%
  ├── [SUCCESS]     100% - Setup database connections (Agent: generator-1)
  ├── ⠋ [IN_PROGRESS]  30% - Implement JWT authentication handler (Agent: generator-2) [1m 15s]
  ├── [PENDING]       0% - Configure CORS and secure cookies (Agent: -)
  └── [PENDING]       0% - Write unit tests for login endpoints (Agent: -)

📋 User Story: US-0002-user-profile.md [PENDING]
Progress: [░░░░░░░░░░░░░░░░░░░░░░░░░░] 0%
  ├── [PENDING]       0% - Create profiles database schema (Agent: -)
  └── [PENDING]       0% - Build GET/POST profile endpoints (Agent: -)

[PART 3]------------------------------------------------------------------------
--------------------------------- Live Logs ------------------------------------
[11:45:10] Agent [generator-1] started refactoring: pkg/services/listener.go
[11:45:15] Executing command: go test -run TestListenerAgent_StatusWithTasks ./pkg/...
[11:45:18] Validation: Test run passed.
[11:45:20] Agent [tester-1] checking for additional edge-case files.
[PART 4]------------------------------------------------------------------------
--------------------------------------------------------------------------------
[q] Quit | [p] Pause/Resume | [x] Cancel | [c] Clarify | [o] Override | Last updated: 12:00:00 (1s)
```

### Visual Styling & Colors
Using ANSI escape codes:
*   **Story Headers (Part 2):** Bold, light blue (`\033[1;36m`) to stand out.
*   **Progress Bars (Part 2):** Filled sections using green block characters (`█`), empty sections using grey light blocks (`░`).
*   **Task Statuses (Part 2):**
    *   `[SUCCESS]`: Bright Green (`\033[32m`)
    *   `[IN_PROGRESS]`: Bright Yellow (`\033[33m`)
    *   `[PENDING]`: Dim/Grey (`\033[90m`)
    *   `[FAILED]`: Bright Red (`\033[31m`)
*   **Log Feed (Part 3):** Dim/Grey text (`\033[90m`) to maintain focus on the main panel.
*   **Status Bar (Part 4):** Reverse colors (black text on white/grey background) for a premium, high-visibility status bar look.

---

## 2. Core Architecture & Data Model Updates

To support displaying multiple stories, task progress percentages, and assigning active agents, we plan to extend the domain layers and database adapters.

### 2.1. Task Progress Tracking
We will extend the `domain.Task` struct in [pkg/domain/task.go](file:///Users/diegoj/repos/noctifab/pkg/domain/task.go):
*   Add a `Progress` integer field (`json:"progress"`) representing the percentage of completion (0 to 100).
*   Add a matching `progress` column in the `tasks` schema database table.

**Milestone-based progress calculation in [pkg/services/orchestrator_execute.go](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator_execute.go):**
*   **Pending / Ready:** 0%
*   **Minimal Implementation Phase starting:** 10%
*   **Minimal Implementation complete / Test writing starting:** 40%
*   **Refactor/Improve phase starting:** 70%
*   **Final validation tests running:** 90%
*   **Success / Merged:** 100%

### 2.2. Tracking Active Agents
*   Currently, `state.ActiveAgents` is populated in tests but is not actively maintained in production execution.
*   We will modify the orchestrator pipeline in [pkg/services/orchestrator_helper.go](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator_helper.go) inside `RunGeneratorAgent` and `RunTesterAgent` to register or update agent statuses (`domain.Agent`) in the state.
*   Specifically, when an agent begins execution, we update its status to `WORKING` and assign it the corresponding `TaskID`. When it completes or fails, we update its status to `IDLE` or `COMPLETED`.

### 2.3. Loading Multiple Stories
*   We will extend the [domain.StateRepository](file:///Users/diegoj/repos/noctifab/pkg/domain/state_repository.go) interface:
    ```go
    type StateRepository interface {
        Load(ctx context.Context) (*State, error)
        LoadByID(ctx context.Context, id string) (*State, error)
        LoadAll(ctx context.Context) ([]*State, error)
        Save(ctx context.Context, state *State) error
    }
    ```
*   We will update `SQLiteRepository` in [sqlite_repository.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/storage/sqlite_repository.go) and `PostgresRepository` in [postgres_repository.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/storage/postgres_repository.go) to implement `LoadAll` and `LoadByID`, correctly loading the states along with their joined `tasks` and `active_agents`.

---

## 3. Daemon API & Subcommand Updates

### 3.1. API Endpoints
We will update the daemon server in [pkg/services/command_channel.go](file:///Users/diegoj/repos/noctifab/pkg/services/command_channel.go):
*   Add endpoint `GET /api/v1/status` (or update `/statusz` if backward compatibility is not required) to load all states using `repo.LoadAll(ctx)` and return them as a JSON list.

### 3.2. CLI Subcommand: `noctifab dashboard`
We will add a new CLI command file [cmd/noctifab/cli/dashboard.go](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/dashboard.go):
*   Register the `dashboard` command.
*   Implement a loop that polls `GET /api/v1/status` every 1 second.
*   Use standard ANSI codes (like `\033[H\033[2J` to home the cursor and clear) to cleanly repaint the terminal screen without flickering.
*   Listen to terminal key inputs (e.g. `q` to quit) to gracefully exit.

---

## 4. Wait Mitigation & Interactive Diagnostics

To ease user anxiety during long-running tasks, we propose adding the following elements to the terminal interface:

### 4.1. Live Activity & Event Logging Feed
At the bottom of the dashboard, render a small split pane (last 4-5 lines of logs) showing real-time agent operations:
```text
--------------------------------- Live Logs ----------------------------------
[11:45:10] Agent [generator-1] started refactoring: pkg/services/listener.go
[11:45:15] Executing command: go test -run TestListenerAgent_StatusWithTasks ./pkg/...
[11:45:18] Validation: Test run passed.
[11:45:20] Agent [tester-1] checking for additional edge-case files.
```
This shows the user the exact sub-step currently being worked on, proving the system is actively processing.

### 4.2. Elapsed Task Timers
For any task in `IN_PROGRESS` state, show a live elapsed time counter:
```text
  ├── ⠋ [IN_PROGRESS]  30% - Implement JWT handler (Agent: generator-2) [Elapsed: 1m 12s]
```
This indicates if a task is taking longer than expected or if a step is close to timing out, removing the "black box" feel.

### 4.3. Aesthetic Loading Spinners
Incorporate a classic rotating terminal spinner (`⠋`, `⠙`, `⠹`, `⠸`, `⠼`, `⠴`, `⠦`, `⠧`, `⠇`, `⠏`) next to the active task and the user story currently running, providing high-fidelity visual feedback of activity.

### 4.4. Budget and Token Consumption Metrics
Display a live resource indicator in the top header:
```text
Cost: $0.1245 / $10.00 Limit (Budget) | Tokens: 42,350 total
```
This alleviates the developer's worry about unexpected costs, showing them exactly what the current session has spent.

### 4.5. Inline Interactive Keyboard Shortcuts
We can support keyboard controls in the terminal:
*   `[c] Answer Clarification`: If an agent raises a question, show a flashing banner: `⚠️  Clarification needed! Press [c] to reply.` Pressing `c` pauses the loop and opens an inline text entry area.
*   `[p] Pause/Resume`: Toggles the execution state of the background orchestrator. Pressing `p` displays a confirmation banner: `⚠️  Are you sure you want to pause execution? (y/n)` (or `⚠️  Are you sure you want to resume execution? (y/n)` if already paused). If confirmed with `y`, it updates the state. When paused, the header status updates to `[PAUSED]`, spinners are frozen, and no further agent actions are scheduled.
*   `[x] Cancel / Abort`: Interrupts the active user story. Pressing `x` displays a confirmation banner: `⚠️  Are you sure you want to cancel the active execution? (y/n)`. If confirmed with `y`, it immediately signals the daemon to abort the running story, revert any uncommitted changes on task branches, release all acquired file locks, and set the story status in the database to `CANCELLED`.
*   `[o] Force Override`: Allow the user to select an active task and override validation checks or merge conflicts directly.

---

## 5. Technical Implementation Details & Verification

### 5.1. Flicker-Free Screen Repainting (Double-Buffering)
To avoid high-frequency terminal screen flickering (which is common when clearing the screen via `clear` commands every second):
*   Build the entire screen frame content in-memory as a single string buffer.
*   Use the cursor home ANSI escape sequence (`\033[H`) to move the terminal cursor to the top-left coordinate `(0,0)` rather than performing a destructive clear (`\033[2J`).
*   Perform a single write to standard output to overwrite the screen cleanly.

### 5.2. Graceful TTY Detection & Fallback
*   The dashboard command will check if standard output is a TTY terminal using the standard library: `golang.org/x/term.IsTerminal(int(os.Stdout.Fd()))`.
*   If standard output is redirected to a file, piped to another command, or run in a headless CI system (e.g. GitHub Actions, GitLab CI, or Docker logs), the command will automatically fall back to **non-interactive log logging mode**. 
*   In non-interactive mode, it prints a single-line text summary of user stories and overall task statuses every 30 seconds rather than drawing ANSI blocks and progress bars.

### 5.3. Verification Plan
*   **Unit/Component Tests:** Write tests in `cmd/noctifab/cli/dashboard_test.go` that pass dummy story states to the layout renderer and verify that progress calculations and output formats match expected regular expressions (without rendering to stdout).
*   **Integration Tests:** Set up a CLI test verifying that the `dashboard` command exits with correct codes when the daemon is offline.

### 5.4. Docker Container Execution Support
Yes, the interface is fully compatible with running inside Docker containers, subject to the execution context:
1. **Interactive Mode (using `-it` or `-t` flags):**
   * If the user runs the container interactively (e.g. `docker run -it --rm noctifab dashboard`), Docker allocates a pseudo-TTY and maps standard input. 
   * `golang.org/x/term.IsTerminal` will successfully return `true` inside the container. 
   * ANSI escape sequences, spinners, color codes, and live terminal resizing (via propagating `SIGWINCH` signals) will render exactly as they would on the host machine. Key bindings (`q`, `c`, `p`, `o`) will be fully functional.
2. **Headless/CI Mode (no TTY allocated):**
   * If the container runs as a detached daemon (`docker run -d`), via a compose setup without terminal allocation, or inside automated CI runner actions, `IsTerminal` returns `false`.
   * The fallback mechanism described in **Section 5.2** takes over automatically, preventing binary-dumped escape sequences from corrupting the docker container logs.
3. **Daemon Networking Configuration:**
   * When running the dashboard in a separate container from the background `noctifab serve` daemon, the CLI command must support a custom connection endpoint using a `--daemon-url` flag (defaulting to `http://127.0.0.1:18080` for single-container setups).

---

## 6. Codebase Implementation Checklist (for Future LLM / Agents)

To implement this dashboard interface, make the following precise changes to the codebase:

### 6.1. Domain Models & Schema Migrations
1.  **Modify [pkg/domain/state.go](file:///Users/diegoj/repos/noctifab/pkg/domain/state.go):**
    *   Add new states to the `StoryStatus` constants:
        ```go
        StoryPaused    StoryStatus = "PAUSED"
        StoryCancelled StoryStatus = "CANCELLED"
        ```
2.  **Modify [pkg/domain/task.go](file:///Users/diegoj/repos/noctifab/pkg/domain/task.go):**
    *   Add a `Progress` int field to the `Task` struct:
        ```go
        Progress int `json:"progress"` // Completion percentage (0 to 100)
        ```
3.  **Modify [pkg/domain/state_repository.go](file:///Users/diegoj/repos/noctifab/pkg/domain/state_repository.go):**
    *   Declare two new methods on the `StateRepository` interface:
        ```go
        LoadByID(ctx context.Context, id string) (*State, error)
        LoadAll(ctx context.Context) ([]*State, error)
        ```
3.  **Add Schema Migrations:**
    *   Create `pkg/infrastructure/storage/migrations/sqlite/0005_add_task_progress.sql`:
        ```sql
        ALTER TABLE tasks ADD COLUMN progress INTEGER NOT NULL DEFAULT 0;
        ```
    *   Create `pkg/infrastructure/storage/migrations/postgres/0005_add_task_progress.sql`:
        ```sql
        ALTER TABLE tasks ADD COLUMN progress INTEGER NOT NULL DEFAULT 0;
        ```

### 6.2. Infrastructure / Storage Layer
1.  **Modify [pkg/infrastructure/storage/sqlite_repository.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/storage/sqlite_repository.go):**
    *   Update `Save()` to write the new `progress` column to the `tasks` table.
    *   Update `Load()` to query only the *latest active story* (instead of an arbitrary `LIMIT 1` row), using `ORDER BY updated_at DESC` or `WHERE story_status = 'RUNNING'`.
    *   Implement `LoadByID(ctx, id)` querying states and nested models where `state_id = ?`.
    *   Implement `LoadAll(ctx)` to load all saved states and return them, resolving tasks and active agents for each state.
2.  **Modify [pkg/infrastructure/storage/postgres_repository.go](file:///Users/diegoj/repos/noctifab/pkg/infrastructure/storage/postgres_repository.go):**
    *   Make equivalent updates to `Save()`, `Load()`, `LoadByID()`, and `LoadAll()` for PostgreSQL.

### 6.3. Orchestrator Loop & Service Updates
1.  **Modify [pkg/services/orchestrator_execute.go](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator_execute.go):**
    *   In `executeTask()`, update `task.Progress` and save state at key checkpoints:
        *   At minimal implementation startup: `task.Progress = 25`
        *   At test writing startup: `task.Progress = 50`
        *   At refactoring / bug fix iterations: `task.Progress = 75`
        *   At final task success: `task.Progress = 100`
2.  **Modify [pkg/services/orchestrator_helper.go](file:///Users/diegoj/repos/noctifab/pkg/services/orchestrator_helper.go):**
    *   In `RunTesterAgent()` and `RunGeneratorAgent()`, register or update the corresponding agent inside `state.ActiveAgents` before calling LLM, setting status to `WORKING` and `TaskID` to `task.ID`.
    *   Upon completion (or failure), update the agent's status to `IDLE` or `COMPLETED` and update `CompletedAt = time.Now()`.
3.  **Modify [cmd/noctifab/cli/serve.go](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/serve.go):**
    *   In `runServerLoop()`, load the state from the repository on each ticker tick.
    *   If `state.StoryStatus == domain.StoryPaused`, skip calling `orchestrator.RunOnce(ctx)` for that cycle.
    *   If `state.StoryStatus == domain.StoryCancelled`, trigger the graceful cancellation flow (marks all running tasks as `INTERRUPTED`, cleans up working branches/locks) and exit `processStory` immediately.
4.  **Modify [pkg/services/command_channel.go](file:///Users/diegoj/repos/noctifab/pkg/services/command_channel.go):**
    *   Register `GET /api/v1/status` to return all enqueued/active story states:
        ```go
        mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
            states, err := repo.LoadAll(r.Context())
            if err != nil { ... }
            _ = json.NewEncoder(w).Encode(states)
        })
        ```
    *   Register `POST /api/v1/pause`, `POST /api/v1/resume`, and `POST /api/v1/cancel` to update the active story's status column:
        ```go
        // Example for Pause route:
        state.StoryStatus = domain.StoryPaused
        _ = repo.Save(r.Context(), state)
        ```
5.  **Modify [pkg/services/daemon_client.go](file:///Users/diegoj/repos/noctifab/pkg/services/daemon_client.go):**
    *   Add `GetStatusAll(ctx) ([]*domain.State, error)` querying `GET /api/v1/status`.
    *   Add `PauseStory()`, `ResumeStory()`, and `CancelStory()` posting to their respective endpoints.

### 6.4. Command Line Interface (CLI)
1.  **Create [cmd/noctifab/cli/dashboard.go](file:///Users/diegoj/repos/noctifab/cmd/noctifab/cli/dashboard.go):**
    *   Define the `dashboard` command.
    *   Initialize raw terminal mode on standard input using `golang.org/x/term.MakeRaw(int(os.Stdin.Fd()))`.
    *   Spawn a keyboard-listener goroutine to handle user input:
        *   `q` -> Gracefully exit raw terminal mode and exit the CLI.
        *   `p` -> Render confirmation prompt banner (`⚠️  Are you sure you want to pause/resume execution? (y/n)`). If the user confirms with `y`, fetch current status; if active, trigger `PauseStory()`, else if paused, trigger `ResumeStory()`.
        *   `x` -> Render confirmation prompt banner (`⚠️  Are you sure you want to cancel the active execution? (y/n)`). If the user confirms with `y`, call `CancelStory()`.
    *   Run a `time.Ticker` every 1 second that fetches states via `GetStatusAll()` and redraws the TUI frame utilizing double-buffering and cursor reset coordinates `\033[H`.





