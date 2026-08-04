# BOTTLENECKS.md — Noctifab Codebase Bottleneck Review

Findings from a performance, scalability, and reliability review of the
`noctifab` codebase. Issues are ordered by impact. File/line references are
against the state of the repo at the time of the review and may drift as the
code evolves.

---

## Critical

### 1. Streaming LLM calls are cancelled at `idle_timeout` and silently re-run

`pkg/infrastructure/llm/openai.go:275-282` wraps the **entire** stream in
`context.WithTimeout(ctx, o.idleTimeout)` (default 15s; validation configs use
8s). A context deadline cancels the whole request — including body
consumption — so any completion streaming longer than `idle_timeout` total is
aborted mid-stream. The doc comment (lines 245-250) claims the deadline only
guards time-to-first-byte and that "long responses are not cut short"; this is
incorrect.

After the abort, the client falls back to a non-streaming POST
(`openai.go:178-188`), meaning **nearly every code-generation call executes
twice**, doubling latency and token spend.

A correct sliding (inter-chunk) idle timer already exists in
`pkg/infrastructure/llm/stream_reader.go:42-74` but is not used on the SDK
streaming path.

**Fix:** replace the total-call timeout with a per-chunk sliding idle timer
(reuse the `stream_reader.go` logic).

### 2. Batch-synchronous dispatch: one straggler stalls the whole pipeline

`RunOnce` launches all ready tasks as goroutines and then blocks on
`wg.Wait()` (`pkg/services/orchestrator.go:300-306`). A single slow task (up
to the 30-minute timeout in `pkg/services/orchestrator_execute.go:27`) blocks
scheduling of any newly-unblocked tasks for the entire batch. There is no
continuous worker pool; concurrency (`cfg.Agents.Generators.Number`, default
3) is enforced only per-batch in `pkg/services/scheduler.go:176-179`.

**Fix:** convert `RunOnce` to a continuous worker pool that dispatches
newly-ready tasks as slots free up.

### 3. `State.LastActions` grows unbounded and embeds full test logs

Appended in `pkg/services/orchestrator.go:377`,
`pkg/services/orchestrator_execute.go:390`, and
`pkg/services/unblocker_commands.go:65,123,148,180` — never truncated. The
`Result` field holds full test failure output
(`orchestrator_execute.go:394`). Since every save is a full DELETE +
re-INSERT of all state rows with one `ExecContext` per row
(`pkg/infrastructure/storage/sqlite_repository.go:128-242`,
`postgres_repository.go:112-227`), each OCC save/retry gets progressively
slower over a long run.

Compounding: in story mode the loop ticks every 2 seconds
(`cmd/noctifab/cli/serve.go:302-343`), and `syncWorkspaceFiles` re-stats every
git-tracked file and saves the full state including the complete file index at
the top of every `RunOnce` (`pkg/services/orchestrator_sync.go:56,66,119`).

**Fix:** cap/rotate `LastActions`, truncate embedded logs, and move saves to
dirty-flag/incremental writes instead of full delete/reinsert per tick.

### 4. Single-row OCC means global write serialization

Every concurrent task goroutine performs its own Load-modify-Save with OCC
retry (`pkg/services/orchestrator_helper.go:34-80`,
`pkg/services/orchestrator.go:310-366`). SQLite runs with
`SetMaxOpenConns(1)` plus a write mutex
(`sqlite_repository.go:22,35,74`); Postgres uses `SELECT ... FOR UPDATE` under
`RepeatableRead` plus the full rewrite. The single `state` row is the true
scalability ceiling — the hardcoded Postgres pool of 10/10
(`cmd/noctifab/cli/serve.go:56`, no `SetConnMaxLifetime`/`SetConnMaxIdleTime`,
`postgres_repository.go:31-32`) is mostly moot. Higher task concurrency raises
the OCC conflict rate, and each retried conflict re-runs the full
delete/insert rewrite.

---

## High

### 5. Untimed subprocesses held under global locks

- `GitClient.Run` uses `CombinedOutput()` with no watchdog
  (`pkg/services/rebase_queue.go:33-35`). A hung git command (stale lock file,
  credential prompt) holds the **global git mutex**, deadlocking every task's
  git operations until the 30-minute task context expires.
- `DockerSandbox.RunCommand` (`pkg/services/sandbox.go:266-277`) has no
  timeout of its own; `exec.CommandContext` kills only the `docker exec`
  client process, not the process inside the container.
- `checkPythonSyntax` spawns `python3 -m py_compile` with `exec.Command` (no
  context/timeout) on every `write_file`/`edit_file`
  (`pkg/services/production_tools.go:22-35`).
