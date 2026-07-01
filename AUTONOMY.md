# Autonomous Software Factory — Implementation Plan

This document is the authoritative implementation plan for transitioning `noctifab` from Level 2/3 to **Level 5 Autonomous Software Factory (Dark Factory)**.

Every task has an ID, estimated effort, acceptance criteria, file-by-file change specification, interface contracts, test requirements, risk assessment, and rollback strategy.

---

## Table of Contents

1. [Plan Overview & Milestones](#1-plan-overview--milestones)
2. [Task Dependency Graph](#2-task-dependency-graph)
3. [Phase 1 — Resilience (Tasks AUT-100 – AUT-199)](#3-phase-1--resilience)
4. [Phase 2 — Liveness (Tasks AUT-200 – AUT-299)](#4-phase-2--liveness)
5. [Phase 3 — Prompt Guard (Tasks AUT-300 – AUT-399)](#5-phase-3--prompt-guard)
6. [Phase 4 — Self-Repair (Tasks AUT-400 – AUT-499)](#6-phase-4--self-repair)
7. [Phase 5 — Self-Healing (Tasks AUT-500 – AUT-599)](#7-phase-5--self-healing)
8. [Phase 6 — Self-Evolution (Tasks AUT-600 – AUT-699)](#8-phase-6--self-evolution)
9. [Schedule & Milestones](#9-schedule--milestones)
10. [Appendices](#10-appendices)

---

## 1. Plan Overview & Milestones

### Milestone Definitions

| Milestone | Phase | Tasks | Definition of Done | Target |
|-----------|-------|-------|--------------------|--------|
| M1: Network Resilience | Phase 1 | AUT-101–103 | Daemon survives 429/503 without human intervention; failover chain operational | Day 3 |
| M2: Hang Prevention | Phase 3 | AUT-301–302 | LLM-generated code includes thread-safety invariants; no deadlock hangs from generated Python/Go | Day 4 |
| M3: Self-Repair | Phase 4 | AUT-401–402 | Watchdog kill triggers automated diagnosis + rewrite loop; failed tasks auto-recover | Day 7 |
| M4: Cost Control | Phase 1 | AUT-102 | Budget tracked in DB across restarts; daily cap enforced | Day 8 |
| M5: Flaky Elimination | Phase 5 | AUT-502 | Flaky tests auto-detected and auto-stabilized | Day 10 |
| M6: Environment Healing | Phase 5 | AUT-501 | Missing toolchains auto-installed | Day 12 |
| M7: Observability | Phase 5 | AUT-503 | OpenTelemetry spans emitted for all major operations | Day 15 |
| M8: Security Baseline | Phase 6 | AUT-603 | SAST scanners run before PR creation; high-severity blocks merge | Day 17 |
| M9: Self-Evolution | Phase 6 | AUT-601–602 | Daemon patches, builds, tests, and hot-reloads its own binary | Day 22 |
| M10: Intent Disambiguation | Phase 6 | AUT-604 | Ambiguous specs resolved via git context without human pause | Day 24 |

### Current State

```
Phase 1 (Resilience):       40% — FailoverClient exists but is DEAD CODE (not wired in serve.go or start_one.go)
Phase 2 (Liveness):        100% — Complete and tested
Phase 3 (Prompt Guard):      0%
Phase 4 (Self-Repair):       0%
Phase 5 (Self-Healing):      0%
Phase 6 (Self-Evolution):    0%
```

---

## 2. Task Dependency Graph

```
AUT-101 ──────────────> AUT-102 ──> AUT-103
                                      │
AUT-301 ──> AUT-302                   │
               │                      │
               ▼                      ▼
AUT-401 ──────────────────────────> AUT-402
                                      │
AUT-501 ──> AUT-502 ──> AUT-503      │
               │                      │
               ▼                      ▼
AUT-601 ──> AUT-602 ──> AUT-603 ──> AUT-604
```

**Legend**:
- Horizontal arrow = sequential dependency
- Vertical alignment = parallelizable
- AUT-103 and AUT-402 are on the critical path to M3

---

## 3. Phase 1 — Resilience

### Goal
Daemon survives network failures, API quota exhaustion, and provider outages without halting.

---

### AUT-101: Wire FailoverClient into Production

| Field | Value |
|-------|-------|
| **Effort** | 2 days |
| **Dependencies** | None |
| **Risk** | Low — existing code, just wiring |
| **Rollback** | Revert `serve.go` and `start_one.go` to `llm.NewClient` |

#### Acceptance Criteria

1. When `llm.failover.enabled: false`, `BuildFailoverClient` returns a plain `*Client` (backward compatible)
2. When `llm.failover.enabled: true`, `BuildFailoverClient` returns a `*FailoverClient` with backends from config
3. CLI flags `serve` and `start-one` use `BuildFailoverClient` instead of `NewClient`
4. Config parsing round-trips correctly (marshal → unmarshal → compare)
5. All backends on cooldown → `Complete` returns error: `all LLM backends failed. Last error: ...`
6. `Enabled: false` with no config change → existing production behavior unchanged

#### File Change Specifications

**`pkg/infrastructure/config/types.go`** (modify):

After line 34 (`MaxBudgetUSD`), insert:
```go
// FailoverConfig controls multi-provider LLM failover behavior.
type FailoverConfig struct {
    Enabled      bool              `yaml:"enabled"`
    Cooldown     Duration          `yaml:"cooldown"`
    MaxCallLimit int               `yaml:"max_call_limit"`
    Backends     []FailoverBackend `yaml:"backends"`
}
type FailoverBackend struct {
    Provider   string `yaml:"provider"`
    Model      string `yaml:"model"`
    APIKeyEnv  string `yaml:"api_key_env"`
    URL        string `yaml:"url"`
    MaxRetries int    `yaml:"max_retries"`
}
```

In `LLMConfig`, add field:
```go
Failover FailoverConfig `yaml:"failover"`
```

**`pkg/infrastructure/config/defaults.go`** (modify):

After `MaxBudgetUSD: 10.0,` add:
```go
Failover: FailoverConfig{
    Enabled:      false,
    Cooldown:     Duration(5 * time.Minute),
    MaxCallLimit: 0,
    Backends:     nil,
},
```

**`pkg/infrastructure/llm/factory.go`** (new file):

```go
package llm

import (
    "os"

    "github.com/diegojromerolopez/noctifab/pkg/domain"
    "github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

// BuildFailoverClient constructs the appropriate LLM client based on config.
// If failover is disabled (default), returns a plain *Client.
// If failover is enabled, returns a *FailoverClient wrapping all backends.
func BuildFailoverClient(cfg *config.LLMConfig) domain.LLMClient {
    if !cfg.Failover.Enabled || len(cfg.Failover.Backends) == 0 {
        return NewClient(
            cfg.Provider, cfg.Model, cfg.APIKeyValue,
            cfg.MaxRetries, time.Duration(cfg.RetryBackoff), cfg.URL,
        )
    }

    backends := make([]NamedClient, 0, len(cfg.Failover.Backends))
    for _, b := range cfg.Failover.Backends {
        apiKey := os.Getenv(b.APIKeyEnv)
        client := NewClient(b.Provider, b.Model, apiKey, b.MaxRetries, time.Duration(cfg.RetryBackoff), b.URL)
        backends = append(backends, NamedClient{
            Name:   b.Provider + "/" + b.Model,
            Client: client,
        })
    }

    return NewFailoverClient(backends, time.Duration(cfg.Failover.Cooldown), cfg.Failover.MaxCallLimit)
}
```

**`cmd/noctifab/cli/serve.go`** — replace line 82–85:
```go
// Before (line 82):
llmClient := llm.NewClient(cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKeyValue, cfg.LLM.MaxRetries, time.Duration(cfg.LLM.RetryBackoff), cfg.LLM.URL)
// After:
llmClient := llm.BuildFailoverClient(&cfg.LLM)
```

**`cmd/noctifab/cli/start_one.go`** — replace line 74:
```go
// Before (line 74):
llmClient := llm.NewClient(cfg.LLM.Provider, cfg.LLM.Model, cfg.LLM.APIKeyValue, cfg.LLM.MaxRetries, time.Duration(cfg.LLM.RetryBackoff), cfg.LLM.URL)
// After:
llmClient := llm.BuildFailoverClient(&cfg.LLM)
```

#### Interface Contracts

- `BuildFailoverClient` must accept `*config.LLMConfig` and return `domain.LLMClient`
- Must not modify `*config.LLMConfig` (no side effects)
- Must be safe for concurrent calls (no shared state in construction)

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestBuildFailoverClient_Disabled` | `factory_test.go` (new) | `Enabled: false` returns non-failover `*Client` |
| `TestBuildFailoverClient_Enabled` | `factory_test.go` | `Enabled: true` returns `*FailoverClient` with correct backend count |
| `TestBuildFailoverClient_EmptyBackends` | `factory_test.go` | `Enabled: true` but empty backends falls back to `*Client` |
| `TestBuildFailoverClient_AllCooldown` | `factory_test.go` | All backends on cooldown → error wraps last error |
| `TestFailoverConfigYAML` | `config/types_test.go` | Marshal/unmarshal round-trip preserves all fields |
| `TestServeUsesFailoverClient` | `cmd/noctifab/cli/serve_test.go` (new) | Integration test verifying `BuildFailoverClient` is called |

---

### AUT-102: Daily USD Budget Tracking with DB Persistence

| Field | Value |
|-------|-------|
| **Effort** | 2 days |
| **Dependencies** | AUT-101 (FailoverClient must exist to add budget to it) |
| **Risk** | Medium — DB schema migration required; both SQLite and Postgres |
| **Rollback** | Revert schema migration; remove `budget` table create from migration |

#### Acceptance Criteria

1. `BudgetStore` interface has `LoadBudget`, `SaveBudget`, `ListBudgets` methods
2. SQLite and Postgres both implement `BudgetStore`
3. `FailoverClient.Complete` checks budget before each call; skips provider if `CostUSD >= maxBudgetUSD`
4. After successful LLM call, budget record is updated (tokens_in, tokens_out, cost_usd)
5. Records are keyed by `(date, provider)` — rollover at midnight UTC resets counter
6. `ResetPeriod` supports `daily`, `weekly`, `monthly`
7. If `budget` table doesn't exist on startup, it's auto-created via migration
8. Existing `NoctifabDB` tests pass without modification (backward compatible)

#### File Change Specifications

**`pkg/domain/budget.go`** (new):

```go
package domain

import (
    "context"
    "time"
    "math"
)

type BudgetRecord struct {
    Date      string    `json:"date"`       // "2026-07-01"
    Provider  string    `json:"provider"`   // "openai"
    TokensIn  int64     `json:"tokens_in"`
    TokensOut int64     `json:"tokens_out"`
    CostUSD   float64   `json:"cost_usd"`
    UpdatedAt time.Time `json:"updated_at"`
}

// CostForTokens estimates USD cost for token counts using standard pricing.
// Returns 0 if provider is unknown (safe default).
func CostForTokens(provider string, tokensIn, tokensOut int64) float64 {
    rates := map[string]struct{ in, out float64 }{
        "openai/gpt-4o":       {0.0000025, 0.00001},
        "gemini/gemini-2.5":   {0.00000125, 0.000005},
        "anthropic/claude-3":  {0.000003, 0.000015},
    }
    // Match prefix for model variants
    for key, rate := range rates {
        if strings.Contains(provider, key) {
            return float64(tokensIn)*rate.in + float64(tokensOut)*rate.out
        }
    }
    // Fallback: openai gpt-4o-mini pricing
    return float64(tokensIn)*0.00000015 + float64(tokensOut)*0.0000006
}

type BudgetStore interface {
    LoadBudget(ctx context.Context, date, provider string) (*BudgetRecord, error)
    SaveBudget(ctx context.Context, record *BudgetRecord) error
    ListBudgets(ctx context.Context, since time.Time) ([]BudgetRecord, error)
}
```

**`pkg/infrastructure/storage/sqlite_budget.go`** (new):

```go
package storage

import (
    "context"
    "database/sql"
    "time"
    "github.com/diegojromerolopez/noctifab/pkg/domain"
)

type sqliteBudgetStore struct {
    db *sql.DB
}

func (s *sqliteBudgetStore) LoadBudget(ctx context.Context, date, provider string) (*domain.BudgetRecord, error) {
    row := s.db.QueryRowContext(ctx,
        `SELECT date, provider, tokens_in, tokens_out, cost_usd, updated_at
         FROM budget WHERE date = ? AND provider = ?`, date, provider)
    // ... scan into BudgetRecord
}

func (s *sqliteBudgetStore) SaveBudget(ctx context.Context, record *domain.BudgetRecord) error {
    _, err := s.db.ExecContext(ctx,
        `INSERT INTO budget (date, provider, tokens_in, tokens_out, cost_usd, updated_at)
         VALUES (?, ?, ?, ?, ?, ?)
         ON CONFLICT(date, provider) DO UPDATE SET
           tokens_in = excluded.tokens_in,
           tokens_out = excluded.tokens_out,
           cost_usd = excluded.cost_usd,
           updated_at = excluded.updated_at`,
        record.Date, record.Provider, record.TokensIn, record.TokensOut, record.CostUSD, record.UpdatedAt)
    return err
}

func (s *sqliteBudgetStore) ListBudgets(ctx context.Context, since time.Time) ([]domain.BudgetRecord, error) {
    // ... SELECT with WHERE updated_at >= ?
}
```

**Migration** — add to existing migration (both SQLite and Postgres):
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

**`pkg/infrastructure/llm/failover_client.go`** — modify `Complete`:

Add after line 50 (the callCount check):
```go
if f.budgetStore != nil && f.maxBudgetUSD > 0 {
    today := time.Now().UTC().Format("2006-01-02")
    record, err := f.budgetStore.LoadBudget(ctx, today, backend.Name)
    if err == nil && record.CostUSD >= f.maxBudgetUSD {
        continue // skip this provider, budget exceeded
    }
}
```

Add after successful response (before `return resp, nil`):
```go
if f.budgetStore != nil {
    today := time.Now().UTC().Format("2006-01-02")
    record, _ := f.budgetStore.LoadBudget(ctx, today, backend.Name)
    if record == nil {
        record = &domain.BudgetRecord{
            Date:      today,
            Provider:  backend.Name,
            UpdatedAt: time.Now(),
        }
    }
    record.TokensIn += countTokens(prompt)
    record.TokensOut += countTokens(resp.Reasoning)
    record.CostUSD = domain.CostForTokens(backend.Name, record.TokensIn, record.TokensOut)
    record.UpdatedAt = time.Now()
    _ = f.budgetStore.SaveBudget(ctx, record)
}
```

**`pkg/infrastructure/config/types.go`** — add to `LLMConfig`:
```go
ResetPeriod string `yaml:"reset_period"` // "daily" (default), "weekly", "monthly"
```

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestSQLiteBudgetStore_CRUD` | `sqlite_budget_test.go` | Create, read, update budget record |
| `TestSQLiteBudgetStore_UpdateExisting` | `sqlite_budget_test.go` | UPSERT increments rather than replacing |
| `TestBudgetExceeded_SkipProvider` | `failover_client_test.go` | Budget > max → skip provider |
| `TestBudgetNotExceeded_CallProceeds` | `failover_client_test.go` | Budget < max → call goes through |
| `TestCostForTokens_KnownProvider` | `budget_test.go` | Correct rate applied for openai/gpt-4o |
| `TestCostForTokens_UnknownProvider` | `budget_test.go` | Safe fallback rate, no error |
| `TestResetPeriod_Daily` | `failover_client_test.go` | New day = new budget window |
| `Edge: zero maxBudgetUSD` | `failover_client_test.go` | 0 = unlimited (skip budget check, no skip) |
| `Edge: missing budget table` | `sqlite_budget_test.go` | Auto-created on first SaveBudget |

---

### AUT-103: Universal Interruptible Sleep

| Field | Value |
|-------|-------|
| **Effort** | 1 day |
| **Dependencies** | AUT-102 (optional — can be done in parallel) |
| **Risk** | Low — well-understood pattern, already proven in `updateStateWithRetry` |
| **Rollback** | Revert changes to `orchestrator.Start()` and `serve.go` |

#### Acceptance Criteria

1. `orchestrator.Start()` replaces `time.NewTicker` loop with `SleepWithInterrupt` using mailbox wakeup channel
2. When mailbox receives a command during poll interval, orchestrator wakes up and re-polls immediately
3. `runServerLoop` in `serve.go` selects on both `storyCh` and `ctx.Done()` (verify it already does)
4. All existing tests continue to pass

#### File Change Specifications

**`pkg/usecase/orchestrator.go`** — replace `Start` method (lines 43–58):

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

**`cmd/noctifab/cli/serve.go`** — verify `runServerLoop` already selects on `ctx.Done()` (line 172). No change needed.

**`pkg/usecase/command_channel.go`** — no changes needed (SleepWithInterrupt already supports wakeup channel).

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| Existing `TestSleepWithInterrupt_*` | `command_channel_test.go` | 5 existing tests pass unchanged |
| `TestOrchestratorStart_WakesOnMailbox` | `orchestrator_test.go` (new) | Send command to mailbox during Start() → RunOnce is called within 1s |

---

## 4. Phase 2 — Liveness

### Completed — No remaining work.

| AUT ID | Item | Status | Evidence |
|--------|------|--------|----------|
| AUT-201 | Watchdog with MaxDuration + IdleTimeout | ✅ | `pkg/usecase/watchdog.go` |
| AUT-202 | HostSandbox integration | ✅ | `pkg/usecase/sandbox.go:95` |
| AUT-203 | Idle timeout E2E tests (5 tests) | ✅ | `tests/e2e/e2e_test.go` |
| AUT-204 | Config schema (IdleTimeoutSeconds) | ✅ | `pkg/infrastructure/config/types.go` |
| AUT-205 | Default idle timeout 30s | ✅ | `pkg/infrastructure/config/defaults.go` |

---

## 5. Phase 3 — Prompt Guard

### Goal
Prevent the LLM from generating code with concurrency bugs: background threads that swallow exceptions, missing `KeyboardInterrupt` propagation, non-daemon threads, goroutines that ignore context cancellation.

---

### AUT-301: Concurrency Invariants in Agent Prompts

| Field | Value |
|-------|-------|
| **Effort** | 1 day |
| **Dependencies** | None |
| **Risk** | Low — additive change, no runtime logic modified |
| **Rollback** | Revert `prompts.go`; remove `PromptBuilder.Build` call in `client.go` |

#### Acceptance Criteria

1. `PromptBuilder` has separate invariant blocks for Python, Go, and default (empty)
2. Python block includes all 4 invariants from AUTONOMY.md §3.A (exception capture, thread join, daemon threads, SIGINT handler)
3. Go block includes all 4 invariants (ctx.Done(), WaitGroup, buffered channels, sync.Once)
4. Unknown language returns empty string (no invariants injected)
5. `PromptBuilder.Build(prompt)` appends invariants after a blank line separator
6. Invariants are not injected when the prompt already contains them (idempotent)
7. Existing tests in `client_test.go` pass unchanged (backward compatible)

#### File Change Specifications

**`pkg/infrastructure/llm/prompts.go`** (new):

```go
package llm

import (
    "strings"
    "github.com/diegojromerolopez/noctifab/pkg/domain"
)

type PromptBuilder struct {
    Role         domain.AgentRole
    DetectedLang string
}

func (pb *PromptBuilder) Build(prompt string) string {
    invariants := pb.concurrencyInvariants()
    if invariants == "" {
        return prompt
    }
    // Idempotency: don't inject if already present
    if strings.Contains(prompt, "CONCURRENCY & THREADING INVARIANTS") {
        return prompt
    }
    return prompt + "\n\n" + invariants
}

func (pb *PromptBuilder) concurrencyInvariants() string {
    switch pb.DetectedLang {
    case "python":
        return pythonInvariants
    case "go":
        return goInvariants
    default:
        return ""
    }
}

const pythonInvariants = `CONCURRENCY & THREADING INVARIANTS (Python):
1. If executing a task function inside a background thread, capture any
   raised exceptions (including BaseException classes like KeyboardInterrupt
   or SystemExit) and propagate them back to the main thread.
2. The main loop must join or check the thread status frequently
   (e.g., t.join(0.1)) and re-raise any captured exception immediately.
3. Set daemon=True on ALL background threads before t.start().
4. Use signal.signal(signal.SIGINT, handler) to handle Ctrl+C explicitly
   when threads are involved.`

const goInvariants = `CONCURRENCY & THREADING INVARIANTS (Go):
1. Always select on ctx.Done() in goroutines that perform blocking
   operations — never block indefinitely without a context check.
2. Use sync.WaitGroup to track goroutine completion; always call
   wg.Wait() before returning from functions that spawn goroutines.
3. Use buffered channels (size >= 1) for signalling to avoid deadlock
   if the receiver has exited.
4. Use sync.Once for lazy initialization in concurrent contexts.`
```

**`pkg/infrastructure/llm/client.go`** — modify `Complete`:

After line where prompt is received, wrap it:
```go
builder := &PromptBuilder{
    Role:         detectRoleFromPrompt(prompt),
    DetectedLang: extractLangFromState(), // passed via context or state
}
enrichedPrompt := builder.Build(prompt)
// Use enrichedPrompt instead of prompt for the LLM call
```

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestPromptBuilder_PythonInvariants` | `prompts_test.go` (new) | All 4 Python invariants present |
| `TestPromptBuilder_GoInvariants` | `prompts_test.go` | All 4 Go invariants present |
| `TestPromptBuilder_UnknownLang` | `prompts_test.go` | Empty string returned |
| `TestPromptBuilder_Idempotent` | `prompts_test.go` | Prompt containing "CONCURRENCY & THREADING INVARIANTS" is not modified |
| `TestPromptBuilder_AppendsAfterBlankLine` | `prompts_test.go` | Invariants separated by `\n\n` from original prompt |
| `Edge: empty prompt` | `prompts_test.go` | Build("") returns "\n\n" + invariants |
| `Edge: role is empty string` | `prompts_test.go` | Defaults to unknown language |

---

### AUT-302: Language Detection in Sandbox

| Field | Value |
|-------|-------|
| **Effort** | 0.5 day |
| **Dependencies** | AUT-301 (PromptBuilder needs detected language) |
| **Risk** | Low |
| **Rollback** | Revert `DetectProjectLanguage` in `sandbox.go` |

#### Acceptance Criteria

1. `DetectProjectLanguage(projectPath)` returns `"go"` if `go.mod` exists
2. Returns `"rust"` if `Cargo.toml` exists
3. Returns `"javascript"` if `package.json` exists
4. Returns `"python"` if `requirements.txt` or `setup.py` exists
5. Returns `"java"` if `pom.xml` exists
6. Returns `""` for empty directory
7. Precedence: `go.mod` > `Cargo.toml` > `package.json` > `requirements.txt` / `setup.py` > `pom.xml`

#### File Change Specifications

**`pkg/usecase/sandbox.go`** — add function before `NewHostSandbox`:

```go
// DetectProjectLanguage inspects the project directory for manifest files
// and returns the detected programming language identifier.
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
    return ""
}
```

**`pkg/usecase/sandbox_test.go`** (new file — no sandbox test existed before):

```go
func TestDetectProjectLanguage_Go(t *testing.T) {
    tmp := t.TempDir()
    os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test"), 0644)
    if got := DetectProjectLanguage(tmp); got != "go" {
        t.Errorf("expected 'go', got %q", got)
    }
}
// ... one test per language, one for empty dir, one for precedence
```

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestDetectProjectLanguage_Go` | `sandbox_test.go` (new) | `go.mod` → "go" |
| `TestDetectProjectLanguage_Rust` | `sandbox_test.go` | `Cargo.toml` → "rust" |
| `TestDetectProjectLanguage_JS` | `sandbox_test.go` | `package.json` → "javascript" |
| `TestDetectProjectLanguage_Python` | `sandbox_test.go` | `requirements.txt` → "python" |
| `TestDetectProjectLanguage_PythonSetup` | `sandbox_test.go` | `setup.py` → "python" |
| `TestDetectProjectLanguage_Java` | `sandbox_test.go` | `pom.xml` → "java" |
| `TestDetectProjectLanguage_Empty` | `sandbox_test.go` | Empty dir → "" |
| `TestDetectProjectLanguage_Precedence` | `sandbox_test.go` | Both `go.mod` and `Cargo.toml` → "go" (go.mod wins) |

---

## 6. Phase 4 — Self-Repair

### Goal
When the Watchdog kills a hanging test, the orchestrator captures partial output, generates a diagnostic prompt, and feeds it back to the Generator Agent for automated rewrite.

---

### AUT-401: Watchdog Diagnostic Prompt Builder

| Field | Value |
|-------|-------|
| **Effort** | 1 day |
| **Dependencies** | AUT-301 (prompt infrastructure) |
| **Risk** | Low — isolated struct, no production wiring |
| **Rollback** | Revert `watchdog_repair.go` |

#### Acceptance Criteria

1. `WatchdogRepair.buildDiagnosticPrompt` includes: task title, task description, watchdog error, last stdout output
2. `WatchdogRepair.buildRetryPrompt` includes: previous prompt, test output, test error
3. `CategorizeFailureLog` returns `FailureTimeout` for idle timeout / max duration messages
4. `CategorizeFailureLog` returns `FailureSandbox` for sandbox violation messages
5. `CategorizeFailureLog` returns `FailureCompile` for compile / syntax error messages
6. `CategorizeFailureLog` returns `FailureTestLogic` for ERROR:/FAIL: lines
7. `CategorizeFailureLog` returns `FailureUnknown` for unrecognized logs

#### File Change Specifications

**`pkg/usecase/watchdog_repair.go`** (new — diagnostic prompt methods only, no repair loop yet):

```go
package usecase

import (
    "fmt"
    "strings"
)

// Categories for failure log classification.
type FailureCategory int

const (
    FailureUnknown     FailureCategory = iota
    FailureTestLogic   FailureCategory = iota
    FailureTimeout     FailureCategory = iota
    FailureCompile     FailureCategory = iota
    FailureSandbox     FailureCategory = iota
)

func (fc FailureCategory) String() string {
    return [...]string{"unknown", "test_logic", "timeout", "compile", "sandbox"}[fc]
}

// CategorizeFailureLog classifies a failure log by error type.
func CategorizeFailureLog(log string) FailureCategory {
    lower := strings.ToLower(log)
    switch {
    case strings.Contains(lower, "no output produced within idle timeout"),
         strings.Contains(lower, "max wall-clock duration exceeded"):
        return FailureTimeout
    case strings.Contains(lower, "sandbox violation"):
        return FailureSandbox
    case strings.Contains(lower, "compile error"),
         strings.Contains(lower, "syntax error"),
         strings.Contains(lower, "compilation error"):
        return FailureCompile
    case strings.Contains(lower, "error:"),
         strings.Contains(lower, "fail:"),
         strings.Contains(lower, "traceback"):
        return FailureTestLogic
    default:
        return FailureUnknown
    }
}

// buildDiagnosticPrompt creates the initial diagnostic prompt for a watchdog timeout.
func buildDiagnosticPrompt(title, description string, watchdogErr error, output string) string {
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

Analyze the output above and fix the issue. Rewrite any files that need changes.
Focus on making the code terminate correctly.
`, title, description, watchdogErr, output)
}

// buildRetryPrompt appends retry context to the previous diagnostic prompt.
func buildRetryPrompt(prevPrompt, testOutput string, testErr error) string {
    return fmt.Sprintf(`%s

The fix attempt was made but tests still failed or hung:

Test output:
%s

Test error: %v

Please try a different approach to fix the hang/deadlock.
`, prevPrompt, testOutput, testErr)
}
```

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestBuildDiagnosticPrompt_IncludesFields` | `watchdog_repair_test.go` (new) | Contains task title, description, error, output |
| `TestBuildRetryPrompt_AppendsContext` | `watchdog_repair_test.go` | Contains previous prompt + new test output |
| `TestCategorizeFailureLog_Timeout` | `watchdog_repair_test.go` | "no output produced within idle timeout" → `FailureTimeout` |
| `TestCategorizeFailureLog_Sandbox` | `watchdog_repair_test.go` | "sandbox violation" → `FailureSandbox` |
| `TestCategorizeFailureLog_Compile` | `watchdog_repair_test.go` | "compile error" → `FailureCompile` |
| `TestCategorizeFailureLog_TestLogic` | `watchdog_repair_test.go` | "ERROR: test failure" → `FailureTestLogic` |
| `TestCategorizeFailureLog_Unknown` | `watchdog_repair_test.go` | "random output" → `FailureUnknown` |

---

### AUT-402: Automated Rewrite Loop in Orchestrator

| Field | Value |
|-------|-------|
| **Effort** | 2 days |
| **Dependencies** | AUT-401 (diagnostic prompts), AUT-301 (prompt builder) |
| **Risk** | Medium — modifies `executeTask`; could cause infinite repair loops if LLM can't fix |
| **Rollback** | Revert changes to `orchestrator.go` executeTask |

#### Acceptance Criteria

1. When `ValidateTask` returns an error, check if `CategorizeFailureLog` is `FailureTimeout`
2. If timeout → call `AttemptRepair` (diagnose → LLM → rewrites → re-run tests)
3. `AttemptRepair` has a `maxRetries` limit (default 3); stops after exceeded
4. If `AttemptRepair` returns `Success: true` → mark task as `TaskSuccess`
5. If `AttemptRepair` returns `Success: false` → fall through to normal retry logic
6. Repair applies write_file and edit_file actions from LLM response
7. Repair is only attempted for timeout failures, not compile/sandbox/test-logic failures
8. Maximum total repair time is bounded (maxRetries × LLM call time × test run time)

#### File Change Specifications

**`pkg/usecase/watchdog_repair.go`** — add `AttemptRepair`:

```go
type WatchdogRepair struct {
    llmClient  domain.LLMClient
    maxRetries int
    sandbox    Sandbox
    tools      map[string]Tool // write_file, edit_file tools
}

type RepairResult struct {
    Success    bool
    Output     string
    FixedCode  bool
    Attempts   int
    FailureLog string
}

func NewWatchdogRepair(llmClient domain.LLMClient, sandbox Sandbox, tools map[string]Tool) *WatchdogRepair {
    return &WatchdogRepair{
        llmClient:  llmClient,
        maxRetries: 3,
        sandbox:    sandbox,
        tools:      tools,
    }
}

func (wr *WatchdogRepair) AttemptRepair(
    ctx context.Context,
    state *domain.State,
    task domain.Task,
    watchdogOutput string,
    watchdogErr error,
) (*RepairResult, error) {

    diagPrompt := buildDiagnosticPrompt(task.Title, task.Description, watchdogErr, watchdogOutput)

    for attempt := 0; attempt < wr.maxRetries; attempt++ {
        resp, err := wr.llmClient.Complete(ctx, diagPrompt)
        if err != nil {
            return nil, fmt.Errorf("repair LLM call failed: %w", err)
        }

        // Apply each action using registered tools
        for _, action := range resp.Actions {
            if tool, ok := wr.tools[action.Tool]; ok {
                if _, err := tool.Execute(ctx, state, action.Args); err != nil {
                    // Log but continue — some tool calls may fail
                    fmt.Fprintf(os.Stderr, "Repair tool %s failed: %v\n", action.Tool, err)
                }
            }
        }

        // Re-run tests — use sandbox directly
        testOutput, testErr := wr.sandbox.RunCommand(ctx, state.ProjectPath, "", "")
        if testErr == nil {
            return &RepairResult{
                Success:   true,
                Output:    testOutput,
                FixedCode: true,
                Attempts:  attempt + 1,
            }, nil
        }

        diagPrompt = buildRetryPrompt(diagPrompt, testOutput, testErr)
    }

    return &RepairResult{
        Success:    false,
        Attempts:   wr.maxRetries,
        FailureLog: "all repair attempts failed to resolve the hang/deadlock",
    }, nil
}
```

**`pkg/usecase/orchestrator.go`** — modify `executeTask`:

After the line `passed, logMsg, _ := o.evaluator.ValidateTask(ctx, state, *task)` (around line 350), add:

```go
if !passed && CategorizeFailureLog(logMsg) == FailureTimeout {
    repair := NewWatchdogRepair(o.llmClient, o.evaluator.Runner, o.registry.AllTools())
    result, repairErr := repair.AttemptRepair(ctx, state, *task, logMsg, ErrWatchdogIdleTimeout)
    if repairErr == nil && result.Success {
        logMsg = "Repaired after watchdog timeout: " + result.Output
        passed = true
    }
}
```

Add to `Registry` interface (or `ToolRegistry`):
```go
func (r *ToolRegistry) AllTools() map[string]Tool {
    return r.tools // return a copy
}
```

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestAttemptRepair_SuccessOnFirstTry` | `watchdog_repair_test.go` | Mock LLM returns fix, sandbox passes → success |
| `TestAttemptRepair_SuccessOnRetry` | `watchdog_repair_test.go` | First sandbox fails, second passes → success |
| `TestAttemptRepair_AllRetriesFail` | `watchdog_repair_test.go` | All 3 attempts fail → FailureResult |
| `TestAttemptRepair_AppliesWriteFile` | `watchdog_repair_test.go` | LLM action `write_file` is executed via tool |
| `TestAttemptRepair_AppliesEditFile` | `watchdog_repair_test.go` | LLM action `edit_file` is executed via tool |
| `TestExecuteTask_TriggersRepairOnTimeout` | `orchestrator_test.go` | `ValidateTask` returns timeout → `AttemptRepair` called |
| `TestExecuteTask_NoRepairOnCompileFailure` | `orchestrator_test.go` | Compile error → normal retry, no repair |
| `Edge: LLM returns no actions` | `watchdog_repair_test.go` | Empty actions list → repair continues (no-op) |
| `Edge: sandbox passes but LLM call fails` | `watchdog_repair_test.go` | LLM error → returns wrapped error, not repair |
| `Edge: maxRetries = 0` | `watchdog_repair_test.go` | Constructor with 0 → no repair attempted |

---

### AUT-403: Failure Log Enhancement in summarizeFailureLog

| Field | Value |
|-------|-------|
| **Effort** | 0.5 day |
| **Dependencies** | AUT-401 (CategorizeFailureLog) |
| **Risk** | Low |
| **Rollback** | Revert changes to `orchestrator_helper.go` |

#### Acceptance Criteria

1. `summarizeFailureLog` now includes failure category annotation in its output
2. The annotation is `[TIMEOUT]`, `[SANDBOX]`, `[COMPILE]`, `[TEST_LOGIC]`, or `[UNKNOWN]`
3. All existing callers of `summarizeFailureLog` continue to work

#### File Change Specifications

**`pkg/usecase/orchestrator_helper.go`** — modify `summarizeFailureLog`:

```go
func summarizeFailureLog(log string) string {
    category := CategorizeFailureLog(log)
    prefix := fmt.Sprintf("[%s] ", strings.ToUpper(category.String()))

    // Existing logic: find ERROR/FAIL lines or last 15 lines
    lines := strings.Split(log, "\n")
    var relevant []string
    for _, line := range lines {
        if strings.Contains(line, "ERROR:") || strings.Contains(line, "FAIL:") ||
           strings.Contains(line, "Traceback") || strings.Contains(line, "Exception") {
            relevant = append(relevant, line)
        }
    }

    result := strings.Join(relevant, "\n")
    if result == "" {
        // Fallback: last 15 lines
        start := len(lines) - 15
        if start < 0 {
            start = 0
        }
        result = strings.Join(lines[start:], "\n")
    }

    return prefix + "\n" + result
}
```

---

## 7. Phase 5 — Self-Healing

### Goal
Daemon survives environment drift (missing toolchains), eliminates flaky tests, and provides observability via OpenTelemetry.

---

### AUT-501: Dynamic Dependency Installation

| Field | Value |
|-------|-------|
| **Effort** | 2 days |
| **Dependencies** | None |
| **Risk** | Medium — shelling out to package managers; must sandbox to allowed managers |
| **Rollback** | Revert `dependency_manager.go`; set `auto_install_deps: false` in config |

#### Acceptance Criteria

1. `DetectMissingTool` matches "executable file not found", "command not found", "No such file or directory" in command output
2. `InstallTool` maps tool names to package manager commands via a lookup table
3. Only package managers in `AllowedPkgManagers` are used (whitelist)
4. `HostSandbox.RunCommand` intercepts `executable file not found` error, calls `InstallTool`, retries once
5. If `AutoInstallDeps: false`, skip installation, return original error
6. If tool is not in the mapping table, return error (don't install unknown tools)

#### File Change Specifications

**`pkg/infrastructure/sandbox/dependency_manager.go`** (new):

```go
package sandbox

import (
    "context"
    "fmt"
    "os/exec"
    "strings"
)

type DependencyManager struct {
    AllowedPkgManagers []string
}

// toolPackageMap maps tool binary names to the package manager command needed to install them.
var toolPackageMap = map[string]struct {
    Manager string
    Pkg     string
}{
    "cargo":           {"curl", "curl -sSf https://sh.rustup.rs | sh -s -- -y"},
    "pytest":          {"pip", "pip install pytest"},
    "golangci-lint":   {"go", "go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"},
    "node":            {"brew", "brew install node"},
    "npm":             {"brew", "brew install node"},
}

func (dm *DependencyManager) DetectMissingTool(output string) (string, bool) {
    lower := strings.ToLower(output)
    patterns := []string{
        "executable file not found",
        "command not found",
        "no such file or directory",
    }
    for _, p := range patterns {
        if strings.Contains(lower, p) {
            // Extract the tool name from common patterns:
            // "exec: \"cargo\": executable file not found" -> "cargo"
            for tool := range toolPackageMap {
                if strings.Contains(lower, tool) {
                    return tool, true
                }
            }
        }
    }
    return "", false
}

func (dm *DependencyManager) IsAllowed(manager string) bool {
    for _, m := range dm.AllowedPkgManagers {
        if m == manager {
            return true
        }
    }
    return false
}

func (dm *DependencyManager) InstallTool(ctx context.Context, tool string) error {
    entry, ok := toolPackageMap[tool]
    if !ok {
        return fmt.Errorf("unknown tool %q: no package mapping available", tool)
    }
    if !dm.IsAllowed(entry.Manager) {
        return fmt.Errorf("package manager %q not in allowed list", entry.Manager)
    }

    parts := strings.Fields(entry.Manager)
    cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("failed to install %q via %q: %w\nOutput: %s", tool, entry.Manager, err, string(output))
    }
    return nil
}
```

**`pkg/infrastructure/config/types.go`** — add to `SandboxConfig`:
```go
AutoInstallDeps bool     `yaml:"auto_install_deps"`
PackageManagers []string `yaml:"package_managers"`
```

**`pkg/usecase/sandbox.go`** — modify `HostSandbox.RunCommand`:

After line where `watchdog.Run` returns error (around line 97):
```go
if err != nil && s.depMgr != nil {
    if tool, found := s.depMgr.DetectMissingTool(string(output)); found {
        if installErr := s.depMgr.InstallTool(ctx, tool); installErr == nil {
            // Retry once after installation
            watchdog := Watchdog{IdleTimeout: s.IdleTimeout}
            output2, err2 := watchdog.Run(ctx, cmd)
            if err2 == nil {
                return string(output2), nil
            }
        }
    }
}
```

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestDetectMissingTool_ExecNotFound` | `dependency_manager_test.go` (new) | "exec: \"cargo\": executable file not found" → ("cargo", true) |
| `TestDetectMissingTool_CommandNotFound` | `dependency_manager_test.go` | "bash: cargo: command not found" → ("cargo", true) |
| `TestDetectMissingTool_NoMatch` | `dependency_manager_test.go` | "unrelated output" → ("", false) |
| `TestInstallTool_KnownTool` | `dependency_manager_test.go` | Tool in map → no error (dry-run with mock) |
| `TestInstallTool_UnknownTool` | `dependency_manager_test.go` | Tool not in map → error |
| `TestInstallTool_DisallowedManager` | `dependency_manager_test.go` | Manager not in allowed list → error |
| `Edge: AutoInstallDeps false` | `sandbox_test.go` | Config `false` → no install attempted |
| `Edge: retry succeeds after install` | `sandbox_test.go` | First run fails, install succeeds, retry passes |
| `Edge: retry fails after install` | `sandbox_test.go` | First run fails, install succeeds, retry also fails → original error returned |

---

### AUT-502: Flaky Test Auto-Stabilization

| Field | Value |
|-------|-------|
| **Effort** | 2 days |
| **Dependencies** | AUT-401 (diagnostic prompt) |
| **Risk** | Medium — LLM may not fix flaky tests reliably |
| **Rollback** | Revert changes to `test_validator.go`; flaky detection falls back to warning-only |

#### Acceptance Criteria

1. `DetectFlaky` returns `Flaky: true` when exactly 2/3 tests pass
2. `DetectFlaky` returns `Flaky: false` for all other combinations
3. When flaky detected, tests are re-run with race detection (append `-race` for Go)
4. Diagnostic prompt includes race detection output for LLM analysis
5. LLM response is applied (write_file actions), then 3x re-validation is performed
6. If after stabilization all 3 pass → task success
7. If after stabilization still flaky → mark task as flaky-stable (accept with warning)

#### File Change Specifications

**`pkg/usecase/flaky_detector.go`** (new):

```go
package usecase

import (
    "context"
    "fmt"
    "strings"
)

type TestRunResult struct {
    RunID  int
    Passed bool
    Output string
}

type FlakyResult struct {
    Flaky       bool
    FailedCount int
    PassedCount int
    Outputs     []string
}

func DetectFlaky(results []TestRunResult) *FlakyResult {
    passed, failed := 0, 0
    outputs := make([]string, len(results))
    for i, r := range results {
        if r.Passed {
            passed++
        } else {
            failed++
        }
        outputs[i] = r.Output
    }
    return &FlakyResult{
        Flaky:       passed >= 2 && failed >= 1,
        FailedCount: failed,
        PassedCount: passed,
        Outputs:     outputs,
    }
}

func BuildFlakyStabilizationPrompt(results []TestRunResult, raceOutput string) string {
    return fmt.Sprintf(`The test suite has a flaky test — it passes inconsistently across 3 runs.

Outputs:
%s

Race detection output:
%s

Analyze the test and implementation for:
- time.Sleep instead of deterministic polling or signals
- Shared state between tests (global variables, file system, env vars)
- Missing mutexes or race conditions
- Network dependency without retry or timeout
- Order-dependent test execution

Rewrite to make the test deterministic.
`, formatResults(results), raceOutput)
}

func formatResults(results []TestRunResult) string {
    var sb strings.Builder
    for i, r := range results {
        status := "PASS"
        if !r.Passed {
            status = "FAIL"
        }
        fmt.Fprintf(&sb, "Run %d: %s\n%s\n", i+1, status, r.Output)
    }
    return sb.String()
}
```

**`pkg/usecase/test_validator.go`** — modify `ValidateTask`:

After majority voting logic, before returning:
```go
flaky := DetectFlaky(results)
if flaky.Flaky {
    // Run with race detection for diagnostic
    raceOutput, _ := t.Runner.RunCommand(ctx, state.ProjectPath, t.raceCommand(), "")
    
    // Send to LLM for stabilization
    prompt := BuildFlakyStabilizationPrompt(results, raceOutput)
    resp, llmErr := t.llmClient.Complete(ctx, prompt)
    if llmErr == nil {
        for _, action := range resp.Actions {
            if action.Tool == "write_file" || action.Tool == "edit_file" {
                // Apply fix via tools
            }
        }
        // Re-validate 3x
        restabilized := t.runWithCount(ctx, state, *task, 3)
        if !DetectFlaky(restabilized).Flaky {
            passed = true
        }
    }
}
```

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestDetectFlaky_ThreePass` | `flaky_detector_test.go` (new) | 3/3 → not flaky |
| `TestDetectFlaky_TwoPassOneFail` | `flaky_detector_test.go` | 2/1 → flaky |
| `TestDetectFlaky_OnePassTwoFail` | `flaky_detector_test.go` | 1/2 → not flaky (consistently failing) |
| `TestDetectFlaky_ZeroPass` | `flaky_detector_test.go` | 0/3 → not flaky |
| `TestBuildFlakyPrompt_IncludesOutputs` | `flaky_detector_test.go` | All outputs present in prompt |
| `TestBuildFlakyPrompt_IncludesRaceOutput` | `flaky_detector_test.go` | Race detection output present |
| `Edge: empty results slice` | `flaky_detector_test.go` | DetectFlaky handles empty input without panic |

---

### AUT-503: OpenTelemetry Integration

| Field | Value |
|-------|-------|
| **Effort** | 3 days |
| **Dependencies** | None |
| **Risk** | Medium — new dependency (`go.opentelemetry.io/otel`); configuration plumbing |
| **Rollback** | Remove `telemetry.Enabled` check in `serve.go`; remove `go.opentelemetry.io/otel` from go.mod |

#### Acceptance Criteria

1. `InitTracer` creates an OTLP HTTP exporter and `TracerProvider`
2. `orchestrator.RunOnce` creates a root span `noctifab.cycle`
3. `executeTask` creates a child span `noctifab.task_worker`
4. `FailoverClient.Complete` creates a child span `noctifab.llm_completion`
5. `HostSandbox.RunCommand` creates a child span `noctifab.sandbox_command`
6. Spans are ended on both success and error paths
7. `telemetry.enabled: false` in config → no tracing, no error
8. `telemetry.enabled: true` with no endpoint → graceful fallback (stdout exporter)

#### File Change Specifications

**`go.mod`** — add:
```
go.opentelemetry.io/otel v1.28.0
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.28.0
go.opentelemetry.io/otel/sdk v1.28.0
```

**`pkg/infrastructure/telemetry/tracer.go`** (new):

```go
package telemetry

import (
    "context"
    "fmt"
    "os"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
    "go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

func InitTracer(serviceName, endpoint string) (*sdktrace.TracerProvider, error) {
    var exporter sdktrace.SpanExporter
    var err error

    if endpoint == "" {
        exporter, err = NewStdoutExporter()
    } else {
        exporter, err = otlptracehttp.New(context.Background(),
            otlptracehttp.WithEndpoint(endpoint),
            otlptracehttp.WithInsecure(),
        )
    }
    if err != nil {
        return nil, fmt.Errorf("telemetry: failed to create exporter: %w", err)
    }

    hostname, _ := os.Hostname()
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceName(serviceName),
            attribute.String("host.name", hostname),
        )),
        sdktrace.WithSampler(sdktrace.AlwaysSample()),
    )

    otel.SetTracerProvider(tp)
    tracer = tp.Tracer(serviceName)
    return tp, nil
}

func Tracer() trace.Tracer {
    return tracer
}
```

**`cmd/noctifab/cli/serve.go`** — add after config load (after line 34):

```go
if cfg.Telemetry.Enabled {
    tp, err := telemetry.InitTracer(cfg.Telemetry.ServiceName, cfg.Telemetry.Endpoint)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Warning: telemetry init failed: %v\n", err)
    } else {
        defer func() { _ = tp.Shutdown(context.Background()) }()
    }
}
```

**`pkg/usecase/orchestrator.go`** — add spans to `RunOnce` and `executeTask`:

```go
import "github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"

func (o *Orchestrator) RunOnce(ctx context.Context) error {
    ctx, span := telemetry.Tracer().Start(ctx, "noctifab.cycle")
    defer span.End()
    // ... existing body
}

// In executeTask:
func (o *Orchestrator) executeTask(ctx context.Context, stateID, taskID string) {
    ctx, span := telemetry.Tracer().Start(ctx, "noctifab.task_worker",
        trace.WithAttributes(attribute.String("task.id", taskID)))
    defer span.End()
    // ... existing body
}
```

**`pkg/infrastructure/llm/client.go`** — add span to `Complete`:

```go
func (c *Client) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
    ctx, span := telemetry.Tracer().Start(ctx, "noctifab.llm_completion",
        trace.WithAttributes(attribute.String("provider", c.provider)))
    defer span.End()
    // ... existing body
    span.SetAttributes(attribute.Int("tokens_used", tokenCount))
}
```

**`pkg/usecase/sandbox.go`** — add span to `RunCommand`:

```go
func (s *HostSandbox) RunCommand(ctx context.Context, projectPath string, command string, pkg string) (string, error) {
    ctx, span := telemetry.Tracer().Start(ctx, "noctifab.sandbox_command",
        trace.WithAttributes(attribute.String("command", command)))
    defer span.End()
    // ... existing body
}
```

**`pkg/infrastructure/config/types.go`** — add:
```go
type TelemetryConfig struct {
    Enabled     bool   `yaml:"enabled"`
    Exporter    string `yaml:"exporter"`
    Endpoint    string `yaml:"endpoint"`
    ServiceName string `yaml:"service_name"`
}
```

Add to `Config`:
```go
Telemetry TelemetryConfig `yaml:"telemetry"`
```

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestInitTracer_OTLP` | `tracer_test.go` (new) | OTLP exporter created with correct endpoint |
| `TestInitTracer_Stdout` | `tracer_test.go` | Empty endpoint → stdout exporter fallback |
| `TestSpanCreatedAndEnded` | `tracer_test.go` | Span is recorded with expected attributes |
| `TestSpanEndedOnError` | `tracer_test.go` | Span is ended even when error is returned |
| `Edge: telemetry disabled` | `serve_test.go` | `Enabled: false` → no tracer init, no error |

---

## 8. Phase 6 — Self-Evolution

### Goal
Daemon patches its own Go binary, hot-reloads without state loss, enforces security gates, and resolves ambiguous specs from git context.

---

### AUT-601: Self-Patching Compiler Loop

| Field | Value |
|-------|-------|
| **Effort** | 4 days |
| **Dependencies** | AUT-502 (flaky fix), AUT-503 (telemetry for monitoring) |
| **Risk** | High — self-modifying code could destabilize production |
| **Rollback** | Revert `self_update.go`; manual re-deploy from known-good commit |

#### Acceptance Criteria

1. `SelfUpdateManager.BuildAndTest` clones noctifab source to temp directory
2. Applies LLM-generated `write_file` patches to `.go` files in `cmd/` and `pkg/`
3. Builds new binary with `go build -o /tmp/noctifab-new ./cmd/noctifab`
4. Runs `go test ./pkg/...` on the patched code
5. If all tests pass → returns nil (binary ready at /tmp/noctifab-new)
6. If build or test fails → rolls back temp directory, returns error
7. Patches to `go.mod`/`go.sum` are rejected (security constraint)
8. Patches to files outside `cmd/` and `pkg/` are rejected

#### File Change Specifications

**`pkg/usecase/self_update.go`** (new):

```go
package usecase

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

type SelfUpdateManager struct {
    RepoPath   string // path to noctifab repository
    BinaryPath string // current binary path (e.g., os.Args[0])
    GoCmd      string // "go" (configurable for testing)
}

const selfUpdateTempDir = "/tmp/noctifab-self-update"

// allowedSelfPatchPrefixes are the only directories that can be patched.
var allowedSelfPatchPrefixes = []string{"cmd/noctifab/", "pkg/"}

func (sum *SelfUpdateManager) BuildAndTest(ctx context.Context, patches []Patch) error {
    tmpDir := filepath.Join(selfUpdateTempDir, "src")
    defer os.RemoveAll(selfUpdateTempDir)

    // 1. Copy repo to temp
    if err := sum.copyRepo(tmpDir); err != nil {
        return fmt.Errorf("self-update: failed to copy repo: %w", err)
    }

    // 2. Validate and apply patches
    for _, p := range patches {
        if err := sum.validatePatch(p); err != nil {
            return fmt.Errorf("self-update: patch validation failed: %w", err)
        }
        fullPath := filepath.Join(tmpDir, p.Path)
        if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
            return fmt.Errorf("self-update: failed to create dir for %s: %w", p.Path, err)
        }
        if err := os.WriteFile(fullPath, []byte(p.Content), 0644); err != nil {
            return fmt.Errorf("self-update: failed to write %s: %w", p.Path, err)
        }
    }

    // 3. Build
    buildCmd := exec.CommandContext(ctx, sum.GoCmd, "build", "-o", "/tmp/noctifab-new", "./cmd/noctifab")
    buildCmd.Dir = tmpDir
    if output, err := buildCmd.CombinedOutput(); err != nil {
        return fmt.Errorf("self-update: build failed: %w\nOutput: %s", err, string(output))
    }

    // 4. Test
    testCmd := exec.CommandContext(ctx, sum.GoCmd, "test", "./pkg/...")
    testCmd.Dir = tmpDir
    if output, err := testCmd.CombinedOutput(); err != nil {
        return fmt.Errorf("self-update: tests failed: %w\nOutput: %s", err, string(output))
    }

    return nil
}

type Patch struct {
    Path    string // relative path like "pkg/usecase/watchdog.go"
    Content string
}

func (sum *SelfUpdateManager) validatePatch(p Patch) error {
    // Reject go.mod / go.sum changes
    if p.Path == "go.mod" || p.Path == "go.sum" {
        return fmt.Errorf("rejected: changes to %s require human review", p.Path)
    }
    // Only allow cmd/noctifab/ and pkg/ prefixes
    allowed := false
    for _, prefix := range allowedSelfPatchPrefixes {
        if strings.HasPrefix(p.Path, prefix) {
            allowed = true
            break
        }
    }
    if !allowed {
        return fmt.Errorf("rejected: path %s is outside allowed patch directories", p.Path)
    }
    return nil
}

func (sum *SelfUpdateManager) copyRepo(dst string) error {
    // Use git clone --depth=1 for speed, or cp -R for local
    cmd := exec.Command("cp", "-R", sum.RepoPath, dst)
    return cmd.Run()
}
```

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestValidatePatch_Allowed` | `self_update_test.go` (new) | `pkg/usecase/x.go` → nil |
| `TestValidatePatch_GoMod` | `self_update_test.go` | `go.mod` → error |
| `TestValidatePatch_GoSum` | `self_update_test.go` | `go.sum` → error |
| `TestValidatePatch_OutsidePrefix` | `self_update_test.go` | `tests/x.go` → error |
| `TestValidatePatch_Docs` | `self_update_test.go` | `docs/x.md` → error |
| `TestBuildAndTest_Success` | `self_update_test.go` | Mock patches to valid files → nil |
| `TestBuildAndTest_BuildFailure` | `self_update_test.go` | Syntax error patch → error, temp dir cleaned up |
| `TestBuildAndTest_TestFailure` | `self_update_test.go` | Correct syntax but test fails → error |
| `Edge: empty patches` | `self_update_test.go` | `[]Patch{}` → builds and tests original code |

---

### AUT-602: Graceful Stateful Hot-Reload

| Field | Value |
|-------|-------|
| **Effort** | 3 days |
| **Dependencies** | AUT-601 (new binary must exist) |
| **Risk** | High — state handoff failure could lose in-flight tasks |
| **Rollback** | `HotReloadManager.Reload` returns error → old binary continues; no handoff |

#### Acceptance Criteria

1. `HotReloadManager.Reload` saves state, spawns new binary, waits for health check
2. New binary starts HTTP server on `127.0.0.1:18081` (port +1)
3. Old binary writes `handoff.json` with `status: handing_off` and new PID
4. New binary reads `handoff.json`, loads state from DB, begins orchestrating
5. New binary writes `handoff.json` with `status: active`
6. Old binary reads `status: active` and exits with code 0
7. If new binary fails health check within 30s, old binary cancels reload (rollback)

#### File Change Specifications

**`pkg/usecase/hot_reload.go`** (new):

```go
package usecase

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "time"
)

type HandoffStatus string

const (
    HandoffPending   HandoffStatus = "pending"
    HandoffHanding   HandoffStatus = "handing_off"
    HandoffActive    HandoffStatus = "active"
    HandoffFailed    HandoffStatus = "failed"
)

type HandoffState struct {
    NewPID  int           `json:"new_pid"`
    Status  HandoffStatus `json:"status"`
    Message string        `json:"message,omitempty"`
}

type HotReloadManager struct {
    PIDPath     string // .noctifab/noctifab.pid
    HandoffPath string // .noctifab/hot_reload.json
    NewBinary   string // path to new binary (from BuildAndTest)
    Workspace   string // project workspace directory
}

func (hrm *HotReloadManager) Reload(ctx context.Context) error {
    // 1. Spawn new binary
    cmd := exec.CommandContext(ctx, hrm.NewBinary, "serve", "--port", "18081")
    cmd.Dir = hrm.Workspace
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Start(); err != nil {
        return fmt.Errorf("hot-reload: failed to start new binary: %w", err)
    }

    newPID := cmd.Process.Pid

    // 2. Write handoff file
    handoff := HandoffState{NewPID: newPID, Status: HandoffHanding}
    hrm.writeHandoff(handoff)

    // 3. Wait for new binary to be healthy (up to 30s)
    if err := hrm.waitForHealth(ctx, "http://127.0.0.1:18081/healthz", 30*time.Second); err != nil {
        handoff.Status = HandoffFailed
        handoff.Message = err.Error()
        hrm.writeHandoff(handoff)
        _ = cmd.Process.Kill()
        return fmt.Errorf("hot-reload: new binary health check failed: %w", err)
    }

    // 4. Wait for active confirmation from new binary
    if err := hrm.waitForActive(ctx, 10*time.Second); err != nil {
        handoff.Status = HandoffFailed
        handoff.Message = err.Error()
        hrm.writeHandoff(handoff)
        _ = cmd.Process.Kill()
        return fmt.Errorf("hot-reload: handoff confirmation failed: %w", err)
    }

    // 5. Exit cleanly — new binary is now handling requests
    fmt.Fprintf(os.Stderr, "Hot-reload complete. New PID: %d. Exiting.\n", newPID)
    return nil
}

func (hrm *HotReloadManager) waitForHealth(ctx context.Context, url string, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        resp, err := http.Get(url)
        if err == nil && resp.StatusCode == http.StatusOK {
            resp.Body.Close()
            return nil
        }
        if resp != nil {
            resp.Body.Close()
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(500 * time.Millisecond):
        }
    }
    return fmt.Errorf("health check did not pass within %s", timeout)
}

