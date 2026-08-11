#!/usr/bin/env bash

set -euo pipefail

readonly NODE_IMAGE="node:24-slim"
readonly NPM_VERSION="11.19.0"
readonly PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly TEMP_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TEMP_DIR"
}
trap cleanup EXIT

cp "$PROJECT_ROOT/package.json" "$TEMP_DIR/package.json"
if [[ -f "$PROJECT_ROOT/package-lock.json" ]]; then
  cp "$PROJECT_ROOT/package-lock.json" "$TEMP_DIR/package-lock.json"
fi

docker run --rm \
  --user "$(id -u):$(id -g)" \
  --volume "$TEMP_DIR:/workspace" \
  --workdir /workspace \
  --env npm_config_cache=/tmp/npm-cache \
  "$NODE_IMAGE" \
  sh -c "npx --yes npm@$NPM_VERSION install --package-lock-only --ignore-scripts --no-audit --no-fund && npx --yes npm@$NPM_VERSION ci --ignore-scripts --no-audit --no-fund"

cp "$TEMP_DIR/package-lock.json" "$PROJECT_ROOT/package-lock.json"

echo "Linux環境でpackage-lock.jsonを更新し、npm ciの成功を確認しました。"
