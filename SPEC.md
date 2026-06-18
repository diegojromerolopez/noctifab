# SPEC.md: noctifab Project Specification

## 1. Executive Summary

`noctifab` is a Dark Factory Platform for GitHub, GitLab, and Bitbucket. A "Dark Factory" (in software engineering context) is an autonomous, long-running agentic harness that operates without human intervention to resolve issues, verify builds, run tests, and manage software project lifecycles.

`noctifab` is compiled as a single Go binary that functions as a Command Line Interface (CLI) tool. It runs as a single-node autonomous loop engine, replacing the manual developer execution bottleneck.

### 1.1. Autonomy Level Matrix

The platform classifies development automation into five distinct levels:

| Level | Name | Platform Behavior |
|---|---|---|
| **Level 1** | Autocomplete | AI suggests code inline. Human drives the editor and makes all decisions. |
| **Level 2** | Interactive Assistant | AI generates entire files/functions. Human reviews every single change in the editor. |
| **Level 3** | Spec-Driven (Gated) | AI generates code autonomously from specifications. Holdout scenarios gate quality. Human clicks merge. |
| **Level 3.5** | Selective Auto-Merge | Same as Level 3, but low-risk modules merge automatically. Human can block. |
| **Level 4** | Full Dark Factory | Specs go in, tested code comes out fully merged. Human reviews only exceptions. |

`noctifab` is designed to run at **Level 3** and **Level 4** autonomy.

---

## 2. Core Architecture & Design Principles

To ensure maintainability, scalability, and clarity for both human developers and future AI coding agents, `noctifab` follows these strict architectural guidelines:

*   **Language:** Go (Golang).
*   **Dependency Injection (DI):** All components must receive their dependencies explicitly via constructors. Global state or hardcoded dependencies are strictly prohibited.
*   **SOLID Principles:** High cohesion and loose coupling. Abstractions (interfaces) must separate the system's logic from external API providers and storage.
*   **Domain-Driven Design (DDD):** Organize packages by domain boundaries rather than by technical layers (e.g., separate domain models/entities, use cases/services, and adapters/infrastructure).
*   **Code Length Constraint:** No Go source code file may exceed **500 lines** of code (defined strictly as **500 physical lines** including comments, blank lines, and imports to facilitate fast lint validation e.g. using simple line count checks). This ensures components remain focused, modular, and easy for LLMs to read and modify without losing context.
*   **Test-Driven Development Layout:** Every package must include unit tests alongside the files they test (e.g., `validator.go` and `validator_test.go`).
*   **Instructive Linter Mandate:** Static analysis checks used in the development loop must print *instructive actions* instead of mere descriptions. For example, instead of *"Service depends on Controller"*, the output must read *"Service class imports from Controller package. Move the shared types to domain/model package instead"*. This drastically increases the agent's first-try correction rate.
*   **Developer Documentation Guidelines:** A `.readthedocs.yaml` configuration file must be present at the root, and a `docs/` folder must contain Markdown documentation detailing project usage, setup instructions, and guides on how developers can extend the orchestration framework.
*   **Repository Standards and Documentation:** The repository must maintain a `VERSION` file at the root containing the current semver release, a `CHANGELOG.md` detailing change history conforming to the "Keep a Changelog" standard, and a comprehensive `README.md` at the root. The `README.md` must include project status badges, links to the Read the Docs page, a description of the project, basic CLI usage instructions, license information, and collaboration guidelines.
*   **Industry Coding Standards:** When modifying or writing code, AI agents must strictly follow the most popular and established standards of the target language and platform (e.g. Go Code Review Comments for Go, standard libraries, and standard formatting conventions), unless explicitly instructed otherwise.

### 2.1. Directory Layout & Go Package Structure

The repository must follow a standardized layout aligning with Go best practices and DDD packaging:

```
noctifab/
├── .github/                   # [Required, To Be Implemented]
│   └── workflows/
│       └── ci.yml             # CI Workflow for linting and unit tests
├── docs/                      # Markdown documentation for developers (usage, extension guides)
├── cmd/
│   └── noctifab/
│       └── main.go            # Entrypoint and CLI subcommand setup
├── pkg/
│   ├── domain/                # Enterprise & domain entities (100% pure Go, no external imports)
│   │   ├── state.go           # State structures & StateRepository interface
│   │   ├── task.go            # Task entities and behaviors
│   │   └── action.go          # Action execution logs
│   ├── usecase/               # Orchestration, main loop, rules validation
│   │   ├── orchestrator.go    # Daemonized polling loop
│   │   ├── registry.go        # Tool Registry implementation
│   │   └── validator.go       # Policy & safety rules checker
│   └── infrastructure/        # Frameworks, drivers, and external adapters
│       ├── llm/               # LLM clients (openai, anthropic, gemini, ollama)
│       ├── storage/           # State persistence (JSON file, SQL)
│       ├── vcs/               # Git & APIs (GitHub, GitLab, Bitbucket adapters)
│       └── jira/              # Jira API Client (authentication & issue fetching)
├── .readthedocs.yaml          # Read the Docs configuration file
├── lint.Dockerfile            # Static analysis container
├── CHANGELOG.md               # [Required] Project changelog following Keep a Changelog
├── LICENSE
├── README.md                  # [Required] Project README with badges, docs links, CLI usage, and collaboration guide
└── VERSION                    # [Required] Project version file (semver)
```


---

## 3. Architecture & Components

`noctifab` consists of five primary core components coordinated by an orchestrator/main loop:

```
                 ┌──────────┐
                 │  State   │
                 └────┬─────┘
                      │
                      ▼
               ┌──────────┐
               │ Prompt   │
               └────┬─────┘
                      │
                      ▼
               ┌──────────┐
               │   LLM    │
               └────┬─────┘
                      │
                      ▼
               ┌──────────┐
               │Validator │
               └────┬─────┘
                      │
                      ▼
               ┌──────────┐
               │ Tool Reg │
               └────┬─────┘
                      │
                      ▼
               ┌──────────┐
               │ Execute  │
               └────┬─────┘
                      │
                      ▼
                 Update
                  State
```

### 3.1. State Struct (The World Model)
The State acts as the single source of truth for the entire system. The LLM is designed to be completely stateless; it has no memory of past runs, actions, or details other than what is explicitly contained inside the `State` provided during the loop cycle.

#### Struct Definition (`pkg/domain/state.go`)
Domain models are split across distinct files to remain clean and adhere to the 500-physical-line file constraint.

**State Model (`pkg/domain/state.go`):**
```go
package domain

import "time"

type ValidationType string

const (
	ValidationCommand     ValidationType = "COMMAND"      // Run verify command (e.g. go test)
	ValidationFileExists  ValidationType = "FILE_EXISTS"  // Check if file exists in workspace
	ValidationFileContent ValidationType = "FILE_CONTENT" // Regex match on file contents
)

type ValidationCriterion struct {
	ID          string         `json:"id"`
	Type        ValidationType `json:"type"`
	Expression  string         `json:"expression"` // Command line, filepath, or regex target
	Description string         `json:"description"`
	Passed      bool           `json:"passed"`
	ErrorLog    string         `json:"error_log,omitempty"`
}

type Clarification struct {
	Question string    `json:"question"`
	Answer   string    `json:"answer,omitempty"`
	Resolved bool      `json:"resolved"`
	AskedAt  time.Time `json:"asked_at"`
}

type Agent struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Role   string   `json:"role"`
	Status string   `json:"status"` // "IDLE", "WORKING", "COMPLETED"
}

type FileInfo struct {
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

type State struct {
	Version            int                   `json:"version"`
	Clarifications     []Clarification       `json:"clarifications,omitempty"`
	ValidationCriteria []ValidationCriterion `json:"validation_criteria,omitempty"`
	Tasks              []Task                `json:"tasks"`
	ActiveAgents       []Agent               `json:"active_agents"`
	Files              []FileInfo            `json:"files"`
	BuildStatus        string                `json:"build_status"`
	LastActions        []Action              `json:"last_actions"`
	Metadata           map[string]any        `json:"metadata"`
}
```

