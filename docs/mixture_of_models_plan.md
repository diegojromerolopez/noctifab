# Mixture of Models (MoM / MoA) Architecture & Strategy Plan

This document defines the architectural design, configuration specification, and failure resilience strategies for multi-model ensembling in **Noctifab**. It introduces high-leverage execution topologies designed to maximize first-time generation quality, minimize latency, and eliminate agent retry loops:
1. **Parallel & Posterior Mix with Speculative Quorum (`parallel`)**
2. **Serial Refinement Pipeline with Deterministic Early Exit (`serial`)**
3. **Dual-Perspective Consensus (`consensus`)**
4. **Speculative First-Valid Race (`race`)**
5. **Divide-and-Conquer Multi-File Generation (`decomposed`)**
6. **Tiered Fast-Path Escalation (`cascade`)**
7. **Deterministic Scored Selection (`best_of_n_scored`)**

---

## 1. Motivation & Core Philosophy: "Shift-Left" Quality

In autonomous software development harnesses, the primary driver of wasted time and token consumption is **downstream failure loops**:
* A single model on Turn 1 generates incomplete code, syntax errors, or stubs.
* The Anti-Stub Validator or Test Validator rejects the turn.
* Downstream repair loops (Refactor turns, Unblocker, and Watchdog) are repeatedly invoked to fix what should have been correct initially.

Combining complementary LLM families (e.g., Claude 3.5 Sonnet, GPT-4o, Gemini 2.5 Pro, DeepSeek V3, Cerebras LLaMA 3.3) at critical decision points achieves **first-time-right generation**, preventing 80–90% of downstream repair cycles.

### 1.1 Ensembling Topologies Overview

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
│ Audit Prompt │      └─────────────────────────┘  ├──► Unanimous Pass ──► Instant Approval
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
```

---

## 2. Role-to-Topology Mapping

| Agent / Service | Recommended Strategy | Rationale & Mechanism |
| :--- | :--- | :--- |
| **Product Manager (Roadmap Generator)** | `parallel` (Quorum) | **Divergent Discovery & Complete DoD:** Models propose independent requirement breakdowns; synthesizer produces explicit public API signatures, file paths, and test scenarios. |
| **Turn 1 Code Generator (`generator`)** | `serial` (Early Exit) or `decomposed` | **First-Time-Right Code:** `serial` with Early Exit provides self-refining code, while `decomposed` produces full-stack domain/service/test slices in 1 parallel hop. |
| **Tester Agent (`tester`)** | `parallel` or `best_of_n_scored` | **Comprehensive Black-Box Test Matrix:** Generates happy-path and boundary scenarios; highest non-tautological test suite selected. |
| **Tool Execution & Micro-Edits** | `race` | **Ultra-Low Latency:** Dispatches to multiple fast providers; first syntactically valid tool action wins in 1–3s. |
| **Budget-Sensitive Generation** | `cascade` | **Cheap Fast-Path:** Uses fast models (1–2s) for 70% of straightforward turns; escalates to frontier models only on complex errors. |
| **Auditor / Quality Reviewer** | `consensus` | **Lightweight Non-Blocking Verification:** Two independent models check user story satisfaction and code completeness concurrently without slow 3-stage serial chaining. |
| **Unblocker / Watchdog Repair** | `parallel` (Diagnosis) $\rightarrow$ `single` (Patch) | **Brainstorm Root Causes:** Fast parallel fan-out generates competing hypotheses for build/test failures; top hypothesis is dispatched to a single focused patch generator. |

---

## 3. Who Measures and Evaluates Candidate Models?

In strategies like `best_of_n_scored`, `serial` (Early Exit), and `cascade`, evaluation is performed **locally and deterministically on the CPU by Noctifab's Engine** ($<5\text{ms}$, zero LLM synthesis cost):

1. **AST & Syntax Gate:** Go/Language parser checks that the code compiles cleanly and contains valid imports.
2. **Anti-Stub Gate:** Regex and AST scanners verify that the code contains no forbidden placeholders (`TODO`, `pass`, empty function bodies, `panic("unimplemented")`).
3. **File Size Invariant Gate:** Verifies that no generated file exceeds the **500-line constraint**.
4. **DoD & Interface Coverage:** Evaluates whether required public functions, CLI flags, and test assertions from the story contract are genuinely present.
5. **Deterministic Scorer:** `Score = (AST_Valid * 100) - (Stub_Violations * 25) - (Line_Overflow * 50) + (Assertions * 5)`. The highest-scoring candidate is immediately promoted as the winner.

---

## 4. Configuration Design

Ensembling is configured directly under `agents.<role>.ensemble` (or `roles.<role>.ensemble`) using the `strategy:` property. Candidate models, stages, tiers, and voters reference named providers defined globally in `llm.providers` by `name:`, with full support for overriding any provider setting per model.

### 4.1 Provider Referencing & Setting Overrides in `models:`
When referencing a provider by `name:`, all base settings (API keys, provider type, default model, URL, headers) are inherited from `llm.providers`. You can selectively **override any setting** for an individual model, stage, or voter:
* `model`: Override the model snapshot (e.g. use `claude-3-7-sonnet` instead of default `claude-3-5-sonnet`).
* `temperature`: Override the temperature (e.g. `0.0` for deterministic syntax vs `0.4` for divergent ideation).
* `max_tokens`: Override the maximum completion output tokens.
* `enable_thinking` & `thinking_budget`: Enable or tune reasoning tokens for thinking models.
* `extra_params`: Pass or override provider-specific parameters (e.g. `top_p`, `seed`, etc.).

```yaml
models:
  - name: "claude"                     # Inherits base 'claude' provider from llm.providers
    model: "claude-3-7-sonnet-20250219" # Overrides model
    temperature: 0.0                   # Overrides temperature
    max_tokens: 4096                   # Overrides max completion tokens
  - name: "qwen"
    enable_thinking: true              # Overrides thinking mode
    thinking_budget: 2048
  - name: "openai"                     # Inherits everything verbatim from llm.providers
