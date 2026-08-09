# Missing Agents: Audit, Rationale, and Implementation Proposal

## Decision Summary

The repository has configuration entries for six roles that do not have a
runtime implementation: `architect`, `qa`, `security`, `performance`, `docs`,
and `devops`. They must not be treated as enabled agents merely because their
YAML sections exist. Today, their settings are placeholders. The implemented
LLM roles are the product manager, planner, generator, tester, and unblocker;
the orchestrator is a deterministic controller, not an LLM role.

**Recommendation: retain only QA as a new specialist agent.** The existing
product manager, planner, generator, tester, and unblocker remain supported.
Remove the other five placeholder roles from the product and configuration
surface, then add QA only behind a bounded, measured rollout. First remove
configuration drift and add observability for skipped roles. If a new
capability is justified by measured failures, implement one bounded phase at a
time:

1. **QA is the only generally justified first candidate.** It can find a
   black-box behavior gap that task-local tests and the existing validator did
   not exercise.
2. **Security is not a retained agent in this plan.** Deterministic SAST,
   dependency scanning, validator policy, and explicit generator tasks remain
   the security controls. A future security experiment would require a new
   proposal and evidence; it is outside the current agent set.
3. **Architect, performance, docs, and devops are task concerns, not agents.**
   The planner creates explicit tasks, the existing generator performs bounded
   mutations, and deterministic validators plus the existing tester verify
   them. These roles must be removed rather than left as dormant configuration.

This is a design proposal, not an assertion that the six roles are implemented.
Phase 0 must pass its acceptance gate before Phase 1 starts. Phase 1 must pass
its deterministic implementation tests before QA can be enabled for an
experiment. The outcome-based rollout gate in §10 is a product decision after
that implementation, not a condition that can block a correct code change. Do
not implement future specialist roles as part of this plan.

## 1. Current-State Audit

The audit must distinguish declaration, prompt availability, construction, and
dispatch. A role is implemented only when all four exist and are covered by a
behavioral test.

| Role | Config declaration | Domain role | Prompt/runtime dispatch | Current conclusion |
| --- | --- | --- | --- | --- |
| `product_manager` | Yes | Configured role | `GenerateRoadmap` | Implemented |
| `planner` | Yes | `PLANNER` | `PlanStory` | Implemented |
| `generator` | `generators` config | `GENERATOR` | Generator loop | Implemented |
| `tester` | `testers` config | `TESTER` | Tester loop | Implemented |
| `unblocker` | Yes | `UNBLOCKER` | Background stall monitor | Implemented |
| `orchestrator` | Yes | None | Stateful Go controller | Implemented, not an LLM agent |
| `architect` | Yes | None | No runtime phase | Remove as agent configuration |
| `qa` | Yes | None | No runtime phase | Retain and implement as the only specialist agent |
| `security` | Yes | None | No runtime phase; `SASTScanner` is separate | Remove as agent configuration |
| `performance` | Yes | None | No runtime phase | Remove as agent configuration |
| `docs` | Yes | None | No runtime phase | Remove as agent configuration |
| `devops` | Yes | None | No runtime phase | Remove as agent configuration |
| `resolver` | No | `RESOLVER` constant only | None | Dead declaration; remove after reference audit |

The repository currently has two different configuration concepts:

- `agents.<role>` contains lifecycle-like settings such as `number` and
  `iterations`.
- `roles.<role>` contains model-routing settings.

They must not be conflated. A routing entry does not construct an agent, and a
worker count does not create concurrency unless the orchestrator reads it.
Before implementing a role, verify every setting has a production read, a
default, validation, and a test. Otherwise remove it or document it as reserved.

The existing `AgentRoleResolver` is not a reason to create an LLM merge agent.
Git/rebase conflict handling is deterministic and should remain so. An LLM
resolver could silently change behavior while presenting a conflict as solved.

## 2. Does the Proposal Have Merit?

Yes, as an audit and as a prompt to close the verification/validation gap. No,
as a proposal to run six additional agents in every story. More agents create
more prompts, cost, latency, conflicting edits, state transitions, and failure
surfaces. An agent is warranted only when it has:

- a failure class not already handled by deterministic tooling or an existing
  role;
- a public, testable output contract;
- a bounded budget and a terminal state for every outcome;
- a safe ownership boundary (read-only is preferable);
- telemetry that can prove it improves outcomes rather than merely producing
  longer reports.

### Problems solved by a new role, and problems not solved

| Capability | New problem it can solve | Why existing flow cannot reliably solve it | Why it may not deserve a permanent agent |
| --- | --- | --- | --- |
| QA | A feature passes task tests but violates a public DoD sequence, malformed-input rule, CLI invariant, or end-to-end behavior | Generator/tester receive task-local goals; `run_tests` only runs declared checks and cannot invent missing black-box scenarios | The product manager can improve DoD and the tester can encode supplied scenarios; start as an optional post-build validator |
| Security | A data-flow or trust-boundary mistake survives lexical SAST, such as authorization applied to one endpoint but not another | Pattern scanners detect known syntax, not project-specific intent and control flow | LLM findings can be false, duplicated, or unsafe; deterministic SAST and dependency scanning remain mandatory |
| Architect | Cross-task design drift caused by individually valid changes | A planner sees the initial DAG, while later tasks can expose constraints that were not known initially | Many checks are static and belong in lint/architecture tests; a read-only audit may be enough |
| Performance | A change creates a semantic regression in query count, allocation growth, or hot-path complexity | Functional tests generally do not measure resource budgets | Without a baseline and workload, an LLM produces opinions; benchmarks and profiles should lead |
| Docs | Public behavior changes but README/API/help documentation does not | Code agents optimize for tests and may not inspect all user-facing docs | A docs checklist task is cheaper and more deterministic than a standing agent |
| DevOps | CI/deployment topology is inconsistent with application commands or runtime dependencies | A normal generator task can edit manifests, but may lack infrastructure-specific validation | YAML/schema/config validators and explicit infra tasks cover much of this; deployment is optional per project |

The only generally new guarantee is QA's ability to ask: **“What public
behavior is still untested?”** Even QA cannot prove completeness. It can only
add reproducible scenarios within a stated budget.

## 2.1 Delegation to the Existing Generator

“No specialized agent” does not mean “ignore the concern.” It means the concern
is represented as an ordinary, explicit task in the story DAG. The generator
owns the edits because it already has the workspace tools, sandbox policy,
retry loop, and tester feedback path. The task description must identify the
scope, target files, public contract, and validation commands; it must not ask
the generator to perform an open-ended review.

| Concern | Planner task example | Generator may change | Verification after the change |
| --- | --- | --- | --- |
| Architecture | “Extract the persistence interface into the domain boundary; preserve public behavior” | Source and co-located tests needed for the refactor, only in declared target paths | Existing tester suite, dependency/package checks, cycle checks, and file-size checks |
| Performance | “Reduce request path allocations below the measured baseline for workload X” | Implementation and benchmark/test files needed to meet the stated threshold | Reproducible benchmark/profile command, threshold comparison, functional tests; no task without a baseline |
| Documentation | “Document the delivered `--format` option, errors, and examples in README and docs” | Documentation paths only; no production code unless the planner splits that into another task | CLI help/API extraction, link/example checks, and the normal test suite |
| DevOps | “Add CI workflow running the repository’s existing test and lint commands” | Workflow, Docker, compose, Makefile, or deployment manifest paths explicitly listed | YAML/schema validation, `docker compose config` or manifest dry-run where applicable, then project tests |