**Task Model (`pkg/domain/task.go`):**
```go
package domain

import "time"

type TaskStatus string

const (
	TaskPending    TaskStatus = "PENDING"
	TaskInProgress TaskStatus = "IN_PROGRESS"
	TaskSuccess    TaskStatus = "SUCCESS"
	TaskFailed     TaskStatus = "FAILED"
)

type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	AssignedTo  string     `json:"assigned_to"`  // Agent ID
	DependsOn   []string   `json:"depends_on"`    // IDs of parent tasks
	Retries     int        `json:"retries"`       // Number of times executed and failed
	MaxRetries  int        `json:"max_retries"`   // Upper retry limit before task is marked TaskFailed
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}
```

**Action Model (`pkg/domain/action.go`):**
```go
package domain

import "time"

type Action struct {
	Timestamp time.Time      `json:"timestamp"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args"`
	Reasoning string         `json:"reasoning"`
	Result    string         `json:"result"`
	Success   bool           `json:"success"`
}
```

#### State Storage Interface (`pkg/domain/state_repository.go`)
```go
package domain

import "context"

type StateRepository interface {
	Load(ctx context.Context) (*State, error)
	Save(ctx context.Context, state *State) error
}
```

#### 3.1.1. Storage Provider Implementations
The orchestrator supports database-backed state persistence configured via CLI flags:

1.  **SQLite Database Provider (`pkg/infrastructure/storage/sqlite_repository.go`):**
    *   **Behavior:** Persists state to a local SQLite database file, storing the system state as a serialized record in a table.
    *   **Concurrent Access:** Uses SQLite Write-Ahead Logging (WAL) mode combined with short-lived database transactions, enabling concurrent reader connections and safe writer synchronization without blocking.

2.  **PostgreSQL Database Provider (`pkg/infrastructure/storage/postgres_repository.go`):**
    *   **Behavior:** Connects to a remote or local PostgreSQL database instance, storing the state in a table using PostgreSQL `JSONB` for optimal performance.
    *   **Concurrent Access:** Relies on PostgreSQL transaction isolation levels and row-level locking (e.g. `SELECT FOR UPDATE` during write-verification) to handle concurrent updates cleanly.

Database transactions are short-lived. A connection handle is never held open during slow external network calls (such as LLM API completions) or execution runs.

#### 3.1.2. Workspace File System Metadata Sync & Prompt Optimization
To ensure that the orchestrator has an accurate representation of the sandbox filesystem, `state.Files` is updated dynamically:
*   **Deterministic Scanning:** At the start of each execution loop cycle (Observe phase), the orchestrator automatically walks the local sandbox repository directory.
*   **FileInfo Mapping:** It filters out VCS ignore directories (such as `.git/`), resolves the paths, reads their file sizes and modification timestamps, and constructs the list of `FileInfo` structs.
*   **Prompt Optimization:** To prevent context token bloat, the complete list of filesystem files (`FileInfo`) is NOT injected in full into the LLM system prompt. Instead, the orchestrator only includes a high-level summary of the workspace filesystem (or modified files) in the prompt, and the agent uses dynamic filesystem query tools (`list_directory`, `find_files`, `grep_search`) to query the environment as needed.
*   **Transaction Update:** The updated `FileInfo` slice is saved to the state database inside a short-lived transaction prior to LLM completion execution.

---

### 3.2. Tool Registry
The Tool Registry defines the actions available to the agent. It dynamically registers tools and routes execute calls to the correct implementation.

#### Interfaces (`pkg/usecase/registry.go`)
```go
package usecase

import (
	"context"

	"github.com/noctifab/pkg/domain"
)

type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error)
}

type Registry interface {
	Register(t Tool)
	Get(name string) (Tool, bool)
	List() []Tool
}

type ToolRegistry struct {
	tools map[string]Tool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

func (r *ToolRegistry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *ToolRegistry) Get(name string) (Tool, bool) {
	t, exists := r.tools[name]
	return t, exists
}

func (r *ToolRegistry) List() []Tool {
	list := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		list = append(list, t)
	}
	return list
}
```

#### Standard Tools List

##### A. Bootstrap Tools
1.  **`add_task`:** Arguments: `title` (string), `description` (string), `depends_on` ([]string), `max_retries` (int). Returns verification message.
2.  **`complete_task`:** Arguments: `id` (string). Updates task status in state to `SUCCESS`.
3.  **`log_message`:** Arguments: `message` (string). Appends message string to the execution state trace.
4.  **`noop`:** Arguments: none. No action, returns success.

##### B. Production Tools
1.  **`read_file`:** Arguments: `path` (string). Returns file content.
2.  **`write_file`:** Arguments: `path` (string), `content` (string). Creates or replaces file.
3.  **`list_directory`:** Arguments: `path` (string). Returns listing of files and directories.
4.  **`find_files`:** Arguments: `pattern` (string). Returns paths of files matching regex/glob patterns.
5.  **`grep_search`:** Arguments: `query` (string), `path` (string). Returns line matches of a substring search.
6.  **`run_tests`:** Arguments: `package` (string). Runs local go test suite and returns execution console output.
7.  **`git_action`:** Arguments: `action` (string e.g. "clone", "checkout", "commit", "push", "pull_request"), `params` (map[string]any). Returns stdout.
8.  **`docker_action`:** Arguments: `command` (string). Executes command in container sandbox.

---

### 3.3. LLM Client
The LLM Client translates the current `State` into a structured prompt, interacts with a configured language model provider, and parses the structured output.

#### Client Interface (`pkg/infrastructure/llm/client.go`)
```go
package llm

import "context"

type LLMResponse struct {
	Reasoning string         `json:"reasoning"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args"`
}

type Client interface {
	Complete(ctx context.Context, prompt string) (*LLMResponse, error)
}
```

#### Prompt Design & Injection Templates

##### A. System Prompt
```
You are a software factory automation agent.
You must respond ONLY in valid JSON. No free text before or after the JSON block.

You may only use the following tools:
{JSON LIST OF REGISTERED TOOLS & DESCRIPTIONS}

Return format:
{
  "reasoning": "explanation of the decision",
  "tool": "tool_name",
  "args": {
     "arg_name": "value"
  }
}
```

##### B. State Injection Prompt
```
Current state representation:
{JSON STATE CONFIG}

What is the next best action?
```

---

### 3.4. Validator & Holdout Scenario Quality Gates

The Validator serves as the safety policy enforcement layer and determines goal accomplishment. It implements a strict split between the code generation execution context and the acceptance validation checks.

#### Interface (`pkg/usecase/validator.go`)
```go
package usecase

