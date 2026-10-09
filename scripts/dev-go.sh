#!/usr/bin/env bash
# 開発中の Go の API（apps/api）を起動する（pnpm dev の dev:go、JUK-96）。
# .env を読み、.env.worktree の値で上書きする。ポートは scripts/local-ports.sh の GO_PORT。
# 自動で再起動はしないので、Go を書き換えたら pnpm dev を起動し直すか、このスクリプトだけ起動し直す。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
set -a
# shellcheck disable=SC1091
source "$ROOT/.env"
# shellcheck disable=SC1091
[ -f "$ROOT/.env.worktree" ] && source "$ROOT/.env.worktree"
set +a
source "$ROOT/scripts/local-ports.sh"

# LINE Login の戻り先などに使う画面のオリジン。手元は Vite。
export PORT=$GO_PORT WEB_ORIGIN="http://localhost:$WEB_PORT"
# go run ではなく、作ってから exec する。止めるときの SIGTERM がサーバー本体に直接届く。
BIN="${TMPDIR:-/tmp}/juken-map-api-$GO_PORT"
(cd "$ROOT/apps/api" && go build -o "$BIN" ./cmd/api)
exec "$BIN"
