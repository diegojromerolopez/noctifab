#!/usr/bin/env bash
# Run the noctifab validation matrix (parallel or serial): one container per
# validation project. Each project builds its own image on top of the shared
# `noctifab-validation:base` base image, runs a validate.sh harness container,
# captures its log to .validation-logs/<project>.log, and writes an
# execution report under validation/projects/<project>/output/report/.
#
# Usage:
#   run_all.sh                           # parallel run for all projects
#   run_all.sh --serial                  # serial run (one project at a time)
#   run_all.sh --projects echo,wc,t4     # run specific project list
#   run_all.sh --serial --projects echo,t4
#   run_all.sh --skip-build              # reuse existing images
#
# Options:
#   -s, --serial                         Run projects sequentially (serial mode)
#   -p, --projects <list>                Comma-separated or space-separated project list
#   --skip-build                         Skip rebuilding noctifab-validation:base
#
# Environment:
#   NOCTIFAB_LOG_DIR                     optional (defaults to "$root/.validation-logs")
#   SERIAL                               1 for serial mode, 0 for parallel mode
#   SKIP_BUILD                           1 to skip building base image
#   PROJECTS                             space or comma separated list of projects
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
LOG_DIR="${NOCTIFAB_LOG_DIR:-${ROOT}/.validation-logs}"
mkdir -p "${LOG_DIR}"

SKIP_BUILD="${SKIP_BUILD:-0}"
SERIAL="${SERIAL:-0}"
CLI_PROJECTS=()

while [ "$#" -gt 0 ]; do
  case "$1" in
    --skip-build)
      SKIP_BUILD=1
      shift
      ;;
    -s|--serial)
      SERIAL=1
      shift
      ;;
    -p|--projects|--project)
      if [ "$#" -lt 2 ]; then
        echo "Error: $1 requires an argument" >&2
        exit 1
      fi
      IFS=',' read -ra split_args <<< "$2"
      for p in "${split_args[@]}"; do
        [ -n "${p}" ] && CLI_PROJECTS+=("${p}")
      done
      shift 2
      ;;
    --projects=*|--project=*)
      val="${1#*=}"
      IFS=',' read -ra split_args <<< "${val}"
      for p in "${split_args[@]}"; do
        [ -n "${p}" ] && CLI_PROJECTS+=("${p}")
      done
      shift
      ;;
    -*)
      echo "Error: Unknown option $1" >&2
      exit 1
      ;;
    *)
      IFS=',' read -ra split_args <<< "$1"
      for p in "${split_args[@]}"; do
        [ -n "${p}" ] && CLI_PROJECTS+=("${p}")
      done
      shift
      ;;
  esac
done

PROJECTS=()
if [ "${#CLI_PROJECTS[@]}" -gt 0 ]; then
  PROJECTS=("${CLI_PROJECTS[@]}")
elif [ -n "${PROJECTS:-}" ]; then
  IFS=',' read -ra split_args <<< "${PROJECTS}"
  for p in "${split_args[@]}"; do
    [ -n "${p}" ] && PROJECTS+=("${p}")
  done
else
  for d in "${ROOT}/validation/projects"/*/; do
    [ -d "${d}" ] && PROJECTS+=("$(basename "${d}")")
  done
fi

if [ "${#PROJECTS[@]}" -eq 0 ]; then
  echo "run_all.sh: no validation projects found under ${ROOT}/validation/projects/" >&2
  exit 1
fi

WRAPPER_LOG="${LOG_DIR}/run_all.$(date +%Y%m%d-%H%M%S).log"
mode_str="parallel"
if [ "${SERIAL}" -eq 1 ]; then
  mode_str="serial"
fi
echo "run_all: starting ${mode_str} validation for ${PROJECTS[*]}" | tee "${WRAPPER_LOG}"

# Step 1. Build (or reuse) the shared base image.
if [ "${SKIP_BUILD}" -ne 1 ] || [ -z "$(docker images -q noctifab-validation:base 2>/dev/null)" ]; then
  TS="$(date +%H:%M:%S)"
  echo "[${TS}] building base image noctifab-validation:base..." | tee -a "${WRAPPER_LOG}"
  docker build -f "${ROOT}/validation/Dockerfile.validation" \
    -t noctifab-validation:base "${ROOT}" >>"${WRAPPER_LOG}" 2>&1
else
  echo "base image already present, skipping build" | tee -a "${WRAPPER_LOG}"
fi

# Step 2. Execute projects (Serial or Parallel)
overall=0

if [ "${SERIAL}" -eq 1 ]; then
  for project in "${PROJECTS[@]}"; do
    mkdir -p "${ROOT}/validation/projects/${project}/output/log"
    echo "[$(date +%H:%M:%S)] [SERIAL] starting run_one.sh for ${project}..." | tee -a "${WRAPPER_LOG}"
    
    if [ "${SKIP_BUILD}" -eq 1 ]; then
      if NOCTIFAB_SKIP_BUILD=1 "${SCRIPT_DIR}/run_one.sh" "${project}" \
        >>"${ROOT}/validation/projects/${project}/output/log/${project}.wrap.log" 2>&1; then
        echo "[$(date +%H:%M:%S)] ${project}: PASS" | tee -a "${WRAPPER_LOG}"
      else
        code=$?
        echo "[$(date +%H:%M:%S)] ${project}: FAIL (exit ${code})" | tee -a "${WRAPPER_LOG}"
        overall=1
      fi
    else
      if "${SCRIPT_DIR}/run_one.sh" "${project}" \
        >>"${ROOT}/validation/projects/${project}/output/log/${project}.wrap.log" 2>&1; then
        echo "[$(date +%H:%M:%S)] ${project}: PASS" | tee -a "${WRAPPER_LOG}"
      else
        code=$?
        echo "[$(date +%H:%M:%S)] ${project}: FAIL (exit ${code})" | tee -a "${WRAPPER_LOG}"
        overall=1
      fi
    fi
  done
else
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
fi

echo "run_all: complete (overall exit ${overall})" | tee -a "${WRAPPER_LOG}"
exit ${overall}