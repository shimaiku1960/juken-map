#!/usr/bin/env bash
# 作業中のnode_modulesやlockfileに触れず、隔離ディレクトリで検証する。
set -euo pipefail

readonly PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT

for dir in . apps/api apps/web; do
  mkdir -p "$TEMP_DIR/$dir"
  cp "$PROJECT_ROOT/$dir/package.json" "$TEMP_DIR/$dir/package.json"
done
cp "$PROJECT_ROOT/pnpm-lock.yaml" "$PROJECT_ROOT/pnpm-workspace.yaml" "$TEMP_DIR/"

case "${1:-}" in
  "")
    # frozen + lockfile-onlyでmanifestとの整合性を検査する。依存は展開しない。
    pnpm --dir "$TEMP_DIR" install --lockfile-only --frozen-lockfile --ignore-scripts
    ;;
  --linux)
    # CI / EC2と同じamd64。Macのnode_modulesや.envは持ち込まない。
    docker run --rm --platform linux/amd64 \
      --volume "$TEMP_DIR:/workspace" --workdir /workspace \
      node:24-slim sh -ec '
        apt-get update -qq && apt-get install -y -qq openssl >/dev/null
        npm install --global "$(node -p "require(\"./package.json\").packageManager")" --no-audit --no-fund
        pnpm install --frozen-lockfile
        pnpm --filter @juken-map/api exec prisma --version
        pnpm --filter @juken-map/web exec vite --version
      '
    ;;
  *)
    echo "使い方: pnpm lock:check / pnpm lock:linux" >&2
    exit 2
    ;;
esac

cmp "$PROJECT_ROOT/pnpm-lock.yaml" "$TEMP_DIR/pnpm-lock.yaml"
cmp "$PROJECT_ROOT/pnpm-workspace.yaml" "$TEMP_DIR/pnpm-workspace.yaml"
echo "workspaceのlockfile検証に成功しました。"
