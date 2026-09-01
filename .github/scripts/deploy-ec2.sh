#!/usr/bin/env bash

set -euo pipefail

IMAGE_TAG="${1:?IMAGE_TAG is required}"
REPO="961457613174.dkr.ecr.ap-northeast-1.amazonaws.com/juken-map"
ENV_FILE="/home/ubuntu/juken-map/.env"
RUNTIME_SECRET_ID="juken-map/production/runtime"

# Secrets Managerから取得した値はコンテナ作成時だけ一時ファイルに置く。
# 既存.envから移行対象キーを除外し、同じ環境変数が重複しない状態でDockerへ渡す。
RUNTIME_ENV_FILE="$(mktemp)"
trap 'rm -f "$RUNTIME_ENV_FILE"' EXIT
chmod 600 "$RUNTIME_ENV_FILE"
grep -Ev '^(LINE_CHANNEL_SECRET|LINE_CHANNEL_ACCESS_TOKEN|LINE_LOGIN_CHANNEL_ID|LINE_LOGIN_CHANNEL_SECRET)=' "$ENV_FILE" > "$RUNTIME_ENV_FILE"

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
unset secret_json LINE_CHANNEL_SECRET LINE_CHANNEL_ACCESS_TOKEN LINE_LOGIN_CHANNEL_ID LINE_LOGIN_CHANNEL_SECRET

aws ecr get-login-password --region ap-northeast-1 \
  | docker login --username AWS --password-stdin 961457613174.dkr.ecr.ap-northeast-1.amazonaws.com

# 今動いているコンテナのイメージを退避（失敗時のロールバック先）
PREV="$(docker inspect --format '{{.Config.Image}}' juken-map 2>/dev/null || true)"

# pull の前に不要イメージを掃除して空き容量を確保する。
# この時点では旧コンテナが稼働中なので、ロールバック先は使用中として保護される。
docker image prune -a -f
df -h / | tail -1

docker pull "$REPO:$IMAGE_TAG"
docker stop juken-map || true
docker rm juken-map || true
docker run -d \
  --name juken-map \
  --restart always \
  --env-file "$RUNTIME_ENV_FILE" \
  -p 3000:3000 \
  "$REPO:$IMAGE_TAG"

# スモークテスト: /login が 200 を返すまで最大45秒待つ
ok=false
code="not-requested"
for _ in $(seq 1 15); do
  code="$(curl -s -o /dev/null -w '%{http_code}' http://localhost:3000/login || true)"
  if [ "$code" = "200" ]; then
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
      --env-file "$RUNTIME_ENV_FILE" \
      -p 3000:3000 \
      "$PREV"
  fi
  exit 1
fi

echo "smoke test passed"

# 入れ替えで未使用になった旧イメージを回収する。
docker image prune -a -f