```

---

### 4.2 "No Max Token Budget" (Unlimited Token Budget Support)
To accommodate long-running tasks or unrestricted model cooperation without artificial aborts, Noctifab supports explicitly disabling token limits across all configuration tiers using `-1`:
* Setting `max_tokens: -1` (or `max_tokens: 0`, or omitting the field) explicitly disables the token limit, designating an **unlimited token budget**.
* Applicable at **Runtime level** (`runtime.max_tokens: -1`), **Loop / Story level** (`runtime.max_tokens_per_story: -1`, `runtime.max_tokens_per_task: -1`), and **Agent / Role level** (`agents.<role>.max_tokens: -1` or `roles.<role>.max_tokens: -1`).
* When `adaptive_budget_throttling` is enabled and a positive `max_tokens` ceiling is set, the system automatically degrades to single-model mode if token usage reaches $> 75\%$ of the budget. When `max_tokens: -1` (unlimited), full ensembling remains active continuously without throttling.

---

### 4.3 YAML Configuration Examples

#### Example 1: Parallel & Posterior Mix with Speculative Quorum (`parallel`)
```yaml
agents:
  product_manager:
    max_tokens: -1                    # Unlimited agent-level token budget (-1)
    ensemble:
      strategy: "parallel"
      timeout_seconds: 45             # Hard timeout
      soft_timeout_seconds: 15        # Speculative Quorum soft deadline
      min_models: 2                   # Synthesize as soon as 2 fastest respond
      fallback_to_single: true
      models:
        - name: "claude"              # References 'claude' from llm.providers
          max_tokens: 8192            # Optional per-model completion cap
        - name: "openai"              # References 'openai' from llm.providers
          max_tokens: 8192
        - name: "deepseek"            # References 'deepseek' from llm.providers
          max_tokens: 8192
      synthesizer:
        name: "gemini"
        max_tokens: 16384