import (
	"context"

	"github.com/noctifab/pkg/domain"
)

type Validator interface {
	// Guardrails check for individual agent actions
	Validate(ctx context.Context, action domain.Action, state *domain.State) error

	// Verification check for overall feature goal completion
	EvaluateGoals(ctx context.Context, state *domain.State) (bool, error)
}
```

#### Holdout Scenarios Architecture (Strict Quality Gates)
To prevent agents from gaming tests or writing overfitted implementations, `noctifab` implements the **Holdout Scenarios Pattern** (equivalent to ML train/test data split):

```
┌─────────────────────────────────┐
│     Code Gen Agent Context      │
└────────────────┬────────────────┘
                 │ (Can read)
                 ▼
          [Feature Spec]
                 │
                 ▼ (Builds)
         [Feature Branch]
                 │
                 ▼ (Evaluated by)
┌─────────────────────────────────┐
│     Scenario Evaluator Agent    │◄─── [Holdout Scenarios (BDD)]
└─────────────────────────────────┘     (Completely hidden from Gen Agent)
```

1.  **Isolation:** Acceptance tests (Holdout Scenarios) are written in plain-English BDD format and stored in a secure folder (e.g. `tests/holdout/`). The code generation agent has **zero access** to read or inspect this directory.
2.  **Test Runner & BDD Context Framework:** Holdout scenarios must always be executed using a structured BDD test runner rather than dynamic code translation. The scenarios must follow the context structure: `"when <scenario>", "it <action happens>"`.
3.  **Deterministic Test Runs:** All holdout scenarios are executed deterministically using standard CLI execution commands (e.g., `go test`). The testing suite expects a 100% success rate without probabilistic assertions or flakiness.
4.  **Failure Feedback Filter:** When a holdout scenario fails, the generator agent **never** receives the scenario details (the actual test code or BDD text). Instead, the Evaluator Agent returns a sanitized stderr/stdout execution log output of the failing integration test run (showing what assertion failed, e.g., `"Holdout failed: SQL Validation Endpoint returned 500"`), providing sufficient context for programmatic debugging without leaking test assets.
5.  **Merge Gate:** 100% of all holdout scenarios must pass before the Validator approves a pull request for merge.

#### Static Policy Safeguards (Default Rules)
In addition to dynamic validation, the Validator blocks actions violating:
1.  **VCS Branch Protection:** Direct push to protected branches (e.g. `main`) is blocked.
2.  **Path Traversal Protection:** Reading/writing files outside the workspace root is blocked.
3.  **Command Execution Whitelist:** Only running commands matching a strict whitelist is allowed.

#### 3.4.3. Harness Sandbox Boundaries (FS Jail, Prefix Limiting & Folder Blacklisting)
To guarantee safe operation and prevent irreversible actions (such as unauthorized commands or data deletion), the Go engine executes all tools inside a restricted agent harness sandbox:
*   **File System Jail & Prefix Limiting:** The directory paths passed to any tool or whitelisted command (like `read_file` or `write_file`) must be resolved to their absolute canonical form, cleaned (using Go's `filepath.Clean`), and verified to be strictly prefixed by the configured workspace directory prefix (e.g., `$HOME/repos/my-repo` or dynamic `cwd` root). Any attempt to read, write, or target files outside this workspace prefix triggers a sandbox validation error and blocks execution. This host-level prefix boundary check provides lightweight security isolation, avoiding the overhead of heavy Docker container isolation.
*   **Configuration Directory Blacklisting:** To prevent security jailbreaks, the File System Jail explicitly blacklists and blocks any operations (read, write, or command execution target) targeting the configuration folder (e.g., `.noctifab/` or `.noctifact/`), even though it resides within the workspace root. This ensures that sandboxed tests or code modifications cannot read, modify, or corrupt the local database file (`noctifab.db`) or access system keys.
*   **Tool & Command Whitelisting:** The execution of external shell commands is restricted. The runner only executes a predefined list of safe utility binaries (e.g., `go`, `git`). Arbitrary command strings, docker execution commands, or unverified scripts are rejected before execution to keep host sandboxing secure.
*   **Resource & Time Quotas:** To prevent runaway loops or denial of service, every executed command is wrapped in a Go `context.WithTimeout` (defaulting to 5 minutes max). Memory and process quotas are enforced, and API call counters track LLM credit consumption, halting the loop once daily budgets are exceeded.

---

### 3.5. Multi-Agent Concurrency & Dependency Orchestrator

`noctifab` supports concurrent execution of multiple autonomous agents mapping to a Directed Acyclic Graph (DAG) of tasks compiled from an input specification, with lifecycle termination controlled by the validation criteria.

#### Orchestrator Loop Implementation (`pkg/usecase/orchestrator.go`)
```go
package usecase

import (
	"context"
	"time"

	"github.com/noctifab/pkg/domain"
	"github.com/noctifab/pkg/infrastructure/llm"
)

type Orchestrator struct {
	repo          domain.StateRepository
	registry      Registry
	llmClient     llm.Client
	validator     Validator
	pollInterval  time.Duration
	maxRetries    int // Maximum outer LLM response retries per cycle
}

