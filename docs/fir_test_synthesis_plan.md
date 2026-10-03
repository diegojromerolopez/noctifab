# Formal Intermediate Representation (FIR) & Test-Locked Synthesis Plan

## 1. Executive Summary

This document specifies the architecture and implementation plan for a **Formal Intermediate Representation (FIR)** coupled with a **Test-Locked Synthesis Barrier** in `noctifab`.

The objective is to eliminate hallucination, prevent shortcuts, and enforce strict goal convergence when autonomous LLM agents synthesize software from natural language specifications.

### The Problem
In standard autonomous coding pipelines, relying directly on natural language across multi-agent handoffs introduces three persistent failure modes:
1. **Semantic Drift & Shortcuts:** Agents infer missing constraints, cut corners, over-mock dependencies, or take "easy" paths that violate core invariants.
2. **Context Window Inefficiency ($N \times S$):** Ingesting the complete `SPEC.md` ($>12,000$ tokens) on every single task prompt inflates latency and token costs while increasing cross-domain hallucinations.
3. **Sycophantic / Self-Confirming Tests:** When an LLM generates both code and tests, it tests only what it built; if it misunderstands an edge case, it generates a flawed test that falsely passes.

### The Solution: The Semantic Straitjacket
The FIR acts as a **semantic compiler target** and an **independent test oracle**:
* `SPEC.ste.md` compiles into a machine-readable semantic graph (FIR).
* Tasks receive only the **minimal semantic closure (slice)** required to implement them ($\sim 250$ tokens).
* An **asymmetric dual-agent barrier** generates hardened, immutable property and BDD tests *before* code synthesis starts.
* The code synthesizer operates under a **Closed-World Assumption** within a read-only test sandbox, governed by a **Red-Green-Mutation Gatekeeper**.

---

## 2. End-to-End System Pipeline

```text
                     +---------------------------------------+
                     |              SPEC.md                  |
                     +---------------------------------------+
                                         |
                                         v
                     +---------------------------------------+
                     |    Simplified Technical English       |
                     |           (SPEC.ste.md)               |
                     +---------------------------------------+
                                         |
                                         v
                     +---------------------------------------+
                     |         Specification Compiler        |
                     +---------------------------------------+
                                         |
                                         v
                     +---------------------------------------+
                     |       Formal IR (Semantic Oracle)     |
                     +---------------------------------------+
                                         |
                        +----------------+----------------+
                        |                                 |
                        v                                 v
          +---------------------------+     +---------------------------+
          |   Task Context Slicer     |     |   Formal Test Synthesizer |
          +---------------------------+     +---------------------------+
                        |                                 |
                        | (Contract Slice)                | (Generates Immutable
                        |                                 |  BDD & Property Tests)
                        v                                 v
          +---------------------------+     +===========================+
          |    Code Synthesizer       |     |   IMMUTABLE TEST BARRIER  |
          |  (Implementation Agent)   |     |    (Read-Only Worktree)   |
          +---------------------------+     +===========================+
                        |                                 |
                        +----------------+----------------+
                                         |
                                         v
                        +---------------------------------+
                        |  Red-Green-Mutation Gatekeeper  |
                        +---------------------------------+
                                         |
                         [Pass] ---------+-------- [Fail]
                           |                         |
                           v                         v
                       Ship Task            Structured Counterexample
                                            (CEGIS Repair Loop)
```

---

## 3. Formal Intermediate Representation (FIR) Ontology

### 3.1 Serialization & Data Format: Why YAML
The FIR uses **YAML** as its canonical storage and prompt injection format:
* **Token Efficiency:** YAML consumes **$25\%\text{--}35\%$ fewer tokens** than equivalent JSON because it eliminates surrounding quotes, trailing commas, and closing braces.
* **Human Readability & Git Diffs:** Clean indentation makes `.noctifab/fir.yaml` easy for engineers to inspect, review, and diff in version control.
* **Syntax-Guaranteed Compilation Pipeline:** To prevent indentation errors during LLM generation, the Specification Compiler requests the LLM to output JSON constrained strictly by a formal **JSON Schema** (`response_schema`). Noctifab immediately parses and persists this AST as canonical **YAML**.
* **Zero Custom Parser Overhead:** Go services in Noctifab use standard struct tagging (`yaml:"..." json:"..."`) via `gopkg.in/yaml.v3`.