```

#### Example 2: Serial Refinement with Early Exit (`serial`)
```yaml
agents:
  generators:
    max_tokens: -1
    ensemble:
      strategy: "serial"
      timeout_seconds: 60
      early_exit_on_pass: true        # If Stage 1 code passes local AST/syntax check, skip Stage 2
      fallback_on_stage_failure: true
      stages:
        - name: "openai"
        - name: "claude"
          refinement_prompt: |
            You are a Principal Software Engineer. Review and complete the following draft implementation.
            Original Task: {{.OriginalPrompt}}
            Previous Draft: {{.PreviousOutput}}
            Ensure all functions are fully implemented with zero stubs, correct imports, and proper error handling.
        - name: "gemini"
```

#### Example 3: Dual-Perspective Consensus (`consensus`)
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

#### Example 4: Speculative First-Valid Race (`race`)
```yaml
agents:
  generators:
    max_tokens: -1
    ensemble:
      strategy: "race"
      timeout_seconds: 15             # First model with valid non-stub AST wins
      models:
        - name: "cerebras-llama"
        - name: "gemini-flash"
        - name: "haiku"
```

#### Example 5: Divide-and-Conquer Multi-File Generation (`decomposed`)
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
          role_prompt: "Focus strictly on implementation logic satisfying the domain interfaces."
        - name: "deepseek"
          role_prompt: "Focus strictly on Chicago-school blackbox unit tests."
```

#### Example 6: Tiered Fast-Path Escalation (`cascade`)
```yaml
agents:
  generators:
    max_tokens: -1
    ensemble:
      strategy: "cascade"
      timeout_seconds: 60
      tiers:
        - name: "gemini-flash"        # Fast Tier 1 (1-2s response)
        - name: "claude-sonnet"       # Frontier Tier 2 (Invoked only if Tier 1 code fails validation)
```

#### Example 7: Deterministic Scored Selection (`best_of_n_scored`)
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

---

## 5. Go Domain Structs & Types

```go
package config

type EnsembleStrategy string

const (
	EnsembleStrategyParallel      EnsembleStrategy = "parallel"
	EnsembleStrategySerial        EnsembleStrategy = "serial"
	EnsembleStrategyConsensus     EnsembleStrategy = "consensus"
	EnsembleStrategyRace          EnsembleStrategy = "race"
	EnsembleStrategyDecomposed    EnsembleStrategy = "decomposed"
	EnsembleStrategyCascade       EnsembleStrategy = "cascade"
	EnsembleStrategyBestOfNScored EnsembleStrategy = "best_of_n_scored"
)

type EnsembleConfig struct {
	Strategy               EnsembleStrategy    `yaml:"strategy"`
	TimeoutSeconds         int                 `yaml:"timeout_seconds"`
	SoftTimeoutSeconds     int                 `yaml:"soft_timeout_seconds,omitempty"`
	MinModels              int                 `yaml:"min_models,omitempty"`
	EarlyExitOnPass        bool                `yaml:"early_exit_on_pass,omitempty"`
	FallbackToSingle       bool                `yaml:"fallback_to_single"`
	FallbackOnStageFailure bool                `yaml:"fallback_on_stage_failure"`

	// Models for Parallel, Race, and BestOfNScored
	Models      []AgentProviderRef `yaml:"models,omitempty"`
	Synthesizer *AgentProviderRef  `yaml:"synthesizer,omitempty"`

	// Serial Refinement Strategy
	Stages []EnsembleStageSpec `yaml:"stages,omitempty"`

	// Consensus Strategy (consensus)
	Voters     []AgentProviderRef `yaml:"voters,omitempty"`
	TieBreaker *AgentProviderRef  `yaml:"tie_breaker,omitempty"`

	// Tiered Cascade Strategy
	Tiers []AgentProviderRef `yaml:"tiers,omitempty"`

	// Decomposed Divide-and-Conquer Strategy
	Targets []DecomposedTargetSpec `yaml:"targets,omitempty"`
}

type DecomposedTargetSpec struct {
	Name        string            `yaml:"name"` // References named provider from llm.providers
	RolePrompt  string            `yaml:"role_prompt,omitempty"`
	MaxTokens   *int              `yaml:"max_tokens,omitempty"`
	ExtraParams map[string]string `yaml:"extra_params,omitempty"`
}

type EnsembleStageSpec struct {
	Name             string            `yaml:"name"` // References named provider from llm.providers
	MaxTokens        *int              `yaml:"max_tokens,omitempty"`
	Temperature      *float64          `yaml:"temperature,omitempty"`
	RefinementPrompt string            `yaml:"refinement_prompt,omitempty"`
	ExtraParams      map[string]string `yaml:"extra_params,omitempty"`
}
```

