#!/usr/bin/env bash

set -euo pipefail

IMAGE_TAG="${1:?IMAGE_TAG is required}"
# observability/alloy/production.alloy を base64 にしたもの（deploy.yml が渡す）。
# 空のときは可観測性の送信を丸ごと省く。
ALLOY_CONFIG_B64="${2:-}"
REPO="961457613174.dkr.ecr.ap-northeast-1.amazonaws.com/juken-map"
ENV_FILE="/home/ubuntu/juken-map/.env"
RUNTIME_SECRET_ID="juken-map/production/runtime"
# アプリと Alloy を同じネットワークに置き、コンテナ名で呼び合えるようにする
# （Alloy → juken-map:9464 のスクレイプ、アプリ → juken-map-alloy:4318 のトレース送信）。
NETWORK="juken-map"
ALLOY_IMAGE="grafana/alloy:v1.19.2"
ALLOY_DIR="/home/ubuntu/juken-map/alloy"
METRICS_PORT="9464"

# ---- 転送先のポートを決める（AWS も Docker も触る前に済ませる）----
# ここだけを手元で試せるよう、パスは環境変数で差し替えられるようにし、
# DEPLOY_PORTS_ONLY=1 なら判断結果だけ出して終わる（scripts/test-deploy-ports.sh）。
UPSTREAM_CONF="${UPSTREAM_CONF:-/etc/nginx/conf.d/juken-map-upstream.conf}"
SITE_CONF="${SITE_CONF:-/etc/nginx/sites-available/default}"
PORT_A=3000
PORT_B=3001

# いま nginx が向いている先。ファイルがまだ無い初回は、これまで通り3000で動いているとみなす。
#
# ⚠️ ここで `sed ... "$UPSTREAM_CONF" | head -1` と書くと、ファイルが無い初回に sed が
# exit 2 を返し、pipefail と set -e でスクリプトが無言で死ぬ（2026-09-23に本番で踏んだ）。
# 読む前に必ず存在を確かめること。
CURRENT_PORT="$PORT_A"
if [ -f "$UPSTREAM_CONF" ]; then
  found="$(sed -nE 's/.*127\.0\.0\.1:([0-9]+).*/\1/p' "$UPSTREAM_CONF" | head -1)"
  [ -n "$found" ] && CURRENT_PORT="$found"
fi
if [ "$CURRENT_PORT" = "$PORT_A" ]; then NEW_PORT="$PORT_B"; else NEW_PORT="$PORT_A"; fi

if [ "${DEPLOY_PORTS_ONLY:-}" = "1" ]; then
  echo "CURRENT_PORT=$CURRENT_PORT NEW_PORT=$NEW_PORT"
  exit 0
fi

# Secrets Managerから取得した値はコンテナ作成時だけ一時ファイルに置く。
# 既存.envから移行対象キーを除外し、同じ環境変数が重複しない状態でDockerへ渡す。
RUNTIME_ENV_FILE="$(mktemp)"
ALLOY_ENV_FILE="$(mktemp)"
trap 'rm -f "$RUNTIME_ENV_FILE" "$ALLOY_ENV_FILE"' EXIT
chmod 600 "$RUNTIME_ENV_FILE" "$ALLOY_ENV_FILE"
# 可観測性の2つは、下で Grafana Cloud の接続情報が揃ったときだけ付け直す。
grep -Ev '^(LINE_CHANNEL_SECRET|LINE_CHANNEL_ACCESS_TOKEN|LINE_LOGIN_CHANNEL_ID|LINE_LOGIN_CHANNEL_SECRET|METRICS_PORT|OTEL_EXPORTER_OTLP_ENDPOINT)=' "$ENV_FILE" > "$RUNTIME_ENV_FILE"

secret_json="$(aws secretsmanager get-secret-value \
  --secret-id "$RUNTIME_SECRET_ID" \
  --region ap-northeast-1 \
  --query SecretString \
  --output text)"

