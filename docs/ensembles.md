# Multi-Model Ensembles (Mixture of Models)

`noctifab` provides a high-performance, resilient **Mixture of Models (MoM / MoA)** ensembling engine. It allows individual agent roles (such as Product Manager, Generator, Tester, Auditor, and Unblocker) to cooperate across complementary LLM providers (e.g. Anthropic Claude, OpenAI GPT-4o, Google Gemini, DeepSeek, Cerebras LLaMA).

Multi-model ensembling introduces **"Shift-Left" Quality** to autonomous Dark Factory harnesses: by synthesizing divergent perspectives or validating AST syntax on Turn 1, it eliminates 80–90% of costly downstream repair loops (Refactor turns, Unblocker interventions, and Watchdog retries).

---

## 1. Topologies Overview

```
TOPOLOGY 1: Parallel & Posterior Mix with Speculative Quorum (parallel)
┌──────────────┐      ┌─────────────────────────┐
│              ├─────►│ Model 1 (name: claude)  ├──┐
│              │      └─────────────────────────┘  │
│              │      ┌─────────────────────────┐  │ (Quorum Reached:     ┌──────────────────────────┐      ┌─────────────────┐
│ Agent Prompt ├─────►│ Model 2 (name: openai)  ├──┼──► Fastest K of N ) ─►│ Primary Synthesizer      ├─────►│ Consolidated    │
│              │      └─────────────────────────┘  │  (Bypasses Stragglers)│ (name: gemini)           │      │ Tool Actions    │
│              │      ┌─────────────────────────┐  │                       └──────────────────────────┘      └─────────────────┘
│              ├─────►│ Model 3 (name: deepseek)├──┘
└──────────────┘      └─────────────────────────┘

TOPOLOGY 2: Serial Refinement with Deterministic Early Exit (serial)
┌──────────────┐      ┌─────────────────────────┐      ┌────────────────────┐ ──[Valid AST / Zero Stubs]──► Instant Output (Exit)
│ Agent Prompt ├─────►│ Stage 1: Initial Draft  ├─────►│ Local Syntax & AST │
│              │      │ (name: openai)          │      │ Pre-Flight Check   │ ──[Invalid / Missing Items]──┐
└──────────────┘      └─────────────────────────┘      └────────────────────┘                              │
                                                                                                           ▼
                                                       ┌─────────────────────────┐      ┌─────────────────────────┐
                                                       │ Stage 2: Critique/Refine├─────►│ Stage 3: Final Polish   ├─────► Refined Output
                                                       │ (name: claude)          │      │ (name: gemini)          │
                                                       └─────────────────────────┘      └─────────────────────────┘

TOPOLOGY 3: Dual-Perspective Consensus (consensus)
┌──────────────┐      ┌─────────────────────────┐
│              ├─────►│ Voter 1: (name: claude) ├──┐
│ Audit Prompt │      └─────────────────────────┘  ├──► Unanimous Pass ──► Instant Approval (1 Hop)
│              │      ┌─────────────────────────┐  │    Discrepancy    ──► Single Tie-Breaker (name: gemini)
│              ├─────►│ Voter 2: (name: openai) ├──┘
└──────────────┘      └─────────────────────────┘

TOPOLOGY 4: Speculative First-Valid Race (race)
┌──────────────┐      ┌───────────────────────────────┐
│              ├─────►│ Model 1 (name: cerebras)      ├──┐
│ Agent Prompt ├─────►│ Model 2 (name: gemini-flash)  ├──┼──► [First Valid AST Response] ──► Cancel Others ──► Instant Return (1-3s)
│              ├─────►│ Model 3 (name: haiku)         ├──┘
└──────────────┘      └───────────────────────────────┘

TOPOLOGY 5: Divide-and-Conquer Multi-File Generation (decomposed)
┌──────────────┐      ┌────────────────────────────────────────────────────────┐
│              ├─────►│ Specialist 1 (name: claude)   ──► domain/models.go     ├──┐
│ Story Spec   ├─────►│ Specialist 2 (name: openai)   ──► services/service.go  ├──┼──► Deterministic File Merger ──► Full Slice Output
│              ├─────►│ Specialist 3 (name: deepseek) ──► services/test.go     ├──┘
└──────────────┘      └────────────────────────────────────────────────────────┘

TOPOLOGY 6: Tiered Fast-Path Escalation (cascade)
┌──────────────┐      ┌────────────────────────┐ ──[Valid AST & Zero Stubs]──► Instant Output (1-2s)
│ Agent Prompt ├─────►│ Tier 1: Fast/Cheap LLM │
│              │      │ (name: gemini-flash)   │ ──[Fails/Stubs]──┐
└──────────────┘      └────────────────────────┘                  ▼
                                                       ┌────────────────────────┐
                                                       │ Tier 2: Frontier LLM   ├─────► High-Reasoning Output
                                                       │ (name: claude-sonnet)  │
                                                       └────────────────────────┘

TOPOLOGY 7: Deterministic Scored Selection (best_of_n_scored)
┌──────────────┐      ┌─────────────────────────┐
│              ├─────►│ Candidate 1 (claude)    ├──┐
│ Agent Prompt ├─────►│ Candidate 2 (openai)    ├──┼──► Local Engine Evaluator (AST + Anti-Stub + Line Bound) ──► Highest Score Promoted
│              ├─────►│ Candidate 3 (deepseek)  ├──┘    (Zero LLM Synthesis Cost)
└──────────────┘      └─────────────────────────┘

TOPOLOGY 8: Adaptive Self-Tuning Dynamic Routing (adaptive)
┌──────────────┐      ┌───────────────────────────────────┐
│              ├─────►│ Fast Tier (Docs/Typos)            ├──► Race / Fast-Path (1–3s)
│ Agent Prompt ├─────►│ Standard Tier (Features/Tests)    ├──► Frontier Model / Scored
│              ├─────►│ Heavy Tier (Concurrency/Assembly) ├──► Speculative Quorum / Serial Refinement
└──────────────┘      └───────────────────────────────────┘
```

