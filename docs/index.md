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
| **Level 3** | Spec-Driven (Gated) | AI generates code autonomously from specifications. Holdout scenarios gate quality. Human clicks merge. |
| **Level 3.5** | Selective Auto-Merge | Same as Level 3, but low-risk modules merge automatically. Human can block. |
| **Level 4** | Full Dark Factory | Specs go in, tested code comes out fully merged. Human reviews only exceptions. |

---

## Contents

```{toctree}
:maxdepth: 2

cli_usage
architecture
developer_guide
secrets
```

---

## Features & Capabilities

- **State-Driven Orchestration**: Operates using a stateless agent controlled by a stateful orchestrator. The orchestrator tracks tasks, action execution, and clarifications in a local SQLite or PostgreSQL database.
- **Topological Task Scheduling**: Automatically constructs a Directed Acyclic Graph (DAG) of task dependencies and runs independent tasks concurrently.
- **Sandboxed Action Execution**: Safely edits code files and runs test suites inside host sandboxes or Docker containers.
- **Holdout BDD Verification**: Prevents regression and guarantees code quality using holdout behavior-driven development (BDD) tests with majority voting.
- **Automated VCS Merging**: Manages Git checkouts, worker branch creation, rebase queues, pull request creation, and merges on GitHub and GitLab.
