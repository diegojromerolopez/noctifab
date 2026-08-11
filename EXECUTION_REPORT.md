# Execution Reports: Implementation Plan

> **Status:** reviewed and implementation-ready  
> **Scope:** plan only; no production behavior is implemented by this document  
> **Canonical plan file:** `EXECUTION_REPORT.md`

## 1. Purpose

Add an optional execution report to noctifab. A non-empty `report` path in `.noctifab/config.yaml` enables deterministic execution measurement and a Markdown diagnostic artifact. If `report` is omitted or blank, reporting is completely disabled.

A motivating use case is the containerized matrix documented in `validation/README.md` and `validation/projects/TESTING_GUIDE.md`. Today each validation container emits a large combined log, `run_one.sh` invokes `gen_feedback.py`, and a developer may still give the log to an external LLM to understand stalls, retries, and failures. The new report must be generated **inside noctifab from structured execution facts**, checkpointable to a host-mounted artifact while a container is running, and detailed enough that normal validation triage never requires an LLM to read the container log.

This is a general noctifab product feature, not validation-harness code. A developer running `noctifab start` for any ordinary project must receive the same issues, bottlenecks, evidence, proposals, timings, and limitations. Core reporter packages must not import `validation/`, inspect validation-project names, branch on `NOCTIFAB_E2E`, know target languages/artifacts, or depend on Docker. The validation harness is only one external consumer of the public report contract.

The validation artifact hierarchy is:

1. `EXECUTION_REPORT.md`: noctifab's authoritative internal execution diagnosis;
2. a small validation-harness result: container exit code and black-box target-artifact checks that happen after noctifab exits;
3. the raw container log: retained only as a deep-debug fallback, not the primary diagnostic input.

For a normal project, the configured Markdown file is directly the developer-facing artifact; no harness wrapper is involved. The report is for a developer, deterministic tooling, or another LLM consuming the already-structured result. It must explain:

- process and user-story wall time;
- time spent in planning, task execution, validation, QA, release, shutdown, and report synthesis;
- active and waiting time by agent invocation, role, task, operation class, and external dependency;
- retries, timeouts, conflicts, failures, and missing telemetry;
- evidence-backed functional, performance, security, configuration, quality, and operational issues;
- deterministic recommendations and, when available, bounded LLM-generated hypotheses;
- the limits of the measurements.

### 1.1 Invariants

Reporting is diagnostic infrastructure. It must not:

- change task scheduling, validation, retries, merge behavior, generated code, state outcomes, or the command exit code;
- expose credentials, `secrets.yaml`, authorization headers, full prompts, or full model responses;
- require an LLM to parse or summarize container logs;
- use an LLM as the source of timing, counters, outcomes, or costs;
- put a report artifact where an agent can edit it or Git can accidentally commit it;
- require a database migration in the first implementation;
- retain an unbounded raw event history in a daemon.

Reporter failures are always secondary. The original noctifab outcome wins.

### 1.2 Commands in scope

The first implementation applies only to execution commands:

- `dist/noctifab start [target_path]`;
- the hidden `dist/noctifab serve` process used by daemon mode.

`init`, `validate`, `maintenance`, `clean`, dashboard-only activity, and prompt inspection do not create execution reports in this phase. If a later story expands command coverage, it must define that command's lifecycle separately.

### 1.3 Current repository facts that affect the design

An implementer must account for the code that exists now, not the older package names in `SPEC.md`:

- orchestration code is under `pkg/services`, not `pkg/usecase`;
- `start` constructs a new `Orchestrator` for each roadmap story, while `serve` reuses one orchestrator;
- product-manager roadmap generation and provider pre-flight pings happen outside `Orchestrator`;
- `domain.LLMClient.Complete` returns `LLMResponse` tool actions and does not expose provider attempts, exact usage, or arbitrary analysis JSON;
- `MetricsCollector` has aggregate fields, but most LLM, phase, and sandbox boundaries do not currently call it;
- `OrchestratorConfig.MetricsEnabled` is not populated in both CLI construction paths;
- tools, sandbox commands, Git calls, QA, and LLM calls have several call sites, so instrumentation must use shared wrappers rather than copy/pasted timers;
- `cmd/noctifab/cli/start.go` is currently 547 lines (exceeding the 500-physical-line limit in AGENTS.md) and must be refactored into `cmd/noctifab/cli/start.go` (<300 lines for Cobra command definition & flags) and `cmd/noctifab/cli/start_runner.go` (<400 lines for workspace verification, roadmap generation, and story loop) before adding report wiring;
- task IDs and current agent IDs can repeat across stories or invocations, so report keys need process-scoped story and invocation IDs.

These are implementation constraints, not reasons to weaken the report contract.

## 2. User-facing contract

### 2.1 Configuration

Add an optional path:

```yaml
config_version: "2.0"

# Omit or leave blank to disable execution reporting.
execution_report: ".noctifab/reports/report.md"
```

Add to `config.Config`:

```go
ExecutionReport string `yaml:"execution_report,omitempty"`
```

The default is `""`. `WriteDefaultConfig` may omit the empty/default key because of `omitempty`; existing generated configuration therefore remains stable.

No CLI flag or environment override is added in this feature. A future `--execution-report` or `NOCTIFAB_EXECUTION_REPORT` must use exactly the same resolution, isolation, warning, and soft-failure rules.

When `execution_report` is non-empty and enabled, noctifab runs the deterministic observer/reporter during execution and performs **at most one additional logical Reporter Analyzer call to `domain.LLMClient.Complete` per process**, at terminal finalization (`Reporter.Finish`). The selected provider may still perform its configured internal HTTP retries/failover attempts. Intermediate checkpoints (`Observe`, task finish, `EndStory`) write deterministic Markdown only and NEVER call the analyzer. For daemon mode (`serve`), `Finish` is invoked only when the daemon receives a graceful shutdown signal (SIGINT/SIGTERM) or exits. See §10.

### 2.2 Disabled, malformed, and invalid values

| Value | Behavior |
| :--- | :--- |
| `execution_report` key omitted | disabled; no reporter, goroutine, directory, file, event retention, or analyzer call |
| `execution_report: "   "` | disabled after trimming |
| valid YAML string | resolve and prepare as described below |
| YAML sequence, map, boolean, or number | normal strict configuration parse error; command retains its existing non-zero exit behavior |
| string with NUL or invalid destination | reporting is softly disabled and execution continues |

A non-string value is not an “invalid optional path”; it violates the YAML schema. It must not be silently ignored.

### 2.3 Path resolution and artifact isolation

Public resolver:

```go
func ResolveReportPath(projectPath, configured string) (path string, enabled bool, err error)
```

`configPath` is intentionally not an argument: relative report paths are based on the target project workspace, not on the location of a possibly external config file.

Resolution rules, in order:

1. Trim Unicode whitespace. Empty means `("", false, nil)`.
2. Reject a NUL byte.
3. Canonicalize `projectPath` to an absolute clean path and extract the sanitized base folder name `<folder_name>` (e.g. `pyedis`, `frontpunch`, `project`).
4. Resolve a relative configured path from that project path; clean an absolute path as-is.
5. Format the destination filename's basename as `YYYYMMDD_HHMMSS_<folder_name>.md` (or prefix a custom configured filename basename with `YYYYMMDD_HHMMSS_<folder_name>_`), incorporating the UTC date and time of execution and the workspace folder name.
6. Apply lexical workspace-boundary rules without touching the filesystem:
   - a destination lexically inside the workspace must be under `<workspace>/.noctifab/reports/`;
   - an absolute destination lexically outside the workspace is allowed (e.g. host bind mount `/app/report_mount/20260811_225122_pyedis.md` when workspace is `/app/tmp_verify_autonomy/pyedis`);
   - `.git/`, `secrets.yaml`, source, roadmap, test, and arbitrary root destinations are rejected.

`ResolveReportPath` is pure apart from standard path manipulation: it does not call `Stat`, resolve symlinks, create directories, or probe permissions. Existing-object and canonical-symlink checks belong to destination preparation.

The in-workspace restriction is deliberate. Allowing `execution_report: README.md`, for example, could overwrite tracked project content, affect scanner state, enter prompts, or be committed by `git add --all`, which would violate the feature's primary invariant.

Examples for workspace `/work/project` (started at `2026-08-11 22:51:22 UTC`):

| Configured value | Result |
| :--- | :--- |
| `""` | disabled |
| `.noctifab/reports/report.md` | `/work/project/.noctifab/reports/20260811_225122_project.md` |
| `README.md` | invalid: workspace reports must be below `.noctifab/reports/` |
| `.git/report.md` | invalid |
| `/tmp/noctifab-run.md` | accepted external absolute path `/tmp/20260811_225122_project_noctifab-run.md` |
| `/work/project/src/report.md` | invalid even though absolute |
| `.noctifab/reports` when it is a directory | invalid destination |

### 2.4 Destination preparation

Path resolution is lexical and must not create files. A separate injected destination preparer performs the writable probe:

```go
type ReportDestinationPolicy struct {
    ProjectPath  string
    ConfigPath   string
    DatabasePath string
    PIDPath      string
}

func PrepareReportDestination(
    ctx context.Context,
    path string,
    policy ReportDestinationPolicy,
    fs FileSystem,
) error
```

It must:

1. find the nearest existing ancestor, resolve existing symlinks, and re-apply the workspace boundary to the canonical destination;
2. reject a destination whose existing final component is a directory or symbolic link;
3. reject the canonical config, database, PID, `.git`, or `secrets.yaml` paths from the policy;
4. create missing parent directories with mode `0700`;
5. create an exclusive random probe file in the destination directory with mode `0600`;
6. close and remove the probe;
7. if preparation fails, remove only empty parent directories created by this call, in leaf-to-root order;
8. never truncate or modify the existing destination;
9. repeat final-component `Lstat` and canonical-boundary checks immediately before the probe.