func NewOrchestrator(
	repo domain.StateRepository,
	reg Registry,
	client llm.Client,
	val Validator,
	interval time.Duration,
) *Orchestrator {
	return &Orchestrator{
		repo:         repo,
		registry:     reg,
		llmClient:    client,
		validator:    val,
		pollInterval: interval,
		maxRetries:   3,
	}
}
```

#### 3.5.1. Planner-Generator-Evaluator Loop & Agentic Roles
`noctifab` utilizes a structured loop that partitions agent cognitive tasks into three distinct roles, preventing "evaluation gaming" (where a generator reviews its own code):
1.  **Planner Agent:** Receives high-level user specifications, resolves initial ambiguities via the Clarification Loop, constructs the task DAG (Directed Acyclic Graph), and populates the state database.
2.  **Generator Agent:** Spawns task-specific sandboxed workers. These workers consume tasks from the DAG queue, check out isolated branches, modify code files, and run local unit tests until compilation passes.
3.  **Evaluator Agent (Isolated):** Operates on the final task branch. It reads BDD scenarios from a secure, hidden directory (e.g. `tests/holdout/`) which coding agents cannot read. It executes automated integration/acceptance tests and evaluates overall goal completion.
The generator agent is only given high-level feedback (e.g., error logs and test failure summaries), never the source test cases themselves, to guarantee code generalization.

#### 3.5.2. Hybrid Execution Model: Agentic vs. Deterministic Nodes
To optimize execution speed and cost, the orchestrator divides the execution loop into agentic nodes (which require LLM reasoning) and deterministic nodes (which run programmatically in Go):

| Node Type | Execution Mode | Example Operations |
|---|---|---|
| **Agentic** | LLM-driven | Task planning, coding implementation, diagnostic error analysis, clarification questions. |
| **Deterministic** | Local Go Runner | Running tests/linters, code formatting (`go fmt`), compiling/building, branching, git commits/merges. |

By offloading formatting, compilation checks, and merge logic to deterministic Go code, the system minimizes LLM token consumption and increases execution robustness.

#### 3.5.3. DB-backed State Coordination & Optimistic Concurrency Engine
The orchestrator itself runs as a single coordinator program, but operates in a multi-agent environment where multiple worker threads or subprocesses (agents) execute tasks and modify the workspace concurrently. To coordinate these tasks:
*   **Centralized Database Repository:** A centralized SQLite database (locally) or PostgreSQL instance (remotely) serves as the shared storage and transactional source of truth.
*   **Optimistic Concurrency Control (OCC):** The system coordinates concurrent state updates using a monotonic `Version` field on the `State` entity. Reads are non-blocking and do not hold database locks, allowing parallel worker agents to fetch state and construct prompts concurrently.
*   **Short-Lived Transactions:** Writes and state updates (such as task status changes or action logs) are executed inside short-lived database transactions that immediately release connection handles. Under no circumstances should database connections or transactions be held open during slow external network calls (such as LLM completions) or long-running shell builds.
*   **Conflict Resolution Loop:** If a worker's update fails due to a version conflict (another worker updated the state first), the worker automatically performs a reload-modify-retry cycle.

#### 3.5.4. DAG Task Splitting, Dependency Computation, & Concurrency Scheduler
To achieve true multi-agent autonomy without collision, `noctifab` implements a formal DAG scheduling and worker dispatching loop:

1.  **Task Splitting (Decomposition):**
    *   The **Planner Agent** parses the raw Markdown input spec (file, Jira, or issue).
    *   It decomposes the feature request into discrete, isolated, and small logical units of work (e.g., "Implement database schema migrator", "Implement storage adapter interface", "Write HTTP controller endpoints").
    *   Each logical unit is converted into a `Task` struct populated with a unique ID, description, and target files.

2.  **Dependency Computation (DAG Construction):**
    *   The Planner Agent computes execution dependencies by determining which tasks are prerequisite for others. For instance, the database schema migration task must complete before the repository adapter task can be built.
    *   It populates the `DependsOn` array of each `Task` with the IDs of its prerequisites.
    *   The orchestrator validates that the resulting task list forms a valid, cycle-free Directed Acyclic Graph (DAG) using a standard depth-first search (DFS) cycle-detection algorithm. Any cycle detected halts planning.

3.  **Topological Scheduling & Parallel Worker Assignment:**
    *   During the execution loop (`noctifab start`), the scheduler continuously polls the task DAG.
    *   It identifies **ready tasks** — tasks that are currently `TaskPending` and whose prerequisite tasks listed in `DependsOn` all have a status of `TaskSuccess`.
    *   For each ready task, if the number of currently active worker threads is less than `--agents` (or `NOCTIFAB_AGENTS_COUNT`), the orchestrator:
        1. Transitions the task status to `TaskInProgress`.
        2. Spawns an independent Go goroutine running a **Generator Agent** instance.
        3. Assigns the `Task` to this instance.
    *   Each goroutine operates in its isolated branch sandbox, which is branched off the main feature integration branch (e.g. `feature/feature-auth`). This ensures zero state pollution between concurrent running tasks.

4.  **Feedback, Integration, and Validation Loop:**
    *   Once a Generator worker completes its task, the orchestrator triggers local linter, compiler, and test checks.
    *   **Automatic Rebase & Merge:** If verification checks pass, the orchestrator automatically attempts to rebase or merge the completed task branch back into the feature integration branch (`feature/feature-auth`). The orchestrator also attempts to automatically rebase or merge this updated integration branch into all other currently active task branches to keep them synchronized.
    *   **Conflict & Failure Escalation:** If an automatic rebase or merge encounters conflicts that cannot be programmatically resolved, or if validation checks fail post-merge, the orchestrator does not halt the entire daemon loop. Instead, it marks the conflicting task status as `CONFLICT_BLOCKED` in the database, isolates the quarantined branch in the repository, and creates an asynchronous draft branch or PR on the remote VCS for manual conflict resolution in the IDE. The orchestrator continues scheduling and executing other independent task paths in the DAG that do not depend on the blocked task.
    *   **State Update:** Once integrated successfully, the task status is updated to `TaskSuccess` in the state database (guarded by transactional OCC write lock). Any subsequent ready tasks that depend on this task will now branch off the updated integration branch.
    *   **Retry & Failure Limits:** If compilation or tests fail and cannot be automatically fixed by diagnostic loops, the task's `Retries` count is incremented. If it exceeds `MaxRetries`, the task becomes `TaskFailed`, downstream dependent tasks are halted, and the orchestrator requests human assistance.

---

## 3.6. Edge Cases & Safety Guardrails

To run autonomously with zero human intervention and prevent infinite loops, workspace corruption, or parse failures, `noctifab` implements the following deterministic behaviors:

### 3.6.1. Infinite Loop Prevention
*   **Task Level:** Every `Task` contains a `Retries` count and a `MaxRetries` boundary. If a worker agent attempts to execute a task and fails (e.g. tests do not compile or linter fails) more than `MaxRetries` times, the orchestrator transitions the task's status directly to `TaskFailed` and raises an exception event rather than looping infinitely.
*   **Orchestrator Level:** The CLI loop implements a global action count ceiling (e.g. max 100 actions per start run) or elapsed duration timeout to automatically terminate execution if the loop does not resolve.

### 3.6.2. Safe JSON Extraction Algorithm
Low-reasoning LLMs sometimes surround JSON structures with conversational text or Markdown formatting (e.g., ` ```json `). The orchestrator parser must extract the JSON payload using the following matchers:
1. Search the raw response using a greedy regex matcher finding the first `{` and the last `}` (`(?s)\{.*\}`).
2. If no brace pattern matches, feed the parsing error back to the LLM as a warning prompt (counting against the retry limit).
3. If braces match, extract the matched substring and decode using `json.Unmarshal`.

### 3.6.3. Git Sandbox Branch Conflicts & Pruning
*   **Sandbox Isolation:** Parallel worker agents checkout task-specific sandboxes formatted as:
    `noctifab/task-<id>-agent-<agent_id>`
*   **Branch Collision Recovery:** If a branch name already exists locally or remotely, the agent fetches the latest commits, pulls from target branch, or appends a random execution suffix to ensure a clean commit sequence.
*   **Pruning on Failures:** If a task fails terminal validation checks, the branch is discarded or pushed to a quarantine tracking prefix `noctifab-quarantine/task-<id>` to keep the clean feature branches unpolluted.

### 3.6.4. Non-Blocking Interactive Stdin & Clarification Loop
*   **Always-Open Stdin Console:** The orchestrator must keep `stdin` open and active for interactive user input throughout the entire execution loop, rather than blocking the execution loop synchronously.
*   **Dynamic User Directions:** The user can input answers, commands, or directions at any time. These inputs can:
    *   Resolve open clarifications raised by the LLM or blocked merging processes.
    *   Provide manual steering or directions to complement currently running tasks.
    *   Create new tasks or modify existing future tasks in the DAG dynamically (e.g., if the initial input specification was incomplete or requires alteration mid-run).
