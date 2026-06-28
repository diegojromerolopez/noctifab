#!/usr/bin/env bash
set -euo pipefail

# 1. Resolve secrets from the host secrets.yaml or environment
if [ -n "${GEMINI_API_KEY:-}" ]; then
  GEMINI_KEY="${GEMINI_API_KEY}"
elif [ -f "/Users/diegoj/repos/frontpunch/.noctifab/secrets.yaml" ]; then
  echo "Extracting GEMINI_API_KEY from host frontpunch secrets..."
  GEMINI_KEY=$(grep "GEMINI_API_KEY:" /Users/diegoj/repos/frontpunch/.noctifab/secrets.yaml | awk -F'"' '{print $2}')
else
  echo "⚠ Error: GEMINI_API_KEY is not set and no secrets.yaml was found in frontpunch."
  exit 1
fi

# 2. Build the Docker verification image
echo "Building Docker verification image..."
docker build -t noctifab-verify -f scripts/Dockerfile.verify .

# 3. Run the containerized E2E autonomous check
echo "Running autonomous E2E verification inside Docker container..."
docker run --rm \
  -e GEMINI_API_KEY="${GEMINI_KEY}" \
  -e GITHUB_TOKEN="dummy-token" \
  noctifab-verify
