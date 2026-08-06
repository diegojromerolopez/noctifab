# AGENTS.md: Development Guidelines for AI Coding Assistants

Welcome, agent. This document outlines the rules, architecture, and coding constraints of the `noctifab` repository. Read and follow these directives strictly before modifying any files.

---

## 1. Core Reference File

Before planning or executing any task in this codebase, you **must** read and understand:
*   [SPEC.md](/SPEC.md) - The technical specification of the project.
*   [TESTS.md](/TESTS.md) - The testing strategy, structures, and verification specifications.

---

## 2. Mandatory Coding Constraints

To maintain modularity and high context compatibility, the following guidelines are absolute:

1.  **File Size Limits:**
    *   No single Go source code file (`.go`) may exceed **500 lines** of code.
    *   If a file you are working on or creating approaches or exceeds this limit, you **must** split/refactor it into smaller, logically coherent domain models or helper packages.
2.  **Architecture & Design:**
    *   **Dependency Injection (DI):** Do not hardcode dependencies. Provide all objects, configurations, and clients through constructors. Code must be built in a way that is easy to test (utilizing dependency injection to make components highly mockable and isolated).
    *   **SOLID:** Keep classes/structs focused on a single responsibility.
    *   **Domain-Driven Design (DDD):** Align packaging boundaries to domain logic (e.g., domain entities, value objects, and service interfaces), not technical categories.
    *   **Provider Struct Composition (LLM Infrastructure):** All LLM provider clients must reside in dedicated per-provider source files (`pkg/infrastructure/llm/<provider>.go`). OpenAI-compatible providers must embed `*baseOpenAIClient` and use the declarative `NewModelParser` engine. Core dispatching in `client.go` must be data-driven via `ProviderSpec.NewClientFunc` with zero protocol `switch` statements.
    *   **Verification vs. Validation Engineering Strategy:** Task execution is divided into two distinct stages: *Verification* (building minimal working functionality that compiles and satisfies baseline checks) and *Validation* (black-box behavioral testing against public contracts, CLI outputs, and API signatures). Tests must never assert internal module implementation details.
    *   **Product Manager Definition of Done (DoD) Mandate:** Generated user stories (`roadmap/US-xxx.md`) must specify explicit public API signatures, binary executable paths, I/O formatting invariants, error prefixes, exit codes, number precision representations, and zero-failure test pass criteria before downstream task planning starts.
    *   **Project & Language Agnosticism:** Noctifab MUST NOT HAVE validation project-specific or language-specific code in its codebase. Noctifab is a dark factory agent; do not add specific instructions, context helpers, or code rules for particular validation projects or programming languages.
3.  **Testing Strategy:**
    *   All code must be **100% unit tested**. Every Go package must be accompanied by unit tests.
    *   After making any change to the codebase, you **must** run the test suite to verify correctness.
    *   Tests must reside in files ending with `_test.go` in the same directory as the target logic. Detailed testing context and architecture details are documented in [TESTS.md](/TESTS.md).
    *   When writing new features, ensure corresponding unit tests are implemented concurrently.
    *   **How to Run Unit & Local Integration Tests:**
        *   Run all unit and in-process CLI integration tests locally:
            ```bash
            go test -v ./pkg/... ./tests
            ```
        *   Alternatively, use the Makefile target:
            ```bash
            make test
            ```
    *   **How to Run End-to-End (E2E) Tests:**
        *   Run the containerized E2E test suite (which sets up a PostgreSQL instance via Docker Compose):
            ```bash
            docker compose -f tests/e2e/docker-compose.yml up --build --exit-code-from test-runner
            ```
    *   **BDD Specifications:** Acceptance tests must always run under a test runner using BDD format with the context pattern: `when <scenario>`, `it <action happens>`. Generated tests must be e2e as much as possible for the happy paths, input validations/edge cases must be unit tests, and complex internal validation flows must be integration tests.
4.  **Formatting & Linting:**
    *   **Formatting:** All Go source code must strictly follow the standard `go fmt` format. Ensure `go fmt ./...` runs clean.
    *   **Linting:** Code must pass static analysis checks. You must run `docker run -t --rm -v $(pwd):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint run` after every change to ensure the code passes all linter rules.
5.  **Continuous Integration (CI):**
    *   A GitHub Actions workflow configured in `.github/workflows/ci.yml` executes on every push and pull request.
    *   All unit tests and static analysis linting checks must pass successfully in the CI pipeline before merging.
6.  **Branching & Commit Guidelines:**
    *   **No Commits on Main**: Never create commits directly on the `main` branch. Always create a new branch with the changes.
    *   **CHANGELOG Updates**: Every commit must contain the corresponding changes documented in `CHANGELOG.md`, incrementing the version accordingly: minor version bump for features, and patch version bump for bug fixes.
7.  **Resilience to Scaffold Errors:**
    *   A bad scaffold or failing scaffold verification test must not stop development. It is mandatory for agents to continue making progress on implementing core business requirements even if there are scaffolding or setup errors. It is better to have an imperfect/partial solution that fulfills core requirements than to stall.