*   **Task Suspension:** The orchestrator will only suspend the execution of tasks that are directly dependent on unresolved clarifications or blocked merges. Independent tasks in the DAG continue executing concurrently.
*   **Stdin Command Structure:** The interactive prompt allows structured directions (e.g., `answer <clarification-id> <response>`, `add-task <title>`, `override-merge <task-id>`) as well as plain text comments appended to the active agent run contexts.
*   **Daemon Control Interface (UNIX Socket / Local REST API):** To support non-blocking interactions when the orchestrator is run in background daemon mode (where standard `stdin` is detached and unavailable), the daemon exposes a local UNIX domain socket (or a lightweight localhost REST API on port `18080`). External clients and manual operators can use the `noctifab` CLI to send signals or answers to the daemon (e.g., `noctifab clarify --id <id> --answer <answer>`), which routes them directly into the state database.
*   **Clarification Deadline & LLM Fallback:** For each clarification waiting for user input, a maximum response deadline of 5 minutes is enforced. If the user does not respond within this 5-minute window, the orchestrator triggers an LLM completion. It prompts the LLM as a Staff Software Engineer to make a robust, production-grade design decision that follows SOLID design and good software engineering practices. The resulting recommendation is automatically written to the clarification's `Answer` field, the clarification is marked as resolved, and the orchestrator resumes execution of the dependent tasks.

### 3.6.5. Digital Twins (API Mocks)
*   To avoid test flakiness and billing costs during scenario evaluation, the system registry integrates mock adapters ("Digital Twins") simulating external dependencies (e.g., payment portals, third-party databases), guaranteeing reliable, deterministic test feedback.

### 3.6.6. Auto-Rollback Policies
To prevent unstable builds or broken endpoints from being committed to the target branch (e.g., if post-merge validations fail or the release deployment encounters problems):
1.  **Verification Failure Trigger:** If a merged pull request or a deployment trigger fails the holdout evaluation checks, the validator signals a rollback event.
2.  **Git Rollback Actions:** The VCS manager automatically executes Git rollback procedures:
    *   Reverting the specific merge commit on the target release branch (`git revert -m 1 <commit-hash>`).
    *   Restoring the last-known-good tag/commit reference.
    *   Pushing a standard revert commit back to the remote VCS provider, thereby respecting branch protection policies and avoiding force-pushes.
3.  **State Synchronization:** The rollback event updates the state database, resetting the failed tasks back to `TaskPending` or `TaskFailed` (depending on remaining retries) and moving the faulty branch into a quarantined namespace (`noctifab-quarantine/`) for diagnostic inspection.

### 3.7. Specification Ingestion & External Clients
To support dynamic task generation from multiple workflow sources, `noctifab` abstracts the feature specification retrieval through an ingestion layer. The CLI command `noctifab plan --input <source>` parses `<source>` to determine the appropriate adapter to execute:

```
                  ┌──────────────────────┐
                  │ noctifab plan --input│
                  └──────────┬───────────┘
                             │
            ┌────────────────┼────────────────┐
            ▼ (Jira URL)     ▼ (VCS URL)      ▼ (Local Path)
     ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
     │ Jira Client │  │ VCS Client  │  │ File Reader │
     └──────┬──────┘  └──────┬──────┘  └──────┬──────┘
            │                │                │
            └────────────────┼────────────────┘
                             ▼
                    [Parsed Markdown]
                             │
                             ▼
                     [Task DAG Plan]
```

#### 3.7.1. Local Markdown File Path
*   **Behavior:** Direct path reading from local disk (e.g., `--input ./docs/features/feature-auth.md`).
*   **Format:** Standard Markdown text matching Section 1 and Section 3 validation criteria.

#### 3.7.2. GitHub / GitLab Issue Ingestion
*   **Behavior:** If the input matches a GitHub/GitLab issue URL or reference (e.g., `https://github.com/owner/repo/issues/42`), the VCS client makes authenticated API calls to fetch the issue details.
*   **Payload Construction:** The system extracts the issue title, body description, and relevant discussion comments, consolidating them into a unified Markdown payload for parsing and DAG planning.
*   **Authentication:** Uses the standard `--vcs-token` / `NOCTIFAB_VCS_TOKEN` configuration.

#### 3.7.3. Jira Issue Ingestion
*   **Behavior:** If the input matches a Jira issue URL (e.g., `https://company.atlassian.net/browse/KEY-101`), the Jira client is initialized.
*   **Jira Client Implementation:** Under `pkg/infrastructure/jira/client.go`, a REST client connects to Atlassian's issue API.
*   **Payload Construction:** The client fetches the issue summary, description, and comments, converting Atlassian Document Format (ADF) or rich-text fields into standard Markdown representation.
*   **Authentication:** Authenticates using basic authentication headers via the developer email (`--jira-user` / `NOCTIFAB_JIRA_USER`) and API token (`--jira-token` / `NOCTIFAB_JIRA_TOKEN`).

### 3.8. Automatic Commits, Centralized Versioning, & Pull Requests
When the automated commit setting is enabled (via CLI flag `--auto-commit` or environment variable `NOCTIFAB_AUTO_COMMIT=true`), the orchestrator automatically manages the integration pipeline: branch creation, centralized version bumping, changelog updates, and pull request creation.

#### 3.8.1. Branch Naming Policy
The branch created by the worker agent is dynamically named based on the specification source:
*   **Markdown File:** Suffix of the filename (e.g., branch name: `feature/feature-auth` from `feature-auth.md`).
*   **Jira Issue:** Suffix of the Jira key (e.g., branch name: `jira/KEY-123`).
*   **GitHub/GitLab Issue:** Suffix of the issue ID (e.g., branch name: `issue/gh-45` or `issue/gl-78`).

#### 3.8.2. Centralized Release Pipeline & Version Bumping
To prevent git merge conflicts and version stagnation in a multi-agent environment, **individual worker agents do not modify the `VERSION` file or `CHANGELOG.md`**. Instead, the release pipeline is managed centrally:
1.  **Partial Changelog Collection:** As each worker agent successfully completes its assigned task, it records a list of specific change description items (a partial changelog list, e.g. `["Added token authorization controller", "Fixed memory leak in connection pool"]`) to its task record in the state database.
2.  **Aggregation:** Once all tasks in the DAG are successfully completed (`TaskSuccess`), the orchestrator coordinator gathers all partial changelog items.
3.  **Bumping Logic:** The orchestrator reads the current version from the `VERSION` file at the workspace root (formatted as `MAJOR.MINOR.PATCH`) and determines the combined upgrade scope based on the change types of all completed tasks:
    *   **Major Bump (`+1.0.0`):** Triggered if any task specification includes breaking changes or explicitly requests a major release.
    *   **Minor Bump (`+0.1.0`):** Triggered if the tasks contain new features (e.g. `feat: ...`) and no breaking changes.
    *   **Patch Bump (`+0.0.1`):** Triggered if all tasks are bug fixes (e.g. `fix: ...`).
4.  **Version Update:** The orchestrator writes the final bumped version string back to the `VERSION` file at the root.

#### 3.8.3. CHANGELOG.md Management (Keep a Changelog Standard)
Once all tasks are done, the orchestrator updates the `CHANGELOG.md` file located at the workspace root, adhering strictly to the **Keep a Changelog** standard. It prepends the unified release section at the top of the file under the `# Changelog` heading, compiling all gathered partial changelog items into categorized lists:
*   Version header (e.g., `## [1.2.0] - YYYY-MM-DD`).
*   Categorized lists of changes under subheadings: `### Added`, `### Changed`, `### Deprecated`, `### Removed`, `### Fixed`, `### Security`.

