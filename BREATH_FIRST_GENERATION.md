# Breadth-First Generation (BFG): Benevolent Iterative Architecture

## 1. Executive Summary

`noctifab` is designed as an autonomous dark factory agent that generates complete, fully verified code from high-level specifications and user stories. Under the existing **Code-First Verification Loop (DFV)**, agents attempt to produce 100% production-ready, linter-clean, edge-case-resilient code on the very first pass of each user story. 

In practice, this depth-first approach often creates execution bottlenecks:
* Agents get stuck in multi-retry loops fixing minor linter nitpicks, formatting guidelines, or obscure edge-case test assertions before core functionality across the rest of the application is even built.
* End-to-end integration and system-wide visibility are delayed until the very last task completes.
* Edge cases implemented prematurely in Task 1 frequently require refactoring once Tasks 2 and 3 introduce new system interactions.

**Breadth-First Generation (BFG)** shifts the generation paradigm from *depth-first perfectionism* to *breadth-first iterative refinement*. 

In BFG:
1. **Pass 1 (Broad Foundation / ~80% Feature Coverage):** Generator and Tester agents implement core happy-path functionality across **all** user stories simultaneously. Minor formatting warnings, non-critical linter complaints, and complex edge cases are explicitly deferred—provided there are no fatal build failures or panics.
2. **Benevolent Judges (QA, Security, Performance, DevOps):** Evaluators operate as benevolent guides rather than strict binary gatekeepers. They accept 70–80% complete solutions during early passes, provided the solution works on happy paths and introduces **zero regressions**.
3. **Iterative Refinement (Passes 2..N):** Subsequent passes progressively expand edge-case coverage, error handling, linter compliance, security hardening, and performance optimization across the entire codebase.

---

## 2. Problem Analysis: Depth-First vs. Breadth-First

| Dimension | Depth-First Verification Loop (DFV) | Breadth-First Generation (BFG) |
| :--- | :--- | :--- |
| **Execution Strategy** | Achieves 100% completion per user story sequentially before starting the next story. | Achieves ~80% completion across *all* user stories first, then deepens all stories together. |
| **Linter & Test Strictness** | Strict enforcement on Pass 1 (rejects on unused imports, whitespace, missing edge case assertions). | Relaxed on Pass 1 (accepts warnings); strictly enforced only on final convergence pass. |
| **Risk of Agent Stalling** | **High:** Agents spend 5–10 retries fixing minor linter nitpicks or edge case mocks. | **Low:** Agents deliver working core logic immediately without blocking on cosmetic or rare edge cases. |
| **System-Wide Visibility** | Delayed until the final task completes. | **Immediate:** Full end-to-end application skeleton works after Pass 1. |
| **Judge Stance** | Rigid gatekeeper (strict binary pass/fail on all standards). | **Benevolent Judge:** Accepts partial functionality; rejects **only** global crashes and regressions. |

---

## 3. Architecture & Iterative Lifecycle

```
                     +-----------------------------------+
                     |      SPEC / User Stories          |
                     +-----------------------------------+
                                       |
                                       v
                      [ Pass 1: Broad Skeleton (70-80%) ]
                      * All User Stories Happy Path
                      * Basic Unit & E2E Acceptance Tests
                                       |
                                       v
                     +-----------------------------------+
                     |   Benevolent Judges Evaluation     |
                     |   - Check Build & Happy Paths     |
                     |   - Enforce ZERO Regressions      |
                     |   - Log Defects to DAG Backlog    |
                     +-----------------------------------+
                                       |
                   +-------------------+-------------------+
                   | (Passes)                              | (Pass N)
                   v                                       v
   [ Pass 2..N-1: Edge Cases & Error Handling ]    [ Pass N: Convergence ]
   * Expand input validations & corner cases       * 100% Linter Clean
   * Add Security & Performance Hardening          * 100% Test Coverage
   * Accumulate Regression Test Suite              * Zero Security Advisories
```

