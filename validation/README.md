# Noctifab E2E Autonomy Validation

This directory contains resources to run fully containerized, isolated, end-to-end (E2E) integration checks of `noctifab` implementing features autonomously inside a target project based only on its user story roadmap and configs.

## Available Projects

1. **`frontpunch`**: Checks out the base `main` branch of the frontpunch spec and executes `roadmap/US-001.md`. Validates that the Python worker infrastructure is successfully planned, written, tested, and passing.
2. **`todo-cli`**: Checks out the base `main` branch of a clean Todo CLI project spec and executes `roadmap/US-001.md`. Validates that task addition and listing commands are planned, implemented in a single `todo.py` module, tested, and validated.

---

## How It Works

1. **Host Setup**:
   The `Makefile` target builds a Docker image (`Dockerfile.validation`) containing `noctifab` compiled in a multi-stage builder, along with the static validation files under `validation/projects/`.
2. **Execution**:
   The container starts with no volume mounts (100% isolated from your host filesystem). 
3. **Workspace Initialization**:
   The validation script (`validate.sh`):
   - Copies the selected project template into a temporary runtime directory (`tmp_verify_autonomy/`).
   - Runs `git init`, checks out branch `main`, and commits the initial configuration files.
   - Runs `noctifab start-one` to autonomously decompose and implement the selected User Story.
4. **Validation Check**:
   The script checks that the target source files are created/modified and that the test suite executes successfully.

---

## How to Run

### 1. Credentials Setup (ignored in git)
To run validation, you need a Gemini API Key. Place it in the `frontpunch` configuration directory so the Makefile can extract it automatically:
```yaml
# validation/projects/frontpunch/.noctifab/secrets.yaml
GEMINI_API_KEY: "your-actual-api-key"
```
Alternatively, set the key directly in your terminal environment:
```bash
export GEMINI_API_KEY="your-actual-api-key"
```

### 2. Execution Command
Execute the default check (which uses `frontpunch`):
```bash
make validate
```

Or run validation on a specific project by passing the `PROJECT` parameter:
```bash
make validate PROJECT=todo-cli
```
```bash
make validate PROJECT=frontpunch
```
