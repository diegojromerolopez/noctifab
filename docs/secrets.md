# Secrets Management

`noctifab` keeps sensitive credentials (API keys, VCS tokens) out of `config.yaml` — and therefore out of version control — using an optional **`secrets.yaml`** file.

---

## The Problem

`config.yaml` is typically committed to the repository so that project settings (LLM model, sandbox commands, queue configuration) are shared across the team. However, committing secrets such as API keys or personal access tokens would expose them in git history.

The `secret:` reference system solves this cleanly:

- `config.yaml` stores a **reference** to a secret key, not the secret itself.
- `secrets.yaml` stores the **actual secret values** and is always gitignored.

---

## How It Works

Any string field in `config.yaml` whose value starts with `secret:` is treated as a reference. During configuration loading, `noctifab` resolves the reference by looking up the remainder of the string as a key in `secrets.yaml`.

**Resolution Precedence (Highest to Lowest):**

1. **CLI Flags** (applied last, highest precedence)
2. **Environment Variables** (e.g. `OPENAI_API_KEY`, `GITHUB_TOKEN`)
3. **Project Secrets** (`.noctifab/secrets.yaml` inside project folder)
4. **Global Home Secrets** (`$HOME/.noctifab/secrets.yaml`)
5. **Literal Values** in `config.yaml`

---

## Setup

### 1. Global Baseline vs Project Secrets

Noctifab supports both single-project credentials and global user-wide credentials:

* **Global Home Directory Secrets (`$HOME/.noctifab/secrets.yaml`)**:  
  Create `$HOME/.noctifab/secrets.yaml` to store baseline API keys used across all projects on your machine. If a project does not contain a local `.noctifab/secrets.yaml`, Noctifab automatically reads your global home secrets file.
* **Project-Level Secrets (`.noctifab/secrets.yaml`)**:  
  Place `secrets.yaml` inside `.noctifab/secrets.yaml` of a specific project workspace. Values in the project secrets file override matching keys from the global home secrets file.

```yaml
# $HOME/.noctifab/secrets.yaml or .noctifab/secrets.yaml
# This file is gitignored — never commit it.

GITHUB_TOKEN: "github_pat_11AA4TEXQ0..."
GEMINI_API_KEY: "AIzaSyATKC77..."
OPENAI_API_KEY: "sk-proj-..."
```

### 2. Reference secrets in `config.yaml`

Use the `secret:<KEY>` syntax in any string field that accepts credentials:

```yaml
# .noctifab/config.yaml
llm:
  provider: gemini
  model: gemini-1.5-pro
  api_key: "secret:GEMINI_API_KEY"       # resolved from secrets.yaml

vcs:
  provider: github
  repository: owner/repo
  token: "secret:GITHUB_TOKEN" # resolved from secrets.yaml
```

### 3. Confirm it is gitignored

`noctifab init` automatically adds `secrets.yaml` to `.noctifab/.gitignore`. Verify:

```bash
cat .noctifab/.gitignore
# should contain: secrets.yaml
```

> [!CAUTION]
> Never commit `secrets.yaml` to version control. If you accidentally do, rotate all exposed credentials immediately and rewrite git history using `git filter-repo`.

---

## Supported Fields

The following `config.yaml` fields support `secret:` references:

| Field | YAML path |
|-------|-----------|
| LLM API key | `llm.api_key` |
| LLM endpoint URL | `llm.url` |
| VCS access token | `vcs.token` |
| Multiple LLM client API key | `llms[].api_key` |
| Multiple LLM client endpoint URL | `llms[].url` |

---

## Supported Provider API Key Environment Variables & `secrets.yaml` Keys

When `llm.api_key` or `vcs.token` are not explicitly defined in `config.yaml`, `noctifab` automatically checks the corresponding environment variable (or resolves `secret:<KEY>` from `secrets.yaml`):