#### 3.8.4. Conventional Commits & Pull Request Creation
*   **Conventional Commit Message:** The orchestrator writes the commit message conforming to the **Conventional Commits** specification (e.g., `feat(auth): integrate oauth2 login`, `fix(db): resolve connection pool leak`), describing the aggregated changes.
*   **Remote Push:** The Git wrapper pushes the branch to the remote repository.
*   **Pull Request Creation:** The VCS client makes a REST/GraphQL call to the remote provider (GitHub/GitLab/Bitbucket) to create a Pull Request targeting the default branch (e.g., `main`), providing a detailed description outlining:
    *   The feature/fix goal.
    *   List of files modified.
    *   A summary of holdout test evaluation outcomes.

---

## 4. Command Line Interface (CLI)

`noctifab` exposes a structured Command Line Interface. It supports multiple subcommands to configure, run, and validate the agent's operations.

### 4.1. CLI Commands

*   `noctifab init`
    Initializes a new `noctifab` workspace config directory and initializes the database.
*   `noctifab start`
    Starts the daemonized execution loop, continuously polling and executing actions.
*   `noctifab run-once`
    Executes exactly one cycle of the orchestrator loop (Observe -> Decide -> Validate -> Execute -> Save) and then terminates. Excellent for debugging and running in crontab.
*   `noctifab validate`
    Runs a dry-run check of the current local state file, project directory constraints, and linter commands without polling the LLM or running actions.
*   `noctifab plan`
    Reads the input specification file, performs the clarification loop if needed, and builds/updates the task DAG in the configuration state.
*   `noctifab maintenance`
    Launches a dedicated Quality Maintenance run. Scans the target workspace repository for code drift, stale document links, and deprecated imports, creating cleanup PRs.

### 4.2. CLI Flags & Environment Mappings

The CLI configuration can be provided via flags or matching environment variables. Flags always take precedence over environment variables:

| Flag Name | Short | Environment Variable | Default Value | Description |
|---|---|---|---|---|
| `--config` | `-c` | `NOCTIFAB_CONFIG` | `cwd/.noctifab/config/noctifab.db` | Path to the database or configuration file |
| `--storage-provider` | | `NOCTIFAB_STORAGE_PROVIDER` | `sqlite` | Storage backend provider: `sqlite`, `postgres` |
| `--storage-conn` | | `NOCTIFAB_STORAGE_CONN` | | Connection string or filepath for the storage database |
| `--input` | `-i` | `NOCTIFAB_INPUT` | | Path, GitHub/GitLab issue URL, or Jira URL to fetch the feature specification |
| `--auto-commit` | | `NOCTIFAB_AUTO_COMMIT` | `false` | Enable automatic branch creation, conventional commit, version bump, and PR creation |
| `--agents` | `-a` | `NOCTIFAB_AGENTS_COUNT` | `3` | Maximum number of parallel workers/agents to spawn |
| `--interval` | `-t` | `NOCTIFAB_INTERVAL` | `5m` | Cycle loop polling duration interval |
| `--vcs-provider` | `-p` | `NOCTIFAB_VCS_PROVIDER` | `github` | Version Control System (VCS) target: `github`, `gitlab`, `bitbucket` |
| `--vcs-token` | | `NOCTIFAB_VCS_TOKEN` | (Required) | API Access Token for the VCS provider |
| `--vcs-repo` | `-r` | `NOCTIFAB_VCS_REPO` | (Required) | Repository identifier format: `owner/repo` |
| `--llm-provider` | `-l` | `NOCTIFAB_LLM_PROVIDER` | `openai` | LLM client API provider: `openai`, `anthropic`, `gemini`, `ollama` |
| `--llm-model` | `-m` | `NOCTIFAB_LLM_MODEL` | `gpt-4o` | LLM Model Identifier (e.g., `gpt-4o`, `claude-3-5-sonnet`, `gemini-1.5-pro`) |
| `--llm-api-key` | `-k` | `NOCTIFAB_LLM_API_KEY` | | API authentication key. Falls back to `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, or `GEMINI_API_KEY` if not set |
| `--llm-url` | `-u` | `NOCTIFAB_LLM_URL` | | Custom endpoint URL (useful for local Ollama instances) |
| `--jira-user` | | `NOCTIFAB_JIRA_USER` | | User email for Jira REST API authentication |
| `--jira-token` | | `NOCTIFAB_JIRA_TOKEN` | | API Token for Jira REST API authentication |
| `--jira-url` | | `NOCTIFAB_JIRA_URL` | | Base URL of the Jira cloud instance (e.g., https://company.atlassian.net) |
| `--http-max-retries` | | `NOCTIFAB_HTTP_MAX_RETRIES` | `10` | Maximum HTTP request retries for API clients |
| `--http-retry-backoff` | | `NOCTIFAB_HTTP_RETRY_BACKOFF` | `100ms` | Base delay time duration for exponential backoff retry logic |

---

## 5. Observability & Logging Specifications

The loop execution output is printed to standard output (`stdout`) or a configured log file. To facilitate debugging and state reconstruction, logging must contain:

1.  **State Snapshots:** Serialized state outputs printed before asking the LLM (at trace log level).
2.  **LLM Decisions:** Logs representing the chosen `tool`, parsed arguments (`args`), and reasoning explanation.
3.  **Validation Failures:** Warnings detailing validation errors (e.g. `validation failed: no tasks to complete`).
4.  **Tool Execution Outcome:** Output results, elapsed execution time, and status codes.

---

## 6. Testing Strategy

Stability is paramount. `noctifab` requires a two-tiered testing approach: Unit and End-to-End.

### 6.1. Unit Testing
*   **Location:** All unit tests must be defined alongside the source files in files matching `*_test.go`.
*   **Command:** Run unit tests via `go test ./...`

### 6.2. E2E Docker Integration Testing
To test the complete orchestrator without making real API calls, we implement a multi-container E2E framework managed by Docker Compose.

```
       ┌────────────────────────┐
       │     Docker Network     │
       └───────────┬────────────┘
                   │
         ┌─────────┼─────────┐
         │         │         │
         ▼         ▼         ▼
    ┌─────────┐┌─────────┐┌─────────┐
    │llm      ││github   ││sqlite/  │
    │(mock)   ││(mock)   ││postgres │
    └────┬────┘└────┬────┘└────┬────┘
         │          │          │
         └──────────┼──────────┘
                    │
                    ▼
              ┌───────────┐
              │harness    │
              │(noctifab) │
              └─────┬─────┘
                    │
                    ▼
              ┌───────────┐
              │mock-proj  │
              │(workspace)│
              └───────────┘