func (hrm *HotReloadManager) waitForActive(ctx context.Context, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        handoff, err := hrm.readHandoff()
        if err == nil && handoff.Status == HandoffActive {
            return nil
        }
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-time.After(200 * time.Millisecond):
        }
    }
    return fmt.Errorf("handoff did not reach 'active' within %s", timeout)
}

func (hrm *HotReloadManager) writeHandoff(state HandoffState) {
    data, _ := json.Marshal(state)
    _ = os.WriteFile(hrm.HandoffPath, data, 0644)
}

func (hrm *HotReloadManager) readHandoff() (*HandoffState, error) {
    data, err := os.ReadFile(hrm.HandoffPath)
    if err != nil {
        return nil, err
    }
    var state HandoffState
    if err := json.Unmarshal(data, &state); err != nil {
        return nil, err
    }
    return &state, nil
}
```

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestHandoffFile_RoundTrip` | `hot_reload_test.go` (new) | Write then read → equal |
| `TestHandoffFile_JSON` | `hot_reload_test.go` | File is valid JSON |
| `TestWaitForHealth_Success` | `hot_reload_test.go` | Mock HTTP server → no error |
| `TestWaitForHealth_Timeout` | `hot_reload_test.go` | No server → error after timeout |
| `TestWaitForActive_Success` | `hot_reload_test.go` | Status transitions to active → no error |
| `TestWaitForActive_Timeout` | `hot_reload_test.go` | Status stays at handing_off → error |
| `Edge: handoff file missing` | `hot_reload_test.go` | readHandoff returns os.ErrNotExist |
| `Edge: handoff file corrupted` | `hot_reload_test.go` | Invalid JSON → unmarshal error |

