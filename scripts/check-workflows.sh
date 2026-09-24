#!/usr/bin/env bash
# GitHub Actions のワークフローが、配布経路の決まり（dev-standards 06 F2）を守っているかを確かめる。
#   1. 外部の Action はコミットのハッシュ（40桁）で固定する。タグは付け替えられるので、
#      乗っ取られた Action のタグが動くと、次の実行で改ざんされたコードが CI の権限で動く。
#   2. どのワークフローにも最上位の permissions を書く。書かないとリポジトリの既定の権限になり、
#      既定が変わったときに黙って広がる。
# 使い方: bash scripts/check-workflows.sh [ワークフローのディレクトリ]
set -euo pipefail

readonly PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly WORKFLOW_DIR="${1:-$PROJECT_ROOT/.github/workflows}"

failed=0

for file in "$WORKFLOW_DIR"/*.yml "$WORKFLOW_DIR"/*.yaml; do
  [[ -e "$file" ]] || continue
  name="$(basename "$file")"

  # 同じリポジトリの Action（./）とコンテナ（docker://）は対象外。
  while IFS= read -r line; do
    text="$(sed -E 's/^[[:space:]]*-?[[:space:]]*//' <<<"${line#*:}")"
    echo "エラー: $name:${line%%:*}: コミットのハッシュで固定していない Action があります: $text" >&2
    failed=1
  done < <(grep -nE '^\s*-?\s*uses:' "$file" \
    | grep -vE 'uses:\s*["'\'']?(\./|docker://)' \
    | grep -vE 'uses:\s*["'\'']?[^@[:space:]]+@[0-9a-f]{40}(["'\''[:space:]]|$)' || true)

  if ! grep -qE '^permissions:' "$file"; then
    echo "エラー: $name: 最上位に permissions がありません。" >&2
    failed=1
  fi
done

if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
echo "ワークフローの検査に成功しました。"
