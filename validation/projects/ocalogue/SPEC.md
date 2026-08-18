# `ocalogue` Technical Specification & Architecture

`ocalogue` is a high-performance, purely functional **Datalog Deductive Logic and Relational Query Engine** built in **OCaml 5.x** using the **Dune** build system.

Named as a portmanteau of **OCaml** and **Datalog** (*dialogue in logic*), `ocalogue` implements a complete first-order relational logic engine featuring **Semi-Naive Bottom-Up Fixpoint Evaluation**, **Stratified Negation**, **Relational Indexing**, and an **Interactive REPL & Batch CLI**, verified against the **Official Datalog Test Suite**.

---

## 1. Core Mathematical Invariants & Logic Rules

> [!IMPORTANT]
> **RULE 1: DECLARATIVE SEMANTICS & LEAST FIXED POINT (LFP)**
> Datalog execution computes the unique **Minimal Herbrand Model** (Least Fixed Point) of the input rules and ground facts.
> The order in which facts and rules are declared in source files MUST NOT alter the final set of derived tuples.

> [!IMPORTANT]
> **RULE 2: SEMI-NAIVE FIXPOINT EVALUATION (DIFFERENTIAL JOINS)**
> Naive bottom-up evaluation repeatedly joins already-known facts, leading to factorial time complexity.
> The evaluation engine MUST implement **Semi-Naive Evaluation**: in each iteration step $k$, at least one body atom in a recursive rule MUST be bound to a *newly derived differential tuple* ($\Delta R_{k-1}$), preventing redundant join computations.

> [!IMPORTANT]
> **RULE 3: STRATIFIED NEGATION & SAFETY**
> Negation (`not atom(...)`) is permitted only if the program is **stratifiable**:
> - Construct a Predicate Dependency Graph $G = (V, E)$ where edges $p \to q$ represent that rule head $p$ depends on body atom $q$.
> - Directed edges through `not` are marked as **negative edges**.
> - The program is valid if and only if there are **NO directed cycles containing a negative edge**.
> - The engine partitions rules into topological strata $S_0, S_1, \dots, S_n$, evaluating stratum $S_i$ to a complete fixpoint before computing stratum $S_{i+1}$.
> - Any program with a negative cycle MUST be rejected with error: `NON_STRATIFIED_NEGATION`.

> [!IMPORTANT]
> **RULE 4: TERM SAFETY & FINITE TERMINATION GUARANTEE**
> Every variable appearing in the head of a rule or in a negated/comparison body literal MUST also appear in at least one positive relational body atom (Safety Condition).
> This guarantees that all derived models are strictly finite and all queries terminate in polynomial time.

> [!IMPORTANT]
> **RULE 5: IDIOMATIC OCAML & PURELY FUNCTIONAL ARCHITECTURE**
> The core AST, Unification, Semi-Naive loop, and Stratification modules MUST be implemented using immutable data structures (persistent `Set`, `Map`, Algebraic Data Types, Tail Recursion, and Functors) without imperative side-effects in core evaluation algorithms.

---

## 2. Datalog Grammar & Lexical Specification

### 2.1 Lexical Tokens
* **Variables**: Identifiers starting with an uppercase letter or underscore (e.g. `X`, `Target`, `_`).
* **Atoms / Symbols**: Identifiers starting with a lowercase letter (e.g. `edge`, `path`, `alice`, `true`).
* **Strings**: Double-quoted strings (e.g. `"Engineering"`, `"Homer"`).
* **Integers**: Signed 64-bit integer literals (e.g. `0`, `42`, `-10`).
* **Operators**:
  * `:-` (Rule implication / "if")
  * `?-` (Query prefix)
  * `not` (Stratified negation)
  * `,` (Logical conjunction / AND)
  * `.` (Clause terminator)
  * Comparison operators: `=`, `!=`, `<`, `<=`, `>`, `>=`
* **Comments**: Single-line `%` or `//`, or multi-line `/* ... */`.