---

### AUT-603: SAST Security Gates

| Field | Value |
|-------|-------|
| **Effort** | 2 days |
| **Dependencies** | None |
| **Risk** | Low — additive check, doesn't affect execution |
| **Rollback** | Set `sast.enabled: false` in config |

#### Acceptance Criteria

1. `SASTScanner.Run` executes gosec (for Go) and bandit (for Python) when configured
2. Parses scanner output into structured `SecurityIssue` with severity, file, line, description
3. `FailOnSeverity: "high"` → only high-severity issues block the PR
4. `FailOnSeverity: "medium"` → medium and high block
5. Results are stored in state as `ValidationCriterion` items
6. If SAST is disabled or scanner not found → no error, no block

#### File Change Specifications

**`pkg/usecase/sast_scanner.go`** (new):

```go
package usecase

import (
    "bufio"
    "context"
    "fmt"
    "os/exec"
    "strconv"
    "strings"
)

type SASTConfig struct {
    Enabled        bool     `yaml:"enabled"`
    Scanners       []string `yaml:"scanners"`
    FailOnSeverity string   `yaml:"fail_on_severity"` // "high", "medium", "low"
}

type SecurityIssue struct {
    Scanner     string `json:"scanner"`
    Severity    string `json:"severity"`
    File        string `json:"file"`
    Line        int    `json:"line"`
    Description string `json:"description"`
}

type SASTResult struct {
    Passed bool            `json:"passed"`
    Issues []SecurityIssue `json:"issues"`
}

type SASTScanner struct {
    Config SASTConfig
}

func (s *SASTScanner) Run(ctx context.Context, projectPath string) (*SASTResult, error) {
    if !s.Config.Enabled {
        return &SASTResult{Passed: true}, nil
    }

    var allIssues []SecurityIssue

    for _, scanner := range s.Config.Scanners {
        switch scanner {
        case "gosec":
            issues, err := s.runGosec(ctx, projectPath)
            if err != nil {
                return nil, fmt.Errorf("SAST: gosec failed: %w", err)
            }
            allIssues = append(allIssues, issues...)
        case "bandit":
            issues, err := s.runBandit(ctx, projectPath)
            if err != nil {
                return nil, fmt.Errorf("SAST: bandit failed: %w", err)
            }
            allIssues = append(allIssues, issues...)
        }
    }

    blocked := false
    for _, issue := range allIssues {
        if s.isBlockingSeverity(issue.Severity) {
            blocked = true
            break
        }
    }

    return &SASTResult{
        Passed: !blocked,
        Issues: allIssues,
    }, nil
}

func (s *SASTScanner) severityScore(sev string) int {
    switch strings.ToLower(sev) {
    case "high":
        return 3
    case "medium":
        return 2
    case "low":
        return 1
    default:
        return 0
    }
}

func (s *SASTScanner) isBlockingSeverity(sev string) bool {
    return s.severityScore(sev) >= s.severityScore(s.Config.FailOnSeverity)
}

func (s *SASTScanner) runGosec(ctx context.Context, projectPath string) ([]SecurityIssue, error) {
    cmd := exec.CommandContext(ctx, "gosec", "-fmt", "json", "./...")
    cmd.Dir = projectPath
    output, err := cmd.Output()
    if err != nil {
        // gosec returns non-zero exit if issues found — parse output anyway
    }
    return parseGosecJSON(string(output))
}

func (s *SASTScanner) runBandit(ctx context.Context, projectPath string) ([]SecurityIssue, error) {
    cmd := exec.CommandContext(ctx, "bandit", "-r", "-f", "json", ".")
    cmd.Dir = projectPath
    output, err := cmd.Output()
    if err != nil {
        // bandit returns non-zero if issues found
    }
    return parseBanditJSON(string(output))
}
```

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestSAST_Disabled` | `sast_scanner_test.go` (new) | `Enabled: false` → Passed: true, no issues |
| `TestSAST_NoScanners` | `sast_scanner_test.go` | Empty scanners list → Passed: true |
| `TestSeverityScore_high` | `sast_scanner_test.go` | "high" → 3 |
| `TestSeverityScore_unknown` | `sast_scanner_test.go` | "critical" → 0 |
| `TestIsBlocking_HighWhenHigh` | `sast_scanner_test.go` | FailOnSeverity=high, issue=high → true |
| `TestIsBlocking_MediumWhenHigh` | `sast_scanner_test.go` | FailOnSeverity=high, issue=medium → false |
| `TestParseGosecJSON` | `sast_scanner_test.go` | Parse valid gosec JSON into SecurityIssue |
| `TestParseBanditJSON` | `sast_scanner_test.go` | Parse valid bandit JSON into SecurityIssue |
| `Edge: gosec not installed` | `sast_scanner_test.go` | exec.ErrNotFound → wrapped error |
| `Edge: bandit output empty` | `sast_scanner_test.go` | Empty JSON → no issues, no error |

---

### AUT-604: Zero-Clarification Intent Disambiguation

| Field | Value |
|-------|-------|
| **Effort** | 2 days |
| **Dependencies** | None |
| **Risk** | Low — clarification pause is the fallback; disambiguation is best-effort |
| **Rollback** | Remove `IntentDisambiguator` from orchestrator wiring |

#### Acceptance Criteria

1. `IntentDisambiguator.Disambiguate` returns an inferred answer for a clarification question
2. Context gathered includes: last 30 git log entries, base branch, feature name
3. If LLM returns a valid response, clarification is auto-resolved with inferred answer
4. If LLM returns error or empty response, clarification remains unresolved (human needed)
5. Disambiguation is only attempted once per clarification (not retried)
6. The inferred answer and decision are logged in state's LastActions

#### File Change Specifications

**`pkg/usecase/intent_disambiguator.go`** (new):

```go
package usecase

