#!/usr/bin/env bash
# Summarizes execution reports from all validation projects.
# Parses validation/projects/*/output/report/*.md and prints a consolidated Markdown table.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

printf "\n# Noctifab Validation Matrix Performance Summary\n\n"
printf "| Project | Status | Stories | Tasks | Errors | Retries | Measured Tokens | Lead Time | Report File |\n"
printf "| :--- | :--- | ---: | ---: | ---: | ---: | ---: | :--- | :--- |\n"

found=0
for proj_dir in "${ROOT}/validation/projects"/*/; do
  [ ! -d "${proj_dir}" ] && continue
  proj="$(basename "${proj_dir}")"
  report_dir="${proj_dir}/output/report"

  if [ ! -d "${report_dir}" ]; then
    continue
  fi

  # Find latest report
  latest_report="$(find "${report_dir}" -name "*.md" -type f -print0 | xargs -0 ls -t 2>/dev/null | head -n 1 || true)"
  if [ -z "${latest_report}" ] || [ ! -f "${latest_report}" ]; then
    continue
  fi

  found=1
  rel_report="validation/projects/${proj}/output/report/$(basename "${latest_report}")"

  # Extract status
  status="$(grep "^> Status:" "${latest_report}" | head -n 1 | awk '{print $3}' || echo "UNKNOWN")"
  if [ -z "${status}" ]; then
    status="UNKNOWN"
  fi

  # Extract lead time
  lead_time="$(grep -E "^\- \*\*Lead Time:\*\*" "${latest_report}" | head -n 1 | sed -E 's/.*- \*\*Lead Time:\*\* ([^*]+).*/\1/' | sed -E 's/^[[:space:]]+|[[:space:]]+$//g' || echo "-")"
  [ -z "${lead_time}" ] && lead_time="-"

  # Extract from Live Status / Execution Status table
  table_line="$(grep -A 5 -E "## (Execution Status|Live Status)" "${latest_report}" | grep -E '^\|[[:space:]]*(RUNNING|SUCCESS|FAILED|CANCELLED)' | head -n 1 || true)"
  
  if [ -n "${table_line}" ]; then
    stories="$(echo "${table_line}" | awk -F'|' '{print $4}' | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"
    tasks="$(echo "${table_line}" | awk -F'|' '{print $5}' | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"
    errors="$(echo "${table_line}" | awk -F'|' '{print $7}' | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"
    retries="$(echo "${table_line}" | awk -F'|' '{print $8}' | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"
    tokens="$(echo "${table_line}" | awk -F'|' '{print $9}' | sed -E 's/^[[:space:]]+|[[:space:]]+$//g')"
  else
    stories="-"
    tasks="-"
    errors="-"
    retries="-"
    tokens="-"
  fi

  printf "| \`%s\` | **%s** | %s | %s | %s | %s | %s | %s | [\`%s\`](%s) |\n" \
    "${proj}" "${status}" "${stories}" "${tasks}" "${errors}" "${retries}" "${tokens}" "${lead_time}" "$(basename "${latest_report}")" "${rel_report}"
done

if [ "${found}" -eq 0 ]; then
  printf "\n*(No execution reports found in validation/projects/*/output/report/ yet. Run a validation project first.)*\n\n"
fi
printf "\n"
