#!/usr/bin/env bash
# 手元で本番と同じ振り分けの nginx を前に立てる（JUK-96）。Ctrl-C か SIGTERM で止まり、コンテナも消える。
#
#   bash scripts/local-proxy.sh <待ち受けるポート> <Node のポート> <Go のポート>
#   bash scripts/local-proxy.sh   … 省くと開発用（PROXY_PORT → API_PORT・GO_PORT）
#
# 振り分けは本番と同じ infra/nginx/juken-map-go-routes.conf をそのまま読む。開発は pnpm dev（dev:proxy）、
# E2E は scripts/e2e-server.sh から呼ばれる。ポートの決め方は scripts/local-ports.sh。
#
# nginx はコンテナの中なので、手元の Node と Go には host.docker.internal で届く。
# Linux（CI）では Docker Desktop と違ってこの名前が無いので、--add-host で作る。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [ $# -eq 0 ]; then
  source "$ROOT/scripts/local-ports.sh"
  set -- "$PROXY_PORT" "$API_PORT" "$GO_PORT"
fi
LISTEN_PORT=${1:?待ち受けるポートを渡してください}
NODE_PORT=${2:?Node のポートを渡してください}
GO_PORT=${3:?Go のポートを渡してください}
IMAGE=${LOCAL_PROXY_IMAGE:-nginx:1.28}
NAME="juken-map-proxy-$LISTEN_PORT"

# 前回が強制終了で残っていれば消す（同じ名前では起動できない）。
docker rm -f "$NAME" >/dev/null 2>&1 || true

echo "nginx: http://localhost:$LISTEN_PORT → Node :$NODE_PORT / Go :${GO_PORT}（振り分けは infra/nginx/juken-map-go-routes.conf）"
exec docker run --rm --name "$NAME" \
  --add-host=host.docker.internal:host-gateway \
  -p "$LISTEN_PORT:8080" \
  -e NODE_PORT="$NODE_PORT" -e GO_PORT="$GO_PORT" -e NGINX_ENTRYPOINT_QUIET_LOGS=1 \
  -v "$ROOT/infra/nginx/local/default.conf.template:/etc/nginx/templates/default.conf.template:ro" \
  -v "$ROOT/infra/nginx/juken-map-go-routes.conf:/etc/nginx/juken-map/go-routes.conf:ro" \
  "$IMAGE"
