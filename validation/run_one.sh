#!/usr/bin/env bash
# Build a per-project noctifab-validation image, run the container in the
# background, capture its combined stdout/stderr to .validation-logs/<project>.log,
# wait for it to exit, then call gen_feedback.py to write the project's
# <PROJECT>_FEEDBACK.md markdown file at the repository root.
#
# Usage:
#   run_one.sh <project>            # uses the default targets for the project
#   run_one.sh <project> <image>    # override image tag (e.g. for a pre-built base)
#
# The container is self-contained: `secrets.yaml` (baked into each
# `noctifab-validation:<project>` image at build time) holds the LLM key, and
# noctifab resolves `secret:OPENCODE_API_KEY` from it at config load
# (`pkg/infrastructure/config/secrets.go`). `validate.sh` itself sets a dummy
# `GITHUB_TOKEN` for pre-flight checks. No host credentials need to be passed
# in via `-e`.
#
# Environment:
#   NOCTIFAB_BUILD_DIR       (optional) – repo root; defaults to script parent/..
#   NOCTIFAB_LOG_DIR         (optional) – log dir; defaults to "$root/.validation-logs"
#   NOCTIFAB_SKIP_BUILD      (optional) – "1" to skip per-project image build
#   MODE                     (optional) – validate.sh MODE (start)
set -euo pipefail

INTERACTIVE=0
if [ "${1:-}" = "-i" ]; then
  INTERACTIVE=1
  shift
fi

PROJECT="${1:?run_one.sh: missing project name}"
IMAGE_TAG_OVERRIDE="${2:-}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="${NOCTIFAB_BUILD_DIR:-$(cd "${SCRIPT_DIR}/.." && pwd)}"