These tasks remain subject to normal generator/tester behavior:

1. The generator reads the current state and target files before editing.
2. It does not broaden a task into a general architecture, security, or
   performance audit.
3. It must not invent commands, deployment targets, workloads, public API
   behavior, or documentation for features not in the specification.
4. The tester verifies the task's public contract and the configured
   deterministic checks. A passing implementation test does not make an
   unmeasured performance claim true.
5. If the task reveals a cross-cutting design decision, missing workload,
   unknown deployment topology, or ambiguous public behavior, the generator
   stops and emits a clarification or `request_decision`; it does not silently
   choose an architecture.

The generator therefore performs the implementation work, not the independent
judgment traditionally associated with these specialist roles. This preserves
one mutation path and avoids conflicting agents editing the same artifact.

## 3. Non-Negotiable Common Contract

Every future role must be a state-machine phase, not an unbounded conversational
loop.

### Inputs

- immutable state snapshot and story/task identifier;
- a parsed `StoryContract`, produced from the roadmap story before task
  execution, containing the story ID, public interfaces, and a normalized,
  machine-readable Definition of Done (DoD). QA is skipped with
  `missing_story_contract` when the parser cannot produce this contract;
- current diff and an immutable artifact identity: the Git commit ID used to
  create the review worktrees. A dirty tree is never an artifact;
- configured role profile and remaining token/time budget;
- prior attempts, findings, and scenario fingerprints.

Never pass credentials, unrestricted environment variables, or an entire
workspace when a bounded diff/file list is sufficient. Secret scrubbing must
apply to prompts, tool results, findings, metrics, and logs.

### Outputs

Persist a structured result before the phase returns:

```json
{
  "role": "qa",
  "phase": "complete",
  "artifact": "source-commit:executable-manifest-sha256",
  "status": "PASS|FINDINGS|SKIPPED|INCONCLUSIVE|BUDGET_EXHAUSTED|ERROR|INTERRUPTED",
  "findings": [],
  "attempts": 1
}
```

Every finding must include a stable fingerprint, severity, evidence, affected
public contract, and remediation or explicit reason it is advisory. Findings
without evidence are discarded, not converted into work.

### QA scenario contract

The QA model returns the existing `domain.LLMResponse` envelope with exactly
one declarative `propose_scenarios` action per review attempt. This action is
parsed by the orchestrator and is not registered as an executable `Tool`. QA
cannot request any other action, execute shell, choose a phase status, or create
findings. This reuses the existing provider/router/parser contract. Go derives
all statuses and findings from execution:

```json
{
  "reasoning": "Derive cases only from the supplied public contract",
  "actions": [
    {
      "tool": "propose_scenarios",
      "args": {
        "scenarios": [
          {
            "name": "empty-input",
            "public_contract_id": "cli.invalid-input",
            "steps": [{
              "command": ["./dist/example", "--input", ""],
              "stdin": "",
              "expected_exit_code": 2,
              "stdout_contains": [],
              "stderr_prefix": "ERROR:"
            }]
          }
        ]
      }
    }
  ]
}
```

Each `command` is an argument vector, not a shell string. Each step inherits
the configured per-command timeout. Steps run serially in one scenario runtime
directory; scenarios use separate runtime directories and run serially in MVP.
The deterministic runner rejects unknown contract IDs, empty names or steps,
duplicate fingerprints, commands whose first argument is not an exact member
of the validation-command allowlist, and expectations not present in the
referenced public contract. The fingerprint is SHA-256 of RFC 8785 canonical
JSON for the scenario with `name` omitted. Duplicate fingerprints are retained
once, in first-occurrence order, and increment a metric; they are not errors.
The runner records capped stdout, stderr, exit code, timeout outcome, and the
artifact ID for every step. A scenario produces one finding when any step
violates its expected observable result; later steps are then not run.

### Exact domain model

Add these types under `pkg/domain/` and add slices for them to `State`. Keep
each source file below 500 physical lines:

```go
type PublicContract struct {
	ID                 string   `json:"id"`
	Interface          string   `json:"interface"`
	ApplicablePathPrefixes []string `json:"applicable_path_prefixes,omitempty"`
	AllowedExecutables []string `json:"allowed_executables"`
	ExitCodes          []int    `json:"exit_codes,omitempty"`
	StdoutContains     []string `json:"stdout_contains,omitempty"`
	StderrPrefixes     []string `json:"stderr_prefixes,omitempty"`
}

type StoryContract struct {
	StoryID        string           `json:"story_id"`
	SourcePath     string           `json:"source_path"`
	SourceSHA256   string           `json:"source_sha256"`
	PublicContracts []PublicContract `json:"public_contracts"`
}

type ReviewStatus string
const (
	ReviewWorking         ReviewStatus = "WORKING"
	ReviewPass            ReviewStatus = "PASS"
	ReviewFindings        ReviewStatus = "FINDINGS"
	ReviewSkipped         ReviewStatus = "SKIPPED"
	ReviewInconclusive    ReviewStatus = "INCONCLUSIVE"
	ReviewBudgetExhausted ReviewStatus = "BUDGET_EXHAUSTED"
	ReviewError           ReviewStatus = "ERROR"
	ReviewInterrupted     ReviewStatus = "INTERRUPTED"
)

type ReviewPhase struct {
	ID, StoryID, TaskID, Role, ArtifactID string
	Attempt int
	Status ReviewStatus
	TerminalReason string
	StartedAt, DeadlineAt, CompletedAt time.Time
	TokensUsed int64
	CostUSD string
}

type QAStep struct {
	Command []string `json:"command"`
	Stdin string `json:"stdin,omitempty"`
	ExpectedExitCode int `json:"expected_exit_code"`
	StdoutContains []string `json:"stdout_contains,omitempty"`
	StderrPrefix string `json:"stderr_prefix,omitempty"`
}

type QAScenario struct {
	ID, ReviewPhaseID, PublicContractID, Name, Fingerprint string
	Steps []QAStep
	Status ReviewStatus
	Evidence string
}

type QAFinding struct {
	ID, ReviewPhaseID, TaskID, ArtifactID, ScenarioFingerprint string
	PublicContractID, Severity, Expected, Actual, Evidence string
	Disposition string
}
```

Use normal multi-line field declarations in production; compact declarations
above only keep this document readable. IDs are UUIDs except `StoryID`, which
comes from the roadmap contract. `Severity` is `blocking` in MVP. Evidence is
sanitized and capped. `Disposition` is one of `OPEN`, `FIXED`, `STALE`, or
`FALSE_POSITIVE`.

### Story contract file format

The Product Manager must emit a fenced `noctifab-contract` JSON block in every
roadmap story. Markdown outside this block remains human documentation:

