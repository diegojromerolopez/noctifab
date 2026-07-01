# Autonomous Software Factory — Implementation Roadmap

This document defines the concrete, implementation-ready plan to transition `noctifab` from its current level to a fully autonomous **Level 5 Dark Factory**.

The plan is organized into **6 implementation phases**, each containing discrete work items with specific file paths, struct signatures, config schemas, and test requirements.

---

## Current State Assessment

```
Phase 1 (Resilience):       40% — FailoverClient exists but is DEAD CODE (not wired)
Phase 2 (Liveness):        100% — Watchdog runnning in HostSandbox, tested
Phase 3 (Prompt Guard):      0%
Phase 4 (Self-Repair):       0%
Phase 5 (Self-Healing):      0%
Phase 6 (Self-Evolution):    0%
```

---

## Phase 1 — Resilience

### Goal
Make the daemon survive network failures, API quota exhaustion, and provider outages without halting.

---

### 1.1 Wire FailoverClient into Production

**Problem**: `pkg/infrastructure/llm/failover_client.go` has a complete multi-provider failover implementation with cooldown tracking, but both `serve.go:82` and `start_one.go:74` use `llm.NewClient` directly. The failover logic is dead code.

**Files to modify**:
- `cmd/noctifab/cli/serve.go`
- `cmd/noctifab/cli/start_one.go`
- `pkg/infrastructure/config/types.go`
- `pkg/infrastructure/config/defaults.go`

**Config schema changes** (`pkg/infrastructure/config/types.go`):

```go
// LLMConfig — add FailoverConfig field
type LLMConfig struct {
    Provider           string         `yaml:"provider"`
    Model              string         `yaml:"model"`
    Temperature        float64        `yaml:"temperature"`
    APIKey             string         `yaml:"api_key"`
    APIKeyEnv          string         `yaml:"api_key_env"`
    APIKeyValue        string         `yaml:"-"` // populated at runtime
    URL                string         `yaml:"url"`
    MaxRetries         int            `yaml:"max_retries"`
    RetryBackoff       Duration       `yaml:"retry_backoff"`
    RetryBackoffFactor float64        `yaml:"retry_backoff_factor"`
    MaxBudgetUSD       float64        `yaml:"max_budget_usd"`
    Failover           FailoverConfig `yaml:"failover"`
}

type FailoverConfig struct {
    Enabled      bool              `yaml:"enabled"`
    Cooldown     Duration          `yaml:"cooldown"`      // default 5m
    MaxCallLimit int               `yaml:"max_call_limit"` // 0 = unlimited
    Backends     []FailoverBackend `yaml:"backends"`
}

type FailoverBackend struct {
    Provider    string `yaml:"provider"`
    Model       string `yaml:"model"`
    APIKeyEnv   string `yaml:"api_key_env"`
    URL         string `yaml:"url"`
    MaxRetries  int    `yaml:"max_retries"`
}
```

**Default config** (`pkg/infrastructure/config/defaults.go`):
```go
Failover: FailoverConfig{
    Enabled:      false,
    Cooldown:     Duration(5 * time.Minute),
    MaxCallLimit: 0,
    Backends:     nil,
},
```

**Implementation plan**:

1. Add `FailoverConfig` struct and embed in `LLMConfig`
2. Add `BuildFailoverClient(cfg *LLMConfig) domain.LLMClient` factory function in `pkg/infrastructure/llm/factory.go`:
   - If `Failover.Enabled == false` → return `NewClient(...)` (existing behavior)
   - If `Failover.Enabled == true` → build `[]NamedClient` from `Failover.Backends`, call `NewFailoverClient(backends, cooldown, maxCalls)`
3. In `serve.go` and `start_one.go`, replace `llm.NewClient(...)` with `llm.BuildFailoverClient(cfg.LLM)`
4. Update `FailoverClient` to accept a `domain.LLMClient` interface for each backend (already done via `NamedClient`)

**Tests**:
- Existing `failover_client_test.go` covers cooldown and fallback
- Add: config parsing test for `FailoverConfig` YAML
- Add: e2e test with mock backends (one fails, one succeeds) through `BuildFailoverClient`
- **Edge case**: all backends on cooldown → verify last error wraps correctly
- **Edge case**: `Enabled: false` → returns non-failover client

---

### 1.2 Daily USD Budget Tracking with DB Persistence

**Problem**: `FailoverClient` uses an in-memory call counter that resets on restart. AUTONOMY.md proposes per-provider daily USD budget stored in the database, surviving restarts. The current `TokenUsageLimit` config value is unused.