### 3.2 Core Semantic Primitives
* **`TYPE` / `ENUM`:** Data types and enumerations.
* **`ENTITY`:** Persistent or domain-relevant state containers.
* **`RELATION`:** Semantic and structural associations between entities.
* **`OPERATION`:** The central behavioral unit (inputs, preconditions, postconditions, invariants, errors).
* **`PRE`:** Conditions that must hold prior to operation dispatch.
* **`POST`:** Assertions guaranteed over mutated state ($x'$ denotes post-state).
* **`INVARIANT`:** Global and entity-level rules that must never be violated in any valid state.
* **`FRAME CONDITION`:** Explicit rules declaring that unmodified state remains untouched (`forall x != target: stock'[x] == stock[x]`).
* **`ERROR`:** Explicit failure classification with atomic rollback guarantees (`state: unchanged`).
* **`EFFECT`:** Externally observable side-effects (e.g., event emissions, persistent transactions).
* **`AMBIGUITY`:** Explicitly captured unresolvable or contradictory specifications (`AMB-*`).
* **`SOURCE`:** Exact bidirectional traceability back to line numbers and sections in `SPEC.ste.md`.

### 3.3 Canonical Representation Example

```yaml
types:
  - id: TYPE-ORDER-STATUS
    kind: enum
    name: OrderStatus
    values: [pending, confirmed, cancelled]

entities:
  - id: ENTITY-INVENTORY
    name: Inventory
    fields:
      - name: stock
        type: Map<Item, Nat>

invariants:
  - id: INV-INVENTORY-001
    scope: Inventory
    expr: "forall item: stock[item] >= 0"

operations:
  - id: OP-INVENTORY-RESERVE
    name: Inventory.reserve
    inputs:
      - name: item
        type: Item
      - name: quantity
        type: Nat
    pre:
      - id: PRE-RESERVE-001
        expr: "quantity > 0"
      - id: PRE-RESERVE-002
        expr: "stock[item] >= quantity"
    post:
      - id: POST-RESERVE-001
        expr: "stock'[item] == stock[item] - quantity"
      - id: POST-RESERVE-002 # Frame condition
        expr: "forall x != item: stock'[x] == stock[x]"
    errors:
      - id: ERR-RESERVE-001
        condition: "stock[item] < quantity"
        error: InsufficientStock
        state: unchanged
```

---

## 4. Task-Specific Semantic Slicing

Instead of repeatedly injecting the entire specification into every task prompt ($N \times S$), the orchestrator computes the **transitive semantic closure** for each task target:

$$\text{Operation} \longrightarrow \text{Referenced Entities} \longrightarrow \text{Applicable Invariants} \longrightarrow \text{Pre/Post/Errors} \longrightarrow \text{Types}$$

### Prompt-Rendered YAML Task Slice
The slice is serialized directly to clean YAML and injected into the task prompt:

```yaml
task_slice:
  entity:
    name: Inventory
    fields:
      stock: Map<Item, Nat>

  invariants:
    - id: INV-INVENTORY-001
      expr: "forall item: stock[item] >= 0"

  operation:
    name: Inventory.reserve
    inputs:
      item: Item
      quantity: Nat
    pre:
      - id: PRE-RESERVE-001
        expr: "quantity > 0"
      - id: PRE-RESERVE-002
        expr: "stock[item] >= quantity"
    post:
      - id: POST-RESERVE-001
        expr: "stock'[item] == stock[item] - quantity"
      - id: POST-RESERVE-002
        expr: "forall x != item: stock'[x] == stock[x]"
    errors:
      - id: ERR-RESERVE-001
        condition: "stock[item] < quantity"
        error: InsufficientStock
        state: unchanged
```

**Result:** A prompt payload reduction from $>12,000$ tokens to $\sim 180\text{--}250$ tokens per task, eliminating unrelated domain noise (e.g. auth, billing, UI) from the model's active attention window while retaining complete mathematical rigor.

---

## 5. The Test-Locked Semantic Straitjacket

To ensure the LLM feels constrained and cannot take easy detours, the testing subsystem enforces four strict structural boundaries:

### 5.1 The 5 Automated Test Dimensions
From every FIR operation node, the Test Synthesizer mechanically constructs tests across 5 dimensions:

1. **Precondition Boundary Tests (`PRE`):**
   - Asserts input bounds (e.g. `quantity = 0`, `quantity = -1`) are rejected deterministically before execution.
2. **State Mutation Assertions (`POST`):**
   - Asserts exact post-state values against mathematical specifications: $S' = S - \Delta$.
3. **Frame Axiom Tests (Anti-Side-Effect Guards):**
   - Seeds adjacent data (items $B, C, D$) and verifies they remain byte-for-byte unmodified when item $A$ is processed.
4. **Negative Error Atomicity (`state: unchanged`):**
   - Triggers the error condition and asserts that the state snapshot after the failure is identical to the snapshot before the failure.
5. **Continuous Property-Based Fuzzing (`INV`):**
   - Employs property-based frameworks (e.g., `pgregory.net/rapid` in Go, `hypothesis` in Python) to execute randomized valid call sequences while continuously asserting that system invariants hold true at every step.

### 5.2 Decoupled Dual-Agent Barrier & Immutable Worktrees
To eliminate test-tampering and LLM self-confirmation:
* **Agent Separation:** The Test Synthesizer runs in isolation, taking only the FIR slice and outputting test code (`*_test.go`).
* **Filesystem Lock:** When the Code Synthesizer is invoked, the test files in the git worktree are marked **read-only** (`chmod 444`).
* **Zero Modification Policy:** Any attempt by the Code Synthesizer to modify, delete, or comment out test assertions causes immediate task failure and triggers a security violation alarm.

### 5.3 Anti-Cheat Red-Green-Mutation Gatekeeper
To prevent vacuous tests (tests that pass because they assert nothing or use dummy assertions):
1. **Red Gate (Fail on Stub):** Before code synthesis begins, tests run against empty stubs. If the test suite passes, the tests are rejected as invalid.
2. **Green Gate (Pass on Implementation):** The code synthesizer writes logic until all unit and property tests exit cleanly (`exit 0`, `failed == 0`, `total_tests > 0`).
3. **Mutation Verification (Anti-Spoofing):** Noctifab injects a synthetic mutant into the code (e.g., inverting a conditional or replacing `-` with `+`). The tests **must fail**. If the tests pass on mutated code, the test suite is flagged as weak and rejected.

### 5.4 Counterexample-Guided Inductive Synthesis (CEGIS)
When tests fail, conversational diagnosis is prohibited. The repair agent receives a structured, machine-generated counterexample:

```yaml
REPAIR_DIAGNOSTIC:
  rule_violated: INV-INVENTORY-001
  invariant: "forall item: stock[item] >= 0"
  failing_test: TestInventory_Reserve_Property
  counterexample:
    initial_state: { stock: { "item-1": 3 } }
    operation: "Inventory.Reserve(item='item-1', quantity=5)"
    expected: "error: InsufficientStock, stock: {'item-1': 3}"
    actual: "error: nil, stock: {'item-1': -2}"
  directive: >
    Guard PRE-RESERVE-002 was bypassed. Modify line 34 in `src/domain/inventory.go`
    to return `InsufficientStock` when requested quantity exceeds available stock.
```

---

## 6. Code Generation & Operational Constraints

When the Code Synthesizer runs, the prompt enforces a **Closed-World Assumption**:
1. **No Speculative Architecture:** The agent is prohibited from introducing design patterns, external caching layers, or helper libraries not mandated by the FIR.
2. **Zero-Mock Domain Logic:** Domain entities and state transitions must be real, hermetic, in-memory implementations. Mocking domain business logic is strictly prohibited.
3. **Explicit Error Signatures:** Returned errors must match the exact FIR error identifiers and project prefix conventions.

---

## 7. Phased Implementation Roadmap

### Phase 1: FIR Schema & YAML Engine
* Implement FIR Go data structures with dual `yaml:"..."` and `json:"..."` struct tags in `pkg/domain/fir/`.
* Define the JSON Schema for constrained LLM compilation and YAML serializer for canonical persistence (`.noctifab/fir.yaml`).
* Build the YAML semantic closure slicer (`pkg/services/fir_slicer.go`) for low-token prompt generation.

### Phase 2: Specification Compiler (`SPEC.ste.md` $\to$ FIR)
* Build the multi-pass extractor in `pkg/services/fir_compiler.go`.
* Implement ambiguity detection (`AMB-*`) and bidirectional source traceability back to line numbers.

### Phase 3: Formal Test Synthesizer
* Implement the test generator translating FIR operations into BDD acceptance tests and property fuzz tests (`rapid`).
* Generate explicit Frame Axiom assertions and error atomicity checks.

### Phase 4: Immutable Worktree & Execution Sandbox
* Update the dark factory worker engine to mount generated tests as read-only.
* Enforce git change restrictions preventing modification of test paths by coding agents.

### Phase 5: Red-Green-Mutation Gatekeeper & CEGIS Loop
* Implement the 3-step test verification gate (Red stub check $\to$ Green execution $\to$ Mutation validation).
* Build the counterexample parser converting test failure telemetry into structured `REPAIR_DIAGNOSTIC` payloads.

---

## 8. Verification Metrics

| Metric | Target | Measurement Method |
| :--- | :--- | :--- |
| **Token Reduction** | $\ge 65\%$ decrease in task prompt tokens | Pre/post comparison against full `SPEC.md` prompting |
| **First-Pass Pass Rate** | $\ge 40\%$ relative increase | Ratio of tasks passing on first Green gate run |
| **Repair Convergence** | $\le 2$ repair iterations per failing task | Count of CEGIS loops before clean pass |
| **Mutation Detection** | $100\%$ detection of injected mutants | Gatekeeper automated mutation check pass rate |
| **Semantic Invariant Leaks** | $0$ undetected invariant violations | Automated property-based fuzz test executions |
