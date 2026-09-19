# Multi-Loop Orchestration & Dark Factory Quality Architecture

`noctifab` utilizes a multi-loop execution architecture to achieve high-resilience autonomous software delivery. In unattended dark factory runs, complex systems cannot always be fully implemented in a single linear pass. The multi-loop engine provides autonomous self-healing, progressive convergence, whole-workspace regression guarding, and strict quality verification.

---

## Key Principles

1. **Backlog Iteration Guarantee**: In each loop pass ($1 \dots N$), Noctifab iterates through 100% of discovered user stories in `roadmap/user-stories/`. A failure in an intermediate story records diagnostic telemetry but does not halt the loop, allowing downstream stories to be attempted.
2. **Two-Stage Story Verification**: A user story is only finalized as `StorySuccess` when:
   - **Stage 1 (Task Integrity)**: 100% of planned tasks achieve `TaskSuccess`.
   - **Stage 2 (DoD & Behavioral Review)**: The `StoryQAAuditor` verifies that the generated codebase satisfies all Definition of Done (DoD) criteria and passes both E2E test suites and whole-workspace regression checks.
3. **Automated Story Refinement**: If a story is incomplete or missing DoD features, Noctifab automatically enriches `roadmap/user-stories/<story>.md` with a `## Refined Acceptance Criteria & Missing Requirements` section and queues a targeted remediation task for the worker pool.
4. **Whole-Workspace Regression Guarding**: In Loop $k \ge 2$, before finalizing any story, the test validator executes the entire repository's test suite (`go test ./...`, `pytest`, `cargo test`, `npm test`, or `make test`) to guarantee changes in shared packages didn't break earlier modules.
5. **Loop Stagnation Circuit Breaker**: If Loop $k+1$ generates 0 codebase mutations and repeats identical failure signatures as Loop $k$, the orchestrator detects stagnation and terminates early to prevent token waste.
6. **Early Convergence Exit**: If all user stories in the backlog achieve verified `StorySuccess` on Loop $k$, Noctifab completes immediately without burning tokens on remaining loops.
7. **Generator-Tester Oscillation Circuit Breaker**: During intra-task multi-turn execution, if a task records $\ge 2$ consecutive passing test suites with 0 errors, $\ge 2$ consecutive turns have only modified test files with unchanged `src/` production code, and task progress is $\ge 70\%$, the orchestrator halts redundant test cosmetic churn and forces the task forward to review and completion.
8. **Global Task DAG & Cross-Story Pipelining**: Within and across iteration loops, task scheduling is evaluated across the entire project graph. Downstream user stories (e.g. `US-002`) unblock their tasks concurrently as soon as prerequisite foundation/interface tasks (e.g. `US-001-TASK-001`) merge into `main`, eliminating idle serialization and allowing multi-story implementations to progress in parallel.
9. **Whole-Project Acceptance & Behavioral Contract Audit Gate**: After all user stories and story-level QA evaluations pass, Noctifab invokes the `AcceptanceAuditor`. The auditor executes the project's black-box E2E test suite (`docker-compose.e2e.yml`, `make e2e`, or configured test commands) and audits the running codebase against the root `SPEC.md`. It verifies that every required command, endpoint, and behavior is verified by genuine, non-tautological tests. If any specification gaps or tautological tests are detected, Noctifab injects an actionable remediation task (`spec-remediation-<n>`) into the internal orchestrator loop. If contracts remain unfulfilled after remediation cycles, the build fails cleanly (`exit 1`), enabling outer autonomous feedback loops to intervene.

---

## Layered E2E Testing & Quality Architecture

Noctifab enforces a **defense-in-depth ("layered fallback")** strategy for End-to-End (E2E) integration testing. This ensures that integration failures are detected and resolved as early as possible without prematurely blocking intermediate tasks on future, unimplemented features.

