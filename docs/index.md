# noctifab Documentation

Welcome to the documentation for **noctifab**, an autonomous, long-running agentic harness designed to operate without human intervention to resolve issues, verify builds, run tests, and manage software project lifecycles on GitHub and GitLab.

`noctifab` acts as a **Dark Factory Platform** for software development, functioning as a single-node autonomous loop engine to replace manual developer bottlenecks.

---

## What is a Dark Factory?

A "Dark Factory" (in a software engineering context) is a fully automated repository management loop. You define specifications or issues, and the orchestrator engine works continuously—spawning subagents, modifying files, executing tests, rebasing branches, and merging code—until the specification is fully satisfied.

### Autonomy Level Matrix

`noctifab` is built to operate at Level 3 and Level 4 autonomy:

| Level | Name | Platform Behavior |
| :--- | :--- | :--- |
| **Level 1** | Autocomplete | AI suggests code inline. Human drives the editor and makes all decisions. |
| **Level 2** | Interactive Assistant | AI generates entire files/functions. Human reviews every single change in the editor. |
| **Level 3** | Spec-Driven (Gated) | AI generates code autonomously from specifications. Test validation gates quality. Human clicks merge. |
| **Level 3.5** | Selective Auto-Merge | Same as Level 3, but low-risk modules merge automatically. Human can block. |
| **Level 4** | Full Dark Factory | Specs go in, tested code comes out fully merged. Human reviews only exceptions. |

---

## Contents

```{toctree}
:maxdepth: 2

getting_started
cli_usage
configuration
configuration_examples
llm_providers
execution_report
prompts
architecture
api
developer_guide
unblocker_agent
last_resort_agent
secrets
noctifab_evaluation_report
```

---

- **Instant 2-Minute Zero-Config Sandbox (`noctifab demo`)**: Ephemeral, 100% offline, deterministic simulation of the complete Dark Factory loop with embedded templates, zero external API keys, and automated cleanup.
- **Always-On Background Standby Daemon & Real-Time Visual Web Dashboard (`noctifab start -w --web-open` / `noctifab dashboard -w --web-open`)**: Modern embedded web UI featuring auto-browser launch (`--web-open`), zero-idle-CPU standby mode (`STANDBY 🟢`), live topological task DAG rendering, syntax-highlighted code diffs, real-time Server-Sent Events (SSE) telemetry stream, prompt order input bar, desktop alerts, and flow pause/resume controls.
- **Mid-Flight Interactive Steerability & Developer Prompt Orders**: Human-in-the-loop steering directives (`noctifab steer`) and ad-hoc prompt orders (`noctifab order`) injected dynamically into the Command Mailbox and agent prompts without halting execution.
- **1-Click Local LLM Profiles & DeepSeek-R1 Parser**: Pre-configured profiles (`ollama-qwen`, `ollama-deepseek`, `vllm-local`, `openai-compat`) with automatic `<think>...</think>` reasoning tag stripping for reasoning models.
- **State-Driven Orchestration**: Operates using a stateless agent controlled by a stateful orchestrator. The orchestrator tracks tasks, action execution, and clarifications in a local SQLite or PostgreSQL database.
- **Story DAG Scheduler & Cross-Story Parallelism**: Parses `depends_on` dependencies from User Story YAML frontmatter and concurrently executes all unblocked user stories across worker slots, dynamically unblocking dependent stories as prerequisites finish.
- **Structured Roadmap Organization & Task Serialization**: Organizes specifications into `roadmap/user-stories/` and automatically serializes task domain models into markdown files in `roadmap/tasks/` (`US-XXX-TASK-YYY-slug.md`).
- **Structured Execution Reports & Telemetry**: Captures fine-grained `execution_log` timeline events during runs and atomically writes Markdown `<TIMESTAMP>_<PROJECT>.md` artifacts to `.noctifab/reports/` with real-time ASCII directory trees (`### Filesystem Hierarchy`), phase execution windows, agent execution spans, and error breakdowns.
- **Topological Task Scheduling**: Automatically constructs a Directed Acyclic Graph (DAG) of task dependencies within each story and runs independent tasks concurrently in isolated Git worktrees.
- **Self-Correcting & Dynamic Prompts Engine**: Dynamically adapts agent prompts using live log tailing, secret scrubbing (`log_tailer.go`), 0-token fast-path regex pre-filtering (`unblocker_fastpath.go`), 10x progressive log escalation, and `[STALL RECOVERY DIRECTIVE]` prompt injection on task retries.
- **Legacy Codebase Stabilization**: Automatically scans pre-existing workspace code (`scanLegacyFiles`) and dynamically injects `roadmap/user-stories/US-001.md` characterization testing mandates into Product Manager, Planner, Generator, and Tester prompts before refactoring or feature additions.
- **Pre-Flight Diagnostics & Capability Caching**: Performs automated pre-flight checks (Git CLI, DB connectivity, LLM `/models` endpoint ping, sandbox mode) before launch, and maintains a thread-safe model parameter capability cache (`providerCapabilityCache`) to omit unsupported parameters automatically.
- **Sandboxed Action Execution**: Safely edits code files and runs test suites inside host sandboxes or Docker containers.
- **Test Validator Verification**: Prevents regression and guarantees code quality by running the project test suite multiple times with majority voting (2/3 consensus).
- **Automated VCS Merging**: Manages Git checkouts, worker branch creation, serialized rebase queues, pull request creation, and merges on GitHub and GitLab.
- **Unblocker Agent**: An autonomous background goroutine that periodically scans the pipeline for stalled tasks and blocked agents, diagnoses root causes via LLM, and injects corrective interventions to restore forward progress — with configurable waking frequency (`unblocker.poll_interval`).
