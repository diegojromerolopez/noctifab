# Plan: Default Prompts + Per-Agent Prompt Customization

**Status:** Implemented (v0.28.0, branch `feat/custom-prompts`)

**Implementation deviations (per §7.9, documented during implementation):**

- **Planner tool list stays in the overridable body.** The legacy planner
  prompt places the `add_task` tool description mid-body (before the CRITICAL
  rules), not in the tail. Moving it into the contract block would have broken
  byte-identity, so the planner contract covers only the `Return format:` JSON
  schema (which itself demonstrates the full `add_task` argument set). All
  other agents carry their tool list in the contract block as designed.
- **Repair role body moved to `watchdog_repair.go`.** The repair prompt
  depended on the deleted `"Repair task: "` prefix dispatch; its role body now
  lives as hardcoded code (`wrapRepairPrompt`) next to its only caller,
  byte-identical to the legacy assembly.
- **Multi-turn continuation wrapper position.** The wrapper previously landed
  inside the role body's Task Details slot (a side effect of prefix dispatch
  running after wrapping); it is now appended after the fully rendered prompt.
  First-turn prompts are byte-identical; continuation turns carry the same
  content with the wrapper at the end instead of mid-body.
- **Generator breadth-first action naming.** The generator actions were
  renamed `breadth_first` → `implement_breadth_first` and `breadth_first_fix`
  → `implement_breadth_first_fix` for symmetry with
  `tester/write_breadth_first` (`<base>_breadth_first` convention). Action
  names are the public contract (directory names, config keys, CLI args), so
  the rename happened before the first release of the feature.
- **Byte-identical extraction is machine-verified**: the golden tests render
  each default against a verbatim test-only copy of the legacy builders
  (`golden_legacy_test.go`) instead of static golden files.

**Decisions locked in:**
- Structure: **`.noctifab/prompts/<AGENT>/<ACTION>.tmpl`** — one directory per
  agent, one self-contained template per agent action. Every customizable
  prompt belongs to exactly one agent and one action; no shared or orphan
  prompts.
- Scope: **14 customizable templates across 4 agents** (`product_manager`,
  `planner`, `tester`, `generator`). Everything else stays **hardcoded** or is
  **removed** — see the assessment table in §1.2.
- Override mechanisms per action: **full-template override** (config `path` or
  `<ACTION>.tmpl` convention file) and **`append`** (config `append` string or
  `<ACTION>.append.tmpl` convention file) — **every append applies to the
  default prompt body, never to an override** (§2.5).
- Convention directory: **`.noctifab/prompts/`** in the target project
  workspace.
- Dead-code prompts are **removed from the codebase**, not templated:
  `invariants_*` / `PromptBuilder`, `flaky_stabilization`,
  `intent_disambiguator` (all verified: no production call sites).
- `json_reminder` stays **non-overridable code** in `llm.Client` (it is the
  schema-repair mechanism — letting users edit it is a circular risk).
- Documentation: a **prompts documentation page** (`docs/prompts.md`) in the
  readthedocs site, registered in the `index.md` toctree, is a required
  deliverable (§2.9).

---

## 1. Background & Current State

Today every agent prompt is a hardcoded Go string literal. Only dynamic *data*
(spec content, task details, state, failure logs) is injected at runtime via
positional `fmt.Sprintf` verbs. There is no external prompt file, no template
directory, and no config option to override prompts.

Two construction patterns exist:

1. **Prefix-dispatched bodies** (`pkg/infrastructure/llm/prompt_templates.go`):
   services build a raw instruction with a magic prefix; `preprocessPrompt()`
   (invoked at `pkg/infrastructure/llm/client.go:188`) wraps it in the role
   body (persona, rules, tool list, JSON schema).
2. **Inline prompts**: direct `fmt.Sprintf` at each call site (reader,
   unblocker, watchdog, listener, flaky, disambiguator, json_reminder).

### 1.1 Prefix-dispatch bug found during inventory