```

The testing suite creates a network consisting of:
1.  **`harness`:** The `noctifab` binary container built from local source. It accepts environmental parameters to connect to mock dependencies.
2.  **`llm` (Mock LLM Provider):** A lightweight service mimicking the OpenAI or Ollama JSON protocols. It executes a deterministic matching engine that matches incoming prompts against defined rules.
3.  **`github` (Mock VCS API and Git Server):** An HTTP service wrapping `git-http-backend` via CGI to host a real Git repository, combined with mock handlers for the GitHub REST API (allowing issue reading, PR creation, and merges).
4.  **`sqlite` / `postgres`:** Database instances configured to check state preservation, migration compatibility, and schema parity.
5.  **`mock-project`:** A target directory mounted into the harness containing intentional issues, broken code, and failing unit tests to act as the test subject for the autonomous agent.

#### 6.2.1. Mock Git & VCS API Server Architecture
The mock VCS service (e.g., matching the GitHub API) provides two main features:
*   **Git CGI Wrapper:** Uses Go's standard library `net/http/cgi` to wrap the standard `git-http-backend` binary. This enables authentic git actions (clone, pull, push, checkout) inside the harness sandbox targeting the mock remote.
*   **REST API Handlers:** Exposes matching HTTP endpoints for VCS interactions:
    *   `GET /api/v3/repos/{owner}/{repo}/issues/{number}` - Returns the issue body/specification for ingestion.
    *   `POST /api/v3/repos/{owner}/{repo}/pulls` - Creates a PR, returning status information and storing it in an in-memory database.
    *   `GET /api/v3/repos/{owner}/{repo}/pulls/{number}` - Queries mergeability and status.
    *   `PUT /api/v3/repos/{owner}/{repo}/pulls/{number}/merge` - Merges the branch and updates the mock repository's default branch.

#### 6.2.2. Mock LLM Rule Engine Architecture
The mock LLM client serves requests dynamically based on rules written for specific test scenarios, allowing verification of both happy and unhappy paths (such as retries and guardrail blocks) deterministically:
*   **Request-Dependent Matching:** The server accepts `/v1/chat/completions` (OpenAI style) or `/api/generate` (Ollama style) requests. It matches the prompt content and the serialized state (injected in the prompt) against a list of rule definitions:
    *   `match.prompt_contains`: Substring/regex matching on the text prompt instructions.
    *   `match.state_contains`: JSON path or substring matching on the current state.
*   **Response Generation:** Upon a successful match, the server returns the associated structured JSON object:
    ```json
    {
      "reasoning": "Explanation of the decision",
      "tool": "tool_name",
      "args": {
        "arg_name": "value"
      }
    }
    ```
*   **Fallback:** If no rule matches the incoming prompt/state, the mock server returns an HTTP 400 error detailing the unmatched prompt to simplify debugging.

#### 6.2.3. Host-Side E2E Test Runner
E2E scenarios are orchestrated by an automated runner on the host system:
1.  **Workspace Isolation:** The runner copies target scenarios from `tests/e2e/scenarios/` (containing target markdown specifications, seed repository templates, and LLM rule JSONs) to isolated temporary directories.
2.  **Environment Provisioning:** It launches the Docker Compose network, configuring the mock LLM with the rule JSON for the specific scenario.
3.  **Process Monitoring & Verification:** The runner starts the `noctifab` harness, polls the state, and asserts post-conditions once the execution terminates:
    *   Ensures version tags are correctly bumped (e.g. `VERSION`).
    *   Ensures conventional commit conventions were strictly followed in the git log.
    *   Ensures `CHANGELOG.md` changes were prepended adhering to Keep a Changelog.
    *   Ensures files match expected implementations and all sandbox boundaries (like path traversal prevention) correctly blocked unauthorized executions.

### 6.3. Continuous Integration Workflows (Required, To Be Implemented)
To support autonomous verification in the VCS pipeline, the repository must incorporate GitHub Actions CI workflows:
*   **Workflow Targets:** A workflow under `.github/workflows/ci.yml` is required.
*   **Pipeline Checks:** The workflow must execute:
    *   **Linting:** A static analysis step running the configured `golangci-lint` runner.
    *   **Unit Tests:** An automated test step running `go test -v -race ./...` on the codebase.
*   **Implementation Status:** These workflows must be defined in the specification as required, but they are currently omitted from the repository implementation and must not be created or committed until specifically instructed.

---

## 7. Implementation Roadmap (Phases)

To ensure high cohesion, low coupling, and compliance with the 500-line source code file limit, the development of `noctifab` must be partitioned into the following sequential implementation phases:

### 7.1. Phase 1: Domain & Core Storage Infrastructure
*   **Objective:** Establish the primary domain models, state persistence, concurrency locking, and basic CLI entrypoints.
    *   `pkg/domain/state.go` - The [State](file:///Users/diegoj/repos/noctifab/SPEC.md#L194-L203) structures, metadata models, and interfaces.
    *   `pkg/domain/task.go` - Task entities, status types, and lifecycle behaviors.
    *   `pkg/domain/action.go` - Action execution logs.
    *   `pkg/domain/state_repository.go` - [StateRepository](file:///Users/diegoj/repos/noctifab/SPEC.md#L206-L216) interface.
    *   `pkg/infrastructure/storage/sqlite_repository.go` - SQLite database implementation of state storage.
    *   `pkg/infrastructure/storage/postgres_repository.go` - PostgreSQL database implementation of state storage.
    *   `cmd/noctifab/main.go` - Main CLI bootstrap routing commands (`noctifab init`, `noctifab validate`).
*   **Verification:** Unit tests for SQLite and PostgreSQL loading/saving, connection management, and transaction OCC safety.

### 7.2. Phase 2: Task DAG & Concurrency Scheduler
*   **Objective:** Implement task plan parsing, Directed Acyclic Graph (DAG) validation, and parallel goroutine task dispatching.
*   **Key Deliverables:**
    *   `pkg/usecase/scheduler.go` - Core task execution schedule scheduler logic.
    *   `pkg/usecase/dag.go` - Depth-First Search (DFS) validation for cycle detection in task dependencies.
*   **Verification:** Unit tests validating cyclic dependencies, topological sorting, and concurrent task executor worker queues.

### 7.3. Phase 3: LLM Client & Tool Registry
*   **Objective:** Build standard agent tool implementations and establish structured model chat interfaces.
*   **Key Deliverables:**
    *   `pkg/usecase/registry.go` - The [Registry](file:///Users/diegoj/repos/noctifab/SPEC.md#L235-L239) and tool routing.
    *   `pkg/infrastructure/llm/client.go` - The [Client](file:///Users/diegoj/repos/noctifab/SPEC.md#L289-L302) interface and OpenAI/Anthropic/Gemini/Ollama implementations.
    *   `pkg/infrastructure/llm/parser.go` - The Safe JSON Extraction logic parser.
    *   Standard tools definitions in `pkg/usecase/tools/`: bootstrap (`add_task`, `complete_task`, `log_message`) and production (`read_file`, `write_file`, `run_tests`).
*   **Verification:** Unit tests for JSON extraction edge-cases (conversational wrappers, malformed JSON regex fallback) and mock API responses.

### 7.4. Phase 4: Validator, Sandbox Boundaries & Holdout Evaluator
*   **Objective:** Define security sandbox filters and holdout scenario gates to ensure code quality and system safety.
*   **Key Deliverables:**
    *   `pkg/usecase/validator.go` - Code quality [Validator](file:///Users/diegoj/repos/noctifab/SPEC.md#L344-L351) engine.
    *   `pkg/usecase/sandbox.go` - Path traversal filtering ([filepath.Clean](file:///Users/diegoj/repos/noctifab/SPEC.md#L387)) and command execution whitelist checks.
    *   `pkg/usecase/holdout.go` - BDD holdout evaluator logic (running tests 3 times, checking majority vote, filtering feedback logs).
*   **Verification:** Unit tests confirming sandbox boundary violations are correctly blocked, and holdout majority voting returns expected boolean values.

### 7.5. Phase 5: VCS API, Versioning & Ingestion Adapters
*   **Objective:** Implement Git commands, release version tags, Keep a Changelog management, and remote issue fetching.
*   **Key Deliverables:**
    *   `pkg/infrastructure/vcs/git.go` - Git operations wrapper (checkout, branch, commit, push, merge).
    *   `pkg/infrastructure/vcs/github.go` - GitHub REST API adapter for PR creation.
    *   `pkg/infrastructure/vcs/gitlab.go` - GitLab REST API adapter.
    *   `pkg/infrastructure/jira/client.go` - Jira REST API client.
    *   `pkg/usecase/release.go` - Version tag bumping algorithm and `CHANGELOG.md` manager.
*   **Verification:** Unit tests mocking issue fetching APIs, GFM conversions, semver bumping rules, and changelog prepending.

### 7.6. Phase 6: E2E Integration Suite & Docker Compose Network
*   **Objective:** Construct a localized containerized testing system allowing real end-to-end runs of the CLI against mock servers.
*   **Key Deliverables:**
    *   `tests/e2e/mock_llm/` - A local HTTP engine mapping prompts to deterministic response templates.
    *   `tests/e2e/mock_vcs/` - Local Git CGI server (`git-http-backend`) and API mock endpoints.
    *   `docker-compose.yml` - Complete testing topology definition.
*   **Verification:** Execute integration suite checking version bumps, conventional commit formats, and sandbox violations on test branches.

---

## 8. Technical Challenges & Resolution Strategies

To guarantee stable and autonomous execution without human intervention, `noctifab` addresses potential technical failures through these robust architectural mechanisms:

### 8.1. Probabilistic and Conversational LLM Outputs
*   **Challenge:** Large Language Models (LLMs) are probabilistic and often return conversational text, markdown formatting (such as ` ```json ` tags), or explanations alongside the JSON payload.
*   **Resolution:** 
    1.  **Greedy Extraction Regex:** The parser implements a greedy regular expression `(?s)\{.*\}` to extract the substring between the first `{` and the last `}`.
    2.  **Schema Unmarshalling:** The matched string is decoded directly using Go's `json.Unmarshal`.
    3.  **Instructional Feedback Loops:** If unmarshalling fails or schema validation fails, the orchestrator does not crash. It formats the error as a lint-like prompt (e.g., `"Error parsing response: missing 'tool' field. Please return only the structured JSON"`), incrementing the loop retry counter.