LINE_CHANNEL_SECRET="$(jq -er '.LINE_CHANNEL_SECRET | strings | select(length > 0)' <<<"$secret_json")"
LINE_CHANNEL_ACCESS_TOKEN="$(jq -er '.LINE_CHANNEL_ACCESS_TOKEN | strings | select(length > 0)' <<<"$secret_json")"
LINE_LOGIN_CHANNEL_ID="$(jq -r '.LINE_LOGIN_CHANNEL_ID // empty' <<<"$secret_json")"
LINE_LOGIN_CHANNEL_SECRET="$(jq -r '.LINE_LOGIN_CHANNEL_SECRET // empty' <<<"$secret_json")"
printf 'LINE_CHANNEL_SECRET=%s\n' "$LINE_CHANNEL_SECRET" >> "$RUNTIME_ENV_FILE"
printf 'LINE_CHANNEL_ACCESS_TOKEN=%s\n' "$LINE_CHANNEL_ACCESS_TOKEN" >> "$RUNTIME_ENV_FILE"
if [ -n "$LINE_LOGIN_CHANNEL_ID" ] && [ -n "$LINE_LOGIN_CHANNEL_SECRET" ]; then
  printf 'LINE_LOGIN_CHANNEL_ID=%s\n' "$LINE_LOGIN_CHANNEL_ID" >> "$RUNTIME_ENV_FILE"
  printf 'LINE_LOGIN_CHANNEL_SECRET=%s\n' "$LINE_LOGIN_CHANNEL_SECRET" >> "$RUNTIME_ENV_FILE"
fi
# 画面のエラーの送り先（Grafana Faro）。ブラウザへ渡す前提の値で秘密ではないが、
# Grafana Cloud の設定を1か所にまとめるため同じシークレットに置く。無ければ画面は送信しない。
FARO_COLLECTOR_URL="$(jq -r '.FARO_COLLECTOR_URL // empty' <<<"$secret_json")"
if [ -n "$FARO_COLLECTOR_URL" ]; then
  printf 'FARO_COLLECTOR_URL=%s\n' "$FARO_COLLECTOR_URL" >> "$RUNTIME_ENV_FILE"
fi
# 可観測性（Grafana Cloud）の接続情報。同じシークレットに入れているので IAM の変更は要らない。
# まだ入れていない間は空になり、その場合はアプリの送信も Alloy も起動しない（今まで通り動く）。
GRAFANA_CLOUD_TOKEN="$(jq -r '.GRAFANA_CLOUD_TOKEN // empty' <<<"$secret_json")"
for key in GRAFANA_CLOUD_LOKI_URL GRAFANA_CLOUD_LOKI_USER GRAFANA_CLOUD_PROM_URL \
  GRAFANA_CLOUD_PROM_USER GRAFANA_CLOUD_OTLP_URL GRAFANA_CLOUD_OTLP_USER; do
  printf '%s=%s\n' "$key" "$(jq -r --arg k "$key" '.[$k] // empty' <<<"$secret_json")" >> "$ALLOY_ENV_FILE"
done
printf 'GRAFANA_CLOUD_TOKEN=%s\n' "$GRAFANA_CLOUD_TOKEN" >> "$ALLOY_ENV_FILE"

OBSERVABILITY=false
if [ -n "$GRAFANA_CLOUD_TOKEN" ] && [ -n "$ALLOY_CONFIG_B64" ]; then
  OBSERVABILITY=true
  # /metrics はアプリ本体（3000番）とは別ポートに出すので、nginx 越しには届かない。
  # トレースの送り先は同じネットワークの Alloy。
  printf 'METRICS_PORT=%s\n' "$METRICS_PORT" >> "$RUNTIME_ENV_FILE"
  printf 'OTEL_EXPORTER_OTLP_ENDPOINT=http://juken-map-alloy:4318\n' >> "$RUNTIME_ENV_FILE"
fi

unset secret_json LINE_CHANNEL_SECRET LINE_CHANNEL_ACCESS_TOKEN LINE_LOGIN_CHANNEL_ID LINE_LOGIN_CHANNEL_SECRET