**Files to create/modify**:
- `pkg/domain/budget.go` (new)
- `pkg/infrastructure/storage/budget_repository.go` (new)
- `pkg/infrastructure/llm/failover_client.go` (modify)
- `pkg/infrastructure/config/types.go` (modify)
- `pkg/domain/state.go` (modify — add budget fields)

**Domain type** (`pkg/domain/budget.go`):

```go
package domain

import "time"

type BudgetRecord struct {
    Date       string    `json:"date"`        // "2026-07-01"
    Provider   string    `json:"provider"`    // "openai"
    TokensIn   int64     `json:"tokens_in"`
    TokensOut  int64     `json:"tokens_out"`
    CostUSD    float64   `json:"cost_usd"`
    UpdatedAt  time.Time `json:"updated_at"`
}

type BudgetStore interface {
    LoadBudget(ctx context.Context, date, provider string) (*BudgetRecord, error)
    SaveBudget(ctx context.Context, record *BudgetRecord) error
    ListBudgets(ctx context.Context, since time.Time) ([]BudgetRecord, error)
}
```

**Storage implementation** (`pkg/infrastructure/storage/budget_repository.go`):

Add a `budget` table:
```sql
CREATE TABLE IF NOT EXISTS budget (
    date       TEXT NOT NULL,
    provider   TEXT NOT NULL,
    tokens_in  INTEGER DEFAULT 0,
    tokens_out INTEGER DEFAULT 0,
    cost_usd   REAL DEFAULT 0.0,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (date, provider)
);
```

Both `SQLiteRepository` and `PostgresRepository` implement `domain.BudgetStore`.

**Config** — add to `LLMConfig`:
```go
MaxBudgetUSD  float64        `yaml:"max_budget_usd"`     // existing
ResetPeriod   string         `yaml:"reset_period"`        // "daily" (default), "weekly", "monthly"
```

**FailoverClient changes**:
```go
type FailoverClient struct {
    backends      []NamedClient
    cooldowns     map[string]time.Time
    duration      time.Duration
    budgetStore   domain.BudgetStore
    maxBudgetUSD  float64
    resetPeriod   string
}
```

`Complete()` checks `budgetStore.LoadBudget(date, provider)` before each call — if `CostUSD >= maxBudgetUSD`, skip that provider. After a successful response, `budgetStore.SaveBudget(updatedRecord)` persists usage.

**Tests**:
- Unit test for budget store (SQLite in-memory)
- Unit test for FailoverClient budget enforcement (exceed → skip provider)
- Unit test for reset period boundary (rollover at midnight UTC)

---

### 1.3 Universal Interruptible Sleep

**Problem**: `SleepWithInterrupt` is only used in `orchestrator.updateStateWithRetry`. Other blocking sleep/poll locations don't respond to mailbox commands.

**Files to modify**:
- `pkg/usecase/orchestrator.go` — `Start()` ticker
- `cmd/noctifab/cli/serve.go` — `runServerLoop` story loop
- `pkg/usecase/command_channel.go` — ensure `SleepWithInterrupt` accepts multiple wakeup channels

**Change in `orchestrator.Start()`** — replace the blocking ticker with interruptible sleep:
```go
func (o *Orchestrator) Start(ctx context.Context) error {
    for {
        if err := o.RunOnce(ctx); err != nil {
            fmt.Fprintf(os.Stderr, "Orchestrator error: %v\n", err)
        }
        var wakeup <-chan struct{}
        if o.mailbox != nil {
            wakeup = o.mailbox.Wakeup()
        }
        if err := SleepWithInterrupt(ctx, o.cfg.PollInterval, wakeup); err != nil {
            if errors.Is(err, ErrInterrupted) {
                continue // woke up for a command, re-poll immediately
            }
            return err
        }
    }
}
```

**Change in `runServerLoop`** — the `select` on `storyCh` should also select on `ctx.Done()` (already does), but the story loop shouldn't block the mailbox. When a clarification comes in during story execution, `SleepWithInterrupt` in `updateStateWithRetry` already handles it.

**Tests**:
- Existing `command_channel_test.go` covers `SleepWithInterrupt`
- Add: end-to-end test where mailbox sends commands during poll interval and orchestrator wakes up

---

## Phase 2 — Liveness

### Completed Items
- ✅ `pkg/usecase/watchdog.go` — Watchdog with `MaxDuration` and `IdleTimeout`
- ✅ `pkg/usecase/sandbox.go` — Watchdog integrated into `HostSandbox.RunCommand`
- ✅ `tests/e2e/e2e_test.go` — 5 E2E tests for idle timeout
- ✅ `pkg/infrastructure/config/types.go` — `IdleTimeoutSeconds` in `SandboxConfig`
- ✅ `pkg/infrastructure/config/defaults.go` — default idle timeout 30s