Define the narrow filesystem seam in `pkg/infrastructure/reportfs` rather than mocking `os` globally:

```go
type SyncFile interface {
    io.Writer
    Name() string
    Sync() error
    Chmod(mode fs.FileMode) error
    Close() error
}

type FileSystem interface {
    Lstat(path string) (fs.FileInfo, error)
    EvalSymlinks(path string) (string, error)
    Mkdir(path string, mode fs.FileMode) error
    CreateTemp(dir, pattern string, mode fs.FileMode) (SyncFile, error)
    Open(path string) (SyncFile, error) // used to sync the parent directory
    Rename(oldPath, newPath string) error
    Remove(path string) error
}
```

The custom `CreateTemp` must apply the requested mode at creation rather than creating broadly and tightening permissions afterward. The production implementation delegates to `os.OpenFile` with `O_CREATE|O_EXCL` and cryptographically random names; tests use a recording fake.

The probe tests the operation the atomic writer actually needs: creation in the parent directory. Permission-bit inspection alone is incorrect because ACLs and effective user permissions may differ.

A failed probe may create directories transiently, but cleanup must leave no report artifact or newly-created empty report directory. Tests inject a fake filesystem for permission and cleanup failures; they must not assume a particular host user or `chmod` behavior.

### 2.5 Soft-failure warnings and exit codes

An invalid or unpreparable destination prints exactly one single-line warning to `stderr`:

```text
noctifab report disabled: <sanitized reason>
```

A write failure after startup prints at most one single-line warning per process:

```text
noctifab report write failed: <sanitized reason>
```

Rules:

- no report status is printed to `stdout`;
- raw path values, control characters, and credentials are not included in warnings;
- invalid report paths do not change the command's pre-existing exit code;
- normal successful execution remains exit code `0`;
- ordinary `start`/`serve` errors continue through existing Cobra handling (normally exit code `1`);
- existing specialized `ExitError` codes remain unchanged;
- a reporter error must never wrap or replace the original command error.

### 2.6 One process, one run

A report represents one OS process execution:

- a run ID is generated once after destination preparation;
- startup atomically creates a report file whose filename incorporates the UTC timestamp and target workspace folder name (`YYYYMMDD_HHMMSS_<folder_name>.md`);
- `start` uses the same reporter even though it currently creates one orchestrator per story;
- `serve` uses the same reporter for every story received during that daemon process;
- every story receives a process-local ID such as `story-0001`; source paths and state IDs are metadata, not report identity;
- a new process using the same path creates a new timestamp-prefixed report file rather than overwriting older runs.

### 2.7 Activation point

For `start`, resolve and start the reporter after configuration and the target workspace root are known, but before SPEC/template validation, pre-flight checks, provider pings, and product-manager roadmap generation. For `serve`, start it after configuration and workspace-root resolution but before storage/LLM/orchestrator construction. This lets an enabled report capture most startup failures.

A configuration parse failure cannot be reported because the `report` value is not trusted yet. A failure before a project root can be determined also remains ordinary stderr-only behavior. State these limitations in user documentation.

Immediately after `Start`, install one terminal guard. Use an outcome variable initialized to `FAILED`; set it to `SUCCESS` only on normal return, `CANCELLED` on explicit cancellation, and `INTERRUPTED` in the signal/context path. Do not infer success merely because a Cobra callback returned nil during signal shutdown.

### 2.8 Outcome mapping

Process and story outcomes use:

```text
RUNNING | SUCCESS | FAILED | CANCELLED | INTERRUPTED
```

Mapping is deterministic:

- `SUCCESS`: normal process completion and all attempted stories succeeded;
- `FAILED`: a planning, execution, validation, release, storage, or other command error caused normal execution to fail;
- `CANCELLED`: an explicit user story cancellation without process signal interruption;
- `INTERRUPTED`: SIGINT, SIGTERM, parent context cancellation, or forced graceful shutdown;
- `RUNNING`: checkpoint only, never a final status.

For a daemon, a shutdown signal makes the process report `INTERRUPTED` even if earlier story sections contain successes or failures. Those story outcomes remain visible. If several non-signal terminal reasons compete, use `INTERRUPTED > CANCELLED > FAILED > SUCCESS` for the process status and list every underlying story outcome.

## 3. Architecture

### 3.1 Components

The implementation has four separate concerns:

1. **Observation:** shared wrappers emit structured facts at the boundary where work happens.
2. **Aggregation:** a thread-safe reporter projects facts into bounded run/story summaries.
3. **Deterministic analysis:** Go code derives issues, bottlenecks, limitations, and fallback proposals.
4. **Optional model analysis:** one bounded read-only LLM request can prioritize existing facts and add explicitly labeled hypotheses.

```text
LLM / Tool / Sandbox / Git / Scheduler / Orchestrator boundaries
                              │
                              ▼
                    ExecutionObserver
                       ├── Metrics projection
                       └── Reporter collector
                              │
                 deterministic snapshot + analysis
                              │
                  Markdown renderer → atomic writer
                              │
                 optional terminal LLM analyzer
```

The reporter is not a DAG worker. It consumes no generator/tester slot, edits no project file, executes no tool, and changes no domain state.

### 3.2 No-op behavior

Use a real no-op implementation instead of scattered nil checks:

```go
type NoopExecutionReporter struct{}
```

It starts no goroutine and retains no data. The CLI constructs the concrete reporter only after a non-empty path resolves and prepares successfully. Disabled mode must not construct analyzer or writer adapters.

### 3.3 Dependency injection

Do not add another positional argument to the already-large `NewOrchestrator` signature. Introduce a focused runtime dependency object and a compatibility constructor:

```go
type OrchestratorRuntimeDependencies struct {
    Mailbox         *CommandMailbox
    WatchdogRepair  RepairHandler
    PromptRenderer  PromptRenderer
    QA              *QARuntimeCoordinator
    Observer        domain.ExecutionObserver
}

func NewOrchestratorWithRuntime(
    repo domain.StateRepository,
    reg Registry,
    client domain.LLMClient,
    val Validator,
    sched *Scheduler,
    git *GitClient,
    queue *RebaseQueue,
    eval *TestValidator,
    vcsClient domain.VCSClient,
    cfg OrchestratorConfig,
    runtime OrchestratorRuntimeDependencies,
) *Orchestrator
```

The existing constructor may delegate to the new constructor with a no-op observer to keep tests and external callers compiling. New production wiring must use the new constructor. Do not use a global reporter or a mutable post-construction setter.

Other shared components receive the same observer through constructors or backward-compatible functional options:

```go
llm.BuildFailoverClient(cfg, budgetStore, llm.WithObserver(observer))
services.NewObservedToolRegistry(observer)
services.NewObservedSandbox(baseSandbox, observer)
services.NewGitClient(path, services.WithGitObserver(observer))
```

The exact option names may follow package conventions, but every dependency must be injectable and testable.

## 4. Domain contracts

### 4.1 Event vocabulary

Create `pkg/domain/execution_report.go`. It imports only standard-library packages. JSON tags are required because snapshots are sent to the analyzer.

Use one event shape with typed correlation fields. Do not put raw tool arguments, arbitrary environment maps, prompts, or response bodies in `Metadata`.

```go
type ExecutionEventKind string

const (
    EventRunStarted          ExecutionEventKind = "run_started"
    EventRunFinished         ExecutionEventKind = "run_finished"
    EventStoryStarted        ExecutionEventKind = "story_started"
    EventStoryFinished       ExecutionEventKind = "story_finished"
    EventPhaseStarted        ExecutionEventKind = "phase_started"
    EventPhaseFinished       ExecutionEventKind = "phase_finished"
    EventTaskAttemptStarted  ExecutionEventKind = "task_attempt_started"
    EventTaskAttemptFinished ExecutionEventKind = "task_attempt_finished"
    EventAgentStarted        ExecutionEventKind = "agent_started"
    EventAgentFinished       ExecutionEventKind = "agent_finished"
    EventLLMCallFinished     ExecutionEventKind = "llm_call_finished"
    EventToolFinished        ExecutionEventKind = "tool_finished"
    EventSandboxFinished     ExecutionEventKind = "sandbox_finished"
    EventValidationFinished  ExecutionEventKind = "validation_finished"
    EventVCSFinished         ExecutionEventKind = "vcs_finished"
    EventWaitFinished        ExecutionEventKind = "wait_finished"
    EventRetryRecorded       ExecutionEventKind = "retry_recorded"
    EventFindingRecorded     ExecutionEventKind = "finding_recorded"
    EventClarification       ExecutionEventKind = "clarification"
    EventReporterDiagnostic  ExecutionEventKind = "reporter_diagnostic"
)

type EventOutcome string

const (
    OutcomeUnknown   EventOutcome = "UNKNOWN"
    OutcomeSuccess   EventOutcome = "SUCCESS"
    OutcomeFailed    EventOutcome = "FAILED"
    OutcomeBlocked   EventOutcome = "BLOCKED"
    OutcomeCancelled EventOutcome = "CANCELLED"
    OutcomeTimeout   EventOutcome = "TIMEOUT"
    OutcomeSkipped   EventOutcome = "SKIPPED"
)

type ExecutionEvent struct {
    ID                string            `json:"id"`
    SpanID            string            `json:"span_id,omitempty"`
    ParentSpanID      string            `json:"parent_span_id,omitempty"`
    RunID             string            `json:"run_id"`
    StoryID           string            `json:"story_id,omitempty"`
    TaskID            string            `json:"task_id,omitempty"`
    AgentInvocationID string            `json:"agent_invocation_id,omitempty"`
    AgentRole         string            `json:"agent_role,omitempty"`
    Kind              ExecutionEventKind `json:"kind"`
    Category          string            `json:"category,omitempty"`
    Name              string            `json:"name,omitempty"`
    At                time.Time         `json:"at"`
    DurationMillis    *int64            `json:"duration_ms,omitempty"`
    Outcome           EventOutcome      `json:"outcome,omitempty"`
    Attempt           int               `json:"attempt,omitempty"`
    Turn              int               `json:"turn,omitempty"`
    Provider          string            `json:"provider,omitempty"`
    Model             string            `json:"model,omitempty"`
    PromptTokens      *int64            `json:"prompt_tokens,omitempty"`
    CompletionTokens  *int64            `json:"completion_tokens,omitempty"`
    CachedTokens      *int64            `json:"cached_tokens,omitempty"`
    CostUSD           string            `json:"cost_usd,omitempty"`
    UsageKind         string            `json:"usage_kind,omitempty"` // exact | estimated | unknown
    Count             *int64            `json:"count,omitempty"`
    Total             *int64            `json:"total,omitempty"`
    ExitCode          *int               `json:"exit_code,omitempty"`
    LinesAdded        *int64            `json:"lines_added,omitempty"`
    LinesDeleted      *int64            `json:"lines_deleted,omitempty"`
    FilesChanged      *int64            `json:"files_changed,omitempty"`
    ErrorCategory     string            `json:"error_category,omitempty"`
    Evidence          string            `json:"evidence,omitempty"`
    Blocked           bool              `json:"blocked,omitempty"`
    Metadata          map[string]string `json:"metadata,omitempty"`
}
```

