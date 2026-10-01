#!/usr/bin/env bash
# Node（apps/api）と Go（apps/api-go）に同じリクエストを送り、応答が同じかを確かめる（JUK-71）。
# 比べるリクエストの一覧は parity_test.go の parityCases。API を Go へ移したら、そこに1行足す。
#
# 使い方（worktree のルートから）:
#   bash apps/api-go/parity.sh
#   USERS=200 bash apps/api-go/parity.sh
#
# 前提: DB が起動していて、合成データ（pnpm db:seed:synthetic）が入っていること。
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
USERS=${USERS:-50}          # 比べる合成ユーザーの人数
NODE_PORT=${NODE_PORT:-18100}
GO_PORT=${GO_PORT:-18180}
WORK=$(mktemp -d)

source "$ROOT/apps/api-go/servers.sh"
trap 'stop_servers; rm -rf "$WORK"' EXIT

ensure_free "$NODE_PORT" "$GO_PORT"

cookies=$(issue_cookies "$USERS")
if [ -z "$cookies" ]; then
  echo "セッションを発行できませんでした。合成データが入っているか確かめてください。" >&2
  exit 1
fi

start_servers "$NODE_PORT" "$GO_PORT"

# BETTER_AUTH_SECRET は「署名は正しいが DB に無いトークン」を作るのに使う。
cd "$ROOT/apps/api-go"
(set -a; source "$ROOT/.env"; set +a
  PARITY_NODE_URL=http://localhost:$NODE_PORT PARITY_GO_URL=http://localhost:$GO_PORT PARITY_COOKIES=$cookies go test -tags parity -run TestParity -count=1 -v .)
