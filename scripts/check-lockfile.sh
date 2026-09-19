#!/usr/bin/env bash
# 作業中のnode_modulesやlockfileに触れず、隔離ディレクトリで検証する。
set -euo pipefail

readonly PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly TEMP_DIR="$(mktemp -d)"
readonly CHECK_ROOT="$TEMP_DIR/check"
trap 'rm -rf "$TEMP_DIR"' EXIT

source_root="$PROJECT_ROOT"
use_linux=0

case "${1:-}" in
  "")
    ;;
  --staged)
    source_root="$TEMP_DIR/staged"
    ;;
  --linux)
    use_linux=1
    ;;
  *)
    echo "使い方: pnpm lock:check / pnpm lock:check -- --staged / pnpm lock:linux" >&2
    exit 2
    ;;
esac

if [[ "${1:-}" == "--staged" ]]; then
  for file in \
    package.json pnpm-lock.yaml pnpm-workspace.yaml \
    apps/api/package.json apps/web/package.json; do
    mkdir -p "$source_root/$(dirname "$file")"
    if ! git -C "$PROJECT_ROOT" show ":$file" > "$source_root/$file"; then
      echo "エラー: コミット予定の $file を読み取れません。" >&2
      exit 1
    fi
  done
fi

for dir in . apps/api apps/web; do
  mkdir -p "$CHECK_ROOT/$dir"
  cp "$source_root/$dir/package.json" "$CHECK_ROOT/$dir/package.json"
done
cp "$source_root/pnpm-lock.yaml" "$source_root/pnpm-workspace.yaml" "$CHECK_ROOT/"

if [[ "$use_linux" -eq 1 ]]; then
  # CI / EC2と同じamd64。Macのnode_modulesや.envは持ち込まない。
  docker run --rm --platform linux/amd64 \
    --volume "$CHECK_ROOT:/workspace" --workdir /workspace \
    node:24-slim sh -ec '
      npm install --global "$(node -p "require(\"./package.json\").packageManager")" --no-audit --no-fund
      pnpm install --frozen-lockfile
      pnpm --filter @juken-map/api exec tsx --version
      pnpm --filter @juken-map/web exec vite --version
    '
else
  # frozen + lockfile-onlyでmanifestとの整合性を検査する。依存は展開しない。
  pnpm --dir "$CHECK_ROOT" install --lockfile-only --frozen-lockfile --ignore-scripts
fi

cmp "$source_root/pnpm-lock.yaml" "$CHECK_ROOT/pnpm-lock.yaml"
cmp "$source_root/pnpm-workspace.yaml" "$CHECK_ROOT/pnpm-workspace.yaml"
echo "workspaceのlockfile検証に成功しました。"