Implementation corrections to the sketch above:

- `DurationMillis`, token counts, exit codes, and other measurements that may be unknown use pointer types (`*int64`, `*int`). A `nil` pointer explicitly represents "Unknown / Not measured", whereas a non-nil pointer pointing to `0` represents a measured zero value (e.g., `ExitCode: &0` for a clean exit with exit code 0, or `PromptTokens: &0`).
- `At` is UTC wall time for display. Duration is measured at the source with the injected clock and is never reconstructed from formatted timestamps.
- `SpanID` pairs start and finish events. A finish event carries the authoritative measured duration.
- `AgentInvocationID` identifies one invocation. It must not reuse the current state ID `agent-<role>-<task>` across generator refactor/fix turns.
- accepted `AgentRole` strings include `product_manager`, `planner`, `reader`, `generator`, `tester`, `qa`, `resolver`, `unblocker`, and `reporter`; this report vocabulary must not depend on whether the role is persisted in `domain.Agent`.
- `Name` carries a sanitized display name such as phase, tool, operation, or task title.
- `Count`, `Total`, and `ExitCode` avoid encoding numeric facts as strings.
- `UsageKind` is mandatory (`exact | estimated | unknown`) whenever token/cost pointer fields are non-nil.
- `Metadata` has a small allowlist (`command_class`, `wait_reason`, `validation_run`, `resolution_source`, `severity`, `finding_id`, `task_status`, `available_slots`, `active_workers`, `termination_reason`, `first_turn_success`, `turn_count`, `critical_path_ms`, `cache_hit`). Unknown keys are discarded before storage.

### 4.2 Run and story metadata

```go
type RunMetadata struct {
    RunID         string    `json:"run_id"`
    Command       string    `json:"command"`
    ProjectPath   string    `json:"project_path"`
    ReportPath    string    `json:"report_path"`
    StartedAt     time.Time `json:"started_at"`
    NoctifabVersion string  `json:"noctifab_version"`
}

type StoryMetadata struct {
    StoryID       string    `json:"story_id"`
    Source        string    `json:"source"`
    FeatureName   string    `json:"feature_name"`
    StateID       string    `json:"state_id,omitempty"`
    Sequence      int       `json:"sequence"`
    StartedAt     time.Time `json:"started_at"`
}

type ExecutionOutcome string

const (
    ExecutionRunning     ExecutionOutcome = "RUNNING"
    ExecutionSuccess     ExecutionOutcome = "SUCCESS"
    ExecutionFailed      ExecutionOutcome = "FAILED"
    ExecutionCancelled   ExecutionOutcome = "CANCELLED"
    ExecutionInterrupted ExecutionOutcome = "INTERRUPTED"
)
```

Display `ProjectPath` and `ReportPath` as workspace-relative values when inside the workspace. For an external absolute report, display only a sanitized clean path according to the path-redaction policy; never infer project source by reading the path.

### 4.3 Ports

```go
type ExecutionObserver interface {
    Observe(ctx context.Context, event ExecutionEvent)
}

type ExecutionReporter interface {
    ExecutionObserver
    Start(ctx context.Context, run RunMetadata)
    BeginStory(ctx context.Context, story StoryMetadata)
    EndStory(ctx context.Context, storyID string, outcome ExecutionOutcome)
    Finish(ctx context.Context, outcome ExecutionOutcome)
}

type ReportWriter interface {
    WriteAtomic(ctx context.Context, path string, content []byte) error
}

type ReportAnalyzer interface {
    Analyze(ctx context.Context, input ReportAnalysisInput) (ReportAnalysis, error)
}

type ReportAnalyzerFactory func() ReportAnalyzer

type Clock interface {
    Now() time.Time
}
```

Lifecycle methods intentionally return no error. The concrete reporter records diagnostics and invokes an injected warning sink; orchestration cannot accidentally propagate a reporter failure. Construction and destination preparation may return errors before execution starts, and the CLI converts those errors to soft-disable warnings.

Required constructor:

```go
func NewReporterAgent(
    path string,
    clock domain.Clock,
    writer domain.ReportWriter,
    analyzerFactory domain.ReportAnalyzerFactory,
    warnings io.Writer,
) (*ReporterAgent, error)
```

A nil factory means analyzer creation is unavailable (e.g. LLM client unavailable). A factory avoids a construction cycle: the reporter/observer must exist before the observed LLM router, while the analyzer needs that router. When `execution_report` is enabled, production passes a closure that returns the analyzer after the LLM client has been assigned; `Finish` resolves the closure once. If the factory returns nil, analysis is skipped with a limitation. Tests inject a factory returning a fake analyzer.

### 4.4 Lifecycle state machine

The reporter enforces:

```text
NEW → RUNNING → FINISHED
          ├── begin/end story pairs (one active story at a time in current CLI)
          └── concurrent Observe calls
```

Rules:

- `Start` is accepted once and writes the initial checkpoint.
- duplicate `Start`, unknown `EndStory`, duplicate event ID, unmatched span finish, and events after `Finish` become bounded reporter diagnostics; they do not panic.
- `Finish` is idempotent. The first terminal outcome wins.
- a duplicate event ID is ignored after the first occurrence.
- when `ID` is empty, the collector assigns `event-000001` under its mutex. Sequence IDs are stable within a report, not across separate runs.
- all final sorts use explicit rank, then scoped IDs, then event ID; map iteration order must never affect Markdown.

## 5. Observation and instrumentation

### 5.1 Correlation context

Add typed context helpers rather than string keys:

```go
ctx = domain.WithExecutionCorrelation(ctx, domain.ExecutionCorrelation{
    RunID: runID, StoryID: storyID, TaskID: taskID,
    AgentInvocationID: invocationID, AgentRole: "generator",
})
```

Wrappers read correlation from context. Context contains IDs only, never credentials or report snapshots.

### 5.2 Required boundaries

| Boundary | Where to instrument | Required facts |
| :--- | :--- | :--- |
| process | `start` and `serve` outer lifecycle | command, run ID, start/end, termination reason |
| pre-flight | provider ping and deterministic checks | category, duration, provider name, success/error |
| product manager | `GenerateRoadmap` call and LLM role | duration, retries, outcome |
| story | around each story closure in `start`; `processStory` in `serve` | source, sequence, final outcome |
| planning | `PlanStory` | attempts, duration, task count, error |
| scheduler | `RunOnce` plus scheduler decision | available slots, ready count, dependency/file-lock/clarification/slot wait |
| task attempt | `executeTask` | task, attempt, start/end, status, downstream impact |
| agent invocation | reader/generator/tester/QA/unblocker entry and exit | invocation ID, role, task, turns, outcome |
| LLM logical call | `ResilientLLMRouter.Complete` | `Category=logical`, role, total latency, selected/failing candidates, outcome; no token aggregation |
| LLM physical call | provider client retry loop | `Category=provider_attempt`, provider/model, attempt number, latency, usage on the successful response if returned |
| policy/tool | validator and observed `Tool.Execute` wrapper | tool, blocked/executed, duration, sanitized evidence |
| sandbox | observed `Sandbox.RunCommand` wrapper | command class only, duration, timeout, exit classification |
| validation | `TestValidator` formatter/linter/test runs | run number, pass/fail, majority/flaky result |
| QA/SAST | QA coordinator and scanner | finding ID, severity, disposition, duration |
| VCS | `GitClient.Run`, rebase queue, VCS API | operation (`commit`, `merge`, etc.), duration, conflict/retry/result, files changed, lines added/deleted |
| code churn | file tools & git diff inspection | files created/modified/deleted, lines added (`+`), lines deleted (`-`) per task |
| self-correction | generator/tester fix-loop transitions | turn count per task, first-turn success flag (`first_turn_success=true`), retry churn count |
| prompt cache | provider client usage response | `CachedTokens` (prompt cache hits vs non-cached tokens) |
| coordination | OCC loop, mailbox, package lock, file locks | wait reason/count/duration |
| clarification | open/resolved/timeout | ID, wait duration, source; do not store answer text by default |
| release | `FinalizeUserStory` | version/changelog/PR phase outcome and duration |
| shutdown | signal handling and cleanup | interrupted task count, cleanup and final persistence duration |

### 5.3 Shared-wrapper requirement