---

## 3. The Stateless Rule for Agents

`noctifab` is designed around the principle of a **stateless agent** controlled by a **stateful orchestrator**:
*   The orchestrator loads, updates, validates, and saves the system state.
*   The LLM agent only operates on the state snapshot provided to it at each step.
*   **Do not rely on the LLM's conversation history** to track what has happened previously. Always inspect the current State struct representation (e.g., in JSON file databases, local configurations, or databases) to determine the next task.

---

## 4. Running Validation Projects (Local E2E Matrix)

To run a fully containerized, isolated, end-to-end (E2E) integration check of `noctifab` implementing features autonomously inside a target project:

> [!TIP]
> For the recommended project order, tier classification, and failure attribution,
> see [`validation/projects/TESTING_GUIDE.md`](validation/projects/TESTING_GUIDE.md).

1. **Credentials Setup**:
   Create a `secrets.yaml` file on the host at `validation/projects/<project>/.noctifab/secrets.yaml` containing the necessary LLM API keys:
   ```yaml
   OPENCODE_API_KEY: "your-key"
   GITHUB_TOKEN: "your-token"
   ```
   *Note: This file is excluded from the build context by `.dockerignore` and `.gitignore` to prevent secret leakage, and is safely mounted at runtime.*

2. **Executing the Validation Harness**:
   - Run a single validation project:
     ```bash
     make validate PROJECT=<project>
     ```
   - Run all validation projects in parallel:
     ```bash
     make validate-all
     ```
   - Reuse existing docker images (skipping the rebuild phase):
     ```bash
     make validate PROJECT=<project> SKIP_BUILD=1
     ```

3. **Output Artifacts**:
   All outputs from the validation run are written directly to the target validation project's output path:
   - **Logs**: Captured container console output (`<project>.log`) and wrapper output (`<project>.wrap.log`) are stored in `validation/projects/<project>/output/log/`.
   - **Feedback**: A structured Markdown report (`<PROJECT>_FEEDBACK.md`) summarizing the run is saved under `validation/projects/<project>/output/feedback/`.
   - **Source Code & Binaries**: The generated codebase and compiled executables are placed in `validation/projects/<project>/output/src/` and `validation/projects/<project>/output/dist/` respectively.

4. **Spec-Driven Validation Rule**:
   Pre-creating or checking in static roadmap user stories (e.g. under `roadmap/`) for validation projects is **strictly forbidden**. Validation projects must be defined and run solely based on `SPEC.md` to verify that `noctifab` is capable of autonomously decomposing specifications into user stories on the fly using its Product Manager Agent.

5. **Monitoring & 60-Second Status Loop**:
   When executing validation projects in parallel or in the background (e.g. `make validate-all`), agents must monitor the execution status and output a periodic update table every 60 seconds using the `schedule` tool (`DurationSeconds=60`).
   
   - **Data Sources**: Inspect container logs (`validation/projects/<project>/output/log/<project>.log` or `.validation-logs/<project>.log`), output source directories, and feedback reports (`<PROJECT>_FEEDBACK.md`).
   - **Stuck Detection**: Flag a project as stuck (`Stuck? = Yes`) if no log output or file modification has occurred for **> 5 minutes**, or if the agent is caught in an infinite error/retry loop.
   - **Required Table Columns**:
     - `Project`: Target project name (e.g. `calculator`, `wc`, `frontpunch`).
     - `Status`: `Running`, `Completed ✅` (MUST use white check mark emoji `✅` when completed), `Failed ❌`, or `Stuck ⚠️`.
     - `Stuck?`: `Yes` or `No`.
     - `Completion (%)`: Percent of total planned user stories / spec tasks finished (e.g., `60% (3/5 stories)`).
     - `Tests (Passed/Total)`: Count of passing tests vs total unit/integration tests (e.g. `14/14`).
     - `Current Activity`: Brief commentary of what the agent is currently executing (e.g., `"Decomposing SPEC.md"`, `"Implementing US-002"`, `"Compiling binary"`).
     - `Elapsed Time`: Duration since validation launch (e.g. `04m 15s`).
     - `Last Log Activity`: Time elapsed since the last log write (e.g. `12s ago`).
   
   **Status Report Table Format**:
   | Project | Status | Stuck? | Completion (%) | Tests (Passed/Total) | Current Activity | Elapsed Time | Last Log Activity |
   | :--- | :--- | :---: | :---: | :---: | :--- | :---: | :---: |
   | `calculator` | Running | No | 60% (3/5 stories) | 8/8 | Writing unit tests for US-003 | 04m 12s | 8s ago |
   | `wc` | Completed ✅ | No | 100% (4/4 stories) | 12/12 | Final verification passed (PASS) | 08m 45s | 2m 10s ago |
   | `fortune` | Stuck ⚠️ | **Yes** | 25% (1/4 stories) | 2/5 | Retrying failed scaffold build (no log update > 5m) | 12m 00s | 5m 45s ago |

