#!/usr/bin/env bash
# Pyedis Autonomous Feedback & Improvement Loop
# Convenience launcher forwarding to single_project_loop.py with project=pyedis
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "${SCRIPT_DIR}/single_project_loop.py" pyedis "$@"