aws ecr get-login-password --region ap-northeast-1 \
  | docker login --username AWS --password-stdin 961457613174.dkr.ecr.ap-northeast-1.amazonaws.com

# pull の前に不要イメージを掃除して空き容量を確保する。
# この時点では旧コンテナが稼働中なので、そのイメージは使用中として保護される。
docker image prune -a -f
df -h / | tail -1

# 既定の bridge は名前で引けないので、自分で作ったネットワークに載せる。
docker network inspect "$NETWORK" >/dev/null 2>&1 || docker network create "$NETWORK"

docker pull "$REPO:$IMAGE_TAG"

# ---------------- 無停止デプロイ ----------------
# 以前は同じ3000番で stop → rm → run としていたため、新しいコンテナが listen するまで
# nginx の転送先が無人になり 502 が出ていた（全期間の502のうち16/19がデプロイ直後）。
#
# 新しいコンテナを空いている方のポートで起こし、health が通ってから nginx を向け替える。
# 切り替わるまで古いコンテナが応え続けるので、無人の時間が生まれない。
# おまけに、起動に失敗しても切り替えないだけで済む＝本番には何も起きない。

write_upstream() {
  cat > "$UPSTREAM_CONF" <<EOF
# .github/scripts/deploy-ec2.sh がデプロイのたびに書き換える。手で編集しない。
upstream juken_map_app {
    server 127.0.0.1:$1;
}
EOF
}

host_port_of() {
  docker inspect --format '{{range $p, $conf := .NetworkSettings.Ports}}{{range $conf}}{{.HostPort}}{{"\n"}}{{end}}{{end}}' "$1" 2>/dev/null | head -1
}

# 初回だけ: nginx を upstream 経由にする。中身は今のポートのままなので挙動は変わらない。
[ -f "$UPSTREAM_CONF" ] || write_upstream "$CURRENT_PORT"
if grep -q 'proxy_pass http://localhost:3000;' "$SITE_CONF"; then
  cp "$SITE_CONF" "$SITE_CONF.bak-$(date +%Y%m%d%H%M%S)"
  sed -i 's|proxy_pass http://localhost:3000;|proxy_pass http://juken_map_app;|' "$SITE_CONF"
  echo "nginx: proxy_pass を upstream 経由へ切り替えた（初回のみ）"
fi
nginx -t
systemctl reload nginx

# ここを確かめずに進むと、ポートを入れ替えても nginx が古い方を見続け、
# 静かに502を出し続けることになる。気づけない壊れ方なので先に止める。
grep -q 'proxy_pass http://juken_map_app;' "$SITE_CONF" || {
  echo "nginx が upstream を見ていない。切り替えを中止する" >&2
  exit 1
}

# 前回の付け替え失敗で juken-map-next が残っていることがある。
# それが現役（nginx の向き先）なら消してはいけないので、名前を戻して直す。
if docker inspect juken-map-next >/dev/null 2>&1; then
  if [ "$(host_port_of juken-map-next)" = "$CURRENT_PORT" ]; then
    echo "juken-map-next が現役（$CURRENT_PORT）。名前の付け替えに失敗した形跡があるので直す"
    docker rm -f juken-map >/dev/null 2>&1 || true
    docker rename juken-map-next juken-map
  else
    docker rm -f juken-map-next >/dev/null 2>&1 || true
  fi
fi

echo "deploy: nginx は $CURRENT_PORT を向いている -> 新しいコンテナを $NEW_PORT で起こす"

docker run -d \
  --name juken-map-next \
  --restart always \
  --network "$NETWORK" \
  --env-file "$RUNTIME_ENV_FILE" \
  -p "$NEW_PORT":3000 \
  "$REPO:$IMAGE_TAG"

