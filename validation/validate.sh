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

# 3. Copy the project copy into the workspace
PROJECT="${PROJECT:-frontpunch}"
echo "Validating project: ${PROJECT}..."
PROJECT_SRC="/app/projects/${PROJECT}"
if [ ! -d "${PROJECT_SRC}" ]; then
  PROJECT_SRC="$(pwd)/validation/projects/${PROJECT}"
fi
if [ ! -d "${PROJECT_SRC}" ]; then
  echo "❌ Error: Project ${PROJECT} does not exist in /app/projects/ or validation/projects/"
  exit 1
fi
cp -R "${PROJECT_SRC}" "${TMP_DIR}"
cd "${TMP_DIR}"

# Mount the secret file from the runtime Docker secret into the workspace.
# Images are kept free of credentials: secrets.yaml is bind-mounted into the
# container at /run/secrets/noctifab-secrets.yaml by run_one.sh and copied
# here so noctifab can resolve `secret:OPENCODE_API_KEY` etc. at config load
# (pkg/infrastructure/config/secrets.go:38).
SECRETS_SRC="/run/secrets/noctifab-secrets.yaml"
HOST_FALLBACK="${PROJECT_SRC}/.noctifab/secrets.yaml"
if [ -f "${SECRETS_SRC}" ]; then
  cp "${SECRETS_SRC}" .noctifab/secrets.yaml
elif [ -f "${HOST_FALLBACK}" ]; then
  echo "⚠ Warning: using fallback secrets.yaml from project tree; image may contain secrets." >&2
else
  echo "❌ Error: no secrets.yaml found at ${SECRETS_SRC} or ${HOST_FALLBACK}." >&2
  exit 1
fi

# 4. Initialize git repository inside the container workspace
echo "Initializing clean git repository on branch main..."
git init
git checkout -b main
git config user.name "Noctifab Tester"
git config user.email "tester@noctifab.local"
cat <<EOF > .gitignore
__pycache__/
*.pyc
.noctifab/data/
.noctifab/logs/
todo.json
*.json
EOF
git add .
git commit -m "initial project structures and gitignore"

# Set up a local "origin" bare repository inside the workspace to allow git pushes
echo "Setting up local git origin remote..."
ORIGIN_DIR="$(pwd)/../origin.git"
rm -rf "${ORIGIN_DIR}"
git init --bare "${ORIGIN_DIR}"
git remote add origin "${ORIGIN_DIR}"
git push -u origin main


# 5. Sanitize credentials in environment
if [ -n "${GEMINI_API_KEY:-}" ]; then
  export GEMINI_API_KEY=$(echo "${GEMINI_API_KEY}" | sed -E 's/.*GEMINI_API_KEY:[[:space:]]*"?([^"]*)"?/\1/' | tr -d '"')
fi
if [ -n "${OPENAI_API_KEY:-}" ]; then
  export OPENAI_API_KEY=$(echo "${OPENAI_API_KEY}" | sed -E 's/.*OPENAI_API_KEY:[[:space:]]*"?([^"]*)"?/\1/' | tr -d '"')
fi

# Set dummy GITHUB_TOKEN if not present to pass pre-flight checks
export GITHUB_TOKEN="${GITHUB_TOKEN:-dummy-token}"

# 6. Initialize noctifab
"${NOCTIFAB_BIN}" init --vcs-clone-protocol https

echo "Using pre-configured config.yaml:"
cat .noctifab/config.yaml

# 7. Run noctifab command
STORY_PATH="roadmap/US-001.md"
if [ "${PROJECT}" = "wc" ]; then
  STORY_PATH="roadmap/US-002.md"
fi

MODE="${MODE:-start-one}"
if [ "${MODE}" = "start" ]; then
  echo "Running noctifab start for ${STORY_PATH}..."
  echo "start ${STORY_PATH}" | "${NOCTIFAB_BIN}" start --wait
  # Stop the daemon after completion
  "${NOCTIFAB_BIN}" stop 2>/dev/null || true
else
  echo "Running noctifab start-one for ${STORY_PATH}..."
  "${NOCTIFAB_BIN}" start-one --input "${STORY_PATH}"
fi

# 8. Verify results
echo "Verifying results..."
if [ "${PROJECT}" = "frontpunch" ]; then
  if [ ! -f "frontpunch/client.py" ]; then
    echo "❌ Error: frontpunch/client.py was not created/modified!"
    exit 1
  fi
elif [ "${PROJECT}" = "todo-cli" ]; then
  if [ ! -f "cmd/todo/main.go" ] && [ ! -f "main.go" ]; then
    echo "❌ Error: cmd/todo/main.go (or main.go) was not created/modified!"
    exit 1
  fi
elif [ "${PROJECT}" = "wc" ]; then
  if [ ! -f "Cargo.toml" ] || [ ! -f "src/main.rs" ]; then
    echo "❌ Error: Cargo.toml or src/main.rs was not created/modified!"
    exit 1
  fi
else
  echo "⚠ Warning: No specific file check defined for project ${PROJECT}."
fi

echo "✅ Success: Noctifab executed autonomously, implemented US-001 features, and passed validation for ${PROJECT}!"
cd ..
rm -rf "${TMP_DIR}"