```noctifab-contract
{
  "story_id": "US-001",
  "public_contracts": [
    {
      "id": "cli.invalid-input",
      "interface": "CLI ./dist/example",
      "applicable_path_prefixes": ["cmd/", "pkg/"],
      "allowed_executables": ["./dist/example"],
      "exit_codes": [2],
      "stdout_contains": [],
      "stderr_prefixes": ["ERROR:"]
    }
  ]
}
```

`ParseStoryContract(path, markdown) (domain.StoryContract, error)` is a pure
function in `pkg/services/story_contract.go`. It requires exactly one block,
non-empty unique contract IDs matching `[a-z0-9][a-z0-9._-]*`, at least one
contract, relative executable paths without `..`, and expectations for every
contract. It sets `SourcePath` to the cleaned path and `SourceSHA256` to the
SHA-256 of the complete Markdown bytes. Unknown JSON fields, duplicate blocks,
duplicate IDs, absolute executable paths, and contracts with no observable
expectation are errors prefixed `story contract:`. QA treats any parser error as
`SKIPPED/missing_story_contract`; normal story execution continues. Phase 0
updates the PM prompt, story template, prompt tests, and DoD documentation so
newly generated stories satisfy this format.

`applicable_path_prefixes` contains cleaned repository-relative directory/file
prefixes, rejects absolute paths and `..`, and uses slash-normalized literal
prefix matching, not glob syntax. An empty list means the contract applies to
all changes. A contract applies when any recursively inherited task target file
has one of its prefixes. The MVP supports executable process/CLI contracts
only. HTTP APIs require a future declarative request-step schema and are
`SKIPPED/not_applicable`; QA must not improvise `curl` or network access.

### Lifecycle and failure behavior

1. Acquire a state snapshot and an artifact identity.
2. Check prerequisites, role disablement, budget, and applicable scope.
3. Register `WORKING` in `ActiveAgents` with a deadline.
4. Execute a bounded number of turns and validate every tool action.
5. Persist the result using OCC; never hold a database transaction over an LLM
   call or subprocess.
6. On every return path, change the QA `ActiveAgents` entry from the existing
   `AgentWorking` to `AgentCompleted`; put a sanitized error in `LastError` for
   non-pass outcomes. Do not add failure statuses to `AgentStatus`. Detailed
   terminal outcomes, including `INTERRUPTED`, belong to `ReviewPhase.Status`.
7. Re-load state before emitting tasks. If the artifact changed, discard stale
   findings and rerun or mark the phase stale.

The following must all terminate: empty LLM output, malformed JSON, forbidden
response fields, context cancellation, provider timeout, duplicate scenarios,
OCC conflict, subprocess timeout, missing artifact, and exhausted budget. MVP
has no advisory or fail-open path; apply the exact blocking behavior below.

### Exact outcome mapping

The orchestrator must not guess whether an exceptional condition blocks the
story. Apply this table in first-match order:

| Condition | Review status | Terminal reason | LLM called? | Task effect |
| --- | --- | --- | --- | --- |
| QA disabled | `SKIPPED` | `disabled` | No | Continue normal flow |
| No valid `StoryContract` or no public contract | `SKIPPED` | `missing_story_contract` | No | Continue normal flow |
| No applicable changed public path/command | `SKIPPED` | `not_applicable` | No | Continue normal flow |
| Missing build/validation command or executable | `INCONCLUSIVE` | `validation_surface_unavailable` | No | Fail current task because MVP is blocking |
| Workspace/sandbox setup or cleanup failure | `INCONCLUSIVE` | `isolation_failed` | No further calls | Fail current task |
| Artifact changes before persistence | `INCONCLUSIVE` | `artifact_changed` | No further calls | Discard findings; retry once as a new attempt, then fail task |
| Context cancellation or daemon shutdown | `INTERRUPTED` | `context_cancelled` | Maybe | Leave task `INTERRUPTED` for normal restart recovery |
| Role/global token or cost budget reached | `BUDGET_EXHAUSTED` | `budget_exhausted` | No further calls | Fail current task |
| Malformed/empty model response or unsupported expectation | `ERROR` | `invalid_model_output` | Yes | Retry up to `iterations`, then fail task |
| Duplicate scenario fingerprint | No separate terminal status | `duplicate_suppressed` metric | No extra call | Execute first occurrence only |
| Scenario command timeout/environment failure | `INCONCLUSIVE` | `scenario_environment_failed` | Yes | Fail current task; do not create finding |
| All accepted scenarios satisfy expectations | `PASS` | empty | Yes | Continue normal flow |
| One or more expectations fail reproducibly | `FINDINGS` | `public_contract_failed` | Yes | Start bounded fix round |
| OCC or atomic persistence failure after retries | `ERROR` | `persistence_failed` | Maybe | Do not invoke fix; fail current task |

“Fail current task” uses the existing task retry transition: increment
`Retries`, set `FailureLog`, and choose `PENDING` or `FAILED` according to
`MaxRetries`. Only cancellation uses `TaskInterrupted`. `SKIPPED` is the only
non-pass review outcome that permits normal completion in MVP.

### Persistence and fix-round ownership

Before QA implementation, add these domain records and normalized storage
tables: `StoryContract`, `ReviewPhase`, `QAScenario`, and `QAFinding`.
`ReviewPhase` is unique by `(story_id, task_id, role, artifact_id, attempt)`;
`QAScenario` is unique by `(review_phase_id, fingerprint)`; and `QAFinding` is
unique by `(artifact_id, fingerprint)`. Each record carries its parent IDs,
terminal status/reason, and timestamps. Persist a terminal phase result and its
scenarios/findings in the same state save transaction.

QA does not create scheduler tasks in the MVP. Reproducible findings trigger a
bounded QA fix round on the current task while it remains `IN_PROGRESS`. The
orchestrator passes a deterministic feedback block to
`RunGeneratorAgent(..., "fix")` containing the public-contract ID,
reproduction steps, expected and actual result, artifact ID, and fingerprint.
After the generator commits a fix, the existing tester runs its `refactor`
action against that commit, normal task validation runs, and QA reruns against
the new commit. The task becomes `SUCCESS` only when normal validation and QA
both pass. On reaching `qa.max_review_rounds`, set the current task to `FAILED`,
increment its existing `Retries`, and put the sanitized open findings in
`FailureLog`; the existing outer retry policy decides whether it becomes
`PENDING` or permanently failed. A failed atomic review save does not invoke
the generator and yields `ERROR/persistence_failed`.

## 4. Disablement, Scope, and Cost Controls

QA enablement is the experimental feature flag; do not add a second,
independent feature flag. Replace `AgentsConfig.QA AgentRoleConfig` with
`AgentsConfig.QA QAConfig`. `QAConfig` contains only the fields shown below;
do not add QA-only fields to the shared `AgentRoleConfig`. Keep `RolesConfig.QA`
only for model routing:

```yaml
agents:
  qa:
    enabled: false      # default false; explicit opt-in for the experiment
    iterations: 1       # maximum model calls per review phase, 1..3
    max_cost_usd: "0"   # decimal USD; "0" uses the global budget
    max_duration: "2m"
    max_scenarios: 8
    max_review_rounds: 2
    max_output_bytes: 65536
    blocking: true
    network: "none"
    build_command: ["make", "build"]
    validation_commands: ["./dist/example"]
    tester_path_prefixes: ["test/", "tests/", "spec/", "specs/"]
```