# スモークテスト: 画面（/login）と DB 接続（/api/health）の両方が 200 を返すまで最大45秒待つ。
# /login は静的な SPA なので、DB に繋がらなくても 200 になる。/api/health が DB を確かめる。
# 見るのは新しいコンテナのポート。この間ずっと、利用者には古いコンテナが応えている。
ok=false
code="not-requested"
for _ in $(seq 1 15); do
  login="$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$NEW_PORT/login" || true)"
  health="$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$NEW_PORT/api/health" || true)"
  code="login=$login health=$health"
  if [ "$login" = "200" ] && [ "$health" = "200" ]; then
    ok=true
    break
  fi
  sleep 3
done

# 失敗なら新しい方を捨てるだけ。nginx は古いコンテナを向いたままなので、本番は無傷。
if [ "$ok" != "true" ]; then
  echo "smoke test failed (last code: $code) -> 新しいコンテナを捨てる（本番は古い方が応え続ける）" >&2
  docker logs --tail 50 juken-map-next 2>&1 || true
  docker rm -f juken-map-next || true
  exit 1
fi

echo "smoke test passed -> nginx を $NEW_PORT へ向ける"
write_upstream "$NEW_PORT"
if ! nginx -t; then
  echo "nginx -t が通らない。向き先を $CURRENT_PORT へ戻す" >&2
  write_upstream "$CURRENT_PORT"
  docker rm -f juken-map-next || true
  exit 1
fi
# reload は既存の接続を処理し終えてから古いワーカーを終わらせる（接続は切れない）。
systemctl reload nginx

# ⚠️ reload はすぐ返るが、古いワーカーは処理中の接続を終えるまで「旧設定」で動き続ける＝
# まだ古いコンテナへ転送している。待たずに止めると、その最中のリクエストだけが502になる。
# 手元のリハーサル（scripts/rehearse-zero-downtime.sh）で実際に1件出たので待つ。
sleep 5

# 切り替わったので古い方を片付け、名前を juken-map に戻す。
# Alloy はコンテナ名でメトリクス（juken-map:9464）とログ（/juken-map）を拾うので、
# 名前を元に戻しておかないと監視が止まる。docker rename は Docker の DNS も追随する。
docker stop juken-map >/dev/null 2>&1 || true
docker rm juken-map >/dev/null 2>&1 || true
docker rename juken-map-next juken-map
echo "deploy: $NEW_PORT へ切り替え完了（旧コンテナを停止）"

# アプリが健全になってから Alloy を入れ替える。ここで失敗してもアプリは動き続け、
# デプロイだけが失敗になるので、監視が壊れたことに気づける。
# 入れ替えの数秒間はトレースの送り先が居らず、その分だけ落ちる（アプリ側は再送を試みる）。
if [ "$OBSERVABILITY" = true ]; then
  mkdir -p "$ALLOY_DIR"
  printf '%s' "$ALLOY_CONFIG_B64" | base64 -d > "$ALLOY_DIR/config.alloy"

  docker pull "$ALLOY_IMAGE"
  docker stop juken-map-alloy || true
  docker rm juken-map-alloy || true
  # t3.micro（メモリ1GB）でアプリと同居するので、Alloy が食う量に上限を付ける。
  # 12345番（Alloy 自身の画面）はネットワークの中だけに開き、ホストには出さない。
  docker run -d \
    --name juken-map-alloy \
    --restart always \
    --network "$NETWORK" \
    --memory 256m \
    --env-file "$ALLOY_ENV_FILE" \
    -v "$ALLOY_DIR/config.alloy:/etc/alloy/config.alloy:ro" \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    "$ALLOY_IMAGE" \
    run \
    --server.http.listen-addr=0.0.0.0:12345 \
    --storage.path=/var/lib/alloy/data \
    /etc/alloy/config.alloy

  echo "observability: alloy started"
else
  echo "observability: skipped (Grafana Cloud の接続情報が未設定)"
  # 接続情報を外したときは、古い設定のまま動き続けないよう止める。
  docker stop juken-map-alloy >/dev/null 2>&1 || true
  docker rm juken-map-alloy >/dev/null 2>&1 || true
fi

# 入れ替えで未使用になった旧イメージを回収する。
docker image prune -a -f