Instrumentation must be added once per common boundary:

- decorate `Tool` objects at registry registration;
- decorate the `Sandbox` passed to tools, validator, and repair handlers;
- add an observer option to `GitClient` and re-use it for worktree clients;
- emit physical provider observations from the provider client and logical routing observations from the router; aggregate usage only from successful physical responses so the logical event does not double-count it;
- use an observer-aware scheduler result rather than trying to infer lock waits later.

Do not time only `RunGeneratorAgent` and call that “LLM time”; it also contains file tools, tests, and nested tester requests.

### 5.4 Scheduler diagnostics

The current `GetReadyTasks` returns only selected tasks, so it cannot explain why other tasks waited. Add a diagnostic result while preserving a compatibility method:

```go
type ScheduleDecision struct {
    ReadyTaskIDs       []string
    ActiveWorkers      int
    AvailableSlots     int
    BlockedByDependency []string
    BlockedByFileLock  []string
    BlockedByClarification []string
    BlockedByWorkerSlot []string
}

func (s *Scheduler) Decide(state *domain.State, limit int) ScheduleDecision
```

`GetReadyTasks` may delegate to `Decide`. The orchestrator tracks when each `(storyID, taskID, reason)` wait begins and emits one `wait_finished` event when the reason clears or the story ends. Poll count multiplied by poll interval is not an acceptable wait-time measurement.

### 5.5 Timing pattern

Use source-boundary timing and emit on every return path:

```go
started := clock.Now()
out, err := base.RunCommand(ctx, projectPath, command, pkg)
duration := clock.Now().Sub(started)
observer.Observe(ctx, sandboxEvent(duration, err, classifyCommand(command)))
return out, err
```

Production clocks use UTC `time.Now`. Fake clocks advance explicitly. Clamp a negative fake/source duration to zero and add a `clock_anomaly` limitation; do not emit a negative duration.

### 5.6 LLM usage limitations

The current `LLMResponse` does not expose token usage. Implement provider parsing where response protocols provide usage, and leave pointers nil otherwise. Estimated tokens must be labeled `estimated`; they must not be rendered as exact provider usage.

Likewise, do not derive per-agent USD cost from `State.Metadata.TotalCostUSD`. Use a decimal string only when a provider/pricing component supplies attributable cost. Otherwise render `Not measured`. Cost arithmetic uses `math/big.Rat` or an equivalent decimal type, never `float64`.

### 5.7 Metrics compatibility

Create one event fan-out:

```text
ExecutionObserver
  ├── ReporterAgent
  └── MetricsProjection (adapts events into MetricsCollector)
```

Refactor `MetricsCollector` to accept an injected clock and expose a copied snapshot. The reporter may consume that snapshot for legacy aggregate fields, but event-derived values are authoritative for new dimensions. There must not be two independently started “total execution” timers.

Populate `MetricsEnabled` from `cfg.Telemetry.Metrics.IsEnabled()` in both CLI paths. Reporting remains enabled independently even when metrics export or OpenTelemetry is disabled.

## 6. Aggregation, checkpoints, and memory bounds

### 6.1 Active versus elapsed time

The report distinguishes:

- execution wall time: run start through original project outcome, before analyzer work;
- report finalization time: original project outcome through the final immutable snapshot immediately before the terminal atomic write;
- observed process wall time: execution wall plus report finalization time;
- report rendering, deterministic analysis, analyzer, and prior checkpoint-write time;
- per-phase interval-union duration;
- summed operation/agent time, explicitly labeled as overlapping;
- active agent time;
- LLM network wait;
- tool/sandbox/validation/VCS time;
- dependency, lock, worker-slot, mailbox, and OCC wait.

For each phase, merge overlapping intervals of the same phase before computing its wall-time ratio. Summed agent or operation durations may exceed 100% of process wall time because workers overlap.

Example:

```text
Generator A active: 10s  ──────────
Generator B active: 10s      ──────────
Process wall time: 15s
Summed generator time: 20s (overlapping)
```

The report must say `15s wall; 20s summed concurrent activity`, never `20s total execution`.

A file cannot truthfully contain the duration of its own final write and directory sync without performing another unmeasured write. Therefore the terminal report includes all prior checkpoint-write durations and measures finalization up to the immutable pre-write snapshot. It lists terminal atomic-write duration as `Not representable in the same artifact` in Reporter Diagnostics. Do not use a recursive second “final” write or pretend this duration is zero.

### 6.2 Required snapshot data

At minimum retain:

- run metadata, execution end time, final snapshot time, latest successful checkpoint-write time, and outcome;
- each story's metadata, phase intervals, outcome, and task summary;
- task attempts, terminal status, elapsed/active/wait duration, retries, and bounded failure evidence;
- every observed agent invocation, including incomplete invocations;
- role aggregates;
- tool, sandbox, validation, LLM, VCS, and wait aggregates;
- exact/estimated/missing usage provenance;
- issue, bottleneck, proposal, and limitation records;
- dropped-event and reporter-error counts.

### 6.3 Memory policy

Constants for the first implementation:

```text
max ordinary flush interval       5 seconds
writer operation timeout      2 seconds
terminal shutdown budget      5 seconds
analyzer timeout             10 seconds
max evidence per event     4,096 bytes
max analyzer input        65,536 bytes
max analyzer output       32,768 bytes
max retained ordinary process events   1,000
max retained ordinary events per active story 10,000
```

Terminal events, issue-producing events, task outcomes, and lifecycle events are never dropped. After a story ends, reduce raw events into immutable bounded story summaries and discard ordinary raw events. A daemon therefore uses `O(active-story events + number of story summaries)` memory, not `O(all historical events)` raw memory.

If the ordinary-event cap is reached, continue aggregate counters, increment `dropped_ordinary_events`, and add a limitation. Do not claim complete per-call evidence afterward.

### 6.4 Checkpoint policy

Write an atomic checkpoint:

- immediately after `Start`;
- after product-manager/planning completion;
- after every task terminal outcome;
- after every issue-producing terminal event;
- after every story terminal outcome;
- no more than once every five seconds for ordinary dirty state;
- once during terminal `Finish`.

`Observe` only updates in-memory state under a mutex; it does not perform file I/O. One flush goroutine handles periodic writes. Terminal lifecycle calls request a synchronous bounded flush so terminal events are not lost behind ordinary events.

`Finish` must never hold the collector mutex while calling the analyzer or writer. Freeze/copy the deterministic snapshot under lock, release the lock, run bounded external work, stop and join the periodic flusher, then render and write the terminal snapshot. Analyzer self-observation is appended through a dedicated reporter-diagnostic path after the analyzer input has been frozen.

If the writer fails, open a process-local writer circuit breaker:

- warn once;
- retain only bounded in-memory diagnostics needed for a possible later terminal retry;
- do not busy-loop on every flush tick;
- allow one final retry during `Finish`;
- skip the model analyzer if no valid report checkpoint can be written.

A hard crash may leave the latest complete checkpoint with status `RUNNING`. It must contain `Checkpoint: yes` and `Run incomplete: no terminal event observed`.

## 7. Deterministic bottleneck analysis

LLM involvement is not required to identify bottlenecks. Use these fixed first-version rules and print the matching rule in the report.

### 7.1 Statistical definitions

- duration values are integer milliseconds;
- percentiles use nearest-rank on sorted values: rank `ceil(p × n)`, minimum rank 1;
- do not render p50/p95 when fewer than 5 samples exist; render count/min/max instead;
- phase wall ratio = merged interval-union duration / execution wall duration;
- operation active ratio = summed operation duration / execution wall duration and is labeled potentially overlapping;
- a performance rule requiring a ratio is skipped when execution wall time is zero or unknown.

### 7.2 Rules

Create a bottleneck candidate when any rule matches:

| Rule ID | Condition |
| :--- | :--- |
| `BN-PHASE-DOMINANT` | phase union duration ≥ 1,000 ms and ≥ 30% of execution wall time |
| `BN-OP-DOMINANT` | operation-class summed duration ≥ 1,000 ms and ≥ 20% of execution wall time |
| `BN-LATENCY-OUTLIER` | at least 5 samples, p95 ≥ 2 × p50, and p95 − p50 ≥ 500 ms |
| `BN-FAILURE-RATE` | at least 3 calls and failure/timeout rate ≥ 20% |
| `BN-RETRY` | same task, provider, VCS operation, or validation scope retried at least twice |
| `BN-CONTENTION` | measured wait ≥ 10% of wall time, or one wait ≥ 5,000 ms while a worker slot was available |
| `BN-TIMEOUT` | any watchdog, command, provider, or analyzer timeout |
| `BN-IDLE-CAPACITY` | available worker slots existed for ≥ 10% of execution while pending tasks were blocked |
| `BN-TOKEN` | one role uses ≥ 50% of measured tokens and at least 10,000 tokens total were measured |

A long successful test suite can be a performance observation; a repeated timeout is ranked higher as a reliability bottleneck. Ranking order is:

1. blocked execution;
2. timeout or repeated failure;
3. contention/retry;
4. dominant wall-time rule;
5. token inefficiency;
6. informational observation.

Within a rank, sort by measured impact descending and then stable scope ID.

## 8. Issues and deterministic proposals

### 8.1 Issue schema

