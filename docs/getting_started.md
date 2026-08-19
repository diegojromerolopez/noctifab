# Getting Started with Noctifab: Step-by-Step Developer Tutorial

Welcome to `noctifab`! This tutorial guides software engineers step-by-step through setting up, configuring, and running Noctifab to autonomously build, test, and ship code from software specifications.

---

## ⚡ 1. Installation

Install `noctifab` on macOS or Linux using the 1-line installer:

```bash
curl -sSL https://raw.githubusercontent.com/diegojromerolopez/noctifab/main/scripts/install.sh | sh
```

Or build from source:

```bash
git clone https://github.com/diegojromerolopez/noctifab.git
cd noctifab
make build
# Binary is located at dist/noctifab
```

Verify your installation:

```bash
noctifab --help
```

## 🎯 The 3-Step Development Lifecycle

Noctifab follows a clean, intuitive 3-step workflow from idea to fully tested, autonomous code delivery:

```
 ┌─────────────────────────┐      ┌─────────────────────────┐      ┌─────────────────────────┐
 │   1. noctifab init      │ ───► │   2. noctifab spec      │ ───► │   3. noctifab start     │
 │ (Infrastructure Setup)  │      │  (Interactive Design)   │      │ (Autonomous Execution)  │
 └─────────────────────────┘      └─────────────────────────┘      └─────────────────────────┘
```

---

## 📁 Step 1: Initializing Workspace Infrastructure (`init`)

To set up a new or existing codebase for Noctifab, run `noctifab init`:

### A. Initialize Current Directory
```bash
cd /path/to/my-project
noctifab init
```

### B. Initialize a New Project Folder
```bash
noctifab init my-awesome-app
cd my-awesome-app
```

### What `noctifab init` Creates:
- **`.noctifab/config.yaml`**: The primary YAML configuration file with intelligent role and LLM defaults.
- **`.noctifab/secrets.yaml`**: API keys and secrets template (gitignored).
- **`.noctifab/data/noctifab.db`**: Local SQLite state repository for tracking task DAGs and execution logs.
- **`.noctifab/.gitignore`**: Excludes local state databases, logs, and secrets from VCS commits.

---

## 📝 Step 2: Interactive Specification Design (`spec`)

Instead of writing complex markdown specifications by hand, use `noctifab spec` to generate, refine, and audit your `SPEC.md` through an interactive Human-in-the-Loop review session:

```bash
noctifab spec "Build an in-memory Redis-compatible key-value server in Go with LRU eviction"
```

1. **4-Stage Multi-Model Generation**: Noctifab coordinates Product Manager, Systems Architect, Test Architect, and QA Specialist roles across different LLMs to minimize bias.
2. **Consensus Audit**: Automatically audits the spec for internal contradictions and ensures test/DoD alignment.
3. **Interactive Review Loop**: Inspect the draft and colored line-by-line diffs in the terminal:
   * Provide feedback: `> "Add support for TLS certificates and a Prometheus /metrics endpoint"`
   * Approve: `> "looks good to me, stop"`
4. **Roadmap Generation**: Upon approval, Noctifab automatically generates atomic user stories under `roadmap/user-stories/`.

> 💡 **Fast-Track Bootstrap**: Combine Step 1 & Step 2 into a single command:
> ```bash
> noctifab init my-project --spec "Build a distributed KV store in Go"
> ```

---

## 🚀 Step 3: Launching Autonomous Dark Factory Execution (`start`)

Once `SPEC.md` is approved, launch Noctifab's autonomous Dark Factory loop:

```bash
noctifab start
```

Noctifab decomposes the specification into a topological task DAG, implements minimal compiling code (Verification), executes isolated black-box test suites (Validation), runs QA acceptance gates, and autonomously merges validated feature branches.

---

## 🔑 4. Configuring Secrets & LLM Providers

Noctifab requires an API key for your chosen LLM provider (OpenAI, Anthropic, Gemini, OpenCode, or local Ollama).

### A. Setting API Keys in Environment or `.noctifab/secrets.yaml`

Create `.noctifab/secrets.yaml` in your project folder:

```yaml
OPENAI_API_KEY: "sk-proj-..."
OPENCODE_API_KEY: "zen-..."
GITHUB_TOKEN: "ghp_..."
```

*(Note: `.noctifab/secrets.yaml` is automatically ignored by `.noctifab/.gitignore` to prevent secret leakage).*

### B. Configuring `.noctifab/config.yaml`

Open `.noctifab/config.yaml` to customize your provider, model, and fallback chain:

```yaml
llm:
  provider: "openai"
  model: "gpt-4o"
  max_retries: 3
  retry_backoff_ms: 1000
```