---

## 2. Supported Ensembling Topologies

### 1. `parallel`: Parallel & Posterior Mix with Speculative Quorum
- **How it works:** Dispatches prompt to $N$ candidate models in parallel. Once a speculative quorum ($K \ge \text{min\_models}$) responds or the `soft_timeout_seconds` deadline fires, slower stragglers are cancelled and the completed proposals are fed into the `synthesizer` model to produce a unified, consolidated output.
- **Best for:** **Product Manager / Roadmap Generation** (discovering edge cases and generating complete Definition of Done) and **Unblocker root-cause diagnosis**.

### 2. `serial`: Sequential Refinement with Deterministic Early Exit
- **How it works:** Executes a multi-stage refinement chain (e.g. Stage 1 drafts interfaces $\rightarrow$ Stage 2 implements functions $\rightarrow$ Stage 3 polishes error handling). When `early_exit_on_pass: true` is enabled, Noctifab checks the output of Stage 1 locally using the Go/language AST parser and anti-stub scanner; if the code is clean and compiles with zero stubs, the remaining stages are skipped, returning in **$<10\text{s}$**.
- **Best for:** **Turn 1 Code Generators** (`generators`).

### 3. `consensus`: Dual-Perspective Consensus Voting
- **How it works:** Dispatches to 2 independent voters in parallel. If both voters agree (unanimous pass or unanimous fail), the verdict is adopted immediately (**1 hop, 5–8s**). If the voters disagree, the prompt and both viewpoints are sent to a `tie_breaker` model for final determination.
- **Best for:** **Auditor / QA Gate**.

### 4. `race`: Speculative First-Valid Race
- **How it works:** Dispatches to multiple fast LLMs simultaneously with context cancellation. The first model that returns a syntactically valid AST response with zero stubs wins immediately, terminating all remaining requests in **1–3s**.
- **Best for:** **Micro-edits, single-function fixes, and fast tool executions**.

### 5. `decomposed`: Divide-and-Conquer Multi-File Generation
- **How it works:** Decomposes a large story into specialized targets (e.g., Domain Specialist, Service Specialist, Test Specialist). Each specialist generates its dedicated files in parallel, and Noctifab's deterministic engine merges the resulting tool actions into a complete full-stack slice.
- **Best for:** **Multi-layered DDD architectures (Domain, Application, Infrastructure, Tests)**.

### 6. `cascade`: Tiered Fast-Path Escalation
- **How it works:** First calls Tier 1 (a fast, inexpensive model like Gemini Flash or Haiku). If Tier 1 produces clean, compilable code without stubs, it returns in **1–2s**. If Tier 1 produces stubs or fails validation, Noctifab automatically escalates to Tier 2 (a frontier reasoning model like Claude 3.5 Sonnet).
- **Best for:** **Budget-sensitive development loops**.