```go
type EvidenceRef struct {
    EventID string `json:"event_id"`
    Excerpt string `json:"excerpt,omitempty"`
}

type ReportIssue struct {
    ID                string        `json:"id"`
    Category          string        `json:"category"`
    Severity          string        `json:"severity"`
    Kind              string        `json:"kind"` // confirmed | observation | hypothesis
    Title             string        `json:"title"`
    Behavior          string        `json:"behavior"`
    Impact            string        `json:"impact"`
    Evidence          []EvidenceRef `json:"evidence"`
    StoryID           string        `json:"story_id,omitempty"`
    TaskID            string        `json:"task_id,omitempty"`
    AgentInvocationID string        `json:"agent_invocation_id,omitempty"`
    Phase             string        `json:"phase,omitempty"`
    Scope             string        `json:"scope"` // noctifab | configuration | generated_project | environment | unknown
    AffectedComponent string        `json:"affected_component,omitempty"`
    Blocked           bool          `json:"blocked"`
    Confidence        string        `json:"confidence"`
    ProposedAction    string        `json:"proposed_action"`
}

type ReportBottleneck struct {
    ID          string        `json:"id"`
    Rank        int           `json:"rank"`
    RuleID      string        `json:"rule_id"`
    Scope       string        `json:"scope"`
    Measurement string        `json:"measurement"`
    Impact      string        `json:"impact"`
    Evidence    []EvidenceRef `json:"evidence"`
}

type ReportProposal struct {
    ID           string   `json:"id"`
    IssueIDs     []string `json:"issue_ids"`
    Scope        string   `json:"scope"`
    Action       string   `json:"action"`
    Components   []string `json:"components,omitempty"`
    Verification string   `json:"verification"`
}

type AnalysisPriority struct {
    IssueID string `json:"issue_id"`
    Rank    int    `json:"rank"`
    Reason  string `json:"reason"`
}

type AnalysisHypothesis struct {
    ID         string `json:"id"`
    IssueID    string `json:"issue_id"`
    Statement  string `json:"statement"`
    Confidence string `json:"confidence"`
}

type ReportAnalysisInput struct {
    RunID              string             `json:"run_id"`
    Outcome            ExecutionOutcome   `json:"outcome"`
    ExecutionWallMS    *int64             `json:"execution_wall_ms,omitempty"`
    DeterministicIssues []ReportIssue      `json:"deterministic_issues"`
    Bottlenecks        []ReportBottleneck `json:"bottlenecks"`
    Limitations        []string           `json:"limitations"`
}

type ReportAnalysis struct {
    Summary    string               `json:"summary"`
    Priorities []AnalysisPriority   `json:"priorities"`
    Hypotheses []AnalysisHypothesis `json:"hypotheses"`
    Proposals  []ReportProposal     `json:"proposals"`
}
```

Categories:

- `functional`: contract, test, validation, merge, or task outcome failure;
- `performance`: dominant latency, low throughput, contention, or token inefficiency;
- `security`: SAST result, sandbox attempt, unsafe path, or redaction event;
- `configuration`: malformed/incompatible setting or disabled capability;
- `quality`: lint, flaky test, incomplete evidence, or repeated self-correction;
- `operational`: timeout, interruption, storage, dependency, provider, or VCS availability failure;
- `unknown`: insufficient evidence to classify safely.

Kinds and scopes are mandatory. A measurement alone is an `observation`, not proof of a defect. LLM-only statements are `hypothesis`. Scope attribution prevents a generated-project test defect or unavailable host dependency from being mislabeled as a noctifab implementation defect. If evidence is insufficient, use `unknown`; do not guess a target language/framework.

### 8.2 Severity mapping

Use source severity when a structured SAST/QA finding supplies one. Otherwise:

| Evidence | Severity |
| :--- | :--- |
| secret exposure detected in persisted output, unsafe merge accepted, or integrity loss | critical |
| terminal task/story failure, max-retry exhaustion, merge/rebase conflict blocking work, storage failure | high |
| timeout, repeated provider/VCS failure, flaky majority result, sustained lock contention | medium |
| one recovered failure, one blocked sandbox attempt, non-blocking lint issue | low |
| missing measurement or successful but dominant operation | info |

A sandbox block normally proves the control worked; report the attempted action and blocked impact without claiming a vulnerability.

### 8.3 Required deterministic findings

Generate issues before analyzer invocation for:

- planning exhaustion or invalid task DAG;
- terminal task/story failure and downstream blocked task count;
- retries and max-retry exhaustion;
- validation failure, no-tests condition, and flaky majority vote;
- watchdog absolute/idle timeout;
- policy and sandbox block;
- SAST and QA findings with disposition;
- merge/rebase failure, including inconsistent `TaskSuccess` followed by failed merge-back;
- OCC retry exhaustion and state persistence failure;
- provider auth/credit/rate-limit/failover exhaustion;
- missing exact token/cost measurements;
- interrupted agents/tasks;
- dropped events, unmatched spans, writer failure, analyzer failure, and clock anomalies.

Stable issue IDs use `ISSUE-<HASH>`, where `HASH` is the first ten uppercase hexadecimal characters of SHA-256 over a normalized semantic key: category, source kind/rule, story sequence, task ID, agent invocation ID, phase, structured source finding/error category, and normalized title key. Do not hash timing values, raw evidence, run ID, or arrival-order event ID. On the unlikely collision, extend both colliding IDs by two hash characters until unique. This keeps an issue ID stable when unrelated findings are added or concurrent event arrival order changes. Evidence references event IDs and bounded excerpts.

### 8.4 Fallback proposals

Each confirmed issue or observation receives at least one deterministic recommendation. Recommendations are not confirmed fixes.

| Trigger | Recommendation |
| :--- | :--- |
| repeated LLM retries | inspect provider health, timeout, retry backoff, key credit, and failover order |
| high LLM latency | compare provider/model p50/p95 and consider a lower-latency role route |
| test timeout | inspect test scope, sandbox max/idle timeout, subprocess output, and safe test parallelism |
| lock contention | reduce overlapping target files or improve task decomposition/file-lock granularity |
| dependency idle time | inspect DAG dependencies and split unnecessary serial prerequisites |
| OCC conflicts | route writes through the mailbox and shorten state transactions |
| flaky validation | isolate nondeterminism and require reproducible evidence or strict validation |
| merge conflict | refine task target files, worktree synchronization, and serialized integration |
| token concentration | reduce pinned context, cache reads, and incremental diagnostic duplication |
| linter churn | run deterministic formatting before lint and avoid repeated unchanged lint attempts |
| missing telemetry | add observation at the named common boundary; do not estimate zero |

Proposals use the same semantic-hash scheme with prefix `PROP-`, based on sorted linked issue IDs plus normalized action and verification keys. They link one or more issue IDs, identify likely noctifab components/files when known, and include a verification action.

## 9. Markdown report contract

The file is UTF-8, uses LF line endings, ends with exactly one newline, and has deterministic section and row ordering. Timestamps are RFC3339Nano UTC. Human durations include an exact integer millisecond value in the same row or adjacent detail. Monetary values are fixed decimal strings.

Required top-level order:

```markdown
# Noctifab Execution Report

> Status: RUNNING|SUCCESS|FAILED|CANCELLED|INTERRUPTED
> Run ID: <id>
> Checkpoint: yes|no

## Executive Summary
## Live Status
## Run Metadata
## Outcome
## Time Spent
## Agent Performance
## Phase Performance
## Code Churn and Workspace Impact
## Self-Correction and Turn Efficiency
## Bottlenecks
## Issues Found
## Proposals and Next Actions
## User Story and Task Results
## LLM, Token, and Cost Usage
## Reliability and Concurrency
## Evidence and Limitations
## Reporter Diagnostics
```

### 9.1 Content invariants

- status, run ID, and checkpoint marker occur in the first ten lines;
- `Live Status` has one stable row with current activity, stories completed/planned, tasks completed/total, validation passes/runs, errors, retries, measured tokens, elapsed time, last meaningful progress time, last event time, provider/failover summary, and deterministic stuck state;
- total execution wall time appears even if no story or task completed;
- observed process time and report/analyzer overhead are separate;
- terminal atomic-write time is explicitly identified as not representable in the artifact itself, not reported as zero;
- every observed agent invocation appears, including invocations with no finish event;
- every terminal task appears with scoped story ID, attempt count, status, elapsed time if measured, and evidence on failure;
- summed overlapping time is labeled;
- every bottleneck includes rule ID, measured values, impact, and evidence;
- every issue says `confirmed`, `observation`, or `hypothesis`;
- proposals link issue IDs, identify scope (`noctifab`, `configuration`, `generated_project`, `environment`, or `unknown`), and include a verification step;
- unknown values render `Not measured`, never `0`;
- empty sections render `None observed` or `Not measured`;
- Markdown table cells escape `|`, backslash, CR/LF, and unsafe HTML;
- raw model Markdown/HTML is treated as text, not trusted document structure.

### 9.2 Live validation/operations status

The checkpoint must make the fields currently inferred from validation logs directly visible:

```markdown
## Live Status

| Status | Current Activity | Stories | Tasks | Tests | Errors | Retries | Tokens | Elapsed | Last Progress | Last Event | Provider / Failovers | Stuck? |
| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | :--- | :--- | :--- | :--- | :---: |
| RUNNING | validating task-02 | 1/3 | 4/7 | 8/9 | 2 | 1 | 14,200 measured | 04m 12s | 8s ago | 2s ago | openai/gpt-4o; 1 failover | No |
```

Definitions:

- `Current Activity` comes from the most recent active phase/task/agent event, not a log tail.
- `Last Progress` changes only on meaningful progress: story/task terminal transition, successful commit/merge, validation improvement, or resolved blocker. Routine poll/heartbeat events do not reset it.
- `Last Event` is the most recent structured event.
- `Stuck? = Yes` when no meaningful progress event occurred for more than five minutes, or one normalized error fingerprint occurred at least three times without intervening progress. The matching reason appears in Reliability and Concurrency.
- Before a total is known, render `?/` notation such as `0/?`, not zero total.
- `Tests` means structured validation runs, not a guessed count parsed from stdout.

This section allows humans and the validation monitor to inspect a small checkpoint instead of reading an active container log. It is equally useful for ordinary local projects and daemons.

