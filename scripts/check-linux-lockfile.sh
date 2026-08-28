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
cp "$PROJECT_ROOT/package-lock.json" "$TEMP_DIR/package-lock.json"

docker run --rm \
  --user "$(id -u):$(id -g)" \
  --volume "$TEMP_DIR:/workspace" \
  --workdir /workspace \
  --env npm_config_cache=/tmp/npm-cache \
  "$NODE_IMAGE" \
  sh -c "npx --yes npm@$NPM_VERSION install --package-lock-only --ignore-scripts --no-audit --no-fund"

if ! cmp --silent "$PROJECT_ROOT/package-lock.json" "$TEMP_DIR/package-lock.json"; then
  echo >&2
  echo "エラー: package-lock.json がLinux環境で再現できません。" >&2
  echo "'npm run lock:linux' を実行し、更新されたlockfileをコミットしてから再度pushしてください。" >&2
  exit 1
fi

echo "Linux環境でpackage-lock.jsonの再現性を確認しました。"
