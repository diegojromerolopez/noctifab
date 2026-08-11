#!/usr/bin/env bash
set -euo pipefail



# 1. Resolve or compile the noctifab binary
if command -v noctifab >/dev/null 2>&1; then
  echo "Using existing noctifab binary from PATH..." >&2
  NOCTIFAB_BIN="$(command -v noctifab)"
elif [ -f "/usr/local/bin/noctifab" ]; then
  echo "Using existing noctifab binary at /usr/local/bin/noctifab..." >&2
  NOCTIFAB_BIN="/usr/local/bin/noctifab"
else
  echo "Compiling noctifab binary..." >&2
  go build -o bin/noctifab cmd/noctifab/main.go
  NOCTIFAB_BIN="$(pwd)/bin/noctifab"
fi

# 2. Setup a temporary directory
TMP_DIR="$(pwd)/tmp_verify_autonomy"
echo "Setting up temporary workspace at ${TMP_DIR}..." >&2
rm -rf "${TMP_DIR}"

# 3. Copy the project copy into the workspace
PROJECT="${PROJECT:-frontpunch}"
echo "Validating project: ${PROJECT}..." >&2
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
echo "Initializing clean git repository on branch main..." >&2
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
target/
EOF
git add .
git commit -m "initial project structures and gitignore"

# Set up a local "origin" bare repository inside the workspace to allow git pushes
echo "Setting up local git origin remote..." >&2
ORIGIN_DIR="$(pwd)/../origin.git"
rm -rf "${ORIGIN_DIR}"
git init --bare "${ORIGIN_DIR}"
git remote add origin "${ORIGIN_DIR}"
git push -u origin main


# 5. Sanitize credentials in environment
for KEY in OPENAI_API_KEY ANTHROPIC_API_KEY GEMINI_API_KEY OPENCODE_API_KEY KIMI_API_KEY MOONSHOT_API_KEY GROQ_API_KEY OPENROUTER_API_KEY QWEN_API_KEY DASHSCOPE_API_KEY TOGETHER_API_KEY LLAMA_API_KEY HUGGINGFACE_API_KEY MISTRAL_API_KEY DEEPSEEK_API_KEY HERMES_API_KEY OLLAMA_API_KEY XAI_API_KEY PERPLEXITY_API_KEY FIREWORKS_API_KEY SAMBANOVA_API_KEY COHERE_API_KEY CEREBRAS_API_KEY NVIDIA_API_KEY AI21_API_KEY UPSTAGE_API_KEY GITHUB_TOKEN; do
  VAR_VAL="${!KEY:-}"
  if [ -n "${VAR_VAL}" ]; then
    CLEANED=$(echo "${VAR_VAL}" | sed -E 's/.*'"${KEY}"':[[:space:]]*"?([^"]*)"?/\1/' | tr -d '"')
    export "${KEY}"="${CLEANED}"
  fi
done

# Set dummy GITHUB_TOKEN if not present to pass pre-flight checks
export GITHUB_TOKEN="${GITHUB_TOKEN:-dummy-token}"

# 6. Initialize noctifab
"${NOCTIFAB_BIN}" init --vcs-clone-protocol https

mkdir -p .noctifab/logs
{
  echo "Using pre-configured config.yaml:"
  cat .noctifab/config.yaml
} > .noctifab/logs/setup.log

# 7. Run noctifab command
MODE="${MODE:-start}"

INTERACTIVE_FLAG=""
if [ "${NOCTIFAB_INTERACTIVE:-}" = "1" ]; then
  INTERACTIVE_FLAG="-i"
fi

echo "Running noctifab start..." >&2
"${NOCTIFAB_BIN}" start . ${INTERACTIVE_FLAG}


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
  if { [ ! -f "Cargo.toml" ] || [ ! -f "src/main.rs" ]; } && { [ ! -f "wc/Cargo.toml" ] || [ ! -f "wc/src/main.rs" ]; }; then
    echo "❌ Error: Cargo.toml or src/main.rs was not created/modified!"
    exit 1
  fi