`preprocessPrompt()` matches raw prompts by string prefix. **Four live prompt
variants match no prefix** and therefore bypass their role body entirely
today — the LLM receives only the raw one-line instruction, with no persona,
no tool list, and no JSON output schema:

| Variant | Call site | Actual prefix | Expected prefix |
| --- | --- | --- | --- |
| tester refactor (code_first retry) | `orchestrator_execute.go:285` | `Write/Refactor tests for task:` | `Write tests for task:` |
| tester write (breadth_first) | `orchestrator_execute_breadth_first.go:39` | `Write tests for task in Breadth-First...` | `Write tests for task:` |
| generator implement (breadth_first) | `orchestrator_execute_breadth_first.go:18` | `Execute task in Breadth-First...` | `Execute task:` |
| generator refine (breadth_first retry) | `orchestrator_execute_breadth_first.go:59` | `Refine implementation and tests for task:` | `Execute task:` |

The rewire (Phase 3a) fixes this by construction: prompts are rendered by
explicit `(agent, action)` key, never by prefix matching. This is a
**deliberate behavior fix** — golden-file tests assert the NEW, correct
assembly for these 4 variants (§4, §7).

### 1.2 Customization assessment (full inventory)

Verdicts: **Customize** (becomes `<AGENT>/<ACTION>.tmpl`), **Hardcode**
(stays as Go code), **Remove** (dead code, deleted in Phase 3b).

