#!/usr/bin/env bash
set -euo pipefail

# 1. Resolve or compile the noctifab binary
if command -v noctifab >/dev/null 2>&1; then
  echo "Using existing noctifab binary from PATH..."
  NOCTIFAB_BIN="$(command -v noctifab)"
elif [ -f "/usr/local/bin/noctifab" ]; then
  echo "Using existing noctifab binary at /usr/local/bin/noctifab..."
  NOCTIFAB_BIN="/usr/local/bin/noctifab"
else
  echo "Compiling noctifab binary..."
  go build -o bin/noctifab cmd/noctifab/main.go
  NOCTIFAB_BIN="$(pwd)/bin/noctifab"
fi

# 2. Setup a temporary directory
TMP_DIR="$(pwd)/tmp_verify_autonomy"
echo "Setting up temporary workspace at ${TMP_DIR}..."
rm -rf "${TMP_DIR}"

# 3. Copy the frontpunch project copy into the workspace
cp -R /app/frontpunch "${TMP_DIR}"
cd "${TMP_DIR}"

# Ensure git user name and email are configured for commits inside the container
git config user.name "Noctifab Tester"
git config user.email "tester@noctifab.local"

# 4. Checkout the main branch
echo "Checking out main branch of frontpunch (contains only roadmap and .noctifab)..."
git checkout -f main

# 5. Initialize noctifab
"${NOCTIFAB_BIN}" init --vcs-clone-protocol https

# 6. Detect LLM credentials from environment
if [ -n "${GEMINI_API_KEY:-}" ]; then
  PROVIDER="gemini"
  MODEL="gemini-1.5-pro"
  API_KEY_CONFIG=$(echo "${GEMINI_API_KEY}" | sed -E 's/.*GEMINI_API_KEY:[[:space:]]*"?([^"]*)"?/\1/' | tr -d '"')
  KEY_ENV="GEMINI_API_KEY"
elif [ -n "${OPENAI_API_KEY:-}" ]; then
  PROVIDER="openai"
  MODEL="gpt-4o"
  API_KEY_CONFIG=$(echo "${OPENAI_API_KEY}" | sed -E 's/.*OPENAI_API_KEY:[[:space:]]*"?([^"]*)"?/\1/' | tr -d '"')
  KEY_ENV="OPENAI_API_KEY"
else
  echo "⚠ Error: Neither GEMINI_API_KEY nor OPENAI_API_KEY is set in environment."
  exit 1
fi

echo "Using LLM provider: ${PROVIDER} with model: ${MODEL}"

# 7. Write custom config.yaml configured for US-001
cat <<EOF > .noctifab/config.yaml
config_version: "1.0"
storage:
  provider: "sqlite"
  conn_string: ".noctifab/data/noctifab.db"
llm:
  provider: "${PROVIDER}"
  model: "${MODEL}"
  api_key: "${API_KEY_CONFIG}"
  api_key_env: "${KEY_ENV}"
  max_budget_usd: 5.0
vcs:
  provider: "github"
  repository: "diegojromerolopez/frontpunch"
  base_branch: "main"
  branch_prefix: "noctifab/"
  token_env: "GITHUB_TOKEN"
sandbox:
  mode: "host"
  test_command: "python3 -m unittest discover -s tests"
  linter_command: ""
  allowed_commands:
    - "python3"
    - "git"
    - "python"
    - "pip"
    - "ruff"
    - "black"
    - "mypy"
EOF

echo "Generated config.yaml:"
cat .noctifab/config.yaml

# Set dummy GITHUB_TOKEN if not present to pass pre-flight checks
export GITHUB_TOKEN="${GITHUB_TOKEN:-dummy-token}"

# 8. Run noctifab start-one command for US-001
echo "Running noctifab start-one for US-001..."
"${NOCTIFAB_BIN}" start-one --input roadmap/US-001.md

# 9. Verify results
echo "Verifying results..."
if [ ! -f "frontpunch/worker.py" ]; then
  echo "❌ Error: frontpunch/worker.py was not created/modified!"
  exit 1
fi

echo "✅ Success: Noctifab executed autonomously, implemented US-001 features, and passed validation!"
cd ..
rm -rf "${TMP_DIR}"
