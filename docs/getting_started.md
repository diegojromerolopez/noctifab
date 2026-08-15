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

---

## 📁 2. Initializing a Project

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
- **`SPEC.md`**: A clean software specification template pre-structured for Noctifab's Product Manager Agent.
- **`.noctifab/config.yaml`**: The primary YAML configuration file with intelligent defaults.
- **`.noctifab/data/noctifab.db`**: Local SQLite state repository for tracking task DAGs and execution logs.
- **`.noctifab/.gitignore`**: Excludes local state databases, logs, and secrets from VCS commits.

---

## 📝 3. Writing Your Specification (`SPEC.md`)

Noctifab reads `SPEC.md` to plan and generate code. Open `SPEC.md` and define your project requirements:

```markdown
# Specification: Quote Generator Service

## 1. Overview
A lightweight CLI quote generator with SQLite persistence.

## 2. Technology Stack & Language Guidelines
- **Primary Language**: Go 1.22 / C17 / Python / Rust
- **Testing Framework**: Native unit tests (`go test ./...`)

## 3. Core Domain Models & Schemas
Define key entities and structs (e.g. `Quote` struct with `ID`, `Author`, `Text`).

## 4. Interfaces & Command Contracts
- CLI command: `quote-app generate --category motivational`

## 5. Acceptance Criteria & Quality Gates
- All unit tests must pass with 100% success rate.
- Zero linter warnings.
```

### 💡 Working with Existing / Legacy Codebases
If you initialize Noctifab in an existing repository with pre-existing code files, Noctifab's **Legacy Codebase Scanning** (`scanLegacyFiles`) automatically detects existing source files. 

The Product Manager agent automatically generates `roadmap/user-stories/US-001.md` titled **`"Legacy Codebase Characterization & Stabilization"`**. This forces the dark factory loop to write comprehensive characterization tests asserting pre-existing behaviors before attempting surgical refactoring (`edit_file`, `apply_patch`) or implementing new user story features.

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

## 🚀 5. Running the Dark Factory Agent

You can run Noctifab in **Live Interactive TUI Mode** or **Headless Automated Mode**:

### Option A: Live Interactive TUI Dashboard Mode (`-i`)
Launch the interactive dashboard to watch real-time task execution, inspect active worker goroutines, view task DAG progress, and answer clarification prompts on the fly:

```bash
noctifab start [my-project-dir] -i
```

**Interactive Keystrokes**:
- **`[n]`**: Submit a new order or requirement prompt on the fly.
- **`[c]`**: Open interactive modal to resolve agent clarification questions.
- **`[p]`**: Toggle Pause / Resume.
- **`[q]`**: Exit dashboard view (daemon continues running safely in background).

### Option B: Headless Automated Mode (CI / Background Run)
For headless server or CI pipeline execution:

```bash
noctifab start [my-project-dir]
```

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