### No remaining work.

---

## Phase 3 — Prompt Guard

### Goal
Prevent the LLM from generating code with common concurrency bugs (background threads that swallow exceptions, missing `KeyboardInterrupt` propagation, non-daemon threads blocking termination).

---

### 3.1 Concurrency Invariants in Agent Prompts

**Files to create/modify**:
- `pkg/infrastructure/llm/prompts.go` (new — centralize all prompt templates)
- `pkg/infrastructure/llm/client.go` (modify — use templates)
- `pkg/infrastructure/llm/client_test.go` (modify — verify invariants in prompts)

**Architecture decision**: Extract prompt construction from `client.go` into a dedicated file. Each agent role gets a `PromptBuilder` that injects invariant blocks based on the role and detected language.

```go
package llm

type PromptBuilder struct {
    Role           domain.AgentRole
    DetectedLang   string // "python", "go", "javascript", "rust"
}

func (pb *PromptBuilder) Build(prompt string) string {
    var sb strings.Builder
    sb.WriteString(prompt)
    sb.WriteString("\n\n")
    sb.WriteString(pb.concurrencyInvariants())
    return sb.String()
}

func (pb *PromptBuilder) concurrencyInvariants() string {
    switch pb.DetectedLang {
    case "python":
        return pb.pythonConcurrencyInvariants()
    case "go":
        return pb.goConcurrencyInvariants()
    default:
        return ""
    }
}
```

**Python invariants** (as proposed in AUTONOMY.md §3.A):

```
CONCURRENCY & THREADING INVARIANTS (Python):
1. If executing a task function inside a background thread, capture any
   raised exceptions (including BaseException classes like KeyboardInterrupt
   or SystemExit) and propagate them back to the main thread.
2. The main loop must join or check the thread status frequently
   (e.g., in a loop with a small timeout t.join(0.1)) and re-raise any
   captured exception to terminate the main loop.
3. Set daemon=True on all background threads before calling start(), so
   they don't prevent process termination on abrupt shutdown.
4. Use signal.signal(signal.SIGINT, handler) to handle Ctrl+C explicitly
   when threads are involved; do not rely on KeyboardInterrupt propagation
   through thread boundaries.
```

**Go invariants**:

```
CONCURRENCY & THREADING INVARIANTS (Go):
1. Always select on ctx.Done() in goroutines that perform blocking
   operations — never block indefinitely without a context check.
2. Use sync.WaitGroup to track goroutine completion; always call
   wg.Wait() before returning from functions that spawn goroutines.
3. Buffered channels (size >= 1) for signalling goroutines to avoid
   deadlock on send if the receiver has exited.
4. Use sync.Once for lazy initialization in concurrent contexts.
```

**Tests**:
- Verify `PromptBuilder.Build("test")` for python role includes all 4 invariants
- Verify `PromptBuilder.Build("test")` for go role includes all 4 invariants
- Verify default (unknown lang) returns no invariants

---

### 3.2 Language Detection in Sandbox

**Files to modify**:
- `pkg/usecase/sandbox.go` — detect language from project files
- `pkg/infrastructure/llm/client.go` — pass detected language to prompt builder

**Detection logic** in `sandbox.go`:

```go
func DetectProjectLanguage(projectPath string) string {
    if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
        return "go"
    }
    if _, err := os.Stat(filepath.Join(projectPath, "Cargo.toml")); err == nil {
        return "rust"
    }
    if _, err := os.Stat(filepath.Join(projectPath, "package.json")); err == nil {
        return "javascript"
    }
    if _, err := os.Stat(filepath.Join(projectPath, "requirements.txt")); err == nil {
        return "python"
    }
    if _, err := os.Stat(filepath.Join(projectPath, "setup.py")); err == nil {
        return "python"
    }
    if _, err := os.Stat(filepath.Join(projectPath, "pom.xml")); err == nil {
        return "java"
    }
    // default based on sandbox config language hint
    return ""
}
```

**Tests**:
- Create temp dirs with `go.mod`, `Cargo.toml`, `package.json`, `requirements.txt`, verify correct detection
- Verify empty dir returns `""`

---

## Phase 4 — Self-Repair

### Goal
When the Watchdog kills a hanging test, the orchestrator must capture the partial output, formulate a diagnostic prompt, and feed it back to the Generator Agent for an automated rewrite.

---

### 4.1 Watchdog Error Analysis & Rewrite Loop

**Files to create/modify**:
- `pkg/usecase/watchdog_repair.go` (new)
- `pkg/usecase/orchestrator.go` (modify `executeTask`)
- `pkg/domain/error.go` (modify — add watchdog-specific error types)

