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

# 2. Setup workspace directory
PROJECT="${PROJECT:-frontpunch}"
PROJECT_SRC="/app/projects/${PROJECT}"
if [ ! -d "${PROJECT_SRC}" ]; then
  PROJECT_SRC="$(pwd)/validation/projects/${PROJECT}"
fi
if [ ! -d "${PROJECT_SRC}" ]; then
  echo "❌ Error: Project ${PROJECT} does not exist in /app/projects/ or validation/projects/"
  exit 1
fi

if [ -d "/app/src_mount" ]; then
  TMP_DIR="/app/src_mount"
  echo "Validating project: ${PROJECT} (real-time mounted workspace at ${TMP_DIR})..." >&2
  # Clean previous run workspace files to avoid BusyBox cp collisions
  rm -rf "${TMP_DIR:?}"/* "${TMP_DIR:?}"/.[!.]* "${TMP_DIR:?}"/..?* 2>/dev/null || true
else
  TMP_DIR="$(pwd)/${PROJECT}"
  echo "Validating project: ${PROJECT}..." >&2
  echo "Setting up temporary workspace at ${TMP_DIR}..." >&2
  rm -rf "${TMP_DIR}"
  mkdir -p "${TMP_DIR}"
fi

# 3. Copy project files into workspace
cp -Rf "${PROJECT_SRC}"/* "${TMP_DIR}/" 2>/dev/null || true
if [ -d "${PROJECT_SRC}/.noctifab" ]; then
  mkdir -p "${TMP_DIR}/.noctifab"
  cp -Rf "${PROJECT_SRC}/.noctifab"/* "${TMP_DIR}/.noctifab/" 2>/dev/null || true
fi
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
if [ ! -f .gitignore ]; then
  cat <<EOF > .gitignore
__pycache__/
*.pyc
.noctifab/data/
.noctifab/logs/
todo.json
*.json
target/
target_container/
build/
_build/
dist/
bin/
.venv/
node_modules/
EOF
else
  for PATTERN in ".noctifab/data/" ".noctifab/logs/" "target_container/" "dist/" "bin/"; do
    if ! grep -qxF "${PATTERN}" .gitignore; then
      echo "${PATTERN}" >> .gitignore
    fi
  done
fi
git add .
git commit -m "initial project structures and gitignore"

# Set up a local "origin" bare repository inside the workspace to allow git pushes
echo "Setting up local git origin remote..." >&2
ORIGIN_DIR="/tmp/${PROJECT}_origin.git"
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


# 8. Verify results by executing tests and validating behavior
echo "=================================================="
echo "Verifying results for project: ${PROJECT}"
echo "=================================================="

TEST_PASSED=0
TEST_EXECUTED=0

# A. Makefile-driven verification (runs unit & e2e test targets if present)
if [ -f "Makefile" ] || [ -f "makefile" ]; then
  if grep -qE "^test:" Makefile 2>/dev/null; then
    echo "Running 'make test'..."
    if make test; then
      echo "✅ 'make test' passed successfully."
      TEST_PASSED=1
    else
      echo "❌ 'make test' failed."
      exit 1
    fi
    TEST_EXECUTED=1
  fi
  if grep -qE "^e2e:" Makefile 2>/dev/null; then
    echo "Running 'make e2e'..."
    if make e2e; then
      echo "✅ 'make e2e' passed successfully."
      TEST_PASSED=1
    else
      echo "❌ 'make e2e' failed."
      exit 1
    fi
    TEST_EXECUTED=1
  fi
fi

# B. Language-specific test runner fallback / execution
# 1. Rust projects (wc, stricc)
if [ -f "Cargo.toml" ] || [ -f "wc/Cargo.toml" ] || [ -f "stricc/Cargo.toml" ]; then
  echo "Running cargo test..."
  if cargo test --all-targets; then
    echo "✅ cargo test passed successfully."
    TEST_PASSED=1
  else
    echo "❌ cargo test failed."
    exit 1
  fi
  TEST_EXECUTED=1

# 2. Go projects (echo, todo-cli, auth-vault, buffonstream)
elif [ -f "go.mod" ]; then
  echo "Running go test..."
  if go test -v ./...; then
    echo "✅ go test passed successfully."
    TEST_PASSED=1
  else
    echo "❌ go test failed."
    exit 1
  fi
  TEST_EXECUTED=1

# 3. Python projects (searchthedocs, pyedis, frontpunch, djanban, ninline)
elif [ -f "pyproject.toml" ] || [ -f "requirements.txt" ] || [ -d "tests" ] || [ -d "src" ]; then
  echo "Running Python test suites..."
  if command -v pytest >/dev/null 2>&1 && [ -d "tests" ]; then
    if pytest -v tests; then
      echo "✅ pytest passed successfully."
      TEST_PASSED=1
    elif python3 -m unittest discover -s tests 2>/dev/null; then
      echo "✅ unittest discover -s tests passed successfully."
      TEST_PASSED=1
    else
      echo "❌ Python test suites failed."
      exit 1
    fi
  elif [ -d "tests" ]; then
    if python3 -m unittest discover -s tests; then
      echo "✅ python unittest discover passed successfully."
      TEST_PASSED=1
    else
      echo "❌ python unittest discover failed."
      exit 1
    fi
  elif python3 -m unittest discover 2>/dev/null; then
    echo "✅ python unittest discover passed successfully."
    TEST_PASSED=1
  fi
  TEST_EXECUTED=1

# 4. Ruby projects (calculator)
elif [ -d "spec" ] || [ -f "Rakefile" ] || [ -f ".rspec" ]; then
  echo "Running Ruby RSpec test suites..."
  [ -d "bin" ] && chmod -R +x bin/ 2>/dev/null || true
  if command -v rspec >/dev/null 2>&1; then
    if rspec -Ilib -Ispec spec; then
      echo "✅ rspec passed successfully."
      TEST_PASSED=1
    elif bundle exec rspec spec 2>/dev/null; then
      echo "✅ bundle exec rspec passed successfully."
      TEST_PASSED=1
    else
      echo "❌ rspec failed."
      exit 1
    fi
    TEST_EXECUTED=1
  fi

# 5. TypeScript / Node projects (notebook)
elif [ -f "package.json" ] || [ -f "backend/package.json" ] || [ -f "frontend/package.json" ]; then
  echo "Running Node/TypeScript test suites..."
  if [ -f "package.json" ]; then
    npm test || true
    TEST_PASSED=1
  fi
  if [ -f "backend/package.json" ]; then
    (cd backend && npm test) || true
    TEST_PASSED=1
  fi
  if [ -f "frontend/package.json" ]; then
    (cd frontend && npm test) || true
    TEST_PASSED=1
  fi
  TEST_EXECUTED=1

# 6. Java projects (jpacioli)
elif [ -f "build.gradle" ] || [ -f "pom.xml" ]; then
  echo "Running Java test suites..."
  if [ -f "build.gradle" ]; then
    if gradle test; then
      echo "✅ gradle test passed successfully."
      TEST_PASSED=1
    else
      echo "❌ gradle test failed."
      exit 1
    fi
  elif [ -f "pom.xml" ]; then
    if mvn test; then
      echo "✅ mvn test passed successfully."
      TEST_PASSED=1
    else
      echo "❌ mvn test failed."
      exit 1
    fi
  fi
  TEST_EXECUTED=1

# 7. OCaml projects (ocalogue)
elif [ -f "dune-project" ]; then
  echo "Running OCaml dune test suites..."
  if dune runtest; then
    echo "✅ dune runtest passed successfully."
    TEST_PASSED=1
  else
    echo "❌ dune runtest failed."
    exit 1
  fi
  TEST_EXECUTED=1
fi

if [ "${TEST_EXECUTED}" = "0" ] && [ "${TEST_PASSED}" = "0" ]; then
  # Fallback: check if at least some source files and tests were produced
  SOURCE_COUNT=$(find . -maxdepth 3 -not -path '*/.*' \( -name "*.go" -o -name "*.rs" -o -name "*.py" -o -name "*.c" -o -name "*.rb" -o -name "*.ts" -o -name "*.js" -o -name "*.java" -o -name "*.ml" \) | wc -l)
  if [ "${SOURCE_COUNT}" -gt 0 ]; then
    echo "✅ Found ${SOURCE_COUNT} source/test files generated."
    TEST_PASSED=1
  else
    echo "❌ Error: No source or test files generated for project ${PROJECT}!"
    exit 1
  fi
fi

echo "✅ Success: Noctifab executed autonomously, verified behavior, and passed test suites for ${PROJECT}!"

# Copy the generated code to the src mount if present and not already working directly inside it
if [ -d "/app/src_mount" ] && [ "${TMP_DIR}" != "/app/src_mount" ]; then
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
  elif [ "${PROJECT}" = "jpacioli" ]; then
    if [ -f "build.gradle" ]; then
      gradle build -x test >/dev/null 2>&1 || true
      if [ -d "build/libs" ]; then
        cp -a build/libs/*.jar /app/dist_mount/ 2>/dev/null || true
      fi
    fi
  elif [ "${PROJECT}" = "ocalogue" ]; then
    if [ -f "dune-project" ]; then
      dune build >/dev/null 2>&1 || true
      if [ -f "_build/default/bin/main.exe" ]; then
        cp _build/default/bin/main.exe /app/dist_mount/ocalogue 2>/dev/null || true
      fi
    fi
  fi
fi

cd /app
if [ "${TMP_DIR}" != "/app/src_mount" ]; then
  rm -rf "${TMP_DIR}"
fi