### 7. `best_of_n_scored`: Deterministic Scored Selection
- **How it works:** Dispatches to $N$ models in parallel. Noctifab's local CPU grading engine evaluates each candidate completion (checking AST validity, anti-stub violations, line count limits, and test assertions) and promotes the highest-scoring candidate directly.
- **Best for:** **Tester Agent** (selecting the most comprehensive, non-tautological test suite with **zero LLM synthesis overhead**).

### 8. `adaptive`: Adaptive Self-Tuning Dynamic Routing
- **How it works:** Dynamically classifies incoming task complexity using semantic, structural, and failure indicators to route each task to the optimal model tier:
  - **Fast Tier (`fast_tier`):** Low-latency models (e.g. Gemini Flash, Haiku, Cerebras LLaMA, 1–2s response) for initial file scaffolding, `.gitignore`, DTOs, dataclasses, documentation, typos, and version bumps.
  - **Standard Tier (`standard_tier`):** Balanced frontier models (e.g. Claude 3.5 Sonnet, Gemini 3.1 Pro, GPT-4o, 3–5s response) for standard domain business logic, CRUD endpoints, and unit test suites.
  - **Heavy Tier (`heavy_tier`):** Frontier reasoning models (e.g. DeepSeek Reasoner, OpenAI o3-mini, Qwen Max, 10–20s response) for complex algorithmic problems (Minimax AI, fixpoint solvers, graph traversal), multi-threaded concurrency/mutexes, low-level binary framing, and multi-round test repair loops.
- **Dynamic Escalation:** If a task fails `run_tests` $\ge 2$ times consecutively in a lower tier, the Orchestrator automatically escalates the generator turn to the `heavy_tier` reasoning model.
- **Best for:** **Generators and coding workers** seeking to cut 60–80% of boilerplate turn latency while preserving heavy reasoning power for complex tasks.

---

## 3. Configuration & Syntax

Ensembles are configured under `agents.<role>.ensemble` (or `roles.<role>.ensemble`) in `.noctifab/config.yaml`.

Candidate models, stages, tiers, and voters reference globally declared providers in `llm.providers` by `name:`, with full support for overriding parameters per model (`model`, `temperature`, `max_tokens`, `enable_thinking`, `thinking_budget`, `extra_params`).

### 3.1 "No Max Token Budget" (`max_tokens: -1`)

To prevent long-running tasks or multi-model collaboration from being artificially cut off by token ceilings:
* Set `max_tokens: -1` (or `0`) at **Runtime level** (`runtime.max_tokens: -1`), **Story level** (`runtime.max_tokens_per_story: -1`), or **Agent level** (`agents.<role>.max_tokens: -1`).
* Setting `-1` designates an **unlimited token budget**.

### 3.2 Multi-Instance Sampling (`count: N`)

Because LLM generation is stochastic, sampling multiple independent paths from the *same* high-performing model at non-zero temperature often outperforms mixing in weaker models (Self-Consistency / Best-of-N).

The `count` field (default: `1`) allows spawning multiple independent client instances of a single provider spec without duplicate YAML boilerplate:

```yaml
models:
  - name: "claude"
    count: 3            # Spawns 3 independent parallel instances: claude-1, claude-2, claude-3
    temperature: 0.6
  - name: "openai"
    count: 2            # Spawns 2 independent parallel instances: openai-1, openai-2
    temperature: 0.4
```

- **In `best_of_n_scored` (Testers):** Evaluates all $N$ attempts via local CPU AST/anti-stub scoring and promotes the highest scoring test suite without LLM synthesis costs.
- **In `parallel` (Product Manager):** Fans out $N$ parallel story formulations to achieve a high-quality speculative quorum.
- **In `consensus` (Auditor):** Spawns $N$ independent voters from the same model to achieve self-consistent consensus voting.
- **In `race` (Fast-Path):** Fans out $N$ parallel requests to hedge against API tail latency.

---

## 4. Configuration Examples