# Clean up output directory contents from previous runs, preserving base directories
# to avoid Docker Desktop mount synchronization race conditions on macOS hosts.
if [ -d "${ROOT}/validation/projects/${PROJECT}/output" ]; then
  find "${ROOT}/validation/projects/${PROJECT}/output" -mindepth 1 -maxdepth 1 -not -name "log" -exec rm -rf {} + || true
  rm -f "${ROOT}/validation/projects/${PROJECT}/output/log"/* || true
else
  mkdir -p "${ROOT}/validation/projects/${PROJECT}/output"
fi

LOG_DIR="${NOCTIFAB_LOG_DIR:-${ROOT}/validation/projects/${PROJECT}/output/log}"
mkdir -p "${LOG_DIR}"

# Per-project artifact file expectations. Keep these in sync with validate.sh
# so the feedback generator matches the same "artifact check" verdicts.
case "${PROJECT}" in
  frontpunch) TARGETS="frontpunch/client.py" ;;
  todo-cli)   TARGETS="cmd/todo/main.go" ;;
  wc)         TARGETS="Cargo.toml;src/main.rs" ;;
  calculator) TARGETS="calculator.rb;lib/calculator/cli.rb" ;;
  echo)       TARGETS="cmd/echo/main.go" ;;
  fortune)    TARGETS="main.c;Makefile" ;;
  t4)         TARGETS="Makefile;docker-compose.yml;src/t4.c" ;;
  pyedis)     TARGETS="app/main.py;pyproject.toml" ;;
  notebook)   TARGETS="src/index.ts;package.json;docker-compose.yml" ;;
  djanban)    TARGETS="manage.py;pyproject.toml;djanban/settings.py" ;;
  *)          TARGETS="" ;;
esac

# 1. Resolve the image (build if missing or if force-rebuild requested).
IMAGE="${IMAGE_TAG_OVERRIDE:-noctifab-validation:${PROJECT}}"
if [ -z "${NOCTIFAB_SKIP_BUILD:-}" ] || [ -z "$(docker images -q "${IMAGE}" 2>/dev/null)" ]; then
  TS="$(date +%H:%M:%S)"
  echo "[${TS}] building base image noctifab-validation:base..."
  docker build \
    --no-cache \
    -f "${ROOT}/validation/Dockerfile.validation" \
    -t "noctifab-validation:base" "${ROOT}" >&2

  echo "[${TS}] building ${PROJECT} image from ${ROOT}/validation/projects/${PROJECT}/Dockerfile..."
  docker build \
    --no-cache \
    -f "${ROOT}/validation/projects/${PROJECT}/Dockerfile" \
    -t "${IMAGE}" "${ROOT}" >&2
fi

# 2. Launch the validation container in the background.
CONTAINER="validate-${PROJECT}-$$"
LOG_FILE="${LOG_DIR}/${PROJECT}.log"
TS="$(date +%H:%M:%S)"
echo "[${TS}] launching ${CONTAINER} (project=${PROJECT}, image=${IMAGE})..."

# Resolve the host secrets.yaml to bind-mount into the container as a Docker
# runtime secret. Images are kept credential-free: secrets.yaml is mounted at
# /run/secrets/noctifab-secrets.yaml and copied into the temporary workspace
# by validate.sh so noctifab can resolve `secret:OPENCODE_API_KEY` etc. at
# config load (pkg/infrastructure/config/secrets.go:38).
SECRETS_FILE="${NOCTIFAB_SECRETS_FILE:-${ROOT}/validation/projects/${PROJECT}/.noctifab/secrets.yaml}"
if [ ! -f "${SECRETS_FILE}" ]; then
  echo "run_one.sh[${PROJECT}]: secrets.yaml not found at ${SECRETS_FILE}." >&2
  echo "Place it there, or set NOCTIFAB_SECRETS_FILE=<path>." >&2
  exit 2
fi

# Ensure host mount points exist for source code, compiled binaries, and execution report
SRC_DIR="${ROOT}/validation/projects/${PROJECT}/output/src"
DIST_DIR="${ROOT}/validation/projects/${PROJECT}/output/dist"
REPORT_DIR="${ROOT}/validation/projects/${PROJECT}/output/report"
mkdir -p "${SRC_DIR}"
mkdir -p "${DIST_DIR}"
mkdir -p "${REPORT_DIR}"
rm -f "${REPORT_DIR}"/* || true

# Prepare cache directories on host to speed up compiler and package manager resolution
CACHE_ARGS=()
if [ -n "${HOME:-}" ] && [ -d "${HOME}" ]; then
  PARENT_CACHE="${HOME}/.noctifab/cache"
  mkdir -p "${PARENT_CACHE}/cargo/registry" "${PARENT_CACHE}/cargo/git" \
           "${PARENT_CACHE}/go-build" "${PARENT_CACHE}/go-mod" \
           "${PARENT_CACHE}/pip" "${PARENT_CACHE}/npm" "${PARENT_CACHE}/m2"
  CACHE_ARGS+=(
    -v "${PARENT_CACHE}:/root/.cache"
    -v "${PARENT_CACHE}/cargo/registry:/usr/local/cargo/registry"
    -v "${PARENT_CACHE}/cargo/git:/usr/local/cargo/git"
    -v "${PARENT_CACHE}/go-mod:/go/pkg/mod"
  )
fi

# Add brief sleep to guarantee Docker Desktop filesystem mount synchronization
sleep 1

# Run with --rm so the container is cleaned up after exit; capture combined
# stdout/stderr to the log file. The bind-mounts include the read-only
# secrets.yaml, and read-write source/dist/report folders.
set +e
if [ "${INTERACTIVE}" = "1" ]; then
  echo "Interactive validation run for ${PROJECT}. Console output printed to terminal." >"${LOG_FILE}"
  docker run \
    -it \
    --rm \
    --name "${CONTAINER}" \
    "${CACHE_ARGS[@]}" \
    -v "${SECRETS_FILE}:/run/secrets/noctifab-secrets.yaml:ro" \
    -v "${SRC_DIR}:/app/src_mount" \
    -v "${DIST_DIR}:/app/dist_mount" \
    -v "${REPORT_DIR}:/app/report_mount" \
    -e PROJECT="${PROJECT}" \
    -e MODE="${MODE:-start}" \
    -e NOCTIFAB_INTERACTIVE="${INTERACTIVE}" \
    "${IMAGE}"
  EXIT_CODE=$?
else
  docker run \
    --rm \
    --name "${CONTAINER}" \
    "${CACHE_ARGS[@]}" \
    -v "${SECRETS_FILE}:/run/secrets/noctifab-secrets.yaml:ro" \
    -v "${SRC_DIR}:/app/src_mount" \
    -v "${DIST_DIR}:/app/dist_mount" \
    -v "${REPORT_DIR}:/app/report_mount" \
    -e PROJECT="${PROJECT}" \
    -e MODE="${MODE:-start}" \
    -e NOCTIFAB_INTERACTIVE="${INTERACTIVE}" \
    "${IMAGE}" >"${LOG_FILE}" 2>&1
  EXIT_CODE=$?
fi
set -e

TS="$(date +%H:%M:%S)"
echo "[${TS}] ${CONTAINER} exited (code=${EXIT_CODE}). Log: ${LOG_FILE}"

exit ${EXIT_CODE}