### 9.3 Example final excerpt

```markdown
# Noctifab Execution Report

> Status: FAILED
> Run ID: run-7d8f0c
> Checkpoint: no

## Executive Summary

Execution failed after 42.310s (42310 ms). Two of three tasks succeeded.
Validation retries and sandbox test time were the largest measured constraints.

## Agent Performance

| Invocation | Role | Story | Task | Active | LLM | Tools | Waiting | Turns | Outcome |
| :--- | :--- | :--- | :--- | ---: | ---: | ---: | ---: | ---: | :--- |
| agent-0003 | generator | story-0001 | task-02 | 12.430s (12430 ms) | 7.100s | 4.900s | Not measured | 3 | SUCCESS |

## Bottlenecks

| Rank | Rule | Scope | Measurement | Impact | Evidence |
| ---: | :--- | :--- | :--- | :--- | :--- |
| 1 | BN-RETRY | task-02 validation | 2 retries; 9.2s summed | delayed dependent task-03 | event-0042, event-0068 |

## Issues Found

| ID | Kind | Category | Severity | Issue | Impact | Evidence |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| ISSUE-F13A09C2D4 | confirmed | functional | high | task-03 exhausted validation retries | story failed | event-0091 |

## Proposals and Next Actions

| ID | Issues | Scope | Recommendation | Verification |
| :--- | :--- | :--- | :--- | :--- |
| PROP-7C21B840A1 | ISSUE-F13A09C2D4 | noctifab | inspect the validator failure classification and retained failure evidence | reproduce with the recorded validation command in an isolated workspace |
```

## 10. Reporter Analyzer LLM step

### 10.1 Why a special adapter is required

`domain.LLMClient` returns the tool-action envelope, not arbitrary report-analysis JSON. The analyzer adapter therefore asks for exactly one virtual action named `submit_report_analysis`. It parses the response but never registers or executes that action as a project tool.

Use role context `reporter`; the existing router may fall back to global provider candidates. No new project permission profile is needed because no registry or executor is involved.

### 10.2 Input

The input is a redacted deterministic JSON snapshot capped at 65,536 bytes. Never truncate serialized JSON bytes in place. Drop or summarize the lowest-priority records, re-marshal, and repeat until the complete JSON document fits. Preserve in this order when reducing:

1. run/story outcomes;
2. deterministic issues and evidence references;
3. bottleneck ranking and rules;
4. phase, agent, retry, timeout, and failure aggregates;
5. usage and limitations;
6. informational successful operations.

Include no prompts, response bodies, source files, tool arguments, environment values, or raw full logs.

Example prompt payload contract:

```json
{
  "run_id": "run-7d8f0c",
  "outcome": "FAILED",
  "execution_wall_ms": 42310,
  "deterministic_issues": [
    {
      "id": "ISSUE-F13A09C2D4",
      "kind": "confirmed",
      "category": "functional",
      "severity": "high",
      "title": "task-03 exhausted validation retries",
      "evidence_event_ids": ["event-0091"]
    }
  ],
  "bottlenecks": [
    {"rule": "BN-RETRY", "scope": "task-02", "retry_count": 2}
  ],
  "limitations": ["exact completion tokens not returned by provider"]
}
```

### 10.3 Output envelope

```json
{
  "reasoning": "Prioritize confirmed execution blockers before performance observations.",
  "actions": [
    {
      "tool": "submit_report_analysis",
      "args": {
        "summary": "Validation retry exhaustion caused the terminal failure.",
        "priorities": [
          {"issue_id": "ISSUE-F13A09C2D4", "rank": 1, "reason": "blocked the story"}
        ],
        "hypotheses": [
          {
            "id": "HYP-001",
            "issue_id": "ISSUE-F13A09C2D4",
            "statement": "Failure classification may be hiding a deterministic setup error.",
            "confidence": "low"
          }
        ],
        "proposals": [
          {
            "issue_ids": ["ISSUE-F13A09C2D4"],
            "scope": "noctifab",
            "action": "inspect validator classification before increasing retries",
            "verification": "replay the captured validation category with a fake sandbox"
          }
        ]
      }
    }
  ]
}
```

Validation rules:

- exactly one action;
- exact tool name `submit_report_analysis`;
- no unknown deterministic issue IDs;
- ranks are unique positive integers;
- confidence is `high`, `medium`, or `low`;
- proposal scope is one of the five deterministic scope values and cannot override an issue's existing scope;
- all strings are bounded and redacted;
- model output cannot alter measurements, outcomes, category, severity, scope, or confirmed issue text;
- new model statements are always hypotheses;
- invalid entries are dropped; if the whole response is invalid, deterministic output remains.

### 10.4 Failure and recursion behavior

When `execution_report` is enabled, the analyzer is attempted once during `Finish`, after project execution end time is fixed. An enabled analyzer uses the normal budget guard. If unavailable, over budget, timed out, malformed, or failed:

- do not retry at reporter level (provider internals may use their configured retries);
- add one analyzer limitation;
- render deterministic proposals;
- preserve the project outcome and exit code.

The analyzer's own LLM event goes only to `Reporter Diagnostics` after the analysis input snapshot is frozen. This prevents self-observation recursion.

## 11. Atomic writer and security

### 11.1 Atomic replacement

The filesystem writer must:

1. verify context before each stage;
2. `Lstat` the destination and reject directory/symlink targets;
3. create a random temporary file in the destination directory using exclusive create and mode `0600`;
4. write all bytes, `Sync`, and close;
5. ensure mode `0600`;
6. rename over the destination on supported POSIX filesystems;
7. sync the parent directory after rename;
8. remove the temporary file on every pre-rename failure.

Never remove the old destination before rename; that would create a missing/partial window. The implementation targets the repository's Linux/macOS environments. Cross-device rename cannot occur because the temporary file is in the destination directory.

### 11.2 Redaction pipeline

Redact twice:

1. before evidence enters the collector;
2. immediately before Markdown or analyzer JSON rendering.

Extend or extract the existing `SanitizeLog` logic into an injected redactor shared by logs and reports. Cover:

- `Authorization: Bearer ...` and basic auth;
- common `*_API_KEY`, `*_TOKEN`, `PASSWORD`, `SECRET`, and configured secret-source names;
- OpenAI/GitHub/GitLab-style key prefixes;
- URL userinfo and sensitive query parameters;
- JSON/YAML key-value forms;
- multiline headers.

Example:

```text
Input:  Authorization: Bearer abc123
Output: Authorization: Bearer [REDACTED_SECRET]

Input:  OPENAI_API_KEY=sk-example
Output: OPENAI_API_KEY=[REDACTED_SECRET]
```

Never load `secrets.yaml` to build the report. The configuration loader may provide secret **environment variable names**, not values, to the redactor. Tests use fake tokens only.

Truncate by UTF-8-safe bytes after redaction and append `… [truncated, original bytes=N]`. Do not split a rune.

### 11.3 Evidence policy

Allowed evidence:

- event IDs;
- category and bounded sanitized error tail;
- command class such as `test`, `lint`, `build`, or `git merge`;
- exit code and timeout classification;
- file path/line from structured linter or SAST output when already part of a finding.

Disallowed by default:

- full shell command strings;
- arbitrary tool argument maps;
- full stdout/stderr;
- source file content;
- LLM prompts/reasoning/responses;
- headers and environment variables;
- clarification answers.

## 12. Validation harness consumption (adapter only)

This section changes the validation harness so it consumes the generic product artifact. None of these paths, project names, or target checks may appear in reporter/domain/service packages.

### 12.1 Why direct host persistence is required

Validation containers run with `--rm`. In `validation/validate.sh`, `set -e` can terminate immediately when `noctifab start` fails, before the temporary workspace is copied to `/app/src_mount`. A report stored only at `.noctifab/reports/` would then disappear with the container—the exact failure case where it is most valuable.

`validation/run_one.sh` must therefore:

1. create `validation/projects/<project>/output/report/` on the host;
2. clear stale report artifacts before launch;
3. bind-mount that directory read/write at `/app/report_mount`;
4. leave raw combined stdout/stderr capture in `output/log/` as fallback;
5. never mount secrets into the report directory.

Each checked-in validation project config opts in with the same container path:

```yaml
execution_report: "/app/report_mount/execution_report.md"
```

These are static fixture changes made as part of feature implementation. `validate.sh` must not patch `.noctifab/config.yaml` at runtime. This preserves the configuration-immutability mandate. No validation roadmap file is added or edited.

Because the destination is a direct host bind mount, the initial and periodic atomic checkpoints survive noctifab failure, `validate.sh` early exit, and container removal. Add a Linux/Docker integration test for atomic rename on this bind mount; do not assume host and container filesystems behave identically.

### 12.2 Separate noctifab outcome from harness verdict

Noctifab finishes before `validate.sh` checks expected generated artifacts and may therefore report `SUCCESS` while the outer black-box harness later returns `FAIL`. Do not rewrite noctifab's report or mislabel its outcome.

Add a tiny structured harness result written to the same mount, for example `harness_result.json`:

```json
{
  "schema_version": "noctifab.validation-result/v1",
  "project": "echo",
  "container_exit_code": 1,
  "stage": "artifact_check",
  "verdict": "FAIL",
  "expected_artifacts": ["cmd/echo/main.go"],
  "missing_artifacts": ["cmd/echo/main.go"]
}
```

`validate.sh` writes this atomically from an `EXIT` trap so early errors still identify the failed harness stage and the trap preserves the original exit code. Enumerate `stage` as `setup`, `git_init`, `noctifab`, `artifact_check`, `build_export`, or `complete`; unknown values are rejected by the collector. It contains no credentials or log tail. SIGKILL cannot run a trap, so a missing harness result with a live/last execution checkpoint is reported as `harness result unavailable`, not guessed from logs.

