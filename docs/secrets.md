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

**Resolution order for a given field:**

1. `secrets.yaml` reference (resolved at YAML load time)
2. Environment variable (applied after)
3. CLI flag (applied last, highest precedence)

---

## Setup

### 1. Create `secrets.yaml`

Place `secrets.yaml` in the same directory as `config.yaml` (i.e. `.noctifab/secrets.yaml`). The format is a flat YAML key-value map:

```yaml
# .noctifab/secrets.yaml
# This file is gitignored — never commit it.

GITHUB_FRONTPUNCH_TOKEN: "github_pat_11AA4TEXQ0..."
GEMINI_API_KEY: "AIzaSyATKC77..."
JIRA_API_TOKEN: "your-jira-token"
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
  token: "secret:GITHUB_FRONTPUNCH_TOKEN" # resolved from secrets.yaml

jira:
  url: "https://mycompany.atlassian.net"
  user: "secret:JIRA_USER"
  token: "secret:JIRA_API_TOKEN"
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
| Jira API token | `jira.token` |
| Jira user/email | `jira.user` |
| Jira instance URL | `jira.url` |

---

## `secrets.yaml` is Optional

If `secrets.yaml` does not exist, noctifab proceeds normally — no error is raised. You can still provide credentials via environment variables or CLI flags:

```bash
# Via environment variable
GEMINI_API_KEY="AIzaSy..." noctifab start-one --input roadmap/US-001.md

# Via CLI flag
noctifab start-one --input roadmap/US-001.md --llm-api-key "AIzaSy..." --vcs-token "github_pat_..."
```

---

## Priority Precedence (highest wins)

```
CLI flag  >  Environment variable  >  secrets.yaml  >  config.yaml literal value
```

This means you can always override a secret from the command line or CI environment without touching any file.

---

## CI/CD Usage

In CI pipelines, prefer environment variables or CLI flags rather than committing `secrets.yaml`:

```yaml
# GitHub Actions example
- name: Run noctifab
  env:
    GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}
    GITHUB_FRONTPUNCH_TOKEN: ${{ secrets.FRONTPUNCH_TOKEN }}
  run: noctifab start-one --input roadmap/US-001.md
```

---

## Security Checklist

- [ ] `secrets.yaml` is listed in `.noctifab/.gitignore`
- [ ] `secrets.yaml` is not tracked by git (`git ls-files .noctifab/secrets.yaml` returns nothing)
- [ ] `config.yaml` contains only `secret:` references — no literal token values
- [ ] CI/CD uses injected environment variables, not a committed `secrets.yaml`