### 8.2. Go File Line Limit Compliance (Max 500 Lines)
*   **Challenge:** Large infrastructure adapters, orchestrators, or CLI flag setups can easily grow past 500 lines of code, violating the strict `AGENTS.md` limit.
*   **Resolution:**
    1.  **Granular Separation of Packages:** Implement interfaces to decouple layers. For example, VCS integrations are split into smaller domain models, interface abstractions, and distinct, smaller adapter files (e.g. `github_client.go`, `gitlab_client.go`, `git_command.go` under `vcs/`).
    2.  **Shared Helpers:** Extract formatting, validation, and serialization routines into dedicated helper files (e.g., `pkg/usecase/dag_cycle.go` separate from `pkg/usecase/scheduler.go`).

### 8.3. Flaky, Slow, and Costly VCS Integration Tests
*   **Challenge:** Running integration tests that perform Git cloning, branches checking, and pull request generation against public GitHub/GitLab servers is slow, subject to network rate-limiting, and requires external developer tokens.
*   **Resolution:** 
    1.  **Local CGI Git Backend:** The E2E environment launches a mock VCS service wrapping `git-http-backend` via Go's `net/http/cgi`. This allows authentic git cloning, pushing, and pulling entirely offline.
    2.  **Local API Handlers:** The mock VCS server exposes REST HTTP mock endpoints for PR creation, queries, and merges, maintaining state in a fast in-memory database to simulate external Git APIs deterministically.

### 8.4. Runaway Loops and LLM Token Credit Depletion
*   **Challenge:** Autonomous agent loops can enter infinite recursion or loop cycles (e.g. fix test -> test fails -> try to fix again in the same way), quickly consuming model credits and API budgets.
*   **Resolution:**
    1.  **Task Retry Boundary:** Each task maintains a `Retries` count. If it fails to execute cleanly (compilation or tests fail) more than `MaxRetries` times, the task state transitions to `TaskFailed`, stopping downstream work.
    2.  **Orchestrator Execution Ceiling:** The CLI enforces a hard limit of max actions per execution run (e.g. 100 actions) and a total loop duration ceiling.
    3.  **Command Context Timeouts:** Every external command execution is wrapped with Go's `context.WithTimeout` (e.g., maximum 5 minutes), terminating hanging build scripts.
    4.  **Shared Token Quota & Graceful Suspension:** The system manages API costs using a shared token bucket record in the database. Workers request token reservations before each LLM call. If daily token quotas or budgets are exhausted:
        *   The orchestrator saves the current state of all active tasks and workers back to the database.
        *   Execution is cleanly suspended without leaving uncommitted files or corrupt branches.
        *   The system notifies the user (via stdout/socket/logs) that the agent runs are paused until the token quota resets or more tokens are allocated by the provider.
    5.  **Configurable Resilient HTTP Retries:** External API clients (LLMs, Jira, VCS) execute all requests through a resilient retry wrapper. It performs up to `--http-max-retries` (default: 10) retries using exponential backoff (starting at `--http-retry-backoff`, e.g., 100ms) with full jitter to transparently handle temporary rate limits (HTTP 429) and network outages.

### 8.5. Multi-Agent Concurrency and Repository Conflicts
*   **Challenge:** Concurrent developer agents processing parallel DAG tasks might cause git conflicts, branch collisions, or corrupt the state.
*   **Resolution:**
    1.  **Optimistic Concurrency Control (OCC):** Access is synchronized through a monotonic versioning system on the database record. Simultaneous writes are rejected at the database transaction level if a conflict is detected, triggering a safe reload-retry loop in the worker.
    2.  **Sandboxed Branch isolation:** Parallel agents checkout tasks to unique branches named `feature/task-<id>-agent-<agent_id>`. Direct pushes to protected branches are blocked at the validator sandbox level.

### 8.6. AI Code Gaming and Test Overfitting
*   **Challenge:** Generator agents might write mocked or hardcoded return values to satisfy specific unit tests rather than implementing generalized production logic.
*   **Resolution:**
    1.  **Holdout Scenario Isolation:** Acceptance/BDD integration scenarios are kept inside a secure directory (`tests/holdout/`) which is not mounted into the code generator agent sandbox.
    2.  **Filtered Error Logs:** When a holdout scenario fails, the Generator Agent is only provided with a filtered summary of the failure (e.g., `Holdout failed: Validation Endpoint returned status 500`), never the source test cases, forcing the agent to build correct logic from the specification.
    3.  **Deterministic CLI Execution:** All holdout scenarios are executed deterministically using standard CLI commands (e.g., `go test`). A 100% success rate is required for the validation runner to approve the build.

### 8.7. Jira ADF Formatting and Rich Text Parsing Issues
*   **Challenge:** Jira API returns descriptions using the complex Atlassian Document Format (ADF) JSON structure rather than Markdown. Directly feeding raw ADF JSON to planning prompts increases token size and causes parser errors.
*   **Resolution:**
    1.  **AST-based ADF Transformer:** The Jira integration client contains a document transformer that recursively walks the ADF node tree (e.g. `heading`, `paragraph`, `bulletList`, `codeBlock`) and maps them directly to standard GitHub Flavored Markdown (GFM) strings.
    2.  **Parser Fallbacks:** If the ADF payload is malformed, the parser falls back to fetching plaintext representation from the Jira API description.

