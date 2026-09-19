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

# 今動いているコンテナのイメージを退避（失敗時のロールバック先）
PREV="$(docker inspect --format '{{.Config.Image}}' juken-map 2>/dev/null || true)"

# pull の前に不要イメージを掃除して空き容量を確保する。
# この時点では旧コンテナが稼働中なので、ロールバック先は使用中として保護される。
docker image prune -a -f
df -h / | tail -1

# 既定の bridge は名前で引けないので、自分で作ったネットワークに載せる。
docker network inspect "$NETWORK" >/dev/null 2>&1 || docker network create "$NETWORK"

docker pull "$REPO:$IMAGE_TAG"
docker stop juken-map || true
docker rm juken-map || true
docker run -d \
  --name juken-map \
  --restart always \
  --network "$NETWORK" \
  --env-file "$RUNTIME_ENV_FILE" \
  -p 3000:3000 \
  "$REPO:$IMAGE_TAG"

# スモークテスト: 画面（/login）と DB 接続（/api/health）の両方が 200 を返すまで最大45秒待つ。
# /login は静的な SPA なので、DB に繋がらなくても 200 になる。/api/health が DB を確かめる。
ok=false
code="not-requested"
for _ in $(seq 1 15); do
  login="$(curl -s -o /dev/null -w '%{http_code}' http://localhost:3000/login || true)"
  health="$(curl -s -o /dev/null -w '%{http_code}' http://localhost:3000/api/health || true)"
  code="login=$login health=$health"
  if [ "$login" = "200" ] && [ "$health" = "200" ]; then
    ok=true
    break
  fi
  sleep 3
done

# 失敗なら前のイメージへロールバックし、デプロイを失敗にする
if [ "$ok" != "true" ]; then
  echo "smoke test failed (last code: $code) -> rollback to $PREV"
  docker stop juken-map || true
  docker rm juken-map || true
  if [ -n "$PREV" ]; then
    docker run -d \
      --name juken-map \
      --restart always \
      --network "$NETWORK" \
      --env-file "$RUNTIME_ENV_FILE" \
      -p 3000:3000 \
      "$PREV"
  fi
  exit 1
fi

echo "smoke test passed"

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