| Agent | Action | Call site | Status | Verdict | Rationale |
| --- | --- | --- | --- | --- | --- |
| `product_manager` | `generate` | `roadmap_generator.go:39` | live | **Customize** | Story granularity, DoD mandates, story limits — core product methodology users legitimately tune |
| `product_manager` | `audit` | `roadmap_generator.go:37` | live | **Customize** | Same family; governs audit/refinement of existing stories |
| `planner` | `decompose` | `orchestrator_server.go:37` | live | **Customize** | Task atomicity/cohesion/DAG strategy shapes all downstream work |
| `tester` | `write` | `orchestrator_execute.go:223` | live | **Customize** | Test methodology (BDD, e2e emphasis, mocking rules) — the #1 customization ask |
| `tester` | `fix` | `orchestrator_generator.go:161` | live | **Customize** | Same family; repairs tests on generator feedback |
| `tester` | `refactor` | `orchestrator_execute.go:285` | live | **Customize** | Same family (code_first retry path); currently broken by §1.1 |
| `tester` | `write_breadth_first` | `orchestrator_execute_breadth_first.go:39` | live | **Customize** | Same family (breadth_first architecture); currently broken by §1.1 |
| `generator` | `implement` | `orchestrator_execute.go:204` | live | **Customize** | Coding standards, DI mandates, surgical-edit policy |
| `generator` | `refactor` | `orchestrator_execute.go:263` | live | **Customize** | Same family (code_first quality pass) |
| `generator` | `fix` | `orchestrator_execute.go:325` | live | **Customize** | Same family (code_first retry path) |
| `generator` | `single_pass` | `orchestrator_execute_single_pass.go:16` | live | **Customize** | Same family (single_pass architecture) |
| `generator` | `single_pass_fix` | `orchestrator_execute_single_pass.go:33` | live | **Customize** | Same family (single_pass retry path) |
| `generator` | `implement_breadth_first` | `orchestrator_execute_breadth_first.go:18` | live | **Customize** | Same family (breadth_first architecture); currently broken by §1.1 |
| `generator` | `implement_breadth_first_fix` | `orchestrator_execute_breadth_first.go:59` | live | **Customize** | Same family (breadth_first retry); currently broken by §1.1 |
| `reader` | `inspect` | `orchestrator_helper.go:184` | live | **Hardcode** | ~90% fixed tool schema + generic "gather context" instruction; role name is injected data; no methodology worth user control |
| `repair` | `diagnose` | `watchdog_repair.go:181,57` | dormant | **Hardcode** | `WatchdogRepair` is injected (`start.go:348`, `serve.go:146`) but `AttemptRepair` has no production call site; wire-or-remove decision tracked in [#15](https://github.com/diegojromerolopez/noctifab/issues/15) |
| `repair` | `retry` | `watchdog_repair.go:118` | dormant | **Hardcode** | Same as above ([#15](https://github.com/diegojromerolopez/noctifab/issues/15)) |
| `unblocker` | `assess` | `unblocker_prompt.go:125` | live | **Hardcode** | Body is ~80% fixed corrective-action schema (`reset_task`/`fail_task`/`log_message`/`noop`) + state JSON; semantics are code-coupled to `unblocker_commands.go`; almost no user-tunable methodology |
| `listener` | `interpret` | `listener.go:19,117` | live | **Hardcode** | `listenerSystemPrompt` maps free-form operator commands onto a fixed command schema; protocol machinery code-coupled to the listener dispatcher, no user-tunable methodology |
| — (`llm.Client`) | `json_reminder` | `client.go:120` | live | **Hardcode** | Schema-repair machinery; letting users edit the repair mechanism is a circular risk |
| tester/generator loops | turn continuation wrapper | `orchestrator_generator.go:199`, `orchestrator_helper.go:399` | live | **Hardcode** | Turn scaffolding/protocol (`TOOL OUTPUTS FROM PREVIOUS TURN...`), not a persona prompt |
| all agents | context fragments (recovery directive, failure summary, tool outputs) | various | live | **Hardcode** | Small data fragments injected into prompts, not standalone templates |
| — (`llm`) | `invariants_python` / `invariants_go` | `prompts.go` | dead | **Remove** | `PromptBuilder` has no production call site (only its own tests) |
| — (`services`) | `flaky_stabilization` | `flaky_detector.go:47` | dead | **Remove** | Source comment: "no production code path invokes them yet" |
| — (`services`) | `intent_disambiguator` | `intent_disambiguator.go:44` | dead | **Remove** | `Disambiguate` is only called from tests |

**Result: 14 customizable templates across 4 agents.**

**What about `architect`, `qa`, `security`, `performance`, `docs`, `devops`?**
Those roles exist only as **config placeholders** in `AgentsConfig`
(provider/model routing slots for future expansion) — they have no prompts
and no implementation. The domain (`pkg/domain/state.go`) defines just five
roles: `PLANNER`, `GENERATOR`, `TESTER`, `RESOLVER`, `UNBLOCKER` — and
`RESOLVER` has no prompt either. Note that `audit` is an *action* of
`product_manager`, not an agent. The 4-agent catalog above covers every LLM
prompt that actually exists in the codebase.

### Pain points to fix

1. **Prefix dispatch is fragile — and already broken.** Four live variants
   silently bypass their role bodies (§1.1).
2. **Positional `fmt.Sprintf` verbs are unsafe for user-edited templates.**
   Reordering or omitting a verb silently corrupts the prompt.
3. **No config surface.** The config loader uses `yaml.KnownFields(true)`, so
   any new `prompts:` section must be added to the schema explicitly.
4. **Flat key schema is not human-understandable.** Prompts must be organized
   by the agent and action they belong to.

---

## 2. Target Design

### 2.1 Directory structure & resolution model

One directory per agent, one self-contained template per action:

```
.noctifab/prompts/
  product_manager/
    generate.tmpl             # optional full-template override
    generate.append.tmpl      # optional append (added to the DEFAULT body)
    audit.tmpl                # optional override
    audit.append.tmpl         # optional append
  planner/
    decompose.tmpl            # optional override
  tester/
    write.tmpl                # optional override
    fix.tmpl                  # optional override
    refactor.tmpl             # optional override
    write_breadth_first.tmpl  # optional override
  generator/
    implement.tmpl            # optional override
    refactor.tmpl             # optional override
    fix.tmpl                  # optional override
    single_pass.tmpl          # optional override
    single_pass_fix.tmpl      # optional override
    implement_breadth_first.tmpl      # optional override
    implement_breadth_first_fix.tmpl  # optional override
```

Every action supports the same pair of files: `<ACTION>.tmpl` (full-template
override) and `<ACTION>.append.tmpl` (append). Only the two
`product_manager` appends are shown above; the pattern applies to all 14
actions.

For each `(agent, action)` key, the effective template is resolved in this
order (first hit wins):

1. **Explicit path in config** — `prompts.<agent>.<action>.path` in
   `.noctifab/config.yaml` (absolute or relative to the project workspace).
2. **Convention file** — `.noctifab/prompts/<agent>/<action>.tmpl` in the
   project workspace, auto-discovered.
3. **Embedded default** — `defaults/<agent>/<action>.tmpl` shipped inside the
   binary via `go:embed`. The current hardcoded text is extracted into these
   files, so default behavior is unchanged — **except** the 4 variants of
   §1.1, whose defaults deliberately include the correct role body (the fix).

All files are optional; a missing file means the embedded default is used. A
project typically overrides one or two actions, not all 14. Hardcoded prompts
(reader, repair, unblocker, json_reminder, wrappers, fragments) have **no
template path** and no config key.

**Append files: every `append.tmpl` appends to the default prompt.** For each
action, an optional `<ACTION>.append.tmpl` file may sit next to the override
file. Its content is appended verbatim to the END of the **embedded default**
body — never to an overridden template. If a full override is active for the
action, the append file is ignored and a warning is logged. See §2.5.

Extension is `.tmpl` (not `.md`) because the files contain Go `text/template`
syntax (`{{...}}` placeholders); `.md` would render placeholders as literal
text in markdown previews. `noctifab prompts init` scaffolds these files with
the embedded defaults pre-filled (never overwriting existing files).

### 2.2 Prompt anatomy (per action)

Every full prompt sent to the LLM is assembled from up to three layers:

1. **Overridable action template** (per `<AGENT>/<ACTION>.tmpl`) — the
   complete body for that agent action: persona, methodology/mandates, and the
   action-specific instruction, with named placeholders for dynamic data.
   (Supersedes the earlier "variants as data" idea: variant instructions are
   now first-class action **files**, not injected placeholders — each file is
   exactly what the agent sees for that action, minus the appended contract.)
2. **Conditional context blocks** (data) — recovery directives
   (`PREVIOUS ATTEMPT STALL RECOVERY DIRECTIVE`), previous-failure summaries,
   file/inspection context, written-tests context. Injected via named
   placeholders; absent = empty string.
3. **Non-overridable appended blocks** (code, never part of any template) —
   the JSON/tool output contract (§2.4) and the multi-turn continuation
   wrapper (`TOOL OUTPUTS FROM PREVIOUS TURN (turn X/Y)...`).

### 2.3 Template engine & data contract

- Engine: Go `text/template` with **named placeholders** (no positional
  verbs).
- Typed data structs define the public customization contract:

```go
// TaskPromptData backs tester/* and generator/* action templates.
type TaskPromptData struct {
    Title              string
    Description        string
    Context            string // file + inspection context block
    Feedback           string // generator->tester fix feedback, if any
    RecentTestsContext string // recently written tests, generator refactor only
    RecoveryDirective  string // stall-recovery directive, if any
    TargetFiles        []string
}

type ProductManagerPromptData struct {
    Spec            string
    ExistingStories string
    LegacyFiles     string
}

type PlannerPromptData struct {
    Spec string
}
```

- Templates are parsed once at startup and cached; rendering happens per call.
- Template parse/render errors on an override produce a **fail-fast startup
  error** naming the file and key (never a silent fallback to default after
  the user explicitly opted in).
- Shared mandate text (e.g. the anti-stalling rules) is **inlined in each
  action template** of an agent. Self-contained files are deliberately
  preferred over include-mechanisms: a user editing one action file sees the
  complete prompt. Golden-file tests keep the embedded defaults coherent.

### 2.4 Safety split (output contract is NOT overridable)

Each prompt is divided into two parts:

- **Overridable action body** — persona, mandates, strategy guidance,
  anti-stalling rules, action instruction. This is what the full-template
  override replaces.
- **Non-overridable output contract** — the JSON envelope schema
  (`{"reasoning": ..., "actions": [...]}`) and the tool list for the role.
  This block is **always appended by code after rendering**, so a custom
  template can never break the machine-readable protocol that the orchestrator
  depends on.

The same exclusion applies one layer down, in `llm.Client`: the
`json_reminder` pullback prompt — the mechanism that *repairs* schema
violations — is non-overridable code for the same reason (letting users edit
the repair mechanism is a circular risk).

### 2.5 Lightweight tweak: `append`

In addition to full-template override, every action supports an **append**
mechanism for small additions (one extra rule, one project-specific mandate)
without copying a whole template file. Two equivalent forms:

- **Config string** — `prompts.<agent>.<action>.append` in
  `.noctifab/config.yaml`:

  ```yaml
  prompts:
    tester:
      write:
        append: "Prefer table-driven tests."
  ```

- **Convention file** — `.noctifab/prompts/<agent>/<action>.append.tmpl`
  (e.g. `.noctifab/prompts/tester/write.append.tmpl`).

**Rule: every append appends to the DEFAULT prompt.** The append content
(config string or `append.tmpl` file — the two forms are equivalent) is added
verbatim to the END of the **embedded default** body, never to an overridden
template. Precedence and interaction rules:

1. If a full-template override is active for the action (config `path` or
   convention `<ACTION>.tmpl`), configuring an append for the same action is
   a **fail-fast startup error** — the override is the complete body, and
   both mechanisms are explicit opt-ins, so silently ignoring one would mask
   a configuration mistake. *(Amended during implementation from the original
   "ignore with warning" design, for consistency with the §2.3 fail-fast
   principle.)*
2. If both the config `append` string and an `<ACTION>.append.tmpl` file
   exist, the **config string wins** (consistent with the config > convention
   resolution order in §2.1) and a warning is logged.
3. Appends are applied to the rendered default body, before the
   non-overridable contract block is appended (§2.4).

### 2.6 Configuration surface

```yaml
# .noctifab/config.yaml (all fields optional)
prompts:
  generator:
    implement:
      path: .noctifab/prompts/generator/implement.tmpl   # full-template override
  tester:
    write:
      append: "Prefer table-driven tests."
```

Convention-based discovery requires **no config at all**: dropping
`.noctifab/prompts/generator/implement.tmpl` into the workspace is enough.

The `Config` struct gains:

```go
type PromptOverride struct {
    Path   string `yaml:"path,omitempty"`
    Append string `yaml:"append,omitempty"`
}

// In Config: agent -> action -> override
Prompts map[string]map[string]PromptOverride `yaml:"prompts,omitempty"`
```

Validation (`Config.Validate()`):
- Unknown agent or action keys are rejected (typo guard).
- `path` must exist and be readable; validated at startup, not first use.
- Every effective template is parsed and test-rendered with fixture data at
  startup; failures abort with a clear, file-named error.

### 2.7 Architecture (DI, per AGENTS.md)

New package: `pkg/infrastructure/prompts`

```
pkg/infrastructure/prompts/
  defaults/                    # embedded .tmpl files, one per agent action
    product_manager/
      generate.tmpl
      audit.tmpl
    planner/
      decompose.tmpl
    tester/
      write.tmpl
      fix.tmpl
      refactor.tmpl
      write_breadth_first.tmpl
    generator/
      implement.tmpl
      refactor.tmpl
      fix.tmpl
      single_pass.tmpl
      single_pass_fix.tmpl
      implement_breadth_first.tmpl
      implement_breadth_first_fix.tmpl
  keys.go                      # (agent, action) catalog: 14 keys / 4 agents + validation
  data.go                      # typed template data structs (public contract)
  resolver.go                  # 3-level resolution: config path -> convention -> embedded
  renderer.go                  # parse/cache/render, startup validation
  contracts.go                 # non-overridable output-contract blocks appended post-render
  *_test.go                    # 100% unit tests
```

- `Renderer` is injected via constructors into `Orchestrator` and
  `GenerateRoadmap`.
- `preprocessPrompt()` prefix dispatch is **deleted**. Call sites render by
  explicit `(agent, action)` key instead of relying on magic string prefixes —
  this fixes the §1.1 bug by construction. The agent role is already
  propagated via the `AgentRoleKey` context value where the router needs it.
- `json_reminder` **stays in `llm.Client` as code** (protocol machinery,
  non-overridable, §2.4) — it is NOT rendered through `Renderer`.
- **Hardcoded prompts stay where they are**: `reader/inspect`
  (`orchestrator_helper.go`), `repair/diagnose` + `repair/retry`
  (`watchdog_repair.go`), `unblocker/assess` (`unblocker_prompt.go`), the turn
  continuation wrapper, and the context fragments are not moved into the
  prompts package and get no customization surface.
- **Dead-code removal** (verified by grep: no production call sites):
  - `PromptBuilder` + `invariants_python` / `invariants_go` constants →
    `pkg/infrastructure/llm/prompts.go` and `prompts_test.go` deleted.
  - `DetectFlaky` / `BuildFlakyStabilizationPrompt` →
    `pkg/services/flaky_detector.go` and `flaky_detector_test.go` deleted.
  - `IntentDisambiguator` → `pkg/services/intent_disambiguator.go` and
    `intent_disambiguator_test.go` deleted.
  - The corresponding subtests in `tests/e2e/scenario_comprehensive_test.go`
    (flaky detection, intent disambiguation) are removed as well.
- Prompt compaction (`CompactCaveman` / `CompactSimpleEnglish`) and the
  `checkPromptSize` guard keep running **after** rendering, unchanged.

### 2.8 CLI (DX)

New command group: `noctifab prompts`

| Command | Behavior |
| --- | --- |
| `noctifab prompts list` | Prints the agent/action tree (4 agents × 14 actions), each with its source (config / convention / embedded) and override/append status. |
| `noctifab prompts show <agent> <action>` | Prints the **effective** prompt for one action and where it came from. |
| `noctifab prompts init [agent] [action]` | Writes embedded defaults into `.noctifab/prompts/<agent>/<action>.tmpl` as starting points for editing (all 14, per-agent, or per-action; never overwrites existing files). |
| `noctifab prompts validate` | Parses and test-renders all effective templates; exit code 0 on success, non-zero with file-named errors otherwise. |

### 2.9 Documentation (readthedocs)

A new page **`docs/prompts.md`** is added to the Sphinx/readthedocs site and
registered in the `index.md` toctree. It is the user-facing reference for the
customization system and documents:

- the `.noctifab/prompts/<AGENT>/<ACTION>.tmpl` structure and the
  config > convention > embedded resolution order (§2.1);
- the append mechanism — config `append` string and `<ACTION>.append.tmpl`
  file, both always appending to the **default** body (§2.5);
- the per-agent data contract: every named placeholder from §2.3, per agent
  and action, with examples;
- which prompts are hardcoded and why (the assessment table, §1.2);
- CLI usage (`prompts list/show/init/validate`) with copy-paste examples.

---

## 3. Implementation Phases

| Phase | Scope | Deliverables | Behavior change? |
| --- | --- | --- | --- |
| **0. Contract** | Freeze the `(agent, action)` catalog (14 keys / 4 agents) and the typed data structs | `keys.go`, `data.go` | No |
| **1. Prompts package** | Extract current strings into embedded `defaults/<agent>/<action>.tmpl`; implement `Resolver`, `Renderer`, `contracts.go`; unit tests for precedence, fallback, missing file, bad template, `append` | `pkg/infrastructure/prompts/*` | No (pure refactor) |
| **2. Config** | Add nested `prompts:` schema to `types.go`, defaults, `Validate()`, startup template validation wiring in `config.Load()` | `pkg/infrastructure/config/*` | No (opt-in) |
| **3a. Rewire agent call sites** | Convert `product_manager` (2), `planner` (1), `tester` (4), `generator` (7) call sites to `Renderer.Render(agent, action, data)`; delete `preprocessPrompt()`; update `prompt_templates_test.go` | `pkg/services/*`, `pkg/infrastructure/llm/*` | **Yes — deliberate fix**: the 4 variants of §1.1 now render with their role body (previously bypassed). All other defaults byte-identical |
| **3b. Dead-code removal** | Delete `PromptBuilder` / `pkg/infrastructure/llm/prompts.go` / `prompts_test.go` (invariants), `flaky_detector.go` / `flaky_detector_test.go`, `intent_disambiguator.go` / `intent_disambiguator_test.go`, plus their subtests in `tests/e2e/scenario_comprehensive_test.go`. Hardcoded prompts (reader, repair, unblocker, json_reminder) untouched | `pkg/services/*`, `pkg/infrastructure/llm/*`, `tests/e2e/*` | No (deleted code was never invoked in production) |
| **4. CLI + docs** | Implement `noctifab prompts list/show/init/validate` with tests; write `docs/prompts.md` and register it in the `index.md` toctree (§2.9) | `cmd/noctifab/cli/prompts*.go`, `docs/*` | New feature |
| **5. Gates** | 100% unit tests on all new code, `go fmt ./...`, `docker run -t --rm -v $(pwd):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint run`, `go test -v ./pkg/... ./tests`, CHANGELOG **minor** bump, work on a feature branch (never commit on `main`) | repo-wide | — |

### File-size constraint

All new `.go` files stay well under the 500-line limit (AGENTS.md §2.1):
prompt text moves out of Go source into `.tmpl` files, which also *reduces*
`prompt_templates.go` pressure (currently 485 lines).

---

## 4. Testing Strategy (per AGENTS.md §2.3 + TESTS.md)

**Unit tests (100% coverage of new code):**
- `resolver_test.go`: precedence order per `(agent, action)` key (config path
  > convention > embedded), missing explicit path → error, missing convention
  file → falls back, unknown agent/action rejected, `<ACTION>.append.tmpl`
  discovery.
- `renderer_test.go`: renders every embedded default with fixture data;
  template parse error surfaces file-named error; **append semantics**: config
  `append` string and `<ACTION>.append.tmpl` file both append to the default
  body, config wins over the file (warning logged), and both are ignored
  (warning logged) when a full override is active.
- `contracts_test.go`: rendered output for each of the 14 keys always ends
  with the non-overridable JSON/tool contract block, for both default and
  overridden templates.
- Golden-file tests: default-rendered prompts match the current assembly
  **byte-for-byte for 10 of 14 variants**; for the 4 variants of §1.1 the
  golden files assert the NEW, correct assembly (role body present) — the fix
  is documented and intentional.
- Config: `KnownFields(true)` acceptance of nested `prompts:`, validation of
  unknown agents/actions / unreadable paths.
- Updated existing tests: `prompt_templates_test.go` (rewire),
  `unblocker_prompt_test.go`, `watchdog_repair_test.go` (unchanged behavior —
  both stay hardcoded).
- Deleted alongside their dead production code: `prompts_test.go`
  (`PromptBuilder`), `flaky_detector_test.go`, `intent_disambiguator_test.go`,
  and the flaky-detection / intent-disambiguation subtests in
  `tests/e2e/scenario_comprehensive_test.go`.

**Integration tests:**
- CLI-level test: workspace with `.noctifab/prompts/generator/implement.tmpl`
  → generator flow uses the custom body and still receives the appended
  contract block; invalid template → startup abort with clear error.

**E2E:** optional fixture in the existing containerized harness is deferred;
validation projects' configs remain untouched (Configuration Immutability
Mandate — the feature is opt-in).

---

## 5. Risks & Mitigations

| Risk | Mitigation |
| --- | --- |
| Custom template breaks the JSON protocol | Non-overridable output-contract block appended by code (§2.4) + startup render validation |
| Custom template drops the tool list | Tool list lives in the non-overridable contract block |
| Broken `json_reminder` repair path | `json_reminder` is non-overridable code (§2.4) — no template exists for it |
| Shared-mandate text drifts across an agent's action files | Accepted by design (self-contained files are clearer); golden-file tests render all 14 defaults on every CI run and keep them coherent |
| Behavior change for the 4 prefix-dispatch-miss variants | Deliberate fix (§1.1), explicitly tested; called out in CHANGELOG |
| Dead-code removal changes live behavior | All three removals (`PromptBuilder`/invariants, flaky detector, intent disambiguator) are verified to have no production call site; removal is a runtime no-op, guarded by the full test suite |
| Template/code drift when data structs evolve | Data structs are the versioned public contract; golden-file tests render every default template |
| User confusion about which prompt is active | `noctifab prompts list/show` exposes the effective prompt per action and its source |
| Oversized user template burns tokens | Existing `checkPromptSize` guard runs post-render, unchanged |
| Silent fallback masks a broken override | Explicit overrides fail fast at startup; only the *convention* layer falls back (by design, file absent = not an override) |
| Secrets accidentally embedded in templates | Templates are project files like any source file; no special handling added, secrets mandate unchanged |

---

## 6. Explicit Non-Goals

- No customization surface for hardcoded prompts: `reader/inspect`,
  `repair/diagnose` + `repair/retry` (dormant flow), `unblocker/assess`,
  `listener/interpret`, `json_reminder`, the turn continuation wrapper, and
  context fragments. They
  stay as Go code; if a future need emerges, each can be promoted to
  `<AGENT>/<ACTION>.tmpl` following this plan's pattern.
- No templates for dead-code prompts: `invariants_*` / `PromptBuilder`,
  `flaky_stabilization`, and `intent_disambiguator` are **removed from the
  codebase**, not templated.
- No changes to any `validation/projects/**` configuration (Configuration
  Immutability Mandate).
- Default templates remain language- and project-agnostic (Agnosticism
  Mandate); customization is the user's escape hatch.
- No hot-reload of templates mid-run (startup-load only; keeps the stateless
  agent model intact).
- No per-provider prompt variants in this iteration (one template per action,
  independent of LLM provider).

---

## 7. Acceptance Criteria (Definition of Done)

1. All 14 `(agent, action)` templates renderable through
   `pkg/infrastructure/prompts`; embedded defaults produce **byte-identical**
   output to today's assembly for 10 variants, and the **corrected** assembly
   (role body present) for the 4 prefix-dispatch-miss variants of §1.1 —
   covered by golden-file tests.
2. Full-template override works for every action via both
   `prompts.<agent>.<action>.path` and
   `.noctifab/prompts/<agent>/<action>.tmpl`, with the documented precedence
   order. Append works for every action via both config `append` and
   `.noctifab/prompts/<agent>/<action>.append.tmpl`, always appending to the
   **default** body (never to an override).
3. The non-overridable JSON/tool contract block is appended to every rendered
   prompt, default or overridden.
4. Invalid override (missing file, parse error, unknown agent/action) aborts
   startup with a clear, file-named error.
5. `noctifab prompts list/show/init/validate` implemented and tested;
   `docs/prompts.md` written and registered in the `index.md` toctree (§2.9).
6. Dead code is deleted: `PromptBuilder` + `pkg/infrastructure/llm/prompts.go`
   + `prompts_test.go` (invariants), `pkg/services/flaky_detector.go` +
   `flaky_detector_test.go`, `pkg/services/intent_disambiguator.go` +
   `intent_disambiguator_test.go`, and their e2e subtests in
   `tests/e2e/scenario_comprehensive_test.go`. `json_reminder`, `reader`,
   `repair`, and `unblocker` prompts remain hardcoded — no `.tmpl` default or
   override path exists for any removed or excluded prompt.
7. `go fmt ./...` clean; `golangci-lint run` (v2.12.2, docker) clean;
   `go test -v ./pkg/... ./tests` 100% pass.
8. `CHANGELOG.md` updated with a minor version bump (including the §1.1
   prefix-dispatch fix); work delivered on a feature branch; no commits on
   `main`.
9. This document (`CUSTOM_PROMPTS.md`) updated if any design detail changes
   during implementation.