### Example 1: `parallel` on Product Manager
```yaml
agents:
  product_manager:
    max_tokens: -1
    ensemble:
      strategy: "parallel"
      timeout_seconds: 45
      soft_timeout_seconds: 15        # Speculative Quorum deadline
      min_models: 2                   # Synthesize as soon as 2 fastest respond
      fallback_to_single: true
      models:
        - name: "claude"
          max_tokens: 8192
        - name: "openai"
          max_tokens: 8192
        - name: "deepseek"
          max_tokens: 8192
      synthesizer:
        name: "gemini"
        max_tokens: 16384
```

### Example 2: `serial` on Code Generator (with Early Exit)
```yaml
agents:
  generators:
    max_tokens: -1
    ensemble:
      strategy: "serial"
      timeout_seconds: 60
      early_exit_on_pass: true        # Returns in ~10s if Stage 1 passes local AST check
      fallback_on_stage_failure: true
      stages:
        - name: "openai"
        - name: "claude"
          refinement_prompt: |
            You are a Principal Engineer reviewing the following draft.
            Original Task: {{.OriginalPrompt}}
            Previous Draft: {{.PreviousOutput}}
            Implement all functions with zero stubs, valid imports, and comprehensive error handling.
        - name: "gemini"
```

### Example 3: `consensus` on Auditor
```yaml
agents:
  auditor:
    max_tokens: -1
    ensemble:
      strategy: "consensus"
      timeout_seconds: 30
      voters:
        - name: "claude"
        - name: "openai"
      tie_breaker:
        name: "gemini"
```

### Example 4: `race` on Fast Generators
```yaml
agents:
  generators:
    max_tokens: -1
    ensemble:
      strategy: "race"
      timeout_seconds: 15
      models:
        - name: "cerebras-llama"
        - name: "gemini-flash"
        - name: "haiku"
```

### Example 5: `decomposed` on Full-Stack Features
```yaml
agents:
  generators:
    max_tokens: -1
    ensemble:
      strategy: "decomposed"
      timeout_seconds: 45
      targets:
        - name: "claude"
          role_prompt: "Focus strictly on domain types and exported interface contracts."
        - name: "openai"
          role_prompt: "Focus strictly on application services implementing the domain interfaces."
        - name: "deepseek"
          role_prompt: "Focus strictly on Chicago-school black-box unit tests."
```

### Example 6: `cascade` on Budget-Optimized Roles
```yaml
agents:
  generators:
    max_tokens: -1
    ensemble:
      strategy: "cascade"
      timeout_seconds: 60
      tiers:
        - name: "gemini-flash"        # Fast Tier 1 (1-2s response)
        - name: "claude-sonnet"       # Frontier Tier 2 (Invoked only if Tier 1 code contains stubs/fails)
```

### Example 7: `best_of_n_scored` on Testers
```yaml
agents:
  testers:
    max_tokens: -1
    ensemble:
      strategy: "best_of_n_scored"
      timeout_seconds: 30
      models:
        - name: "claude"
        - name: "openai"
        - name: "deepseek"
```

### Example 8: `adaptive` for Self-Tuning Dynamic Routing
```yaml
agents:
  generators:
    max_tokens: -1
    ensemble:
      strategy: "adaptive"
      timeout_seconds: 45
      fast_tier:
        - name: "gemini-flash"
        - name: "cerebras-llama"
      standard_tier:
        - name: "claude"
      heavy_tier:
        - name: "claude"
        - name: "openai"
        - name: "deepseek"
```

---

## 5. Local CPU Scoring Engine

In strategies like `best_of_n_scored`, `serial` (Early Exit), `race`, and `cascade`, candidates are evaluated **locally and deterministically on the CPU by Noctifab's Go engine** ($<5\text{ms}$, zero LLM synthesis cost):

1. **AST & Syntax Gate:** Language parser verifies that the code parses cleanly and contains valid imports.
2. **Anti-Stub Gate:** Regex and AST scanners verify that the code contains no forbidden placeholders (`TODO`, `pass`, empty function bodies, `panic("unimplemented")`).
3. **File Size Invariant Gate:** Verifies that no generated file exceeds the **500-line constraint**.
4. **Assertion Density:** Evaluates non-tautological test assertions (`assert`, `require`, `t.Error`, `t.Fatal`).
5. **Deterministic Scorer:**
   $$\text{Score} = (\text{AST\_Valid} \times 100) - (\text{Stub\_Violations} \times 50) - (\text{Line\_Overflow} \times 100) + (\text{Assertions} \times 5)$$