There is no `disabled` compatibility alias because QA is not yet shipped. YAML
containing it must fail as an unknown field. Validate decimal budget syntax,
positive duration and limits, `max_review_rounds` in `1..5`,
`max_output_bytes` in `1024..1048576`, `network` equal to `none`, non-empty
clean relative validation commands without arguments, and that enabled QA uses
`agents.architecture: code_first` and `vcs.use_worktrees: true`; other modes are
not part of the MVP. `blocking` defaults to `true`; Phase 1 rejects `false`
rather than implementing fail-open behavior. A disabled phase means no LLM
call, no worker entry, no mutation, and a persisted `SKIPPED` reason of
`disabled`. It must not silently look like a successful review.

The production config types are:

```go
type QAConfig struct {
	Enabled            bool               `yaml:"enabled"`
	Iterations         int                `yaml:"iterations"`
	MaxCostUSD         string             `yaml:"max_cost_usd"`
	MaxDuration        Duration           `yaml:"max_duration"`
	MaxScenarios       int                `yaml:"max_scenarios"`
	MaxReviewRounds    int                `yaml:"max_review_rounds"`
	MaxOutputBytes     int                `yaml:"max_output_bytes"`
	Blocking           bool               `yaml:"blocking"`
	Network            string             `yaml:"network"`
	BuildCommand       []string           `yaml:"build_command"`
	ValidationCommands []string           `yaml:"validation_commands"`
	TesterPathPrefixes []string           `yaml:"tester_path_prefixes"`
	Model              string             `yaml:"model,omitempty"`
	Temperature        float64            `yaml:"temperature,omitempty"`
	Profile            string             `yaml:"profile,omitempty"`
	Providers          []AgentProviderRef `yaml:"providers,omitempty"`
}
```

Defaults are exactly those in the YAML example. Parse money with decimal
string operations; never convert budget values through `float64`.

`tester_path_prefixes` must contain at least one cleaned, slash-normalized,
repository-relative prefix without `..`. The tester patch may add, modify, or
delete only paths under one of these prefixes. This is the complete MVP test
path policy; do not infer test files from language-specific suffixes.

`build_command` is one argument vector executed without a shell. It is required
when QA is enabled. `validation_commands` contains executable paths only; model
scenarios may add arguments but may not choose another executable. The build
command must be permitted by the existing sandbox command policy. The build
runs once in a dedicated writable build workspace created from the initial
generator commit. Its output is copied into a read-only runtime-artifact
directory declared to the QA sandbox. The build workspace is never the QA
source workspace. A non-zero build exit, timeout, missing declared executable,
or output exceeding the cap maps to
`INCONCLUSIVE/validation_surface_unavailable`.

The immutable QA `ArtifactID` is
`<source-commit>:<sha256-manifest>`. The manifest is sorted by validation
executable path and hashes each executable's bytes. Persist the manifest with
the review phase. Recompute it before every scenario and before terminal save;
any mismatch maps to `INCONCLUSIVE/artifact_changed`. The sandbox executes the
read-only runtime artifact, not a binary produced inside the source mount.

Use these gates before each phase:

- story type and changed-path applicability;
- explicit project opt-in for experimental roles;
- remaining global and role budget;
- maximum wall-clock duration;
- maximum scenarios and review rounds;
- maximum one active phase for a story unless the artifact is immutable;
- duplicate fingerprint suppression.

`number` should mean actual concurrency only if the scheduler enforces it.
Otherwise remove it from new role configuration. More workers do not improve a
read-only review of one immutable diff and can make state races worse.

## 5. Recommended Pipeline

Do not put every phase in every story. The safe default is:

```text
product_manager -> planner -> existing generator/tester loop -> deterministic validation
                                                        |
                               optional QA (black-box) -> final validation
                               docs/infra/performance/security as explicit tasks
```

The concurrent QA MVP applies only to the `code_first` architecture. When QA is
enabled, configuration validation rejects `single_pass` and `breadth_first`
runs rather than silently changing their behavior. A later proposal must name
the snapshot point and retry behavior for each architecture before widening the
scope.

If QA requests a fix round, the current task remains `IN_PROGRESS` and the
generator/tester loop runs again. The artifact identity changes. Prior QA
results are marked `STALE` and must not be applied to the new artifact. Re-run
QA only after the generator's fix, within `max_review_rounds` per task attempt.

The QA MVP is concurrent. After the generator finishes the initial
implementation, the orchestrator commits the source and creates three isolated
workspaces from that exact commit:

- the build copy, where configured executable artifacts are built;
- the tester copy, where the tester writes and runs tests;
- the QA copy, which is read-only and never receives tester or generator
  writes.

The orchestrator starts tester and QA at the same time. QA executes scenarios
against the frozen initial source while the tester works from its own copy:

```text
generator commits initial implementation
       |                    |                    |
       v                    v                    v
build artifacts      tester writes tests      QA workspace prepared
       |                    |                    |
       +--------------------+----> QA runs after build
                            |
                            v
                 orchestrator waits for all
                               |
                               v
                     collect tester and QA results
```

### Exact `code_first` integration sequence

Extract the existing initial-attempt body of `executeTask` into a small
`executeCodeFirstTask` coordinator; keep architecture dispatch in
`executeTask`. The coordinator follows this sequence exactly:

1. Run generator `implement` in the existing task worker worktree and commit
   all staged changes. If there is no commit or the worktree remains dirty,
   use the existing task-failure transition; QA does not start.
2. Record that commit as `sourceCommit`. Parse the story file at
   `State.Metadata.InputPath` into `StoryContract` and persist it. The
   `SourceSHA256` must still match immediately before review setup.
3. If QA is disabled or skipped, run the unchanged existing tester `write` ->
   generator `refactor` -> validator sequence in the task worker worktree.
4. If QA applies, create build, tester, and QA worktrees from `sourceCommit`.
   Run `build_command` in build while tester `write` runs in tester. After a
   successful build, copy and hash only configured validation executables, then
   start QA scenario derivation/execution. Tester and QA contexts are siblings
   of one cancellable phase context; a fatal isolation/build error cancels both.
5. Tester success means its LLM loop terminated without orchestration error and
   produced a patch containing only allowed test paths. Tests are allowed to
   fail against the initial implementation; that is expected before generator
   refactoring. Apply and commit its patch to the task worker branch. A tester
   orchestration error, invalid path, or patch conflict fails the task.
6. Wait for both operations and persist QA atomically. Clean temporary
   worktrees only after persistence succeeds or terminal persistence retries
   are exhausted.
7. For `PASS` or `SKIPPED`, run generator `refactor` in the task worker
   worktree with the committed test context, commit, and run normal validation.
8. For `FINDINGS`, apply tester's patch first, run generator `fix` with both
   test context and QA evidence, commit, run tester `refactor`, commit any test
   changes, run normal validation, then build and rerun QA from the new commit.
   Repeat step 8 only up to `max_review_rounds`.
9. For `INCONCLUSIVE`, `BUDGET_EXHAUSTED`, or `ERROR`, do not run generator
   refactoring/fix. Apply the exact task transition from the outcome table.