**New error types** (`pkg/domain/error.go`):

```go
var (
    ErrWatchdogTimeout   = errors.New("command killed by watchdog")
    ErrMaxRetriesReached = errors.New("maximum retries exceeded")
    ErrBudgetExhausted   = errors.New("LLM token budget exhausted")
)
```

**Repair orchestrator** (`pkg/usecase/watchdog_repair.go`):

```go
package usecase

import (
    "context"
    "fmt"
    "strings"
)

// WatchdogRepair handles the automated repair loop when a watchdog kills a
// command. It captures the partial output, generates a diagnostic prompt,
// and feeds it to a Generator Agent for rewrite.
type WatchdogRepair struct {
    llmClient   domain.LLMClient
    maxRetries  int
    sandbox     Sandbox
}

type RepairResult struct {
    Success     bool
    Output      string
    FixedCode   bool
    Attempts    int
    FailureLog  string
}

func (wr *WatchdogRepair) AttemptRepair(
    ctx context.Context,
    projectPath string,
    task domain.Task,
    watchdogOutput string,
    watchdogErr error,
) (*RepairResult, error) {
    // 1. Formulate diagnostic prompt
    diagPrompt := wr.buildDiagnosticPrompt(task, watchdogOutput, watchdogErr)

    // 2. Loop: generate fix → run test → if passes → done
    for attempt := 0; attempt < wr.maxRetries; attempt++ {
        // Send diagnostic to LLM
        resp, err := wr.llmClient.Complete(ctx, diagPrompt)
        if err != nil {
            return nil, fmt.Errorf("repair LLM call failed: %w", err)
        }

        // Apply the fix actions (write_file, edit_file, etc.)
        for _, action := range resp.Actions {
            if action.Tool == "write_file" {
                // ... apply write with WriteFileTool
            }
            if action.Tool == "edit_file" {
                // ... apply edit with EditFileTool
            }
        }

        // Re-run tests
        testOutput, testErr := wr.sandbox.RunCommand(ctx, projectPath, "", "")
        if testErr == nil {
            return &RepairResult{
                Success:    true,
                Output:     testOutput,
                FixedCode:  true,
                Attempts:   attempt + 1,
            }, nil
        }

        // Append test failure to next prompt for incremental repair
        diagPrompt = wr.buildRetryPrompt(diagPrompt, testOutput, testErr)
    }

    return &RepairResult{
        Success:    false,
        Attempts:   wr.maxRetries,
        FailureLog: "all repair attempts failed",
    }, nil
}

func (wr *WatchdogRepair) buildDiagnosticPrompt(
    task domain.Task,
    output string,
    err error,
) string {
    return fmt.Sprintf(`The test suite hung and was forcefully terminated by the watchdog.

Task: %s - %s

Watchdog error: %v

Last stdout output before timeout:
%s

This usually indicates:
- An infinite loop or deadlock
- An unjoined non-daemon thread
- A blocking operation (wait/sleep) that is never unblocked
- A resource leak exhausting file descriptors

Analyze the output above and fix the issue. Rewrite any files that need
changes. Focus on making the code terminate correctly.
`, task.Title, task.Description, err, output)
}

func (wr *WatchdogRepair) buildRetryPrompt(
    prevPrompt string,
    testOutput string,
    testErr error,
) string {
    return fmt.Sprintf(`%s

The fix attempt was made but tests still failed or hung:

Test output:
%s

Test error: %v

Please try a different approach to fix the hang/deadlock.
`, prevPrompt, testOutput, testErr)
}
```

**Modify `orchestrator.go` — `executeTask`** to intercept watchdog errors:

```go
// inside executeTask, after evaluator.ValidateTask returns failed:
if errors.Is(err, usecase.ErrWatchdogIdleTimeout) || errors.Is(err, usecase.ErrWatchdogMaxDuration) {
    repair := &WatchdogRepair{
        llmClient:  o.llmClient,
        maxRetries: 3,
        sandbox:    o.evaluator.Runner,
    }
    result, repairErr := repair.AttemptRepair(ctx, state.ProjectPath, *task, logMsg, err)
    if repairErr == nil && result.Success {
        // repair succeeded — mark task success
        task.Status = domain.TaskSuccess
        continue
    }
    // repair failed — fall through to normal retry logic
}
```

**Tests**:
- Unit test `WatchdogRepair.buildDiagnosticPrompt` — verify error and output are included
- Unit test `buildRetryPrompt` — verify previous attempt context is appended
- Mock test: `AttemptRepair` with mock LLM that returns a fix, mock sandbox that passes on second call → verify repair succeeds
- Mock test: `AttemptRepair` with mock LLM that returns fixes that never pass → verify failure after maxRetries