```
┌─────────────────────────────────────────────────────────────────────────────────────────┐
│  1. Whole-Project Acceptance Audit Gate (AcceptanceAuditor)                             │ <- Runs FULL E2E against SPEC.md at project conclusion
├─────────────────────────────────────────────────────────────────────────────────────────┤
│  2. Post-Pipeline Sovereign Rescue Agent (start_sovereign_rescue)                       │ <- Runs FULL E2E on rescue turns if any story failed
├─────────────────────────────────────────────────────────────────────────────────────────┤
│  3. Story QA Completeness Gate (StoryQAAuditor)                                         │ <- Runs FULL E2E at the end of each completed user story
├─────────────────────────────────────────────────────────────────────────────────────────┤
│  4. In-Loop Fallback Agent (RunFallbackAgent)                                           │ <- Runs FULL E2E on task stalls, retries exhausted, or sovereign directives
├─────────────────────────────────────────────────────────────────────────────────────────┤
│  5. QA & Spec Remediation Tasks (qa-remediation-*, spec-remediation-*)                  │ <- Runs FULL E2E before declaring remediation task green
├─────────────────────────────────────────────────────────────────────────────────────────┤
│  6. Generator-Tester Loop (TestValidator.ValidateTask)                                  │ <- Runs FEATURE-SCOPED E2E: in-scope failures enforced;
│                                                                                         │    out-of-scope downstream failures ignored
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

### 1. Whole-Suite E2E Enforcement Points
In the following gates, the **entire E2E test suite** (`make e2e`, `docker compose.e2e.yml`, or custom commands) is executed and strictly enforced:
- **Story QA Auditor (`StoryQAAuditor`)**: Executed after all tasks of a user story achieve `TaskSuccess`. Verifies that the completed story integrates with the system and passes the full E2E suite.
- **In-Loop Fallback Agent (`RunFallbackAgent`)**: Triggered when a task stalls, exhausts retries, or encounters QA deadlocks. Evaluates full E2E tests on each sovereign repair turn, rejecting turns where integration fails.
- **Post-Pipeline Sovereign Rescue (`start_sovereign_rescue`)**: Activated if any story remains unfulfilled after pipeline completion. Mandates full E2E execution and anti-stub quality gating on every turn.
- **Whole-Project Acceptance Gate (`AcceptanceAuditor`)**: Evaluated at the finale of the run against `SPEC.md`. Requires full E2E verification across all system commands.
- **Remediation Tasks (`qa-remediation-*`, `spec-remediation-*`)**: Created specifically to resolve audit gaps; every failure is considered in-scope and must be fixed before the task passes.

### 2. Feature-Scoped E2E in the Generator-Tester Loop
During intra-task execution in the generator-tester loop (`TestValidator.ValidateTask`):
- When unit tests pass, `TestValidator` evaluates detected E2E tests for tasks targeting integration files, final tasks in a story, or tasks with E2E directives.
- **Scope Filtering (`isE2EFailureInScope`)**: Noctifab parses failure traces from test runners (Pytest, Go test, Cargo, generic runners) and checks failing test names/files against the **active feature scope** (story ID, target files, and feature keywords).
- **In-Scope Failures**: If a failure corresponds to the active feature under development, `ValidateTask` returns `Passed: false` with the failure log, immediately triggering the generator's single-turn **surgical repair turn**.
- **Out-of-Scope Failures**: If the E2E suite reports failures in downstream features that have not yet been developed (e.g. `Hashes` or `PubSub` failing while developing `US-001 Connection & Ping`), the failure is logged and safely ignored in the task loop (`⚠️ Orchestrator: Task ... E2E failure(s) are outside the scope of active feature; ignoring out-of-scope failure`). This guarantees forward momentum without masking errors in the current feature.

---

## Configuration

In `.noctifab/config.yaml`:

```yaml
runtime:
  loop:
    count: 3                # Number of iteration loops (defaults to 1)
  max_tokens: 500000        # Global token consumption boundary
  max_duration: "10m"       # Total execution time limit
```

*(Note: Legacy `runtime.loops: 3` is also supported for backwards compatibility).*

### Product Manager Backlog Sizing
The total number of user stories created for the project is configured independently from loop execution passes:

```yaml
agents:
  product_manager:
    user_stories:
      max_count: 5          # Maximum user stories in roadmap (default: 5)
      complexity:
        min: 15             # Minimum target complexity units per story
        max: 35             # Maximum target complexity units per story
    passes: 2               # PM refinement passes
```

---

## CLI Multi-Loop Override

Override the configured loop count directly from the command line using the `--loops` / `-L` flag:

```bash
# Execute with 3 iterative loops
noctifab start . --loops 3

# Resume from first incomplete story with 2 loops
noctifab resume . -L 2
```

---

## Multi-Loop Convergence Matrix

Execution reports (`output/report/<TIMESTAMP>_<PROJECT>.md`) and the Web Dashboard (`GET /api/v1/convergence`) provide a dedicated **Convergence Matrix** table:

```markdown
## Multi-Loop Convergence Matrix

| Loop # | Stories Attempted | Stories Succeeded | Remediations Triggered | Tokens Used | Duration | Outcome |
| :--- | ---: | ---: | ---: | ---: | :--- | :--- |
| **Loop 1** | 4 | 3 | 2 | 125000 | 3m 12s | FAILED |
| **Loop 2** | 1 | 1 | 1 | 45000 | 1m 5s | SUCCESS |
```
