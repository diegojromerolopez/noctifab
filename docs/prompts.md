# Prompt Customization

noctifab lets you customize the prompt of every LLM agent action without
rebuilding the binary. Each customizable prompt belongs to exactly one agent
and one action; the effective template is resolved per `(agent, action)` key.

## The (agent, action) catalog

There are **14 customizable templates across 4 agents**:

| Agent | Actions |
| --- | --- |
| `product_manager` | `generate`, `audit` |
| `planner` | `decompose` |
| `tester` | `write`, `fix`, `refactor`, `write_breadth_first` |
| `generator` | `implement`, `refactor`, `fix`, `single_pass`, `single_pass_fix`, `breadth_first`, `breadth_first_fix` |

Run `noctifab prompts list` to see the catalog with each action's effective
source.

## Resolution order

For each `(agent, action)` key the effective template is resolved in this
order (first hit wins):

1. **Explicit path in config** — `prompts.<agent>.<action>.path` in
   `.noctifab/config.yaml` (absolute, or relative to the project workspace).
2. **Convention file** — `.noctifab/prompts/<agent>/<action>.tmpl` in the
   project workspace, auto-discovered (no config needed).
3. **Embedded default** — shipped inside the binary.

All files are optional; a missing file means the embedded default is used.
A project typically overrides one or two actions, not all 14.

```
.noctifab/prompts/
  tester/
    write.tmpl             # full-template override
    write.append.tmpl      # append (added to the DEFAULT body)
  generator/
    implement.tmpl
```

Invalid overrides (missing config path, template parse error, unknown
agent/action key) abort startup with a clear, file-named error — an explicit
override never falls back silently.

## Appends: small additions without copying a template

For a small addition (one extra rule, one project-specific mandate) use an
**append** instead of copying the whole template. Two equivalent forms:

- Config string:

  ```yaml
  # .noctifab/config.yaml
  prompts:
    tester:
      write:
        append: "Prefer table-driven tests."
  ```

- Convention file: `.noctifab/prompts/<agent>/<action>.append.tmpl`.

Rules:

1. **Every append applies to the embedded DEFAULT body, never to an
   override.** If a full-template override is active for the action, the
   append is ignored and a warning is logged.
2. If both the config `append` string and an `.append.tmpl` file exist, the
   config string wins (consistent with the config > convention order) and a
   warning is logged.
3. Appends are inserted before the non-overridable output contract block.

## Full-template overrides via config

```yaml
# .noctifab/config.yaml
prompts:
  generator:
    implement:
      path: .noctifab/prompts/generator/implement.tmpl
```

## Template syntax and data contract

Templates use Go [`text/template`](https://pkg.go.dev/text/template) syntax
with named placeholders. The available placeholders per agent:

### `tester/*` and `generator/*` — TaskPromptData

| Placeholder | Content |
| --- | --- |
| `{{.Title}}` | Task title |
| `{{.Description}}` | Detailed task description |
| `{{.Context}}` | Combined context block: existing file contents, inspection results, recent tests, recovery directives, previous failure summaries. Empty or starts with a blank line — append it verbatim. |
| `{{.Feedback}}` | Generator's test-fix feedback (`tester/fix` only) |
| `{{.RecentTestsContext}}` | Recently written test files (generator refactor/fix paths) |
| `{{.RecoveryDirective}}` | Stall-recovery directive from the unblocker, if any |
| `{{.TargetFiles}}` | Relative file paths the task targets |

### `product_manager/*` — ProductManagerPromptData

| Placeholder | Content |
| --- | --- |
| `{{.Spec}}` | Raw SPEC.md content |
| `{{.ExistingStories}}` | Concatenated existing user stories (`audit` only) |
| `{{.LegacyFiles}}` | Pre-formatted legacy codebase context block, or empty |

### `planner/*` — PlannerPromptData

| Placeholder | Content |
| --- | --- |
| `{{.Spec}}` | The user story / specification content to decompose |

Example custom `tester/write.tmpl`:

```
You are the Tester Agent for an embedded C project.
Write CMocka tests for the task below. Only test the public API.

Task: {{.Title}} - {{.Description}}{{.Context}}
```

## The non-overridable output contract

Every rendered prompt — default or overridden — always ends with a
**non-overridable output contract block**: the JSON envelope schema
(`{"reasoning": ..., "actions": [...]}`) and the tool list for the role. It
is appended by code after rendering, so a custom template can never break the
machine-readable protocol the orchestrator depends on. Do not repeat the
output schema in your template.

## What stays hardcoded (and why)

The following prompts are deliberately **not** customizable:

| Prompt | Why |
| --- | --- |
| Reader (context gathering) | ~90% fixed tool schema; no methodology worth user control |
| Repair (`watchdog_repair`) | Dormant flow; protocol machinery |
| Unblocker assessment | Corrective-action schema is code-coupled to the unblocker commands |
| Listener (operator commands) | Command-interpretation protocol |
| JSON format reminder | The mechanism that repairs schema violations; letting users edit the repair mechanism is a circular risk |
| Turn continuation wrapper | Turn scaffolding (`TOOL OUTPUTS FROM PREVIOUS TURN...`), not a persona prompt |

## CLI

| Command | Behavior |
| --- | --- |
| `noctifab prompts list` | Prints the agent/action tree with each action's source (config / convention / embedded) and append status. |
| `noctifab prompts show <agent> <action>` | Prints the effective template of one action, where it came from, and the appended contract. |
| `noctifab prompts init [agent] [action]` | Writes embedded defaults into `.noctifab/prompts/` as editable starting points (never overwrites existing files). |
| `noctifab prompts validate` | Parses and test-renders all effective templates; exit code 0 on success, non-zero with file-named errors otherwise. |

Typical workflow:

```bash
# 1. Materialize the default template you want to change
noctifab prompts init tester write

# 2. Edit it
$EDITOR .noctifab/prompts/tester/write.tmpl

# 3. Verify
noctifab prompts validate
noctifab prompts show tester write
```