import (
    "context"
    "fmt"
    "time"

    "github.com/diegojromerolopez/noctifab/pkg/domain"
)

type IntentDisambiguator struct {
    gitClient *GitClient
    llmClient domain.LLMClient
}

func NewIntentDisambiguator(gitClient *GitClient, llmClient domain.LLMClient) *IntentDisambiguator {
    return &IntentDisambiguator{
        gitClient: gitClient,
        llmClient: llmClient,
    }
}

func (id *IntentDisambiguator) Disambiguate(ctx context.Context, clarification domain.Clarification, state *domain.State) (string, error) {
    // 1. Gather context
    gitLog, _ := id.gitClient.Run(ctx, false, "log", "--oneline", "-30")
    if gitLog == "" {
        gitLog = "(no git history available)"
    }

    codeFiles := ""
    for _, f := range state.Files {
        codeFiles += f.Path + "\n"
    }

    // 2. Build prompt
    prompt := fmt.Sprintf(`The system needs to resolve an ambiguity during autonomous development.

Question from the agent: %s

Context:
- Base branch: %s
- Feature: %s
- Files in workspace:
%s
- Recent commits:
%s

Analyze this context and infer the most likely intended behavior.
Respond with a JSON object: {"answer": "your inferred answer here"}
Be concise — answer in 1-2 sentences.
`, clarification.Question, state.Metadata.BaseBranch, state.Metadata.FeatureName,
        codeFiles, gitLog)

    // 3. Call LLM
    resp, err := id.llmClient.Complete(ctx, prompt)
    if err != nil {
        return "", fmt.Errorf("disambiguation LLM call failed: %w", err)
    }

    if len(resp.Actions) == 0 {
        return "", fmt.Errorf("disambiguation LLM returned no actions")
    }

    answer, ok := resp.Actions[0].Args["answer"].(string)
    if !ok || answer == "" {
        return "", fmt.Errorf("disambiguation LLM response missing 'answer' field")
    }

    return answer, nil
}
```

**`pkg/usecase/orchestrator.go`** — add field and init:

```go
type Orchestrator struct {
    // ... existing fields ...
    intentDisambiguator *IntentDisambiguator
}