- The `Watchdog` (PGID kill, max duration + sliding idle timeout,
  `pkg/services/watchdog.go:49-112`) is well designed but only covers the
  `HostSandbox` path.

### 6. Retry-multiplier stacking without short-circuits

Client retries (default 5, with backoff) × the unbounded lower-model fallback
loop (`pkg/infrastructure/llm/client.go:199`) × router/failover candidate
iteration × the agents' own 5-turn loops. Deterministic errors (401/403/404)
are retried through the full ladder (`client.go:245` string-matches errors).
Additional costs:

- Each fallback iteration calls `GetAvailableModels` — an uncached network
  call (`client.go:335`).
- `latest`/`auto` model aliases re-fetch the model catalog on **every**
  completion (`client.go:192-197`).
- Gemini builds a fresh custom `Transport` per call with HTTP/2 disabled
  (`pkg/infrastructure/llm/gemini.go:104-109`), defeating connection pooling
  (new TLS handshake per request).
- `NewClient` hardcodes a 5-second HTTP timeout fallback (`client.go:132`);
  any path constructing a client without config override gets a 5s LLM
  timeout.
- Router puts a provider on a 5-minute cooldown after *any* error, including
  non-transient ones (`pkg/infrastructure/llm/router.go:381-407`).
- `ResolveCandidatesForRole` rebuilds all client objects (including
  `os.Getenv` scans) on every completion (`router.go:374`).

**Fix:** cache model-catalog/alias lookups; short-circuit non-retryable HTTP
errors; reuse transports.

### 7. Unbounded prompt growth — no real context-window management

- Agent loops re-embed all prior tool outputs into the next prompt with no
  cap outside the Reader phase (`pkg/services/orchestrator_generator.go:171-174`,
  `pkg/services/orchestrator_helper.go:356-359`; only Reader outputs get the
  2000-char cap at `orchestrator_helper.go:221-224`).
- `grep_search` reads every workspace file fully into memory with no size cap
  or binary detection (`pkg/services/production_tools.go:400-410`), and its
  output flows back into the prompt.
- Target-file contexts are injected verbatim without the ContextSlicer on the
  `orchestrator_execute.go:186-194` path.
- Token counting is `len(prompt)/4` estimation only
  (`pkg/infrastructure/llm/failover_client.go:17,147-149`); there is no
  pre-send token check — oversized prompts simply error at the provider.
  (`estTokensPerChar` is also inverted naming; it is chars-per-token.)

---

## Medium

### 8. Concurrency-safety gaps

- `taskState := *state` is a shallow copy — the `Tasks`/`Files` slices are
  shared across concurrent task goroutines
  (`pkg/services/orchestrator_execute.go:180`).
- `o.storyStartedAt` and `o.lastWorkspaceSync` are written without
  synchronization (`pkg/services/orchestrator.go:83,239-240`,
  `pkg/services/orchestrator_sync.go:15-21`).
- Non-worktree mode performs `git reset --hard` / `git clean -fd` /
  `git checkout` on the **shared** working directory per task
  (`orchestrator_execute.go:82-176`); concurrency > 1 with
  `use_worktrees: false` corrupts workspaces (only individual git commands are
  serialized by the mutex, not the checkout+write+commit sequence).
- The scheduler's active-agent count is eventually-consistent (persisted via
  async OCC writes), so over-dispatch beyond `Generators.Number` is possible
  in the window before `registerAgentStart` persists
  (`pkg/services/scheduler.go`, `orchestrator_helper.go:34-80`).
- `SetMetricsCollector` swaps the collector unguarded
  (`orchestrator.go:127-131`).
- Double-start hazard: `cmd/noctifab/cli/serve.go:194-195` starts the
  unblocker, and `Orchestrator.Start` (`orchestrator.go:144-146`) would start
  it again — two unblocker loops issuing duplicate reset/fail commands if
  `Start` is ever used in server mode.
- `RebaseQueue.Push` blocks until the 30-minute task context cancels if
  `Start` was never launched for the queue
  (`pkg/services/rebase_queue.go:64-74`).

### 9. Redundant test-suite executions and misleading "3x consensus"

- `ValidateTask` logs/comments claim 3x flaky consensus
  (`pkg/services/test_validator.go:15`,
  `orchestrator_execute.go:364`) but actually runs once —
  `runWithCount(ctx, state, 1)` (`test_validator.go:78`) — bypassing the
  flaky-detection machinery (`pkg/services/flaky_detector.go`).
