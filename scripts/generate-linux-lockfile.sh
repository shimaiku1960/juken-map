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

# ルートと apps/* は別パッケージで lockfile も別。本番イメージは apps/* の lockfile で
# npm ci するので、どれか1つでも Linux 用依存が欠けるとデプロイ時に落ちる。
for pkg_dir in "" "apps/api" "apps/web"; do
  target="${PROJECT_ROOT}${pkg_dir:+/$pkg_dir}"
  [[ -f "$target/package.json" ]] || continue

  work="$TEMP_DIR/${pkg_dir:-root}"
  mkdir -p "$work"
  cp "$target/package.json" "$work/package.json"
  if [[ -f "$target/package-lock.json" ]]; then
    cp "$target/package-lock.json" "$work/package-lock.json"
  fi

  docker run --rm \
    --user "$(id -u):$(id -g)" \
    --volume "$work:/workspace" \
    --workdir /workspace \
    --env npm_config_cache=/tmp/npm-cache \
    "$NODE_IMAGE" \
    sh -c "npx --yes npm@$NPM_VERSION install --package-lock-only --ignore-scripts --no-audit --no-fund && npx --yes npm@$NPM_VERSION ci --ignore-scripts --no-audit --no-fund"

  cp "$work/package-lock.json" "$target/package-lock.json"
  echo "更新: ${pkg_dir:-（ルート）}/package-lock.json"
done

echo "Linux環境で全ての package-lock.json を更新し、npm ci の成功を確認しました。"