Update `gen_feedback.py` (or replace it with a focused structured collector) to support both the new flag-based signature and backward-compatible positional arguments:

```bash
python3 validation/gen_feedback.py \
  --execution-report <output/report/execution_report.md> \
  --harness-result <output/report/harness_result.json> \
  --log-fallback <output/log/project.log> \
  --output <output/feedback/PROJECT_FEEDBACK.md>
```

It must prefer:

1. `harness_result.json` for outer verdict/artifact checks;
2. `execution_report.md` for noctifab phases, issues, bottlenecks, retries, tests, usage, and proposals;
3. raw log parsing only as a clearly labeled legacy fallback when the embedded report is missing.

The resulting `<PROJECT>_FEEDBACK.md` must link or copy bounded sections from the execution report and clearly show both `Noctifab outcome` and `Validation harness verdict`. It must not ask an LLM to interpret the raw log.

### 12.3 Monitoring running containers

`run_one.sh`/`run_all.sh` and the 60-second monitor may read the host-mounted report's `Live Status` section and file modification time. The report supplies current activity, completion, validation runs, errors, retries, token usage, provider/failover, elapsed time, and stuck reason. Wrapper logs remain the source only for Docker build/launch/exit failures that happen before noctifab starts.

Required host artifacts become:

```text
validation/projects/<project>/output/
├── report/
│   ├── execution_report.md
│   └── harness_result.json
├── feedback/<PROJECT>_FEEDBACK.md
├── log/<project>.log
├── src/
└── dist/
```

Update `validation/README.md` and `validation/projects/TESTING_GUIDE.md` so the execution report is the primary noctifab diagnostic source and the raw log is explicitly fallback-only.

### 12.4 Generality guard tests

Add a dependency/source guard asserting production packages under `pkg/domain`, `pkg/services/reporting`, and `pkg/infrastructure/reportfs` contain no imports or runtime branches referencing `validation/`, project names, `/app/report_mount`, `NOCTIFAB_E2E`, or language-specific behavior. Only files under `validation/` may know the harness mount and project artifact matrix.

## 13. File and package plan

Keep every Go file below 500 physical lines.

### Domain

- `pkg/domain/execution_report.go`: events, metadata, outcomes, observer/reporter ports.
- `pkg/domain/report_analysis.go`: issue, bottleneck, proposal, analyzer input/output types.
- adjacent black-box unit tests.

### Configuration and filesystem infrastructure

- `pkg/infrastructure/config/report_path.go`: pure resolution and protected-path checks.
- `pkg/infrastructure/reportfs/destination.go`: injected destination preparation.
- `pkg/infrastructure/reportfs/atomic_writer.go`: atomic file writer.
- tests for paths, cleanup, symlinks, replacement, final newline, and injected errors.

### Reporting service

Use `pkg/services/reporting/` to avoid growing the already-large `services` files:

- `collector.go`: lifecycle and concurrency-safe projection;
- `snapshot.go`: immutable copied snapshots and interval merging;
- `redactor.go`: bounded evidence and central redaction;
- `bottlenecks.go`: fixed rules;
- `issues.go`: issue normalization and fallback proposals;
- `renderer.go` plus focused table helpers;
- `agent.go`: checkpoint loop, writer circuit breaker, finalization;
- `llm_analyzer.go`: virtual-action adapter and strict validation;
- public-behavior tests in the same package.

### Observation adapters

- `pkg/services/execution_observer.go`: no-op/fan-out and correlation helpers if not in domain;
- observed tool registry/decorator;
- observed sandbox decorator;
- Git observer option;
- LLM client/router observer option under `pkg/infrastructure/llm`;
- scheduler diagnostic decision and wait tracking;
- MetricsCollector event projection.

### CLI

- split `cmd/noctifab/cli/start.go` (currently 547 lines) into `cmd/noctifab/cli/start.go` (<300 lines, Cobra setup & flags) and `cmd/noctifab/cli/start_runner.go` (<400 lines, workspace verification, roadmap generation, story execution loop) before adding report wiring, ensuring all files comply with the 500-line constraint in AGENTS.md;
- keep `serve.go` below 500 by extracting reporter/runtime construction and signal finalization;
- create reporter once outside the `start` story loop and once outside the `serve` server loop;
- defer a terminal finish guard immediately after successful reporter start so all ordinary returns finalize it;
- use the command/signal context instead of new `context.Background()` values for reportable execution boundaries;
- use a separate bounded background context only for final shutdown writes.

## 14. Implementation stories

Each story below is independently compilable and tested. Do not begin a later story while earlier public contracts are failing.

### US-ER-001: Configuration, path safety, and soft disable

**Goal:** recognize `execution_report`, resolve it safely, prepare its destination, and preserve all existing behavior when disabled or invalid.

**Public API:**

```go
func ResolveReportPath(projectPath, configured string) (string, bool, error)
func PrepareReportDestination(ctx context.Context, path string, policy ReportDestinationPolicy, fs FileSystem) error
```

**Acceptance:**

- **when** the key is omitted or whitespace, **it** returns disabled without filesystem calls;
- **when** a valid `.noctifab/reports/...` relative path is supplied, **it** resolves from the project root and formats the filename basename as `YYYYMMDD_HHMMSS_<folder_name>.md`;
- **when** an external absolute path is supplied, **it** remains absolute and formats the filename basename as `YYYYMMDD_HHMMSS_<folder_name>_<filename>`;
- **when** a path targets source, `.git`, config, DB, PID, or secrets, **it** soft-disables with the exact warning prefix;
- **when** probe creation fails, **it** cleans only directories it created;
- **when** YAML contains a non-string report value, **it** remains a strict parse error;
- **when** report is invalid, **it** preserves stdout and the original exit code.

### US-ER-002: Domain events, no-op observer, and bounded collector

**Goal:** provide complete public types and race-free lifecycle aggregation.

**Acceptance:**

- **when** observations arrive concurrently, **it** loses no terminal event and passes `go test -race`;
- **when** IDs repeat, **it** ignores duplicates and records a diagnostic;
- **when** spans overlap, **it** reports union wall time and summed active time separately;
- **when** an agent never finishes, **it** appears as incomplete;
- **when** values are unknown, **it** preserves nil/`Not measured` semantics;
- **when** the ordinary cap is reached, **it** keeps aggregates and terminal events and reports dropped count;
- **when** disabled, **it** starts no goroutine and retains no events.

### US-ER-003: Redaction, renderer, and atomic checkpoints

**Goal:** produce a safe deterministic `RUNNING` and final Markdown file.

**Acceptance:**

- **when** a run starts, **it** writes all required headings and `Checkpoint: yes`;
- **when** report values contain pipes, HTML, CR/LF, or backslashes, **it** cannot corrupt tables/headings;
- **when** evidence contains each supported secret shape, **it** is redacted before collection and rendering;
- **when** two flush requests overlap, **it** leaves a complete old or complete new file, never a partial file;
- **when** rename/write/sync fails, **it** removes temp files, warns once, and preserves project outcome;
- **when** no issue exists, **it** renders `None observed`;
- **when** measurement is absent, **it** renders `Not measured`.

### US-ER-004: Common observation adapters and metrics projection

**Goal:** instrument LLM, tool, sandbox, Git, validator, scheduler, OCC, and metrics at shared boundaries.

**Acceptance:**

- **when** one logical LLM call tries two candidates, **it** records one logical call and each physical candidate attempt without double-counting tokens;
- **when** provider usage is absent, **it** marks token/cost values unknown or estimated with provenance;
- **when** a tool is policy-blocked, **it** records blocked rather than executed;
- **when** sandbox validation runs formatter, linter, and tests, **it** records distinct command classes;
- **when** Git merge fails after validation, **it** records the merge failure and any status inconsistency;
- **when** a task waits on a lock/dependency/slot, **it** measures an interval rather than poll count;
- **when** metrics are enabled, **it** projects the same events into `MetricsCollector`;
- **when** metrics/OTel are disabled but report is enabled, **it** still records report facts.

### US-ER-005: Orchestrator, multi-story CLI, daemon, and shutdown lifecycle

**Goal:** create one reporter per process and correlate all story/agent/task phases.

**Acceptance:**

- **when** `start` executes multiple stories and creates multiple orchestrators, **it** creates one run with ordered story sections;
- **when** product-manager and pre-flight work occurs before story planning, **it** appears in the process report;
- **when** `serve` processes several stories, **it** updates the same report;
- **when** one story fails, **it** finalizes `FAILED` while preserving the command error;
- **when** an explicit story cancel occurs, **it** records `CANCELLED`;
- **when** SIGINT/SIGTERM occurs, **it** records `INTERRUPTED` using a bounded shutdown context;
- **when** reporting is disabled, **it** creates no artifact and no extra LLM call;
- **when** reporter writes fail, **it** leaves state, task status, and exit code unchanged.

### US-ER-006: Deterministic issues, bottlenecks, and proposals

**Goal:** implement §7 and §8 without an LLM.

**Acceptance:**

- **when** phase intervals overlap, **it** computes merged phase ratio correctly;
- **when** sample count is below five, **it** does not invent p95;
- **when** a test timeout occurs, **it** emits operational/performance evidence with `BN-TIMEOUT`;
- **when** retries reach two, **it** emits `BN-RETRY` and a linked proposal;
- **when** a sandbox attempt is blocked, **it** records the successful control without claiming compromise;
- **when** a long successful operation is dominant, **it** is an observation rather than a confirmed defect;
- **when** telemetry is missing, **it** creates a limitation rather than zero;
- **when** input order changes, **it** renders identical sorted issue/proposal order.

### US-ER-007: Bounded Reporter Analyzer

**Goal:** make one read-only terminal LLM request during `Finish` and safely merge valid hypotheses/proposals.