- The full test suite runs many times per task: agents' own `run_tests`
  calls, the auto-triggered `run_tests` on noop
  (`orchestrator_generator.go:152-164`), linter/formatter retries
  (`test_validator.go:56-76`), then final validation. The `diagnosticCache`
  (`pkg/services/diagnostic_cache.go`) is recreated per agent
  (`orchestrator_generator.go:47`), so results never carry between the
  tester, generator, and final validation phases.
- Reader phase runs before every generator and tester invocation — up to 6+
  times per task attempt (`orchestrator_generator.go:15`,
  `orchestrator_helper.go:235`), each a potential LLM call plus a
  `git ls-files` of the whole repo (`orchestrator_helper.go:133`).

### 10. Scheduler dependency resolution is O(T²)–O(T³)

`resolveDependencyID` linearly scans all tasks with normalization/substring
matching per dependency, inside the iterative block-propagation loop
(`pkg/services/scheduler.go:71-166`). Fine at ~10 tasks; quadratic-cubic
blowup at hundreds.

### 11. Memory concerns

- Unbounded output buffers: `outputCapturer.buf`
  (`pkg/services/watchdog.go:28`), the docker output builder
  (`sandbox.go:268`), and the Python isolated runner (`sandbox.go:363`) hold
  full subprocess output in memory, then copy it via `string(output)`
  conversions (`sandbox.go:209-219`).
- Python test isolation runs each `test_*.py` serially in its own process and
  leaks one goroutine per file blocked on `<-ctx.Done()` until the task
  context ends (`sandbox.go:350-388`).
- `state.Files` — the full workspace index — is held in memory and rewritten
  to the DB on every sync.
- The `/api/v1/status` handler calls `LoadAll`, loading every historical
  story state with all relations per request
  (`pkg/services/command_channel.go:265`,
  `pkg/infrastructure/storage/sqlite_repository_load.go:35`).

### 12. Polling, sleeps, and retry loops

- Story execution polls every 2 seconds (hardcoded,
  `cmd/noctifab/cli/serve.go:302-304`, `start.go:399`), doing Load + workspace
  walk + full-state Save per tick even when nothing can change.
- REST pause/resume/cancel handlers retry OCC with fixed 50ms sleeps, no
  backoff (`command_channel.go:297,330,363`).
- The Unblocker ticks every 30 seconds with a full state Load
  (`pkg/services/unblocker.go:70-82`); with `LLMAssessment=true` (default,
  `pkg/infrastructure/config/defaults.go:176`) it can call the LLM every 30
  seconds on the same stall for up to 30 minutes.

### 13. Miscellaneous reliability issues

- OCC-exhaustion errors are swallowed:
  `_ = o.updateStateWithRetry(...)` at `orchestrator.go:218,250,280,369` — the
  story silently continues with inconsistent state.
- The daemon HTTP server has no `ReadTimeout`/`WriteTimeout`/`IdleTimeout`
  (`command_channel.go:424-427`); `/statusz` and `/api/v1/status` serialize
  the full state (including `LastActions` with embedded test logs) per
  request.
- Budget accounting is inconsistent: the Router increments usage by 1 per
  call (`router.go:396-397`) while FailoverClient estimates tokens
  (`failover_client.go:147`) — a daily "token" limit means different things
  depending on which client `factory.go` built. Estimates also ignore tool
  outputs echoed back in multi-turn prompts, so budgets systematically
  undercount.
- No retention/vacuum for old story states, actions, or `workspace_files`
  rows — a long-running daemon's database grows monotonically (only the
  manual `clean` command exists).
- Agent loops are hard-capped at `maxTurns = 5`
  (`orchestrator_generator.go:43`, `orchestrator_helper.go:258`); the
  configured per-role `iterations` values appear unused for these loops.

---

## Top 5 fixes by impact

1. Fix the streaming idle timeout in `openai.go` — use a per-chunk sliding
   timer (reuse `stream_reader.go`), not a total-call deadline. Currently
   doubles every LLM call longer than `idle_timeout`.
2. Decouple task execution from the batch `wg.Wait()` in `RunOnce`
   (continuous worker pool) so stragglers don't stall the pipeline.
3. Cap/rotate `State.LastActions`, stop persisting the full workspace file
   index on every tick, and move saves to dirty-flag/incremental writes.
4. Add a watchdog/timeout to `GitClient.Run` and `DockerSandbox.RunCommand`;
   cap tool outputs before they are embedded in prompts.
5. Cache `GetAvailableModels`/latest-alias resolution and short-circuit
   non-retryable HTTP errors (401/403/404) in the retry ladder.
