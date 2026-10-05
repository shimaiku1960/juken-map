#!/usr/bin/env bash
# E2E 用に、本番と同じ構成を起動する。Playwright の webServer から呼ばれる（E2E_BASE_URL を指定した場合は使われない）。
#
#   ブラウザ → nginx（E2E_PORT）─┬─ infra/nginx/juken-map-go-routes.conf のパス → Go（E2E_GO_PORT、SPA も配る）
#                                └─ それ以外                                 → Node（E2E_NODE_PORT）
# JUK-111 から go-routes.conf の最後の受け皿が残りを全部 Go へ送るので、Node には何も届かない（消すのは JUK-109）。
#
# 本番と同じ振り分けを通すので、E2E は Go が返す API を確かめる（JUK-96。それまでは Node だけを立てていて、
# Go へ移した API を E2E が一度も通っていなかった）。ポートの決め方は scripts/local-ports.sh。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$ROOT/scripts/local-ports.sh"
WORK="$(mktemp -d)"

pids=()
cleanup() {
  for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; done
  # pnpm は tsx・node を子として起動するので、親を止めても残ることがある。待ち受けているプロセスで止める。
  local port
  for port in "$E2E_NODE_PORT" "$E2E_GO_PORT"; do
    lsof -ti tcp:"$port" -sTCP:LISTEN 2>/dev/null | xargs kill 2>/dev/null || true
  done
  docker rm -f "juken-map-proxy-$E2E_PORT" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT
trap 'exit 143' TERM INT

pnpm --dir "$ROOT" --filter @juken-map/web build
(cd "$ROOT/apps/api-go" && go build -o "$WORK/api-go" .)

# apps/api は cwd が apps/api になるので、配信元は絶対パスで渡す。
WEB_DIST_DIR="$ROOT/apps/web/dist" API_PORT="$E2E_NODE_PORT" \
  pnpm --dir "$ROOT" --filter @juken-map/api start &
pids+=($!)

# Go は .env を自分では読まないので、ここで読む（CI は .env が無く、ジョブの環境変数だけで動く）。
# 先に決めた値が .env で上書きされないよう、読んだ後に改めて渡す。
(
  if [ -f "$ROOT/.env" ]; then set -a; source "$ROOT/.env"; set +a; fi
  # WEB_ORIGIN はメールのリンクと外部ログインの戻り先に使う画面のオリジン。ブラウザが開く nginx の番号に合わせる。
  # E2E は登録・再設定でメールを送る操作をするので、Resend のキーを空にして本物のメールを送らない
  # （送れなかったことはログに残るだけ。トークンは db/e2e-auth.ts が発行する）。
  # WEB_DIST_DIR は画面のビルド成果物（JUK-111。Go が画面も配る）。
  PORT="$E2E_GO_PORT" WEB_ORIGIN="http://localhost:$E2E_PORT" RESEND_API_KEY="" WEB_DIST_DIR="$ROOT/apps/web/dist" \
    exec "$WORK/api-go"
) &
pids+=($!)

# Node と Go の両方が応答してから nginx を立てる。Playwright は nginx の応答で起動完了とみなすので、
# 先に立てると、どちらかがまだ起動中のうちにテストが始まることがある。
for _ in $(seq 1 120); do
  curl -sf -o /dev/null "localhost:$E2E_NODE_PORT/api/health" && curl -sf -o /dev/null "localhost:$E2E_GO_PORT/api/health" && break
  sleep 0.5
done

bash "$ROOT/scripts/local-proxy.sh" "$E2E_PORT" "$E2E_NODE_PORT" "$E2E_GO_PORT" &
pids+=($!)

# どれか1つでも止まったら全部止める（片方だけ動いていると、E2E が分かりにくい失敗をする）。
# macOS の bash は 3.2 で wait -n が無いので、見回って確かめる。
while true; do
  for pid in "${pids[@]}"; do
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "e2e-server: 起動したプロセス（${pid}）が止まったので、全部止めます" >&2
      exit 1
    fi
  done
  sleep 1
done