---

### 4.2 Failure Log Summarization Enhancement

**Problem**: `summarizeFailureLog` in `orchestrator_helper.go` extracts ERROR/FAIL lines, but doesn't distinguish watchdog timeouts from test logic failures.

**File to modify**: `pkg/usecase/orchestrator_helper.go`

```go
// CategorizeFailureLog classifies a failure log by error type.
type FailureCategory int

const (
    FailureUnknown     FailureCategory = iota
    FailureTestLogic   FailureCategory = iota
    FailureTimeout     FailureCategory = iota
    FailureCompile     FailureCategory = iota
    FailureSandbox     FailureCategory = iota
)

func CategorizeFailureLog(log string) FailureCategory {
    logLower := strings.ToLower(log)
    if strings.Contains(logLower, "no output produced within idle timeout") ||
       strings.Contains(logLower, "max wall-clock duration exceeded") {
        return FailureTimeout
    }
    if strings.Contains(logLower, "sandbox violation") {
        return FailureSandbox
    }
    if strings.Contains(logLower, "compile") || strings.Contains(logLower, "syntax error") {
        return FailureCompile
    }
    if strings.Contains(logLower, "error:") || strings.Contains(logLower, "fail:") {
        return FailureTestLogic
    }
    return FailureUnknown
}
```

---

## Phase 5 — Self-Healing

### Goal
The daemon survives environment drift (missing toolchains), validates code in staging, eliminates flaky tests, and uses telemetry feedback to auto-repair production regressions.

---

### 5.1 Dynamic Dependency Installation

**Problem**: If the sandbox lacks `cargo`, `pytest`, `golangci-lint`, etc., the task fails immediately. A human must install the dependency.

**Files to create/modify**:
- `pkg/infrastructure/sandbox/dependency_manager.go` (new)
- `pkg/infrastructure/config/types.go` (modify — add `auto_install_deps`)
- `pkg/usecase/sandbox.go` (modify — intercept `executable file not found`)

**Config**:
```go
type SandboxConfig struct {
    // ... existing fields ...
    AutoInstallDeps bool     `yaml:"auto_install_deps"`      // default false
    PackageManagers []string `yaml:"package_managers"`        // e.g., ["brew", "apt", "pip", "cargo"]
}
```

**Dependency manager**:

```go
type DependencyManager struct {
    AllowedPkgManagers []string   // whitelist from config
    AllowedCommands    []string   // sandbox whitelist
}

// DetectMissingTool parses command output for "not found" signatures.
func (dm *DependencyManager) DetectMissingTool(output string) (string, bool) {
    patterns := []string{
        "executable file not found",
        "command not found",
        "No such file or directory",
    }
    // ... check output for patterns, return tool name
}

// InstallTool runs the appropriate package manager command.
func (dm *DependencyManager) InstallTool(ctx context.Context, tool string) error {
    // Map tool to package manager:
    // "cargo" → "brew install rust" or "apt install rustc"
    // "pytest" → "pip install pytest"
    // "golangci-lint" → "go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
}
```

**Modify `HostSandbox.RunCommand`**: if command fails with `executable file not found`, call `DependencyManager.InstallTool()` and retry once.

**Tests**:
- Unit test `DetectMissingTool` with various "not found" error strings
- Unit test `InstallTool` (dry-run mode that logs instead of executing)
- Edge case: tool maps to unknown package manager → return error, don't install
- Edge case: `AutoInstallDeps: false` → skip installation, return original error

---

### 5.2 Flaky Test Auto-Stabilization

**Problem**: The 3x majority vote in `TestValidator` detects flaky tests (2 pass, 1 fail) but only logs a warning. A Level 5 agent should fix the flaky test automatically.

**Files to modify**:
- `pkg/usecase/test_validator.go`
- `pkg/usecase/flaky_detector.go` (new)

**Flaky detection**:

```go
type FlakyResult struct {
    TaskID      string
    Flaky       bool    // true if 2/3 pass
    FailedCount int
    PassedCount int
    Outputs     []string
}

func DetectFlaky(results []TestRunResult) *FlakyResult {
    passed := 0
    failed := 0
    for _, r := range results {
        if r.Passed {
            passed++
        } else {
            failed++
        }
    }
    return &FlakyResult{
        Flaky:       passed >= 2 && failed >= 1,
        FailedCount: failed,
        PassedCount: passed,
    }
}
```

**When flaky detected**: run tests with race detection, collect output, pass to Generator Agent with prompt:
```
The test suite has a flaky test — it passes inconsistently.
Outputs across 3 runs show non-deterministic behavior.
Run tests with race detection: go test -race ./...
Analyze the test and implementation for:
- time.Sleep instead of deterministic polling
- Shared state between tests (global variables, file system)
- Missing mutexes or race conditions
- Network dependency without retry/timeout
Rewrite to make the test deterministic.
```