func NewOrchestrator(..., intent *IntentDisambiguator) *Orchestrator {
    return &Orchestrator{
        // ... existing assignments ...
        intentDisambiguator: intent,
    }
}
```

#### Test Specifications

| Test | File | What it verifies |
|------|------|------------------|
| `TestDisambiguate_ReturnsAnswer` | `intent_disambiguator_test.go` (new) | Mock LLM returns answer → answer returned |
| `TestDisambiguate_LLMFails` | `intent_disambiguator_test.go` | Mock LLM returns error → error propagated |
| `TestDisambiguate_EmptyAnswer` | `intent_disambiguator_test.go` | Mock LLM returns empty answer → error |
| `TestDisambiguate_GitLogInContext` | `intent_disambiguator_test.go` | Prompt contains git log output |
| `TestDisambiguate_FileContext` | `intent_disambiguator_test.go` | Prompt contains workspace files |
| `Edge: no git history` | `intent_disambiguator_test.go` | gitClient.Run fails → "(no git history available)" in prompt |
| `Edge: nil disambiguator` | `orchestrator_test.go` | orchestrator with nil disambiguator → clarification blocks normally |

---

## 9. Schedule & Milestones

### Critical Path

```
AUT-101 (2d) → AUT-102 (2d) → AUT-103 (1d)
                                    │
