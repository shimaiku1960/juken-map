#!/usr/bin/env bash

set -euo pipefail

IMAGE_TAG="${1:?IMAGE_TAG is required}"
REPO="961457613174.dkr.ecr.ap-northeast-1.amazonaws.com/juken-map"
ENV_FILE="/home/ubuntu/juken-map/.env"

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
  --env-file "$ENV_FILE" \
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
      --env-file "$ENV_FILE" \
      -p 3000:3000 \
      "$PREV"
  fi
  exit 1
fi

echo "smoke test passed"

# 入れ替えで未使用になった旧イメージを回収する。
docker image prune -a -f