| Provider Category | Provider (`llm.provider`) | Environment Variable(s) | Default Base URL |
|---|---|---|---|
| **LLM** | `openai` | `OPENAI_API_KEY` | `https://api.openai.com/v1` |
| **LLM** | `anthropic` | `ANTHROPIC_API_KEY` | `https://api.anthropic.com/v1` |
| **LLM** | `gemini` | `GEMINI_API_KEY` | `https://generativelanguage.googleapis.com/v1beta` |
| **LLM** | `opencode` | `OPENCODE_API_KEY` | `https://opencode.ai/api/v1` |
| **LLM** | `kimi`, `moonshot` | `KIMI_API_KEY`, `MOONSHOT_API_KEY` | `https://api.moonshot.ai/v1` |
| **LLM** | `groq` | `GROQ_API_KEY` | `https://api.groq.com/openai/v1` |
| **LLM** | `openrouter` | `OPENROUTER_API_KEY` | `https://openrouter.ai/api/v1` |
| **LLM** | `qwen`, `dashscope` | `DASHSCOPE_API_KEY`, `QWEN_API_KEY` | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| **LLM** | `together` | `TOGETHER_API_KEY` | `https://api.together.xyz/v1` |
| **LLM** | `llama`, `meta` | `LLAMA_API_KEY`, `META_API_KEY` | `https://api.together.xyz/v1` |
| **LLM** | `huggingface` | `HUGGINGFACE_API_KEY` | `https://api-inference.huggingface.co/v1` |
| **LLM** | `mistral` | `MISTRAL_API_KEY` | `https://api.mistral.ai/v1` |
| **LLM** | `deepseek` | `DEEPSEEK_API_KEY` | `https://api.deepseek.com/v1` |
| **LLM** | `hermes` | `HERMES_API_KEY` | `https://api.together.xyz/v1` |
| **LLM** | `ollama` | `OLLAMA_API_KEY` *(optional)* | `https://ollama.com/v1` |
| **LLM** | `xai`, `grok` | `XAI_API_KEY`, `GROK_API_KEY` | `https://api.x.ai/v1` |
| **LLM** | `perplexity` | `PERPLEXITY_API_KEY` | `https://api.perplexity.ai` |
| **LLM** | `fireworks` | `FIREWORKS_API_KEY` | `https://api.fireworks.ai/inference/v1` |
| **LLM** | `sambanova` | `SAMBANOVA_API_KEY` | `https://api.sambanova.ai/v1` |
| **LLM** | `cohere` | `COHERE_API_KEY`, `CO_API_KEY` | `https://api.cohere.com/v2` |
| **LLM** | `cerebras` | `CEREBRAS_API_KEY` | `https://api.cerebras.ai/v1` |
| **LLM** | `nvidia` | `NVIDIA_API_KEY` | `https://integrate.api.nvidia.com/v1` |
| **LLM** | `ai21` | `AI21_API_KEY` | `https://api.ai21.com/studio/v1` |
| **LLM** | `upstage` | `UPSTAGE_API_KEY` | `https://api.upstage.ai/v1/solar` |
| **LLM** | *Any (Generic Override)* | `NOCTIFAB_LLM_API_KEY` | *(Provider Default)* |
| **VCS** | `github` | `GITHUB_TOKEN`, `NOCTIFAB_VCS_TOKEN` | `https://api.github.com` |
| **VCS** | `gitlab` | `GITLAB_TOKEN`, `NOCTIFAB_VCS_TOKEN` | `https://gitlab.com/api/v4` |

---

### GitHub CLI (`gh`) Fallback Mechanism

For GitHub VCS operations (`CreatePullRequest` and `MergePullRequest`), if `GITHUB_TOKEN` is missing, empty, or fails authentication against the GitHub REST API:
1. `noctifab` attempts to dynamically retrieve an authentication token using `gh auth token`.
2. If token retrieval is unavailable, `noctifab` invokes the host `gh` CLI directly (`gh pr create` and `gh pr merge`).
3. If both API calls and `gh` CLI commands fail (or if `git push` fails), `noctifab` logs a non-fatal warning and preserves all generated source code locally in the workspace without breaking execution.

---

If `secrets.yaml` does not exist, noctifab proceeds normally — no error is raised. You can still provide credentials via environment variables:

```bash
# Via standard provider environment variables
GEMINI_API_KEY="AIzaSy..." GITHUB_TOKEN="github_pat_..." noctifab start SPEC.md

# Via explicit noctifab environment overrides
NOCTIFAB_LLM_API_KEY="AIzaSy..." NOCTIFAB_VCS_TOKEN="github_pat_..." noctifab start SPEC.md
```

---

## Priority Precedence (highest wins)

To protect sensitive keys from leaking into shell command histories, `noctifab` does not register CLI flags for secrets. The precedence for resolving credentials is:

```
Environment variable  >  secrets.yaml  >  config.yaml literal value
```

*(Note: For non-sensitive configurations like `--agents` or `--storage-provider`, CLI flags are supported and take the highest precedence: CLI flag > Environment variable > secrets.yaml > config.yaml).*

This means you can always override a secret using environment variables or a CI environment without touching any file.

---

## CI/CD Usage

In CI pipelines, prefer environment variables rather than committing `secrets.yaml`:

```yaml
# GitHub Actions example
- name: Run noctifab
  env:
    GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: noctifab start SPEC.md
```

---

## Security Checklist

- [ ] `secrets.yaml` is listed in `.noctifab/.gitignore`
- [ ] `secrets.yaml` is not tracked by git (`git ls-files .noctifab/secrets.yaml` returns nothing)
- [ ] `config.yaml` contains only `secret:` references — no literal token values
- [ ] CI/CD uses injected environment variables, not a committed `secrets.yaml`