AUT-301 (1d) → AUT-302 (0.5d)      │
                      │             │
                      ▼             ▼
               AUT-401 (1d) → AUT-402 (2d) → M3 (Day 7)
                                    │
AUT-501 (2d) → AUT-502 (2d) → AUT-503 (3d) → M7 (Day 15)
                                              │
                    AUT-601 (4d) → AUT-602 (3d) → AUT-603 (2d) → AUT-604 (2d) → M10 (Day 24)
```

### Milestone Schedule

```
Week 1 (Days 1-5):    M1 (Day 3), M2 (Day 4)
Week 2 (Days 6-10):   M3 (Day 7), M4 (Day 8), M5 (Day 10)
Week 3 (Days 11-15):  M6 (Day 12), M7 (Day 15)
Week 4 (Days 16-20):  M8 (Day 17), M9 (Day 22)
Week 5 (Days 21-25):  M10 (Day 24)
```

### Parallel Tracks

```
Track A (Phases 1,4): AUT-101 → AUT-102 → AUT-103 → AUT-401 → AUT-402
Track B (Phase 3):    AUT-301 → AUT-302
Track C (Phases 5):   AUT-501 → AUT-502 → AUT-503
Track D (Phase 6):    AUT-601 → AUT-602 → AUT-603 → AUT-604
```

Tracks A and B merge at AUT-402 (self-repair needs prompt infrastructure).
Track C joins after AUT-402.
Track D starts after AUT-503 has telemetry for monitoring self-patching.

---

## 10. Appendices

### A. Risk Register

| Risk | Phase | Likelihood | Impact | Mitigation |
|------|-------|-----------|--------|------------|
| Failover chain never used (always returns to primary) | 1 | Low | Low | Add integration test with mock 429 |
| Budget table migration fails on Postgres | 1 | Low | Medium | Test both SQLite and Postgres migration paths |
| LLM repair loop makes code worse | 4 | Medium | Medium | maxRetries=3; manual override via CLI `--skip-repair` flag |
| Package install blocked by permissions | 5 | Medium | High | `AutoInstallDeps: false` by default; require explicit opt-in |
| Self-patch introduces compile error | 6 | High | High | Build in temp dir; never modify source; full test suite before promote |
| Hot-reload handoff drops in-flight task | 6 | Low | High | Tasks are atomic per RunOnce cycle; interrupted tasks reloaded as PENDING |
| SAST scanner not installed | 6 | Medium | Low | Scanner missing → log warning, continue (no block) |

### B. Error Taxonomy (Final)

```go
// Phase 1 — Resilience
var ErrBudgetExhausted   = errors.New("LLM token/budget exhausted")
var ErrAllBackendsFailed = errors.New("all LLM backends exhausted or on cooldown")

