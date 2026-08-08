#!/usr/bin/env bash
# Run the full noctifab validation matrix in parallel: one container per
# validation project (frontpunch, todo-cli, wc). Each project builds its own
# image on top of the shared `noctifab-validation:base` base image, runs a
# validate.sh harness container, captures its log to
# .validation-logs/<project>.log, and writes a <PROJECT>_FEEDBACK.md file at
# the repository root.
#
# Usage:
#   run_all.sh                         # build base + all per-project images
#   run_all.sh --skip-build            # reuse existing noctifab-validation:* images
#   run_all.sh <project>...            # run a subset (e.g. wc todo-cli)
#
# Environment:
#   NOCTIFAB_LOG_DIR                   optional (defaults to "$root/.validation-logs")
#   SKIP_BUILD                         (use `--skip-build` flag instead)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
LOG_DIR="${NOCTIFAB_LOG_DIR:-${ROOT}/.validation-logs}"
mkdir -p "${LOG_DIR}"

# Project list: defaults to every directory under validation/projects/.
if [ "${1:-}" = "--skip-build" ]; then
  SKIP_BUILD=1
  shift
else
  SKIP_BUILD=0
fi

if [ "$#" -gt 0 ]; then
  PROJECTS=()
  for arg in "$@"; do
    IFS=',' read -ra split_args <<< "${arg}"
    for p in "${split_args[@]}"; do
      [ -n "${p}" ] && PROJECTS+=("${p}")
    done
  done
else
  PROJECTS=()
  for d in "${ROOT}/validation/projects"/*/; do
    PROJECTS+=("$(basename "${d}")")
  done
fi

if [ "${#PROJECTS[@]}" -eq 0 ]; then
  echo "run_all.sh: no validation projects found under ${ROOT}/validation/projects/" >&2
  exit 1
fi

# The containers are self-contained: each per-project image already contains
# `secrets.yaml` (baked in at `docker build` time), and noctifab resolves
# `secret:OPENCODE_API_KEY` from it at config load time. The `validate.sh`
# harness sets a dummy `GITHUB_TOKEN` for pre-flight checks. No host
# credentials need to be passed in via `-e`.

# Shared wrapper log for the launch.
WRAPPER_LOG="${LOG_DIR}/run_all.$(date +%Y%m%d-%H%M%S).log"
echo "run_all: starting parallel validation for ${PROJECTS[*]}" | tee "${WRAPPER_LOG}"

# Step 1. Build (or reuse) the shared base image.
if [ "${SKIP_BUILD}" -ne 1 ] || [ -z "$(docker images -q noctifab-validation:base 2>/dev/null)" ]; then
  TS="$(date +%H:%M:%S)"
  echo "[${TS}] building base image noctifab-validation:base..." | tee -a "${WRAPPER_LOG}"
  docker build -f "${ROOT}/validation/Dockerfile.validation" \
    -t noctifab-validation:base "${ROOT}" >>"${WRAPPER_LOG}" 2>&1
else
  echo "base image already present, skipping build" | tee -a "${WRAPPER_LOG}"
fi

# Step 2. Launch every project's run_one.sh in parallel.
pids=()
for project in "${PROJECTS[@]}"; do
  mkdir -p "${ROOT}/validation/projects/${project}/output/log"
  if [ "${SKIP_BUILD}" -eq 1 ]; then
    NOCTIFAB_SKIP_BUILD=1 "${SCRIPT_DIR}/run_one.sh" "${project}" \
      >>"${ROOT}/validation/projects/${project}/output/log/${project}.wrap.log" 2>&1 &
  else
    "${SCRIPT_DIR}/run_one.sh" "${project}" \
      >>"${ROOT}/validation/projects/${project}/output/log/${project}.wrap.log" 2>&1 &
  fi
  pids+=("$!")
  echo "[$(date +%H:%M:%S)] launched run_one.sh for ${project} (pid=$!)" | tee -a "${WRAPPER_LOG}"
  sleep 2
done

# Step 3. Wait for all parallel jobs to complete; record their exit codes.
overall=0
for i in "${!pids[@]}"; do
  project="${PROJECTS[$i]}"
  if wait "${pids[$i]}"; then
    echo "[$(date +%H:%M:%S)] ${project}: PASS" | tee -a "${WRAPPER_LOG}"
  else
    code=$?
    echo "[$(date +%H:%M:%S)] ${project}: FAIL (exit ${code})" | tee -a "${WRAPPER_LOG}"
    overall=1
  fi
done

echo "run_all: complete (overall exit ${overall})" | tee -a "${WRAPPER_LOG}"
exit ${overall}