---

## 6. Failure Modes & Resilience Engineering

A provider outage, rate limit (HTTP 429), authentication error, or context timeout must **never crash the orchestrator** or leave broken state.

### 6.1 Failure Matrix by Strategy

| Strategy | Failure Scenario | Mitigation & Resilience Action | Final Result |
| :--- | :--- | :--- | :--- |
| **`race`** | 1 or more models fail / return stubs | Ignored; engine waits for the first valid candidate. If all fail, falls back to default single provider. | **Zero latency penalty**, clean fallback. |
| **`cascade`** | Tier 1 fails or returns broken/stub code | Automatically escalates to Tier 2 (Frontier LLM). | **Reliable execution** with automatic healing. |
| **`decomposed`** | 1 specialist target fails | Retry target with failover model; if still failing, synthesize slice using remaining targets. | **Modular recovery**. |
| **`best_of_n_scored`** | 1 candidate fails / errors | Discard failed candidate; grade remaining valid candidates. | **Highest valid proposal promoted**. |
| **`parallel` (Quorum)** | Quorum met in soft timeout ($K \ge \text{min\_models}$) | Cancel stragglers; synthesize immediately with fastest $K$ models. | **Fast synthesis**, stragglers eliminated. |
| **`serial` (Early Exit)** | Stage 1 output passes local AST/Anti-stub gate | `early_exit_on_pass: true` triggers immediate return; skip remaining stages. | **Instant return**, saves time & tokens. |
| **`consensus`** | Unanimous vote (Both PASS / Both FAIL) | Result adopted immediately without second call. If split, call `tie_breaker`. | **Fast 1-hop consensus**. |

---

## 7. Implementation Roadmap

1. **`pkg/infrastructure/config`**:
   - Update `types.go` and `runtime_types.go` with `EnsembleConfig`, `EnsembleStrategy`, `EnsembleStageSpec`, `DecomposedTargetSpec`, and unlimited token budget representations (`max_tokens: -1` / `0`).
2. **`pkg/infrastructure/llm/ensemble`**:
   - `race_client.go`: Implements speculative first-valid racing with AST validation.
   - `cascade_client.go`: Implements tiered escalation from fast to frontier models.
   - `decomposed_client.go`: Implements divide-and-conquer parallel target generation and file merging.
   - `scored_client.go`: Implements deterministic local scoring of candidate completions.
   - `parallel_client.go`: Implements speculative quorum fan-out + structured action synthesis.
   - `serial_client.go`: Implements sequential refinement with deterministic AST/anti-stub early exit.
   - `consensus_client.go`: Implements parallel voting and tie-breaking (`consensus`).
   - `ensemble_client.go`: Unified `domain.LLMClient` dispatcher based on configured strategy.
3. **`pkg/infrastructure/llm/router.go`**:
   - Update candidate resolver to instantiate ensemble clients when `ensemble.strategy` is specified for an agent role.
   - Integrate token budget checks (`<= 0` or `-1` = unlimited, positive = enforce with adaptive throttling).
4. **Unit & Integration Tests**:
   - Comprehensive test suite covering quorum triggers, race cancellation, early exits, decomposed merging, voter tie-breakers, timeout cancellations, and token accountability.