// Phase 2 — Liveness
var ErrWatchdogMaxDuration = errors.New("command killed: max wall-clock duration exceeded")
var ErrWatchdogIdleTimeout = errors.New("command killed: no output produced within idle timeout")

// Phase 4 — Self-Repair
var ErrRepairFailed       = errors.New("all repair attempts failed to fix the issue")
var ErrHangDiagnosed      = errors.New("hang detected and diagnostic prompt generated")

// Phase 5 — Self-Healing
var ErrMissingDependency  = errors.New("required toolchain not found and auto-install failed")
var ErrFlakyTestDetected  = errors.New("test suite has non-deterministic results post-stabilization")

// Phase 6 — Self-Evolution
var ErrSelfPatchFailed    = errors.New("self-patch build or test suite failed")
var ErrHotReloadFailed    = errors.New("hot-reload handshake failed")
var ErrSecurityVulnerability = errors.New("SAST scan found blocking security vulnerabilities")
```

### C. Final Config Schema (`config.yaml`)

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
```

### D. Effort Summary

| Priority | Tasks | Days | Cumulative |
|----------|-------|------|------------|
| P0 | AUT-101, AUT-301 | 3 | 3 |
| P1 | AUT-102, AUT-103, AUT-302, AUT-401, AUT-402, AUT-502 | 8.5 | 11.5 |
| P2 | AUT-501 | 2 | 13.5 |
| P3 | AUT-503, AUT-603 | 5 | 18.5 |
| P4 | AUT-601, AUT-602, AUT-604 | 9 | 27.5 |

**Total estimated effort: 25–28 working days**

### E. Rollback Strategy

Each task has a specific rollback documented in its header. General principles:

1. **Config gated**: Every new behavior is gated behind a config flag (`enabled: false` by default). Rollback is a config change.
2. **Schema versioned**: DB migrations are additive only (CREATE TABLE IF NOT EXISTS). No destructive migrations.
3. **Binary rollback**: Hot-reload keeps the old binary for 30s before deleting. If new binary fails health check, old binary continues.
4. **Git revert**: Each phase builds on the previous. If Phase X must be rolled back, revert commits for Phases >= X.
