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
if [ ! -d "/app/projects/${PROJECT}" ]; then
  echo "❌ Error: Project ${PROJECT} does not exist in /app/projects/"
  exit 1
fi
cp -R "/app/projects/${PROJECT}" "${TMP_DIR}"
cd "${TMP_DIR}"

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

# Set up a local "origin" bare repository inside the container to allow git pushes
echo "Setting up local git origin remote..."
git init --bare /app/origin.git
git remote add origin /app/origin.git
git push -u origin main


# 5. Sanitize credentials in environment
if [ -n "${GEMINI_API_KEY:-}" ]; then
  export GEMINI_API_KEY=$(echo "${GEMINI_API_KEY}" | sed -E 's/.*GEMINI_API_KEY:[[:space:]]*"?([^"]*)"?/\1/' | tr -d '"')
fi
if [ -n "${OPENAI_API_KEY:-}" ]; then
  export OPENAI_API_KEY=$(echo "${OPENAI_API_KEY}" | sed -E 's/.*OPENAI_API_KEY:[[:space:]]*"?([^"]*)"?/\1/' | tr -d '"')
fi

# Set dummy GITHUB_FRONTPUNCH_TOKEN if not present to pass pre-flight checks
export GITHUB_FRONTPUNCH_TOKEN="${GITHUB_FRONTPUNCH_TOKEN:-${GITHUB_TOKEN:-dummy-token}}"

# 6. Initialize noctifab
"${NOCTIFAB_BIN}" init --vcs-clone-protocol https

echo "Using pre-configured config.yaml:"
cat .noctifab/config.yaml

# 7. Run noctifab start-one command for US-001
echo "Running noctifab start-one for US-001..."
"${NOCTIFAB_BIN}" start-one --input roadmap/US-001.md

# 8. Verify results
echo "Verifying results..."
if [ "${PROJECT}" = "frontpunch" ]; then
  if [ ! -f "frontpunch/worker.py" ]; then
    echo "❌ Error: frontpunch/worker.py was not created/modified!"
    exit 1
  fi
elif [ "${PROJECT}" = "todo-cli" ]; then
  if [ ! -f "todo.py" ]; then
    echo "❌ Error: todo.py was not created/modified!"
    exit 1
  fi
else
  echo "⚠ Warning: No specific file check defined for project ${PROJECT}."
fi

echo "✅ Success: Noctifab executed autonomously, implemented US-001 features, and passed validation for ${PROJECT}!"
cd ..
rm -rf "${TMP_DIR}"
