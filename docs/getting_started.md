# Getting Started with Noctifab

Welcome! This guide walks you through setting up and running **noctifab** from scratch on your local machine.

---

## 1. Quick Installation

### Option A: 1-Line Installer (macOS & Linux)
```bash
curl -sSL https://raw.githubusercontent.com/diegojromerolopez/noctifab/main/scripts/install.sh | sh
```

### Option B: Build from Source (Go 1.22+)
```bash
git clone https://github.com/diegojromerolopez/noctifab.git
cd noctifab
make build
# The binary is placed at ./dist/noctifab
```

Verify your installation:
```bash
noctifab version
```

---

## 2. Instant 60-Second Test Drive (`noctifab demo`)

Before setting up API keys or configuring a project, you can test drive Noctifab with its built-in offline simulation:

```bash
# Run the zero-config demo sandbox:
noctifab demo
```

This autonomously unpacks the `t4` project (an ISO C17 S3-style HTTP server) into `./demo`, simulates the multi-agent task DAG planning, code generation, and 3x consensus testing without calling external APIs.

Once completed, inspect the generated project:
```bash
cd demo
make test   # Compiles and runs C17 HTTP wire protocol unit tests
make build  # Compiles bin/t4
./bin/t4    # Launches S3-compatible server on port 8080
```

---

## 3. Initializing a Workspace: The `init` Command

The `noctifab init` command prepares a directory to be driven by Noctifab's dark factory engine.

```bash
# Initialize current directory:
noctifab init

# Or initialize a specific project folder:
noctifab init my-new-project
cd my-new-project
```

### What `noctifab init` Creates:

```text
.noctifab/
├── config.yaml          # Core platform settings (LLM, sandbox, VCS, and autonomy gates)
├── data/
│   └── noctifab.db     # SQLite database storing task DAGs, mutations, and telemetry
├── logs/               # Full real-time execution transcripts and compiler diagnostics
└── .gitignore          # Automatically ignores noctifab.db, logs/, and secrets.yaml
```

> **Cleanliness Protection**: If you run `noctifab init` inside a folder that already contains files but is **not** a git repository and has no `.noctifab` directory, Noctifab will safely abort with an advisory warning to prevent overwriting unversioned files. Run `git init` first in existing projects.

---

## 4. Configuring LLM Credentials

Noctifab supports multiple LLM providers as well as local open-source models via Ollama and vLLM.

### Option A: 1-Click Local LLMs (Ollama / vLLM - 100% Free & Private)
Run without sending code or tokens to external cloud providers:

1. Start your local Ollama daemon:
   ```bash
   ollama run qwen2.5-coder:32b
   ```
2. Edit `.noctifab/config.yaml`:
   ```yaml
   llm:
     provider: ollama
     model: qwen2.5-coder:32b
     # Default base URL connects to http://localhost:11434/v1 automatically
   ```

### Option B: Cloud Providers (Gemini, Claude, OpenAI, DeepSeek)
Set your API key in your environment:
```bash
export GEMINI_API_KEY="AIzaSy..."
# Or: export OPENAI_API_KEY="sk-..."
# Or: export ANTHROPIC_API_KEY="sk-ant-..."
# Or: export DEEPSEEK_API_KEY="sk-..."
```

Or configure via gitignored `.noctifab/secrets.yaml`:
```yaml
# .noctifab/secrets.yaml (Never committed)
GEMINI_API_KEY: "AIzaSy..."
```
```yaml
# .noctifab/config.yaml
llm:
  provider: gemini
  model: gemini-2.5-flash
  api_key: "secret:GEMINI_API_KEY"
```

### Real-World Token & Cost Estimation
Based on empirical runs across the 20 benchmark repositories:

| Stage | Token Usage | Wall-Clock Time | Estimated Cost (Gemini Flash / DeepSeek) |
| :--- | :---: | :---: | :---: |
| **Walking Skeleton (`spike`)** | 10k–15k tokens | 38s–65s | < $0.02 |
| **Average User Story** | 25k–45k tokens | 2m–4m | ~$0.04 |
| **Full Greenfield Project (5 Stories)** | ~143,000 tokens | ~11.4 min | ~$0.15–$0.40 |
| **Local LLM (Ollama / vLLM)** | Unlimited | System speed | **$0.00 (100% Free)** |

---

## 5. Crafting the Specification (`SPEC.md`)

Noctifab is **specification-driven**. Rather than micro-managing code function by function, you author a `SPEC.md` that defines what the system should do, its architecture, and acceptance criteria.

