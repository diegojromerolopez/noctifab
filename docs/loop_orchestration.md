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
