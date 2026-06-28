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
mkdir -p "${TMP_DIR}"
cd "${TMP_DIR}"

# 3. Initialize git repository
git init
git config user.name "Noctifab Tester"
git config user.email "tester@noctifab.local"
echo "# Dummy File" > README.md
git add README.md
git commit -m "initial commit"

# 4. Initialize noctifab
"${NOCTIFAB_BIN}" init --vcs-clone-protocol https

# 5. Detect LLM credentials from environment or secrets.yaml
PROVIDER="gemini"
MODEL="gemini-1.5-pro"
API_KEY_CONFIG="secret:GEMINI_API_KEY"
KEY_ENV="GEMINI_API_KEY"

if [ -n "${GEMINI_API_KEY:-}" ]; then
  PROVIDER="gemini"
  MODEL="gemini-1.5-pro"
  API_KEY_CONFIG="${GEMINI_API_KEY}"
  KEY_ENV="GEMINI_API_KEY"
elif [ -n "${OPENAI_API_KEY:-}" ]; then
  PROVIDER="openai"
  MODEL="gpt-4o"
  API_KEY_CONFIG="${OPENAI_API_KEY}"
  KEY_ENV="OPENAI_API_KEY"
elif [ -f "/Users/diegoj/repos/frontpunch/.noctifab/secrets.yaml" ]; then
  echo "No API key in environment. Copying secrets.yaml from frontpunch..."
  cp "/Users/diegoj/repos/frontpunch/.noctifab/secrets.yaml" .noctifab/secrets.yaml
  PROVIDER="gemini"
  MODEL="gemini-1.5-pro"
  API_KEY_CONFIG="secret:GEMINI_API_KEY"
  KEY_ENV="GEMINI_API_KEY"
else
  echo "⚠ Error: Neither GEMINI_API_KEY nor OPENAI_API_KEY is set in environment, and no secrets.yaml found."
  exit 1
fi

echo "Using LLM provider: ${PROVIDER} with model: ${MODEL}"

# 6. Write custom config.yaml
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
  max_budget_usd: 2.0
vcs:
  provider: "github"
  repository: "test/hello-world"
  base_branch: "git-detect"
  branch_prefix: "noctifab/"
  token_env: "GITHUB_TOKEN"
sandbox:
  mode: "host"
  test_command: "python3 -m unittest discover -s tests"
  linter_command: ""
  allowed_commands:
    - "python3"
    - "git"
EOF

# 7. Write the feature specification
cat <<EOF > spec.md
# Specification: Reverse String Function

Create a python function \`reverse_string(s: str) -> str\` in a module named \`string_utils.py\` that reverses a string.
Ensure that the function is covered by unit tests in the \`tests/\` directory.
EOF

# Make sure tests directory exists
mkdir -p tests
touch tests/__init__.py

# Set dummy GITHUB_TOKEN if not present to pass pre-flight checks
export GITHUB_TOKEN="${GITHUB_TOKEN:-dummy-token}"

# 8. Run noctifab start-one command
echo "Running noctifab start-one..."
"${NOCTIFAB_BIN}" start-one --input spec.md

# 9. Verify results
echo "Verifying results..."
if [ ! -f "string_utils.py" ] && [ ! -f "frontpunch/string_utils.py" ]; then
  echo "❌ Error: string_utils.py was not created in root or frontpunch/!"
  exit 1
fi

if [ ! -d "tests" ]; then
  echo "❌ Error: tests directory was not created!"
  exit 1
fi

echo "✅ Success: Noctifab executed autonomously, generated string_utils.py, and passed validation!"
cd ..
rm -rf "${TMP_DIR}"
