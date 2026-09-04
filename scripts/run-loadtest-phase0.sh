#!/usr/bin/env bash

set -euo pipefail

BASE_URL="${BASE_URL:-http://host.docker.internal:3000}"

case "$BASE_URL" in
  http://host.docker.internal:3000|http://localhost:3000|http://127.0.0.1:3000)
    ;;
  *)
    echo "Phase 0 refuses non-local BASE_URL: $BASE_URL" >&2
    exit 1
    ;;
esac

command -v docker >/dev/null || {
  echo "Docker is required." >&2
  exit 1
}

seed_json="$(npx tsx --env-file=.env prisma/seed-load-test-phase0.ts)"
load_test_email="$(jq -er '.email' <<<"$seed_json")"
load_test_password="$(jq -er '.password' <<<"$seed_json")"
load_test_plan_id="$(jq -er '.planId' <<<"$seed_json")"

docker run --rm -i \
  -e BASE_URL="$BASE_URL" \
  -e LOAD_TEST_EMAIL="$load_test_email" \
  -e LOAD_TEST_PASSWORD="$load_test_password" \
  -e LOAD_TEST_PLAN_ID="$load_test_plan_id" \
  grafana/k6:latest run - < load-tests/phase0.js
