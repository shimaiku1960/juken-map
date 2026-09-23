#!/usr/bin/env bash
# デプロイスクリプトの「どのポートへ入れ替えるか」の判断だけを手元で試す。
#
# 2026-09-23、無停止デプロイの初回デプロイがここで無言のまま落ちた。
# upstream ファイルがまだ無い状態を一度も試していなかったのが原因なので、
# その状態を含めて確かめられるようにした。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEPLOY="$ROOT/.github/scripts/deploy-ec2.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fail=0
check() {  # $1=見出し $2=期待する出力 $3=upstreamファイルの中身（省略＝ファイルを作らない）
  local conf="$WORK/upstream.conf"
  rm -f "$conf"
  [ $# -ge 3 ] && printf '%s\n' "$3" > "$conf"
  # 本番と同じ配送（base64 → パイプ → bash -s）で流す。配送の違いで壊れないことも一緒に見る。
  local got
  got="$(base64 < "$DEPLOY" | tr -d '\n' | base64 -d \
    | UPSTREAM_CONF="$conf" DEPLOY_PORTS_ONLY=1 bash -s -- dummy-tag "" 2>&1 || echo "EXIT=$?")"
  if [ "$got" = "$2" ]; then
    printf '  ✅ %s\n' "$1"
  else
    printf '  ❌ %s\n     期待: %s\n     実際: %s\n' "$1" "$2" "$got"
    fail=1
  fi
}

echo "デプロイのポート判断"
check "初回（upstream ファイルがまだ無い）" "CURRENT_PORT=3000 NEW_PORT=3001"
check "3000を向いている → 3001へ" "CURRENT_PORT=3000 NEW_PORT=3001" "upstream juken_map_app {
    server 127.0.0.1:3000;
}"
check "3001を向いている → 3000へ" "CURRENT_PORT=3001 NEW_PORT=3000" "upstream juken_map_app {
    server 127.0.0.1:3001;
}"
check "中身が壊れている → 3000とみなす" "CURRENT_PORT=3000 NEW_PORT=3001" "# 空っぽ"

exit "$fail"