> **Detailed Configuration Guides**:
> - [Complete Configuration Reference](file:///Users/diegoj/repos/noctifab/docs/configuration.md)
> - [LLM Providers & Fallback Setup](file:///Users/diegoj/repos/noctifab/docs/llm_providers.md)
> - [Secrets & Credentials Management](file:///Users/diegoj/repos/noctifab/docs/secrets.md)

---

## 🚀 5. Running the Dark Factory Agent & Instant Demo

### Option A: Instant 2-Minute Zero-Config Sandbox (`noctifab demo`)
Test Noctifab immediately without setting up any API keys or creating configuration files:

```bash
noctifab demo
```
This runs a 100% offline, deterministic mock simulation of the entire Dark Factory loop with embedded templates and automated cleanup.

### Option B: Real-Time Visual Web Dashboard (`noctifab start -w --web-open` / `noctifab dashboard -w --web-open`)
Launch the visual web dashboard to explore the topological task DAG, watch streaming code diffs, and inspect real-time agent event logs (pass `--web-open` to auto-open in your default browser):

```bash
# Launch during execution and auto-open browser:
noctifab start [my-project-dir] -w --web-open

# Or inspect an existing project workspace anytime:
noctifab dashboard -w --web-open
# Automatically opens http://127.0.0.1:8080 in your browser
```

### Option C: Live Interactive TUI Dashboard Mode (`-i`)
Launch the terminal dashboard to watch real-time task execution, inspect active worker goroutines, view task DAG progress, and inject mid-flight steering directives:

```bash
noctifab start [my-project-dir] -i
```

**Interactive Keystrokes**:
- **`[s]`**: Inject a mid-flight steering directive to guide the active worker (e.g. "Use PostgreSQL instead of SQLite").
- **`[o]` / `[n]`**: Submit a new feature prompt order directly into the execution queue.
- **`[c]`**: Open interactive modal to resolve agent clarification questions.
- **`[p]`**: Toggle Pause / Resume.
- **`[d]`**: Open the Log & Failure Inspector.
- **`[q]`**: Exit dashboard view (daemon continues running safely in background).

### Option D: Headless Automated Mode (CI / Background Run)
For headless server or CI pipeline execution:

```bash
noctifab start [my-project-dir]
```

### Option E: Running via Background Daemon & Remote Orchestration
You can run Noctifab as a continuous background daemon (`--standby` or `-d`) and feed it project initialization, specifications, and new feature orders dynamically:

#### 1. Start the Noctifab Standby Daemon:
```bash
# Starts the background orchestrator and HTTP REST server on 127.0.0.1:8080
noctifab start --standby -d
```

#### 2. Submit Projects & Orders to the Daemon:
```bash
# A. Submit a new specification/feature order to the active daemon:
noctifab order "Build a high-performance REST API in Go with SQLite persistence and JWT auth"

# B. Inject a live steering directive while the daemon is implementing code:
noctifab steer "Ensure port is 9000 and add Prometheus metrics at /metrics"

# C. Or submit directly via HTTP REST API:
curl -X POST http://127.0.0.1:8080/api/v1/orders \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Add Redis caching layer to user queries"}'
```

The daemon automatically manages the complete Dark Factory lifecycle (planning, generation, testing, QA, and git branch merging) continuously in the background.

---

## ⚙️ 6. Configuration Settings Reference Table

| Category | Parameter | Description | Default |
|---|---|---|---|
| **Storage** | `storage.provider` | Database backend (`sqlite`, `postgres`) | `sqlite` |
| **Storage** | `storage.conn_string` | SQLite path or Postgres DSN | `.noctifab/data/noctifab.db` |
| **LLM** | `llm.provider` | API provider (`openai`, `anthropic`, `gemini`, `opencode`, `ollama`) | `openai` |
| **LLM** | `llm.model` | Primary model name (`gpt-4o`, `claude-3-5-sonnet`, `gemini-1.5-pro`) | `gpt-4o` |
| **Sandbox** | `sandbox.mode` | Isolation mode (`host`, `docker`) | `host` |
| **Sandbox** | `sandbox.auto_install_deps` | Auto-detect and install missing system tools (`true`, `false`) | `true` |
| **Autonomy** | `vcs.pull_request.auto_create` | Automatically create pull request on story completion | `true` |
| **Autonomy** | `vcs.pull_request.auto_merge` | Automatically merge pull request when CI tests pass | `false` |

---

## 📚 Further Reading & References

- [CLI Command Usage Guide](file:///Users/diegoj/repos/noctifab/docs/cli_usage.md)
- [Architecture & Dark Factory Loop](file:///Users/diegoj/repos/noctifab/docs/architecture.md)
- [REST API Reference](file:///Users/diegoj/repos/noctifab/docs/api.md)
- [Developer & Contributor Guide](file:///Users/diegoj/repos/noctifab/docs/developer_guide.md)