**Tests**:
- Unit test `DetectFlaky`: 3/3 pass → not flaky; 2/1 → flaky; 1/2 → not flaky (failing consistently)
- Mock test: orchestrator calls `AutoStabilize` with flaky test, mock LLM returns fix, re-run passes

---

### 5.3 Telemetry Integration (OpenTelemetry)

**Problem**: No APM data is collected. The daemon operates blind — no insight into cycle times, failure rates, LLM latency, or error correlations.

**Spec reference**: SPEC.md §5.1 defines the telemetry architecture.

**Files to create/modify**:
- `pkg/infrastructure/telemetry/tracer.go` (new)
- `pkg/infrastructure/telemetry/tracer_test.go` (new)
- `cmd/noctifab/cli/serve.go` (modify — init tracer)
- `pkg/usecase/orchestrator.go` (modify — add spans)
- `pkg/infrastructure/llm/client.go` (modify — add spans)
- `go.mod` (add dependencies)

**New dependency**: `go.opentelemetry.io/otel`

**Tracer setup**:

```go
package telemetry

import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func InitTracer(serviceName, endpoint string) (*sdktrace.TracerProvider, error) {
    exporter, err := otlptracehttp.New(context.Background(),
        otlptracehttp.WithEndpoint(endpoint),
        otlptracehttp.WithInsecure(),
    )
    if err != nil {
        return nil, err
    }

    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceName(serviceName),
        )),
    )
    otel.SetTracerProvider(tp)
    return tp, nil
}
```

**Span hierarchy**:
```
noctifab.cycle (orchestrator RunOnce)
  ├── noctifab.task_worker (per-task execution)
  │   ├── noctifab.action (per tool call: write_file, run_tests, ...)
  │   └── noctifab.llm_completion (per LLM Complete call)
  ├── noctifab.sandbox_command (per RunCommand execution)
  └── noctifab.occ_retry (OCC conflict retry backoff)
```

**Config**:
```go
type TelemetryConfig struct {
    Enabled      bool   `yaml:"enabled"`
    Exporter     string `yaml:"exporter"`      // "otlp", "stdout"
    Endpoint     string `yaml:"endpoint"`      // "localhost:4318"
    ServiceName  string `yaml:"service_name"`  // "noctifab"
}
```

**Tests**:
- Unit test span creation and attribute attachment (with stdout exporter in test)
- Unit test that spans are properly ended even on error paths

---

## Phase 6 — Self-Evolution

### Goal
The daemon can patch its own Go binary, hot-reload without losing state, and enforce security gates.

---

### 6.1 Self-Patching Compiler Loop

**Problem**: If a bug exists in `noctifab` itself, a human must fix it. A Level 5 agent should be able to compile, test, and deploy its own updated binary.

**Files to create**:
- `pkg/usecase/self_update.go` (new)
- `pkg/usecase/self_update_test.go` (new)

**Design**:

```go
type SelfUpdateManager struct {
    RepoPath    string   // path to noctifab repo
    BuildCmd    string   // "go build -o /tmp/noctifab-new ./cmd/noctifab"
    TestCmd     string   // "go test ./..."
    BinaryPath  string   // current binary path
}

func (sum *SelfUpdateManager) BuildAndTest(ctx context.Context) error {
    // 1. Clone or pull latest noctifab source
    // 2. Apply LLM-generated patches to Go source files
    // 3. Run `go build -o /tmp/noctifab-new ./cmd/noctifab`
    // 4. Run `go test ./pkg/...`
    // 5. If tests pass → return nil (ready for hot-reload)
    // 6. If tests fail → rollback patches, return error
}
```

**Patch application**:
- LLM generates `write_file` actions for `.go` files in `cmd/` and `pkg/`
- `SelfUpdateManager` collects patches, applies to a temp clone, builds, runs full test suite
- Only if all tests pass is the binary ready for hot-reload

**Safety constraints**:
- Only modify files in `cmd/noctifab/` and `pkg/` (not `tests/`, `docs/`, vendor dirs)
- Build must produce a working binary (`go vet` + `go test ./pkg/...` must pass)
- Never modify `go.mod` or `go.sum` (dependency changes require human review)

**Tests**:
- Mock test: create temp Go project, apply mock patches, verify build succeeds
- Mock test: apply broken patch (syntax error), verify rollback

---

### 6.2 Graceful Stateful Hot-Reload

**Problem**: When replacing the binary, active task state must be preserved and handed off.

