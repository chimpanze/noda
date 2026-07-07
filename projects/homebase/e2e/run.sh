#!/usr/bin/env bash
# Homebase E2E: boots the compose stack from scratch, runs the Go suite, tears down.
# Usage: ./projects/homebase/e2e/run.sh   (from anywhere)
set -euo pipefail
cd "$(dirname "$0")/.."

export SETUP_TOKEN="${SETUP_TOKEN:-e2e-setup-token}"
export PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-http://localhost:3000}"

docker compose down -v --remove-orphans 2>/dev/null || true
docker compose up -d --build
trap 'docker compose down -v --remove-orphans' EXIT

echo "waiting for noda ..."
for _ in $(seq 1 60); do
  if curl -fso /dev/null http://localhost:3000/health/ready; then
    break
  fi
  sleep 1
done

(cd ../.. && SETUP_TOKEN="$SETUP_TOKEN" go test -tags e2e -count=1 -v ./projects/homebase/e2e/)