10. Mark the task `SUCCESS` and enqueue its worker branch into the existing
    `RebaseQueue` only after normal validation passes and the latest applicable
    QA phase is `PASS` or `SKIPPED`. No temporary review branch enters the
    rebase queue.

Retry attempts (`task.Retries > 0`) preserve the existing tester `refactor` ->
generator `fix` sequence, then run QA serially after normal validation but
before marking success. A QA finding returns the task to generator `fix` within
the bounded review round. This avoids creating another concurrent tester patch
on retries. Existing behavior is unchanged when QA is disabled.

The tester normally changes only test files. If the generator later changes
production files to fix a QA finding, the QA result still proves that the
previous implementation failed, but it says nothing about the fixed code. The
orchestrator must therefore run QA again on the fixed code before final
approval. This is a normal repeat of the same QA phase, not a new kind of
agent. Never let QA read a live mutable directory: it could observe half of a
tester or generator edit and report a false failure. If isolated copies cannot
be created, stop with `INCONCLUSIVE`; do not silently use a shared workspace.

**Meaning of isolated copies:** introduce an injected `ReviewWorkspaceFactory`
with `Create(ctx, sourceCommit) (build Workspace, tester Workspace, qa
Workspace, error)` and `Cleanup(ctx, ...Workspace) error`. It creates three Git
worktrees from the same committed artifact, on distinct branches and paths.
Build and tester workspaces are writable. The QA workspace is executed only
through an injected `QASandboxRunner` which mounts source and `.git` metadata
read-only, mounts the runtime-artifact directory read-only, supplies distinct
writable `TMPDIR`, `HOME`, and cache directories, denies network access, caps
processes/output/time, and blocks writes elsewhere. QA never receives a host
shell or direct filesystem write tool. File permissions alone do not satisfy
this requirement.

Use these interfaces so tests can inject failures without Git, Docker, or host
permission assumptions:

```go
type ReviewWorkspace struct {
	Path   string
	Branch string
}

type ReviewWorkspaceFactory interface {
	Create(ctx context.Context, repositoryPath, sourceCommit string) (
		build ReviewWorkspace, tester ReviewWorkspace, qa ReviewWorkspace, err error,
	)
	Cleanup(ctx context.Context, workspaces ...ReviewWorkspace) error
}

type QACommand struct {
	Argv        []string
	Stdin       string
	Timeout     time.Duration
	OutputLimit int
}

type QACommandResult struct {
	Stdout, Stderr string
	ExitCode       int
	TimedOut       bool
	Truncated      bool
}

type QASandboxRunner interface {
	Verify(ctx context.Context, sourcePath, artifactPath, runtimePath string) error
	Run(ctx context.Context, command QACommand) (QACommandResult, error)
}
```

Provide production constructors in `pkg/services` with explicit Git command
runner, process/container runner, clock, and filesystem dependencies. Do not
instantiate them inside orchestration methods.

The tester workspace is not merged. After tester completion, the orchestrator
creates one patch with `git diff --binary <sourceCommit>` in the tester
   worktree, verifies that every changed path is under a configured
   `tester_path_prefixes` entry, and applies that patch to the task worker
worktree. It then commits the tests there using the existing test commit
message. A non-test change or patch conflict fails the task. This preserves the
current worker branch as the only merge source. QA findings remain attached to
the immutable artifact. All workspaces contain the same tracked source at
creation; generated runtime data is excluded from source comparison.

**How the orchestrator verifies isolation before starting either agent:**

1. Create the build, tester, and QA directories using the existing worktree
   mechanism. Do not implement a second directory-copy mechanism if the
   repository already provides one.
2. Resolve both paths to absolute, cleaned paths.
3. Verify all paths are different and that none is inside another.
4. Verify the QA path is not the original workspace path.
5. Verify the QA policy profile contains no mutation tools, and verify the
   sandbox runner has a read-only source mount plus separate writable runtime
   directories. Do not infer this from an allowed-command list.
6. Compare `git ls-tree -r --full-tree <sourceCommit>` and tracked file hashes
   in all worktrees. If setup changes any copy, discard all and start again.
7. Start QA only after all checks succeed. There is no valid “best effort” mode.

**Return `INCONCLUSIVE` when any of these happens:**

- the worktree/sandbox creation command fails;
- either path cannot be resolved or is empty;
- any operations would receive the same or nested paths;
- the sandbox cannot prove its source and Git mounts are read-only;
- a source file is missing, unreadable, or differs between the two copies;
- the copy operation detects a source change while copying;
- the QA profile or sandbox runner permits source or Git metadata writes;
- temporary directory creation, permissions, disk space, or cleanup cannot be
  verified;
- the project requires a shared database, port, cache, or external service and
  no isolated test instance is configured.

**Behavior after `INCONCLUSIVE`:**

- do not call the QA LLM again for the same attempt;
- do not execute any QA scenario in the shared workspace;
- do not convert an inconclusive result into a passing result;
- persist `INCONCLUSIVE` with a sanitized reason and the failed setup step;
- mark the QA phase terminal so it cannot remain `WORKING` after a restart;
- clean up every directory and process that was created;
- leave production and test files untouched;
- allow the orchestrator to continue only according to explicit policy: either
  create a human clarification or block final approval. It must not silently
  claim that QA passed.

**Example:** if the tester worktree cannot be created, QA must not run in the
original workspace just because it is available. The correct result is
`INCONCLUSIVE: unable to create isolated tester workspace`. If the QA command
starts but attempts to write a generated cache into the source directory, the
validator must stop it and return `INCONCLUSIVE`; it must not merely log a
warning and continue.

## 6. Candidate Specifications

### 6.1 QA: first candidate

**Purpose:** derive and execute deterministic black-box scenarios from the
story's DoD. QA does not write code or tests.

**Starts:** in `code_first` mode only, after the initial generator changes have
been committed as the immutable task artifact and before the tester begins work
in its separate workspace. QA scenario derivation and scenario execution run
concurrently with tester work. Skip when no executable public contract applies
to the task. If one applies but its configured command or built executable is
unavailable, use `INCONCLUSIVE/validation_surface_unavailable`.

**Actions:** derive one bounded scenario array from `StoryContract`; Go then
runs accepted scenarios serially in the QA-only sandbox. Each scenario contains
one or more command steps and observable expectations. Go supplies timeout and
fingerprint values.

**Example:** If the DoD says “invalid input prints `ERROR:` to stderr and exits
with code 2,” QA may execute the binary with empty, malformed, maximum-size,
and repeated input. It must not inspect a private function or demand a specific
database schema.

**Pass:** all bounded scenarios pass. **Findings:** reproducible failures start
one bounded generator fix round on the current task. **Exhausted:** persist
`BUDGET_EXHAUSTED` and fail the current task through its normal retry policy.

**Dark corners:** do not repeat a scenario after a code change unless its
artifact fingerprint changes; do not call external services unless a hermetic
mock is configured; isolate temp files and ports; cap output; distinguish a
product failure from an unavailable test environment; never treat “could not
run” as pass.

### 6.1.1 QA Scope: What It Detects