**Files to create**:
- `pkg/usecase/hot_reload.go` (new)

**Design**:

```go
type HotReloadManager struct {
    PIDPath     string
    OldBinary   string
    NewBinary   string
    StateRepo   domain.StateRepository
}

func (hrm *HotReloadManager) Reload(ctx context.Context) error {
    // 1. Save current state (already persisted by repo)
    // 2. Spawn new binary as child: exec.Command(newBinary, "serve", "--restore")
    // 3. New binary starts HTTP server on a different port (+1)
    // 4. Old binary waits for new binary health check to pass
    // 5. Old binary signals it's shutting down, stops accepting work
    // 6. New binary takes over the PID file
    // 7. Old binary exits
    return nil
}
```

**Protocol**: The old binary passes state to the new binary via the shared database (already persisted). The handoff uses a `handshake` file:
1. Old writes `{ "new_pid": ..., "status": "handing_off" }` to `.noctifab/hot_reload.json`
2. New polls this file; when it sees `status: "handing_off"`, it loads state and begins orchestrating
3. New writes `{ "status": "active", "pid": ... }`
4. Old reads this and exits

**Tests**:
- Integration test: start old binary, trigger hot-reload, verify new binary processes tasks
- Unit test: handshake file protocol

---

### 6.3 SAST Security Gates

**Problem**: No automated security scanning. The daemon can merge code with known vulnerabilities.

**Files to create/modify**:
- `pkg/usecase/sast_scanner.go` (new)
- `pkg/infrastructure/config/types.go` (modify — add scanner config)

**Config**:
```go
type SASTConfig struct {
    Enabled      bool     `yaml:"enabled"`
    Scanners     []string `yaml:"scanners"`       // ["gosec", "bandit", "cargo-audit"]
    FailOnSeverity string `yaml:"fail_on_severity"` // "high" (default), "medium", "low"
}
```

**Scanner**:

```go
type SASTScanner struct {
    Scanners       []string
    AllowedCmds    []string
}

// Run scans the project directory with all configured SAST tools.
func (s *SASTScanner) Run(ctx context.Context, projectPath string) (*SASTResult, error) {
    var issues []SecurityIssue

    for _, scanner := range s.Scanners {
        switch scanner {
        case "gosec":
            out, err := s.runGosec(ctx, projectPath)
            // parse output, append issues
        case "bandit":
            out, err := s.runBandit(ctx, projectPath)
            // parse output, append issues
        }
    }

    return &SASTResult{
        Passed:  countHigh(issues) == 0,
        Issues:  issues,
    }, nil
}

type SecurityIssue struct {
    Scanner     string `json:"scanner"`
    Severity    string `json:"severity"`
    File        string `json:"file"`
    Line        int    `json:"line"`
    Description string `json:"description"`
}
```

**Integration**: Run SAST in `FinalizeUserStory` before creating the PR. If high-severity issues found, block the PR and feed the report to the Generator Agent for remediation.

**Tests**:
- Unit test `Run` with mock scanner commands (write temp files with known vulnerabilities)
- Unit test severity filtering (only high-severity blocks PR)
- Unit test `FailOnSeverity: "medium"` — medium issues also block

---

### 6.4 Zero-Clarification Intent Disambiguation

**Problem**: When a spec is ambiguous, the orchestrator raises a clarification and waits for a human. A Level 5 agent should infer intent from git history, issue tracker, and code context.

**Files to create**:
- `pkg/usecase/intent_disambiguator.go` (new)

**Design**:

```go
type IntentDisambiguator struct {
    gitClient   *GitClient
    vcsClient   domain.VCSClient
    llmClient   domain.LLMClient
}

// Disambiguate attempts to resolve a clarification without human input.
// It searches git log, commit messages, and similar issues for context.
func (id *IntentDisambiguator) Disambiguate(ctx context.Context,
    clarification domain.Clarification, state *domain.State) (string, error) {

    // 1. Gather context: recent git log, similar past clarifications, code symbols
    gitLog, _ := id.gitClient.Run(ctx, false, "log", "--oneline", "-30")
    
    // 2. Build prompt for LLM to infer intent
    prompt := fmt.Sprintf(`The system needs to resolve this ambiguity:

Question: %s

Context:
- Base branch: %s
- Feature: %s
- Recent commits:
%s

Analyze the context and infer the most likely intended behavior.
Respond with a brief answer to the question above.
`, clarification.Question, state.Metadata.BaseBranch, 
   state.Metadata.FeatureName, gitLog)

    // 3. Call LLM with the disambiguation prompt
    resp, err := id.llmClient.Complete(ctx, prompt)
    if err != nil {
        return "", err
    }

    // 4. Log the inferred answer and decision
    inferred := resp.Actions[0].Args["answer"].(string)
    return inferred, nil
}
```