### 2.2 EBNF Syntax Grammar
```ebnf
program         ::= statement* EOF ;
statement       ::= fact | rule | query ;

fact            ::= atom "." ;
rule            ::= atom ":-" literal ("," literal)* "." ;
query           ::= "?-" literal ("," literal)* "." ;

literal         ::= ["not"] atom | comparison ;
comparison      ::= term comp_op term ;
comp_op         ::= "=" | "!=" | "<" | "<=" | ">" | ">=" ;

atom            ::= PREDICATE "(" [ term ("," term)* ] ")" ;
term            ::= VARIABLE | CONSTANT | WILDCARD ;
CONSTANT        ::= SYMBOL | STRING | INTEGER ;
```

---

## 3. Package & Directory Layout (OCaml / Dune)

```
ocalogue/
├── dune-project
├── Makefile
├── Dockerfile
├── test_suite/                                 # Official Datalog Test Fixtures
│   ├── 01_transitive_closure.dl
│   ├── 02_same_generation.dl
│   ├── 03_stratified_negation.dl
│   ├── 04_mutual_recursion.dl
│   ├── 05_cyclic_termination.dl
│   ├── 06_unification_filters.dl
│   └── 07_unstratified_negation_error.dl
├── bin/
│   ├── dune
│   └── main.ml                                 # CLI & Interactive REPL entrypoint
├── lib/
│   ├── dune
│   ├── ast.ml                                  # AST type definitions & pretty printers
│   ├── lexer.mll (or hand-written lexer)       # OCamllex tokenizer
│   ├── parser.mly (or Menhir / recursive descent)# Grammar parser
│   ├── types.ml                                # Symbol, Term, Tuple, Relation types
│   ├── relation.ml                             # Indexed relational store (Set/Map functors)
│   ├── unification.ml                          # Pattern matching & substitution bindings
│   ├── stratify.ml                             # Dependency graph cycle detection & strata
│   ├── semi_naive.ml                           # Semi-naive fixpoint evaluation engine
│   └── engine.ml                               # Top-level Datalog solver & Query runner
└── test/
    ├── dune
    ├── test_ast.ml
    ├── test_unification.ml
    ├── test_stratification.ml
    ├── test_semi_naive.ml
    └── test_official_suite.ml                  # Runs all test_suite/*.dl test cases
```

---

## 4. Domain Data Types (AST & Engine)

```ocaml
(* lib/ast.ml *)
type symbol = string

type term =
  | Var of string
  | ConstString of string
  | ConstInt of int
  | ConstSymbol of symbol
  | Wildcard

type atom = {
  predicate: symbol;
  args: term list;
}

type comp_op = Eq | Neq | Lt | Le | Gt | Ge

type literal =
  | PosAtom of atom
  | NegAtom of atom
  | Comparison of term * comp_op * term

type clause =
  | Fact of atom
  | Rule of atom * literal list
  | Query of literal list

type program = clause list
```

---

## 5. Semi-Naive Evaluation Algorithm

### 5.1 Relational Store & Differential Representation
For each intensional database (IDB) predicate $P$, maintain three sets:
1. $R[P]$: Full accumulated relation set.
2. $\Delta R[P]$: Tuples newly discovered in iteration $k-1$.
3. $\Delta R_{\text{next}}[P]$: Tuples discovered in current iteration $k$.

### 5.2 Semi-Naive Step Loop
```
For each stratum S_i from 0 to N:
  Initialize R[P] = EDB facts for all P in S_i
  Initialize ΔR[P] = R[P]
  
  Repeat until all ΔR[P] are empty:
    Initialize all ΔR_next[P] = empty
    
    For each rule: Head :- Body_1, Body_2, ..., Body_m in S_i:
      For each j from 1 to m:
        // Evaluate rule using ΔR for Body_j and R for other positive body atoms:
        Join: Body_1(R) ⋈ ... ⋈ Body_j(ΔR) ⋈ ... ⋈ Body_m(R) ⋈ NegatedAtoms(R_prev_strata)
        For each derived tuple t:
          If t ∉ R[Head] and t ∉ ΔR_next[Head]:
            Add t to ΔR_next[Head]
            
    For each predicate P in S_i:
      R[P] = R[P] ∪ ΔR_next[P]
      ΔR[P] = ΔR_next[P]
```