QA detects **observable product-behavior failures**, not general code quality.
Its oracle is the specification, the story's Definition of Done, and the
declared public contracts. It should generate a bounded scenario matrix and
execute those scenarios against the immutable artifact.

QA should detect:

- explicit specification requirements that are missing or incomplete;
- Definition-of-Done requirements that task-local tests did not cover;
- edge cases such as empty input, malformed input, boundaries, duplicates,
  repeated operations, and invalid state transitions;
- incorrect executable/CLI output, error prefixes, exit codes, or serialized
  response schemas;
- logical behavior bugs visible through public interfaces;
- incorrect sequencing, such as calling an operation twice or using commands in
  an unexpected order;
- regressions in existing public behavior;
- integration failures across components that unit tests do not expose.

Examples:

- The specification requires invalid input to return `ERROR:` and exit `2`,
  but the binary exits `1`.
- A parser handles normal input but crashes on an empty file.
- `create -> update -> delete -> update` produces an invalid result instead of
  the documented error.
- A command succeeds once but fails when executed twice.
- A CLI emits valid JSON but omits a required public field.
- A feature works in isolation but fails when combined with an existing feature.

QA should not primarily detect:

- formatting, naming, or internal code-quality issues;
- architectural drift;
- generic security vulnerabilities already covered by scanners;
- unmeasured performance concerns;
- private-function behavior or other implementation details;
- bugs that cannot be demonstrated through a public contract.

Those concerns belong to these mechanisms:

| Concern | Primary mechanism |
| --- | --- |
| Unit-level defects and implementation correctness | Tester |
| Public behavior gaps and edge cases | QA |
| Security patterns and dependency vulnerabilities | Deterministic scanners |
| Semantic security concerns | Explicit security tasks; no security agent is retained by this plan |
| Performance regressions | Benchmarks and profiling |
| Architecture and maintainability | Architecture checks and generator tasks |
| Documentation drift | Documentation tasks and link/example validation |
| CI/deployment correctness | Infrastructure validators |

The distinction is important: the tester asks **“does the implementation pass
the tests we wrote?”** QA asks **“what public behavior does the specification
require that we have not adequately proven?”** QA is not a second generic code
reviewer and must not produce checklist-only findings.

Every QA finding must include all of the following:

- the public contract that was violated;
- reproduction input and setup;
- actual result;
- expected result;
- artifact/version tested;
- stable scenario fingerprint;
- the deterministic generator fix feedback.

If the specification or DoD is ambiguous, QA returns no invented expectation;
Go rejects one that is not present in `StoryContract`. If the environment is
unavailable, Go records `INCONCLUSIVE`, never `PASS`. If a finding cannot be
reproduced against the recorded artifact, it must not start a fix round.

### 6.2 Security Tasks: no security agent

Security is handled by deterministic SAST, dependency scanning, validator policy,
and explicit generator tasks. A security task may ask the generator to inspect
data flow and trust boundaries after deterministic checks. It is not a retained
agent, a penetration tester, or a certification mechanism.

Security tasks apply only when the story changes an input boundary,
identity/access control, serialization, cryptography, secrets, or dependency
exposure. They must not expose secret values, must cite sanitized file and line
ranges plus source-to-sink reasoning, and must never auto-merge a security fix
without tests and deterministic verification.

An LLM claim such as “this looks risky” is not actionable. For example, a
scanner finding string concatenation near a query is useful only when the task
can demonstrate that untrusted input reaches that query without validation.

### 6.3 Architecture Tasks: no architect agent

Prefer architecture tests, dependency checks, file-size checks, and planner
checklists. The generator performs architecture refactors as explicit tasks
   under §2.1. Do not implement an architect agent in this plan. Architecture
decisions belong in planner tasks and ADRs containing context, alternatives,
choice, consequences, and expiry/revisit conditions. The generator cannot
silently choose an architecture or block indefinitely.

### 6.4 Performance Tasks: no performance agent

Prefer benchmarks, profilers, query counters, memory limits, and regression
 thresholds. The generator performs measured optimizations as explicit tasks
under §2.1. Do not implement a performance agent. An optimization task is
allowed only when a baseline workload and measurement exist. A complexity
opinion without a hot path, input bound, or measurement must not create work.

### 6.5 Documentation Tasks: no docs agent

Make documentation updates explicit planner tasks derived from changed public
interfaces. The generator performs those docs-only edits under the docs profile.
Restrict writes to declared documentation paths, preserve existing navigation,
and validate links/examples/help output. Never document an aspirational or
merely proposed behavior. Do not implement a documentation agent in this plan.

### 6.6 Infrastructure Tasks: no devops agent

Route infrastructure changes to ordinary generator tasks with dedicated
validators: workflow/schema validation, compose config validation, manifest
dry-run, and secret-reference checks. The generator performs the manifest edits
but may not generate credentials or kubeconfigs. Generate Kubernetes resources
only when the specification requests Kubernetes. Do not implement a DevOps
agent. “Valid YAML” alone is not a valid deployment check.

## 7. State and Persistence Requirements

The current state model has generic tasks, actions, agents, and validation
criteria but no first-class review result, artifact identity, finding, scenario,
or phase state. Do not hide these in `LastActions` or free-form log messages.
Before adding a role, introduce versioned domain models and storage migrations
for at least:

- phase status, attempt, start/deadline, and terminal reason;
- artifact commit/tree hash and validation environment identity;
- finding/scenario fingerprint, severity, evidence, and disposition;
- parent story/task IDs and fix-round count;
- budget consumption and tool invocation summary.

Persistence must be atomic with the state transition. Restart behavior must be
defined: a stale `WORKING` phase is recovered as `INTERRUPTED`, never resumed
from presumed conversation memory. A phase may resume only from persisted input
and a verified unchanged artifact.

Add one next-numbered embedded migration for SQLite and PostgreSQL. It creates
`story_contracts`, `review_phases`, `qa_scenarios`, and `qa_findings` with
foreign keys to `state`; scenario/finding payload slices are JSON/JSONB using
the repository's existing encoding conventions. Add the unique constraints
from §3 and indexes on `(state_id, task_id)`, `(story_id, status)`, and
`artifact_id`. Add `StoryContracts`, `ReviewPhases`, `QAScenarios`, and
`QAFindings` slices to `domain.State`; repository `Save` replaces these child
rows in the same transaction as the existing state children, and `Load`
reconstructs them deterministically ordered by creation time then ID.

On daemon startup, after loading a state, every `ReviewWorking` row whose
deadline is before `now` or whose matching active agent is absent becomes
`ReviewInterrupted` with reason `restart_recovery`; its QA `ActiveAgents` entry
becomes `AgentCompleted` with `LastError="restart recovery"`; its task becomes
`TaskInterrupted`; and the update is saved atomically. Do not resume an LLM
turn. A subsequent normal orchestrator cycle may move that task through the
existing interrupted-task recovery path only after recreating and verifying a
new immutable artifact.

## 8. Prompt and Tool Contract

Prompts belong at `.noctifab/prompts/<role>/<action>.tmpl` and use the existing
renderer. Add `qa`/`acceptance` to the prompt catalog. Its code-owned contract
is the existing envelope restricted to exactly one `propose_scenarios` action
whose args use the scenario schema in §3. Existing agent rendering remains
byte-identical. The QA template must say:

- the role is stateless and must inspect the supplied state;
- that `propose_scenarios` is its only declarative action and cannot execute
  commands itself;
- what constitutes evidence and what is forbidden;
- to return one non-empty bounded `scenarios` array in that action;
- that Go code, not the model, determines statuses and findings.

Reject extra response fields, any other action/tool name, paths outside the
profile, shell strings, secret output, and invented expectations. Runner output
must be size-capped and tagged with exit status and timeout information. Prompt
injection in source, documentation, test output, or issue text is untrusted
data, not an instruction.

## 9. Testing and Observability

Tests must be black-box and adjacent to each implementation. At minimum, each
new role needs tests for:

- disabled, not-applicable, and missing-prerequisite paths;
- successful pass and evidence-backed finding;
- malformed model output, extra fields, and forbidden tool/action envelopes;
- timeout, cancellation, budget exhaustion, and output truncation;
- duplicate finding/scenario suppression;
- artifact change invalidation;
- restart recovery of stale work;
- OCC conflict and atomic persistence failure;
- secret scrubbing in prompts, state, findings, and logs;
- deterministic ordering and bounded counts.

Add metrics before rollout: phase calls, tokens/cost, latency, skipped reasons,
duplicate suppression, fix rounds, regressions found, and finding dispositions.
Extend `MetricsSummary` with deterministic QA counters and millisecond totals;
do not place fingerprints or contract text in metric labels. Log role, story,
artifact, phase, and outcome, but never prompt contents or credentials.

Required verification remains the repository suite:

```bash
go test -v ./pkg/... ./tests
go fmt ./...
docker run -t --rm -v "$(pwd):/app" -w /app golangci/golangci-lint:v2.12.2 golangci-lint run
```

## 10. Phased Implementation Plan and Acceptance Gates

### Phase 0: reconcile the contract

- Remove `architect`, `security`, `performance`, `docs`, and `devops` from the
  product surface. This is a deletion, not a disable-by-default feature.
- Remove their fields from `AgentsConfig`, `RolesConfig`, defaults, config
  decoding/validation, role-routing tables, prompt registration, and any
  orchestrator configuration fields or CLI plumbing that exists only for them.
- Remove their entries from every checked-in configuration, especially every
  `validation/projects/*/.noctifab/config.yaml`. The validation projects must
  contain only supported agent settings; they must not silently test dead roles.
- Bump the configuration schema version from `1.0` to `2.0` and make version
  validation live. `1.0` is rejected with `unsupported config_version "1.0":
  migrate to "2.0"`; any version other than `2.0` is rejected with
  `unsupported config_version %q: supported version is "2.0"`. Before typed
  decode, inspect a `yaml.Node` for the five removed keys directly under
  `agents` or `roles`; reject the first key in document order with
  `unsupported agent role %q: delete the %s.%s section`. Then run the existing
  strict `KnownFields(true)` decode. Do not add a compatibility alias.
- Remove stale descriptions and examples from `README.md`, `docs/`, and
  configuration fixtures. A config using a removed role should fail validation
  with a clear unsupported-role error rather than being silently ignored.
- Keep explicit generator tasks and deterministic validators for architecture,
  security, performance, documentation, and infrastructure work as described in
  §2.1. Retain only `qa` as the new specialist role.
- Make unused `number`/`iterations` fields either live or remove them; do not
  claim support based on wiring alone.
- Add a role capability registry used by startup and `noctifab validate`. After
  the existing success line, validation prints deterministic sorted lines in
  this exact format: `role <name>: <capability>`. The Phase 0 set is
  `orchestrator: deterministic-controller`, `product_manager: implemented`,
  `planner: implemented`, `generator: implemented`, `tester: implemented`,
  `unblocker: implemented`, and `qa: experimental-disabled`. In Phase 1, QA is
  `experimental-enabled` only when `agents.qa.enabled` is true.
- Remove `RESOLVER` only after a repository-wide production and fixture audit.
- Add config-version/parser tests proving removed roles cannot load or make LLM
  calls and that all validation-project configurations load after cleanup.
  Database migrations belong to Phase 1, not this configuration cleanup.

Use this inventory when implementing the deletion. Search production code,
tests, defaults, fixtures, and documentation for each removed role name and for
the corresponding fields, then inspect every match before deleting it:

```text
AgentsConfig fields and YAML defaults
RolesConfig fields and provider-routing defaults
OrchestratorConfig fields and CLI construction/plumbing
role capability/profile/permission registries
prompt embedding, template lists, and prompt tests
config validation and schema-version tests
README, docs, examples, and checked-in fixtures
validation/projects/*/.noctifab/config.yaml
```

**Gate:** no production code, supported schema, default configuration, or
validation-project configuration refers to the five removed roles; unsupported
configuration fails clearly; existing supported workflows behave unchanged.

### Phase 1: QA experiment

Implement QA behind the explicit `agents.qa.enabled` opt-in. Start with one
read-only scenario-generation action and the schema in §3. Convert reproducible
findings into bounded generator fix rounds on the current task; do not let QA
mutate files. The MVP is limited to `code_first`; it must not add implicit
behavior for other execution architectures.

**Implementation gate:** deterministic fixtures cover a passing public path, a
seeded invalid/boundary defect, and a seeded multi-step workflow defect. They
assert the exact persisted outcome, fix-round deduplication, isolation, and zero
LLM calls when QA is disabled. A no-op QA run must be safe.

**Rollout decision:** after implementation, run QA on at least three
representative real stories. Evaluate reproducible defects found,
false-positive dispositions, cost, and latency against configured budgets. This
is an operational decision that may keep the feature disabled; it is not an
automated acceptance test and does not require a defect to exist in every
sample.

### Phase 2: measure and maintain

Measure QA's defect yield, false-positive rate, cost, latency, and artifact
staleness. Improve deterministic validators and generator task templates for
the removed concerns. Do not reintroduce any removed role without a new proposal
showing a failure class that cannot be addressed by those mechanisms.

## 11. Lower-LLM Implementation Checklist

For QA, and for any future role approved by a new proposal, implement in this
order:

1. Define the input/output structs and terminal statuses.
2. Define applicability and disablement before writing a prompt.
3. Add the state migration and repository round-trip tests.
4. Add a read-only tool profile; start with zero write permissions.
5. Implement one bounded action and reject every other action.
6. Persist start and terminal result on every return path.
7. Add artifact and duplicate-fingerprint checks.
8. Add failure-injection tests for timeout, cancellation, malformed output, and
   OCC conflict.
9. Add metrics and sanitized logs.
10. Run the full verification commands before enabling QA in any project.

Do not implement removed roles. Implement Phase 0 before QA. A role that cannot
explain what new failure it catches, how the orchestrator terminates it, and how
its result is validated is not ready to be added.

## 12. Dumb-LLM Execution Runbook

This section is intentionally repetitive. An implementer must follow the steps
in order and must not infer additional scope. “Remove a role” means remove its
agent declaration and agent routing, not every use of the English word in the
repository.

### 12.1 Phase 0 Exact Order