You can create `SPEC.md` manually or generate an initial blueprint interactively:
```bash
noctifab spec "Build a command-line URL shortener in Go with SQLite persistence and redirect tracking"
```

A minimal `SPEC.md` includes:
- **System Goal & Language/Stack** (e.g. Go 1.24 CLI with SQLite)
- **Public Contracts & Exit Codes** (CLI inputs, stdout/stderr formatting, return codes)
- **Test Expectations** (100% test coverage, anti-spoofing assertions)

---

## 6. Launching the Dark Factory Loop

Once `SPEC.md` is in place, start the autonomous engine:

```bash
# Start with Visual Web Dashboard & Spec Studio (Recommended):
noctifab start -w

# Or launch in interactive Terminal Dashboard mode (TUI):
noctifab start -i
```

### What Happens Automatically:
1. **Product Manager Agent**: Translates `SPEC.md` into unambiguous ASD-STE100 Simplified Technical English (`SPEC.ste.md`) and generates user stories under `roadmap/`.
2. **Planner Agent**: Decomposes stories into a topological Directed Acyclic Graph (DAG) of isolated tasks.
3. **Execution Pipeline**:
   - **Verification**: Generator writes the minimal code to compile.
   - **Black-Box Testing**: Tester writes behavioral tests against public APIs/CLIs.
   - **Validation**: Generator refactors and hardens code under the test safety net.
4. **3x Consensus Quality Gate**: The test suite is executed 3 times in a clean sandbox; changes require ≥ 2/3 pass consensus to prevent flaky test regressions.
5. **Git Auto-Merge**: Passes merge automatically into `main` via git rebase queues.

---

## 7. Real-World Projects Built with Noctifab

Noctifab has been validated across 20 distinct technological stacks, producing complete, passing implementations published to dedicated `feat/noctifab-generated` branches on GitHub:

| Project | Stack | Description | Generated Code & Branch |
| :--- | :--- | :--- | :---: |
| [**`echo`**](https://github.com/diegojromerolopez/echo) | Go 1.24 CLI | Single-process CLI with public contracts | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/echo/tree/feat/noctifab-generated) |
| [**`todo-cli`**](https://github.com/diegojromerolopez/todo-cli) | Go 1.24 + JSON DB | CLI checklist manager with file persistence | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/todo-cli/tree/feat/noctifab-generated) |
| [**`wc`**](https://github.com/diegojromerolopez/wc) | Rust 1.88 CLI | UNIX `wc` streaming clone with `#![deny(unsafe_code)]` | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/wc/tree/feat/noctifab-generated) |
| [**`calculator`**](https://github.com/diegojromerolopez/calculator) | Ruby 3.4 + RSpec | Dynamic CLI & REPL calculator with RSpec suite | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/calculator/tree/feat/noctifab-generated) |
| [**`fortune`**](https://github.com/diegojromerolopez/fortune) | C17 + SQLite3 | Native ANSI C17 quote server with embedded SQLite C API | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/fortune/tree/feat/noctifab-generated) |
| [**`t4`**](https://github.com/diegojromerolopez/t4) | C17 HTTP Server | POSIX socket HTTP daemon & S3-style object storage | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/t4/tree/feat/noctifab-generated) |
| [**`pyedis`**](https://github.com/diegojromerolopez/pyedis) | Python 3.14 (asyncio) | In-memory Redis RESP2/RESP3 database server | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/pyedis/tree/feat/noctifab-generated) |
| [**`notebook`**](https://github.com/diegojromerolopez/notebook) | TypeScript + Fastify | Full-stack SPA with Fastify REST API and PostgreSQL | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/notebook/tree/feat/noctifab-generated) |
| [**`frontpunch`**](https://github.com/diegojromerolopez/frontpunch) | Python + Valkey | Distributed task worker and background queue processor | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/frontpunch/tree/feat/noctifab-generated) |
| [**`djanban`**](https://github.com/diegojromerolopez/djanban) | Python 3.13 / Django | Brownfield Kanban analytics app with Trello sync | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/djanban/tree/feat/noctifab-generated) |
| [**`stricc`**](https://github.com/diegojromerolopez/stricc) | Rust 1.88 + LLVM 18 | Safe C compiler with deterministic AST validation | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/stricc/tree/feat/noctifab-generated) |
| [**`searchthedocs`**](https://github.com/diegojromerolopez/searchthedocs) | Python 3.12 + FastAPI | Async doc scraper and vector similarity RAG search | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/searchthedocs/tree/feat/noctifab-generated) |
| [**`auth-vault`**](https://github.com/diegojromerolopez/auth-vault) | Go 1.24 + PKI / OIDC | Zero-trust OAuth2/OIDC server & PKI key rotation vault | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/auth-vault/tree/feat/noctifab-generated) |
| [**`buffonstream`**](https://github.com/diegojromerolopez/buffonstream) | Go 1.24 + Protobuf | Append-only streaming engine with gRPC endpoints | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/buffonstream/tree/feat/noctifab-generated) |
| [**`jpacioli`**](https://github.com/diegojromerolopez/jpacioli) | Java 21 + Spring Boot | Enterprise double-entry ledger with Event Sourcing & CQRS | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/jpacioli/tree/feat/noctifab-generated) |
| [**`ocalogue`**](https://github.com/diegojromerolopez/ocalogue) | OCaml 5.x + Dune | Strongly-typed functional catalog with algebraic data types | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/ocalogue/tree/feat/noctifab-generated) |
| [**`ninline`**](https://github.com/diegojromerolopez/ninline) | Python 3.14 + Minimax AI | Generalized (M,N,K) game engine with ray-casting AI | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/ninline/tree/feat/noctifab-generated) |
| [**`actodis`**](https://github.com/diegojromerolopez/actodis) | Erlang/OTP 29 + Rebar3 | Distributed Redis store with GenServer supervision trees | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/actodis/tree/feat/noctifab-generated) |
| [**`thredis`**](https://github.com/diegojromerolopez/thredis) | C# / .NET 9 | Concurrent thread-safe Redis server | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/thredis/tree/feat/noctifab-generated) |
| [**`dotchess`**](https://github.com/diegojromerolopez/dotchess) | C# / .NET 9 | UCI chess engine with bitboard move generation | [`feat/noctifab-generated`](https://github.com/diegojromerolopez/dotchess/tree/feat/noctifab-generated) |

---

## 8. Frequently Asked Questions & Troubleshooting

### What happens if tests fail during a run?
You do not need to intervene. Noctifab incorporates an intra-turn feedback loop:
1. **Intra-turn retry**: When `run_tests` fails, compiler errors, assertion diffs, and stack traces are fed directly into the Generator Agent's prompt context to fix the bug in its next turn (up to 8 turns per task).
2. **Watchdog self-repair**: If a task exhausts turns, the Watchdog handler categorizes the error (Timeout, Compile, Test Assertion) and executes surgical targeted repairs.
3. **Sovereign unblocker**: If a persistent deadlock occurs across tasks, the Fallback Agent takes cross-domain authority to repair code, tests, and specifications simultaneously to restore green build invariants.

### How does the 3x Consensus Gate eliminate flaky tests?
In traditional CI, running tests once can result in a "lucky" green pass for a race condition or flaky test. Noctifab dispatches the test runner **3 times in parallel**. Only if at least **2 out of 3 runs pass** is the task approved for rebase and merge. If any run exhibits fatal crash signals (`SIGSEGV`, `SIGBUS`, Go/Rust panics, `OOMKilled`), it is rejected immediately.

### Can I steer or order features while Noctifab is running?
Yes! Noctifab features a daemon Command Mailbox. Without interrupting the active loop, you can inject directives:
```bash
# Provide steering guidance to active workers:
noctifab steer "Prefer std::string_view instead of raw char pointers"

# Enqueue an ad-hoc feature request into the roadmap:
noctifab order "Add Prometheus /metrics endpoint tracking request latencies"
```
You can also steer directly from the Visual Web Dashboard input bar (`http://127.0.0.1:8080`).

### Can I run Noctifab on existing (brownfield) codebases?
Yes. When launched in a repository containing pre-existing code, `scanLegacyFiles` detects existing source files and automatically injects a `LEGACY CODEBASE STABILIZATION` mandate into the Product Manager. The system creates characterization tests for existing code first before generating new features, ensuring zero regressions to legacy behavior.

### How do I keep token usage and costs low?
- **100% Free**: Use local Ollama models (`qwen2.5-coder:32b`, `deepseek-r1`).
- **Context Compaction**: Enable `caveman` telegraphic prompt compression in `.noctifab/config.yaml` (`context.compaction: "caveman"`), cutting prompt tokens by 25–40%.
- **Budget Ceilings**: Set strict token caps in `.noctifab/config.yaml`:
  ```yaml
  runtime:
    max_tokens: 500000        # Caps total token burn across entire run
    max_actions: 100          # Caps total loop turns
  ```