**Acceptance:**

- **when** report is disabled, **it** makes zero analyzer calls;
- **when** report is enabled and finalization succeeds, **it** makes at most one logical analyzer call during `Finish`;
- **when** a process checkpoints or ends an intermediate story, **it** makes zero analyzer calls;
- **when** output uses the exact virtual action and known issue IDs, **it** includes priorities and labeled hypotheses;
- **when** output changes a measured value or references unknown issues, **it** rejects those entries;
- **when** analyzer times out, is over budget, or returns malformed output, **it** retains deterministic output and exit status;
- **when** analyzer runs, **it** receives at most 65,536 redacted bytes and its output is capped at 32,768 bytes;
- **when** its own LLM observation is recorded, **it** does not recursively invoke analysis.

### US-ER-008: Documentation and black-box E2E validation

**Goal:** document and validate the generic public contract through `dist/noctifab`, then adapt the container harness to consume it without coupling production code to validation projects.

**Deliverables:**

- update `SPEC.md` configuration schema and report behavior;
- update `TESTS.md` with package and E2E coverage;
- update `README.md` and `docs/` with ordinary-project enablement, security, lifecycle, and deterministic-analysis examples;
- update `validation/README.md` and `validation/projects/TESTING_GUIDE.md` with report-first triage;
- statically configure validation fixtures with the shared report mount path without editing any roadmap;
- update `run_one.sh`, `validate.sh`, and structured feedback collection as specified in §12;
- update `VERSION` with a minor feature bump and `CHANGELOG.md` in the same implementation commit;
- build `dist/noctifab` and run isolated black-box scenarios.

**E2E acceptance:**

- **when** valid reporting is configured for an ordinary non-validation workspace, **it** creates the same generic report with required headings and status;
- **when** a validation container starts, **it** checkpoints directly into `output/report/EXECUTION_REPORT.md` on the host;
- **when** noctifab fails before `validate.sh` copies source output, **it** still preserves the last valid host-mounted checkpoint;
- **when** harness artifact checks fail after noctifab succeeds, **it** keeps noctifab `SUCCESS` separate from harness `FAIL`;
- **when** structured report/result artifacts exist, **it** generates feedback without parsing the raw container log;
- **when** omitted, **it** creates no report path;
- **when** invalid, **it** prints the exact warning prefix on stderr and preserves exit code;
- **when** a mocked validation fails, **it** includes evidence, issue, bottleneck rule, and proposal;
- **when** mocked analyzer fails, **it** still writes deterministic final output;
- **when** SIGTERM interrupts the binary, **it** leaves a complete `INTERRUPTED` or last-valid `RUNNING` checkpoint, never a partial file;
- **when** the same path is reused by a new process, **it** contains only the new run ID.

## 15. Test plan

### 15.1 Test style

Tests assert public behavior, not private maps or mutexes. Use table-driven tests, `t.Helper()`, `testify`, and `t.Parallel()` where no shared process state exists. BDD subtests use:

```go
t.Run("when the destination is a directory", func(t *testing.T) {
    t.Run("it disables reporting without changing execution", func(t *testing.T) {
        // ...
    })
})
```

Use fake clocks, writers, analyzers, observers, warning sinks, and filesystems. Temp directories are acceptable for public atomic-file behavior; permission failures use injected filesystem errors.

### 15.2 Required unit matrices

Configuration/path:

- omitted, whitespace, relative, external absolute, NUL, directory, symlink target, symlink parent, protected files, source-tree path, missing parent, probe failure, cleanup failure;
- strict YAML scalar/non-scalar behavior;
- no file operation in disabled mode.

Collector/timing:

- lifecycle transitions and idempotent finish;
- concurrent event ingestion under race detector;
- duplicate/missing IDs and unmatched spans;
- overlap union, active sum, wait intervals, negative clock anomaly;
- story reduction and event cap;
- process versus report synthesis duration.

Renderer/writer/security:

- golden Markdown for running/success/failure/interruption;
- stable order despite randomized input maps/slices;
- Markdown/HTML escaping;
- UTF-8-safe byte truncation;
- every redaction pattern;
- file/parent sync, temp cleanup, rename preservation, restrictive modes through fake FS calls;
- final LF and exactly one trailing newline.

Analysis:

- every bottleneck threshold just below/at/above boundary;
- nearest-rank percentile examples;
- severity and kind mapping;
- linked stable issue/proposal IDs;
- virtual analyzer action valid/invalid/unknown-ID/oversized/timeout/budget cases.

Adapters/integration:

- one event per wrapper boundary on every success/error/timeout return;
- no double counting in nested tool/sandbox calls;
- role/story/task context propagation into goroutines;
- scheduler reason transitions;
- report write failure has no effect on task status;
- multiple orchestrators share one process reporter;
- analyzer execution during `Finish` proves one logical analyzer call when enabled.

Validation harness:

- direct bind-mounted checkpoints survive a failing `noctifab start` and `--rm` container cleanup;
- `HARNESS_RESULT.json` is written on success, early command failure, artifact failure, and signal exit;
- feedback distinguishes noctifab outcome from harness verdict;
- structured-artifact mode does not open/parse the raw log (verify with an unreadable fake log);
- missing execution report triggers an explicit legacy-fallback diagnostic;
- production reporter packages pass the validation-coupling source/dependency guard.

### 15.3 Verification commands

Run after implementation, with zero failures:

```bash
go fmt ./...
go test -v ./pkg/... ./tests
go test -v ./cmd/...
go test -race ./cmd/... ./pkg/... ./tests
docker run -t --rm -v $(pwd):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint run
make build
docker compose -f tests/e2e/docker-compose.yml up --build --exit-code-from test-runner
```

Also enforce the physical-line limit:

```bash
find cmd pkg -name '*.go' -print0 | xargs -0 wc -l | awk '$2 != "total" && $1 > 500 { print; failed=1 } END { exit failed }'
```

Coverage for each new package must be 100% under the repository mandate. E2E tests validate public output and exit behavior; they must not inspect private reporter fields.

## 16. Definition of Done

The feature is done only when all statements are true:

1. Work occurs on a non-`main` branch.
2. `execution_report` is an optional top-level YAML string with empty default and no new flag/env override.
3. Omitted/blank mode allocates no reporter, starts no goroutine, writes no file, and makes no analyzer call.
4. Enabling `execution_report` automatically runs both the deterministic event collector and the single terminal LLM analyzer call at process exit.
5. Invalid destinations soft-disable with exactly `noctifab report disabled:` on stderr and preserve exit behavior.
6. Workspace destinations are isolated under `.noctifab/reports`; external absolute destinations are supported.
7. The first checkpoint atomically replaces an older run and terminal writes never expose partial Markdown.
8. One process has one run ID and one reporter across all stories/orchestrators.
9. The report has every required section, including stable `Live Status`, fixed ordering, UTF-8/LF/final newline, UTC timestamps, integer milliseconds, and fixed-decimal costs.
10. Unknown measurements render `Not measured`; estimates are labeled; overlapping sums are explained.
11. Every observed agent invocation and terminal task appears with scoped IDs and outcome.
12. Deterministic bottlenecks implement the exact published rules and show rule/evidence.
13. Issues include stable ID, kind, category, severity, behavior, impact, affected scope/component, confidence, evidence, blocked flag, and proposal; generated-project/environment failures are not mislabeled as noctifab defects.
14. Analyzer input/output is bounded and redacted; it cannot change facts; failure is non-fatal.
15. Analyzer runs at most once only when explicitly enabled and never in disabled/default/checkpoint/story-finalization paths.
16. Reporter failures cannot change state, scheduling, task/story status, merge behavior, generated files, original errors, or exit codes.
17. Reports and warnings contain no credentials, secret values, headers, prompts, full model responses, source dumps, or `secrets.yaml` content.
18. Raw event memory is bounded per active story and collapsed after story completion.
19. Existing metrics and report metrics derive from the same observation stream.
20. Validation containers persist checkpoints directly to `output/report/`; harness verdict remains separate; structured feedback does not require log parsing.
21. Core report code has no validation-project, Docker, target-language, target-artifact, or project-name branches/imports.
22. `start.go` is split below 500 lines before report wiring; every new/modified Go file remains at or below 500 physical lines.
23. New packages have adjacent 100%-covered unit tests; integration and E2E tests use public contracts and required BDD naming.
24. `SPEC.md`, `TESTS.md`, `README.md`, validation guides, developer docs, `VERSION`, and `CHANGELOG.md` are updated with implementation.
25. Formatting, unit/integration, race, lint, build, line-limit, and E2E commands pass with zero failures.

## 17. Recommended implementation order

1. Create a feature branch; split `start.go` without behavior changes and verify tests.
2. Implement config field, pure path resolution, protected-path rules, and destination preparation.
3. Define domain events, correlation, no-op/fan-out observer, fake clock, and bounded collector.
4. Implement redaction before adding any production evidence source.
5. Implement immutable snapshots, deterministic renderer, and atomic writer.
6. Implement common observed wrappers and MetricsCollector projection.
7. Add scheduler wait diagnostics and orchestrator phase/task/agent events.
8. Wire one reporter around `start` and `serve`, including every return and signal path.
9. Implement deterministic bottleneck, issue, and proposal analysis.
10. Adapt the validation harness to direct-mount checkpoints and produce a separate structured harness verdict; verify the generic reporter has no validation coupling.
11. Implement and validate the explicitly opt-in one-shot virtual-action LLM analyzer.
12. Add ordinary-project and containerized black-box CLI/E2E coverage with failure injection.
13. Update specification, tests documentation, ordinary user docs, validation guides, version, and changelog.
14. Run every Definition of Done command.

This order makes secure deterministic measurement complete before model-generated explanation is introduced, and it keeps every intermediate commit compiling and runnable.