1. Create a work branch. Do not work directly on `main`.
2. Before editing, run:

   ```bash
   git status --short --branch
   rg -n "architect|security|performance|docs|devops|qa" pkg cmd docs README.md validation/projects
   ```

3. Make a list of every match. Do not delete a match without classifying it as
   an agent declaration, agent routing, prompt customization, documentation,
   test fixture, or an unrelated domain concept.
4. Remove only `architect`, `security`, `performance`, `docs`, and `devops` as
   agent roles. Keep `qa` configuration and add no other new role.
5. In `pkg/infrastructure/config/types.go`, remove the five fields from both
   `AgentsConfig` and `RolesConfig`. Keep the `QA` fields. Do not remove the
   unrelated `Architecture` execution-mode field.
6. Remove the five role entries from config defaults, role validation, provider
   routing, profile registries, prompt catalogs, and CLI configuration wiring.
   In particular, inspect `pkg/infrastructure/config/config.go`,
   `pkg/infrastructure/llm/router.go`, `pkg/services/orchestrator.go`,
   `cmd/noctifab/cli/serve.go`, and `cmd/noctifab/cli/start.go`.
7. Remove tests and fixtures that assert the five roles are valid. Replace them
   with tests asserting that each removed YAML key returns an unsupported-role
   validation error. Keep tests for the deterministic SAST scanner, validator
   security policy, performance metrics, documentation tooling, and execution
   architecture. Those are not agents and must not be deleted.
8. Remove five-role prompt tests and prompt examples. Keep the prompt renderer,
   generic prompt validation, and QA prompt support. A prompt test mentioning
   `security` may be retained if it tests secret scrubbing or validator policy,
   not a security agent.
9. In every `validation/projects/*/.noctifab/config.yaml`, delete the five role
   mapping blocks. Do not edit project specifications, roadmaps, secrets, or
   unrelated settings. Keep the existing `qa` block.
10. Remove stale agent claims from README, configuration documentation, examples,
    and architecture documentation. Do not remove documentation infrastructure
    such as `docs/`, Sphinx configuration, or Read the Docs files.
11. Set `config_version` to `2.0`, implement the exact version-validation
    errors in §10, and update every supported fixture to `2.0`.
12. Run the config parser tests before touching QA. If any removed role still
    loads successfully, stop and fix Phase 0; do not begin Phase 1.

### 12.2 Phase 0 Classification Rules

The following are **not** removed:

- `SASTScanner`, `ErrSecurityVulnerability`, security sandbox checks, and secret
  scrubbing;
- performance metrics, benchmark commands, and timeout/resource limits;
- `docs/` and documentation validation;
- the `Architecture` execution-mode setting such as `code_first`;
- deterministic orchestration, Git conflict handling, and the `RESOLVER` audit
  until its references have been checked;
- existing product manager, planner, generator, tester, and unblocker roles.

The following **are** removed:

- configuration keys named `architect`, `security`, `performance`, `docs`, or
  `devops` under agent or model-role configuration;
- provider-routing branches that select those five as agent roles;
- prompt catalog entries and agent profiles whose only purpose is one of those
  five roles;
- orchestrator worker-count/iteration fields that exist only for those roles;
- documentation claiming those five are runnable agents.

### 12.3 QA MVP Exact Order

Implement concurrent QA in the MVP. Implement these steps only:

1. Add the QA configuration fields, `StoryContract`, review domain types, and
   normalized storage migrations with repository round-trip tests.
2. Limit the initial scheduler hook to `code_first`. Reject enabled QA for
   every other execution architecture until a later design extends it.
3. Add an injected `ReviewWorkspaceFactory` and a read-only sandbox runner. The
   sandbox must enforce source/Git read-only mounts and separate writable
   runtime directories; a policy profile is not sufficient enforcement.
4. Add one prompt action, `qa/acceptance`, using the existing prompt renderer.
   It returns exactly one declarative `propose_scenarios` action using the
   schema in §3; it cannot execute tools. The prompt forbids source edits,
   private-function assertions, credentials, and invented requirements.
5. After the initial generator commit, parse and persist the `StoryContract`.
   If it is absent, malformed, or has no public validation surface, persist
   `SKIPPED` and make no QA LLM call.
6. Create build, tester, and QA worktrees from that exact commit through the
   factory. Verify all tracked-source manifests match. If isolation cannot be
   proved, persist `INCONCLUSIVE` and do not run QA.
7. Start build and tester as sibling, cancellable operations. After build
   artifacts are hashed and mounted read-only, start QA under the same phase
   context. Tester writes only in its workspace; QA commands execute only
   through its read-only sandbox.
8. Ask QA for a bounded scenario list derived only from the `StoryContract`.
   Reject empty, malformed, or unsupported scenarios and suppress duplicate
   fingerprints before running anything.
9. Execute each accepted scenario with a timeout and capped stdout/stderr.
   Record exit status, artifact ID, and fingerprint. Environment failures are
   `INCONCLUSIVE`, never `PASS`.
10. Wait for all operations. Persist the complete QA result atomically before
    cleanup. Apply the test-only tester patch to the task worker branch; never
    merge a temporary review branch.
11. Convert reproducible failed scenarios into bounded generator `fix` feedback
    on the current task. QA never writes production code or test files and
    creates no scheduler task.
12. If the generator changes production files after QA starts, retain the
    finding as evidence for the old artifact, then create a new snapshot and
    run QA again before final approval. Do not reuse a passing old result.
13. Delete all three worktrees and all sandbox runtime directories on success,
    failure, cancellation, timeout, and interruption. Test cleanup explicitly.

### 12.4 QA MVP Terminal Conditions

Do not guess or ask the implementer to classify failures. Apply the exact
outcome table in §3. Missing contracts are `SKIPPED`; duplicate scenarios are
suppressed; budget exhaustion is `BUDGET_EXHAUSTED`; isolation, build, runtime,
and artifact failures are `INCONCLUSIVE`; invalid model output and exhausted
persistence retries are `ERROR`; cancellation is `INTERRUPTED`; observable
contract violations are `FINDINGS`; and complete success is `PASS`.

### 12.5 Required QA Tests Before Enablement

Before enabling QA in any project, add black-box tests for exactly these cases:

- disabled QA makes zero LLM calls and records `SKIPPED`;
- no public behavior records `SKIPPED`;
- missing executable validation records `INCONCLUSIVE`;
- a passing scenario records `PASS`;
- a reproducible public failure records `FINDINGS` and starts one bounded
  generator fix round on the current task;
- an environment timeout records `INCONCLUSIVE`, not `PASS`;
- malformed model output records `ERROR` without a panic;
- duplicate scenarios execute once;
- artifact changes invalidate results;
- a sandboxed process cannot write source, Git metadata, a sibling workspace,
  or an arbitrary host path, but can write its assigned runtime directories;
- secrets are absent from prompts, state, findings, and logs;
- concurrent cancellation and restart remove all review worktrees and leave no
  `WORKING` agent;
- an OCC conflict does not duplicate findings or generator fix rounds.

After these tests pass, run the full commands in §9. QA cannot be enabled unless
snapshot isolation, sibling-operation cancellation, and artifact-rerun tests
pass.