---

## 6. Command-Line Interface (CLI) & REPL

### 6.1 Batch Execution (`ocalogue run`)
```bash
# Run a Datalog file and evaluate queries contained within
ocalogue run test_suite/01_transitive_closure.dl

# Run a file with an ad-hoc query from command line
ocalogue run test_suite/01_transitive_closure.dl -q "path(a, Target)"

# Output JSON structured results
ocalogue run test_suite/01_transitive_closure.dl -q "path(a, Target)" --format=json
```

**Standard Text Output Format:**
```
path(a, b)
path(a, c)
path(a, d)
path(a, e)
```

**JSON Output Format:**
```json
{
  "query": "path(a, Target)",
  "bindings": [
    {"Target": "b"},
    {"Target": "c"},
    {"Target": "d"},
    {"Target": "e"}
  ],
  "count": 4
}
```

### 6.2 Official Test Suite Runner (`ocalogue test`)
```bash
# Run the official test set
ocalogue test test_suite/
```
Output:
```
[PASS] 01_transitive_closure.dl (4 derived tuples)
[PASS] 02_same_generation.dl (3 derived tuples)
[PASS] 03_stratified_negation.dl (1 derived tuple)
[PASS] 04_mutual_recursion.dl (3 derived tuples)
[PASS] 05_cyclic_termination.dl (3 derived tuples)
[PASS] 06_unification_filters.dl (2 derived tuples)
[PASS] 07_unstratified_negation_error.dl (Properly rejected with NON_STRATIFIED_NEGATION)

All 7 official Datalog test cases passed (100% success rate).
```

### 6.3 Interactive REPL (`ocalogue repl`)
```
ocalogue Datalog REPL v1.0.0
Type .help for commands, or enter facts, rules, and queries.

ocalogue> .load test_suite/01_transitive_closure.dl
Loaded 4 facts, 2 rules.

ocalogue> ?- path(a, X).
+--------+
| X      |
+--------+
| b      |
| c      |
| d      |
| e      |
+--------+
4 tuples found.

ocalogue> edge(e, f).
Fact asserted: edge(e, f).

ocalogue> ?- path(a, X).
+--------+
| X      |
+--------+
| b      |
| c      |
| d      |
| e      |
| f      |
+--------+
5 tuples found.

ocalogue> .quit
```

---

## 7. Official Validation Test Set Details

The engine MUST correctly evaluate all 7 official test scenarios provided in `test_suite/`:

1. **`01_transitive_closure.dl`**: Proves reachability over linear graph $a \to b \to c \to d \to e$. Expected: `path(a, b)`, `path(a, c)`, `path(a, d)`, `path(a, e)`.
2. **`02_same_generation.dl`**: Proves recursive cousin resolution over family tree. Expected: `same_generation(ann, ann)`, `same_generation(ann, bill)`, `same_generation(ann, pat)`.
3. **`03_stratified_negation.dl`**: Proves complementary set computation using `not reachable(X, Y)`. Expected: `unreachable(1, 4)`.
4. **`04_mutual_recursion.dl`**: Proves even/odd mutual recursive derivation across successor chains. Expected: `even(0)`, `even(2)`, `even(4)`.
5. **`05_cyclic_termination.dl`**: Proves termination without infinite loops on cyclic graphs ($x \to y \to z \to x$). Expected: `connected(x, x)`, `connected(x, y)`, `connected(x, z)`.
6. **`06_unification_filters.dl`**: Proves relational joins and arithmetic comparisons (`Salary >= 100000`). Expected: `high_earner_dept("Alice", "Engineering")`, `high_earner_dept("Dave", "Engineering")`.
7. **`07_unstratified_negation_error.dl`**: Proves rejection of unstratifiable loops (`p(X) :- q(X), not p(X)`). Expected: Exit code 1 / Stratification error.

---

## 8. Makefile & Build Discipline

```makefile
.PHONY: build test lint format clean

build:
	dune build

test:
	dune runtest

lint:
	dune build @check 2>/dev/null || dune build

format:
	dune fmt 2>/dev/null || true

clean:
	dune clean
```