**Integration**: in `orchestrator_helper.go`'s clarification check, before pausing:
```go
if o.intentDisambiguator != nil {
    answer, err := o.intentDisambiguator.Disambiguate(ctx, clar, state)
    if err == nil {
        clar.Answer = answer
        clar.Resolved = true
        continue // do not block
    }
}
```

---

## Implementation Ordering

### Recommended sequence (highest ROI first)

| Priority | Phase | Item | Effort | Impact |
|----------|-------|------|--------|--------|
| P0 | 1.1 | Wire FailoverClient | 2 days | Eliminates single-provider 429 death |
| P0 | 3.1 | Concurrency prompts | 1 day | Prevents infinite-loop test hangs |
| P1 | 4.1 | Watchdog repair loop | 3 days | Auto-recovers from hangs |
| P1 | 1.2 | Budget persistence | 2 days | Stops runaway costs |
| P1 | 5.2 | Flaky auto-stabilization | 2 days | Reduces manual flaky-test burden |
| P2 | 5.1 | Dynamic dependency install | 2 days | Reduces env setup failures |
| P2 | 1.3 | Universal interruptible sleep | 1 day | Makes daemon responsive during backoff |
| P3 | 5.3 | Telemetry integration | 3 days | Observability for all phases |
| P3 | 6.3 | SAST gates | 2 days | Security baseline |
| P4 | 6.1 | Self-patching | 4 days | Meta-autonomy |
| P4 | 6.2 | Hot-reload | 3 days | Zero-downtime updates |
| P4 | 6.4 | Intent disambiguation | 2 days | Reduces human interaction |

### Effort estimate
- **P0**: ~3 days
- **P0+P1**: ~10 days
- **All phases**: ~25 days

---

## Appendix: Config Schema Final State

Full `config.yaml` with all Phase 1–6 settings:

```yaml
config_version: "2.0"

orchestrator:
  max_tools_per_response: 5
  concurrency: 3
  poll_interval: 5m
  max_clarification_wait: 30m
  clarification_timeout_action: abort

llm:
  provider: openai
  model: gpt-4o
  temperature: 0.0
  api_key_env: OPENAI_API_KEY
  max_retries: 5
  retry_backoff: 100ms
  retry_backoff_factor: 2.0
  max_budget_usd: 10.00
  reset_period: daily
  failover:
    enabled: true
    cooldown: 5m
    max_call_limit: 1000
    backends:
      - provider: gemini
        model: gemini-2.5-flash
        api_key_env: GEMINI_API_KEY
      - provider: anthropic
        model: claude-3-5-haiku-latest
        api_key_env: ANTHROPIC_API_KEY

sandbox:
  mode: host
  timeout_seconds: 300
  idle_timeout_seconds: 30
  grace_period_seconds: 30
  test_command: "go test -v ./..."
  linter_command: "golangci-lint run"
  formatter_command: "go fmt ./..."
  auto_install_deps: false
  package_managers: ["brew", "apt", "pip"]
  exclude_paths: ["node_modules/", "vendor/", ".noctifab/"]
  allowed_commands: ["go", "git", "npm", "python"]

telemetry:
  enabled: false
  exporter: otlp
  endpoint: localhost:4318
  service_name: noctifab

sast:
  enabled: false
  scanners: ["gosec"]
  fail_on_severity: high

editor:
  auto_install_deps: false
  package_managers: ["brew", "apt", "pip"]
```

---

## Appendix: Complete Error Taxonomy

```go
// Sentinel errors — all domains
var (
    // Phase 1
    ErrBudgetExhausted   = errors.New("LLM token/budget exhausted")
    ErrAllBackendsFailed = errors.New("all LLM backends exhausted")

    // Phase 2
    ErrWatchdogMaxDuration = errors.New("command killed: max wall-clock duration exceeded")
    ErrWatchdogIdleTimeout = errors.New("command killed: no output produced within idle timeout")

    // Phase 3
    // (no new sentinel errors)

    // Phase 4
    ErrRepairFailed    = errors.New("all repair attempts failed")
    ErrHangDiagnosed   = errors.New("hang detected and analyzed")

    // Phase 5
    ErrMissingDependency   = errors.New("required toolchain not installed")
    ErrFlakyTestDetected   = errors.New("test suite has non-deterministic results")

    // Phase 6
    ErrSelfPatchFailed     = errors.New("self-patch build or test failed")
    ErrHotReloadFailed     = errors.New("hot-reload handshake failed")
    ErrSecurityVulnerability = errors.New("SAST scan found security vulnerabilities")
)
```