### 3.1. Pass 1: Broad Foundation (70-80% Feature-Wise)
* **Generator Goal:** Implement functional code for all user stories covering primary happy paths.
* **Tester Goal:** Write acceptance tests verifying happy paths. Edge-case test failures are marked as `DEFERRED` rather than blocking pipeline execution.
* **Linter Tolerance:** Style and formatting linter errors (e.g., line lengths, comment formatting, minor cyclomatic complexity warnings) are logged as soft warnings and do not fail the pass.
* **Exit Gate:** Code builds without fatal compiler errors, happy path tests pass, and application binary can execute end-to-end.

### 3.2. Benevolent Judges Evaluation
The evaluation layer (QA, Security, Performance, DevOps agents) evaluates each pass candidate.
* **Benevolent Criteria:**
  * **Acceptance:** Code that works for the standard use cases is accepted even if error handling is basic or edge cases are missing.
  * **Feedback Generation:** Missing edge cases, formatting errors, or performance bottlenecks identified by judges are automatically converted into **Task Refinement Nodes** in the Orchestrator DAG for the next pass.
* **Non-Negotiable Rule — Zero Regressions:**
  * Before evaluating Pass $K$, the test runner executes the accumulated baseline test suite from Pass $K-1$.
  * If any previously passing test fails or a previously working feature breaks, the judge **rejects** the commit immediately with a `REGRESSION_DETECTED` signal.

### 3.3. Iterative Refinement Passes (Pass 2 .. N-1)
* Each subsequent iteration picks up the **Task Refinement Nodes** generated by the Benevolent Judges.
* **Pass 2 (Edge Cases & Boundaries):** Implements invalid input handling, boundary conditions, empty file handling, division by zero, null pointer checks, and error wrapping.
* **Pass 3 (Security, Performance & Robustness):** Adds resource cleanup, SAST scan compliance, concurrency safety, and performance optimizations.

### 3.4. Pass N: Final Convergence
* Strict enforcement of all project standards:
  * 100% `go fmt` and `golangci-lint` clean pass.
  * 100% unit test coverage for target logic.
  * Zero high/medium security vulnerabilities.

---

## 4. Orchestrator State & Data Model Integration