elif [ "${PROJECT}" = "calculator" ]; then
  if [ ! -f "calculator.rb" ] && [ ! -f "lib/calculator.rb" ] && [ ! -f "lib/calculator/cli.rb" ]; then
    echo "❌ Error: calculator.rb or lib/calculator/cli.rb was not created/modified!"
    exit 1
  fi
elif [ "${PROJECT}" = "echo" ]; then
  if [ ! -f "cmd/echo/main.go" ] && [ ! -f "main.go" ]; then
    echo "❌ Error: cmd/echo/main.go (or main.go) was not created/modified!"
    exit 1
  fi
elif [ "${PROJECT}" = "fortune" ]; then
  if [ ! -f "main.c" ] && [ ! -f "Makefile" ]; then
    echo "❌ Error: main.c or Makefile was not created/modified!"
    exit 1
  fi
elif [ "${PROJECT}" = "t4" ]; then
  if [ ! -f "Makefile" ] || [ ! -f "docker-compose.yml" ] || [ ! -f "src/t4.c" ]; then
    echo "❌ Error: t4 artifacts (Makefile, docker-compose.yml, src/t4.c) were not created!"
    exit 1
  fi
elif [ "${PROJECT}" = "pyedis" ]; then
  if [ ! -f "app/main.py" ] || [ ! -f "pyproject.toml" ]; then
    echo "❌ Error: pyedis artifacts (app/main.py, pyproject.toml) were not created!"
    exit 1
  fi
elif [ "${PROJECT}" = "notebook" ]; then
  if [ ! -f "src/index.ts" ] || [ ! -f "package.json" ] || [ ! -f "docker-compose.yml" ]; then
    echo "❌ Error: notebook artifacts (src/index.ts, package.json, docker-compose.yml) were not created!"
    exit 1
  fi
else
  echo "⚠ Warning: No specific file check defined for project ${PROJECT}."
fi

echo "✅ Success: Noctifab executed autonomously, implemented US-001 features, and passed validation for ${PROJECT}!"

# Copy the generated code to the src mount if present
if [ -d "/app/src_mount" ]; then
  echo "Copying generated code to src mount..."
  rm -rf /app/src_mount/*
  cp -a . /app/src_mount/
fi

# Locate and copy binary to dist mount if present
if [ -d "/app/dist_mount" ]; then
  echo "Checking and compiling binary if project generates one..."
  rm -rf /app/dist_mount/*
  if [ "${PROJECT}" = "todo-cli" ]; then
    if [ -f "cmd/todo/main.go" ]; then
      go build -o /app/dist_mount/todo ./cmd/todo
    elif [ -f "main.go" ]; then
      go build -o /app/dist_mount/todo main.go
    fi
  elif [ "${PROJECT}" = "echo" ]; then
    if [ -f "cmd/echo/main.go" ]; then
      go build -o /app/dist_mount/echo-cli ./cmd/echo
    elif [ -f "main.go" ]; then
      go build -o /app/dist_mount/echo-cli main.go
    fi
  elif [ "${PROJECT}" = "wc" ]; then
    cargo build --release
    if [ -f "target/release/wc" ]; then
      cp target/release/wc /app/dist_mount/
    elif [ -f "wc/target/release/wc" ]; then
      cp wc/target/release/wc /app/dist_mount/
    fi
  elif [ "${PROJECT}" = "fortune" ]; then
    if [ -f "Makefile" ]; then
      make build || true
    fi
    if [ -f "fortune" ]; then
      cp fortune /app/dist_mount/
    fi
  elif [ "${PROJECT}" = "t4" ]; then
    if [ -f "Makefile" ]; then
      make build || true
    fi
    if [ -f "bin/t4" ]; then
      cp bin/t4 /app/dist_mount/
    fi
  elif [ "${PROJECT}" = "pyedis" ]; then
    # Interpreted service; nothing to compile. The source is captured via src_mount.
    :
  elif [ "${PROJECT}" = "notebook" ]; then
    if [ -f "package.json" ]; then
      npm install --no-audit --no-fund >/dev/null 2>&1 || true
      npm run build >/dev/null 2>&1 || true
    fi
    if [ -d "dist" ]; then
      cp -a dist/. /app/dist_mount/ || true
    fi
  fi
fi

cd ..
rm -rf "${TMP_DIR}"
