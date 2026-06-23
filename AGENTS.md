# AGENTS.md: Development Guidelines for AI Coding Assistants

Welcome, agent. This document outlines the rules, architecture, and coding constraints of the `noctifab` repository. Read and follow these directives strictly before modifying any files.

---

## 1. Core Reference File

Before planning or executing any task in this codebase, you **must** read and understand:
*   [SPEC.md](/SPEC.md) - The technical specification of the project.

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
3.  **Testing Strategy:**
    *   All code must be **100% unit tested**. Every Go package must be accompanied by unit tests.
    *   After making any change to the codebase, you **must** run the test suite to verify correctness.
    *   Tests must reside in files ending with `_test.go` in the same directory as the target logic.
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
    *   **BDD Specifications:** Holdout scenarios and acceptance tests must always run under a test runner using BDD format with the context pattern: `when <scenario>`, `it <action happens>`.
4.  **Formatting & Linting:**
    *   **Formatting:** All Go source code must strictly follow the standard `go fmt` format. Ensure `go fmt ./...` runs clean.
    *   **Linting:** Code must pass static analysis checks. You must run `docker run -t --rm -v $(pwd):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint run` after every change to ensure the code passes all linter rules.
5.  **Continuous Integration (CI):**
    *   A GitHub Actions workflow configured in `.github/workflows/ci.yml` executes on every push and pull request.
    *   All unit tests and static analysis linting checks must pass successfully in the CI pipeline before merging.

---

## 3. The Stateless Rule for Agents

`noctifab` is designed around the principle of a **stateless agent** controlled by a **stateful orchestrator**:
*   The orchestrator loads, updates, validates, and saves the system state.
*   The LLM agent only operates on the state snapshot provided to it at each step.
*   **Do not rely on the LLM's conversation history** to track what has happened previously. Always inspect the current State struct representation (e.g., in JSON file databases, local configurations, or databases) to determine the next task.