To support BFG without violating the **Stateless Agent / Stateful Orchestrator** principle ([AGENTS.md](file:///Users/diegoj/repos/noctifab/AGENTS.md)), the state schema in `pkg/domain/state.go` is augmented with iteration pass metadata:

```go
// IterationPass denotes the current Breadth-First Generation cycle
type IterationPass int

const (
    PassSkeleton     IterationPass = 1 // 70-80% Happy Path Broad Coverage
    PassEdgeCases    IterationPass = 2 // Error handling, boundary conditions
    PassHardening    IterationPass = 3 // Security, performance, robustness
    PassConvergence  IterationPass = 4 // Strict linter, 100% coverage, production polish
)

// StoryProgress tracks feature completeness and deferrals per story
type StoryProgress struct {
    StoryID           string        `json:"story_id"`
    CurrentPass       IterationPass `json:"current_pass"`
    CompletenessScore float64       `json:"completeness_score"` // e.g. 0.80 for 80%
    DeferredDefects   []DefectNode  `json:"deferred_defects"`
    PassedScenarios   []string      `json:"passed_scenarios"`
}
```

During execution, `pkg/services/orchestrator.go` passes the `CurrentPass` in the prompt context to Generator and Tester agents, instructing them on the required strictness level and allowing them to focus strictly on the current iteration's goals.

---

## 5. Concrete Examples

### Example 1: Unix Word Count (`wc`) Project

Consider building a `wc` clone in Go with requirements for counting lines (`-l`), words (`-w`), bytes (`-c`), characters (`-m`), handling multiple files, reading from `stdin`, and returning proper exit codes.

#### Iteration 1: Broad Foundation (80% Feature-Wise)
* **User Stories Targeted:**
  1. Read from `stdin` and print line, word, and byte counts.
  2. Read from single file path argument.
* **Generator Implementation:**
  * Implements `main.go` using standard `bufio.Scanner`.
  * Computes `-l`, `-w`, `-c` in a single pass.
  * *Omitted/Deferred in Pass 1:* `-m` (multibyte character count), multiple file aggregates (`total`), custom error messages for unreadable files, strict linter comment formatting.
* **Tester Implementation:**
  * `wc_test.go` verifies `echo "hello world" | wc` yields `1 2 12`.
* **Benevolent Judge Decision:**
  * **Result:** `ACCEPTED (Pass 1)`.
  * **Judge Notes:** "Happy path stdin and single file work cleanly. No global crashes. Deferred items logged: multi-file support, `-m` flag, file non-existence error exit codes."

#### Iteration 2: Edge Cases & Multi-File Support
* **Refinements Applied:**
  * Generator adds multi-file argument loop and aggregates output into a `total` line.
  * Adds error handling for missing files (`wc: non_existent.txt: No such file or directory`) with exit code `1`.
* **Tester Implementation:**
  * Adds unit tests for missing files and multiple file arguments.
  * **Regression Check:** Reruns Iteration 1 `stdin` test suite. All pass.
* **Benevolent Judge Decision:**
  * **Result:** `ACCEPTED (Pass 2)`.
  * **Judge Notes:** "Multi-file and error handling functional. Deferred items: multibyte support (`-m`), linter comment check."

#### Iteration 3: Final Convergence & Polishing
* **Refinements Applied:**
  * Implements `-m` character count using `utf8.RuneCountInString`.
  * Formats code with `go fmt ./...` and resolves all `golangci-lint` warnings.
  * Refactors functions to maintain <500 lines per file ([AGENTS.md](file:///Users/diegoj/repos/noctifab/AGENTS.md)).
* **Benevolent Judge Decision:**
  * **Result:** `CONVERGED (Pass 3 / Production Ready)`.

---

### Example 2: Expression Calculator Project

Consider building a CLI expression calculator supporting `+`, `-`, `*`, `/`, parentheses, decimal precision, and error handling for division by zero and invalid syntax.

#### Iteration 1: Broad Foundation (80% Feature-Wise)
* **User Stories Targeted:**
  1. Evaluate simple binary arithmetic expressions (`2 + 3`, `10 * 4`, `8 / 2`).
* **Generator Implementation:**
  * Basic scanner + stack-based evaluator for left-to-right operations.
  * *Omitted/Deferred in Pass 1:* Parenthesis nesting (`(2+3)*4`), operator precedence order (`*` before `+`), division by zero error handling (returns `Inf` or panics), floating point precision formatting.
* **Tester Implementation:**
  * Tests `2 + 3` -> `5`, `10 - 4` -> `6`.
* **Benevolent Judge Decision:**
  * **Result:** `ACCEPTED (Pass 1)`.
  * **Judge Notes:** "Basic binary operations work. Deferred items: operator precedence, parentheses, division by zero check."

#### Iteration 2: Precedence, Parentheses & Error Boundary
* **Refinements Applied:**
  * Implements Dijkstra's Shunting-yard algorithm for operator precedence (`*`, `/` over `+`, `-`) and nested parentheses `(...)`.
  * Catches division by zero and returns user-friendly error `Error: division by zero`.
* **Regression Check:**
  * All baseline tests from Iteration 1 run and pass cleanly.
* **Benevolent Judge Decision:**
  * **Result:** `ACCEPTED (Pass 2)`.

#### Iteration 3: Final Convergence & Robustness
* **Refinements Applied:**
  * Floating-point precision formatting (e.g. trimming trailing zeros).
  * Comprehensive input validation (unmatched parentheses, unknown characters).
  * Full linter compliance and unit test coverage.
* **Benevolent Judge Decision:**
  * **Result:** `CONVERGED (Pass 3 / Production Ready)`.

---

## 6. Summary of Benefits

1. **Eliminates Stalls:** Generator and Tester agents complete Pass 1 without getting trapped in repetitive retry loops over minor formatting or non-essential edge cases.
2. **Fast Time-to-First-Working-Prototype:** A functional, end-to-end application skeleton is available in ~20% of the total execution time.
3. **Targeted Refinement:** Benevolent judges convert real observed gaps into explicit task nodes, directing LLM budget toward functional weaknesses rather than hypothetical edge cases.
4. **Guaranteed Stability:** The non-negotiable **Zero Regressions** rule ensures that iterative refinement never degrades previously validated features.
