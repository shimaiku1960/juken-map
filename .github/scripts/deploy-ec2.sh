#!/usr/bin/env bash

set -euo pipefail

IMAGE_TAG="${1:?IMAGE_TAG is required}"
# observability/alloy/production.alloy を base64 にしたもの（deploy.yml が渡す）。
# 空のときは可観測性の送信を丸ごと省く。
ALLOY_CONFIG_B64="${2:-}"
# infra/nginx/juken-map-go-routes.conf（Go へ振り分けるパス）を base64 にしたもの（deploy.yml が渡す）。
# 空のときは Go へ振り分けない＝全部 Node が返す。
GO_ROUTES_B64="${3:-}"
# infra/systemd/（毎日の通知のタイマー、JUK-85）を tar.gz にして base64 にしたもの（deploy.yml が渡す）。
# 空のときはタイマーに触らない。
SYSTEMD_UNITS_B64="${4:-}"
REPO="961457613174.dkr.ecr.ap-northeast-1.amazonaws.com/juken-map"
# Go の API（apps/api-go、JUK-72）。Node と同じコミットから作ったイメージを並べて動かす。
REPO_GO="961457613174.dkr.ecr.ap-northeast-1.amazonaws.com/juken-map-go"
ENV_FILE="${ENV_FILE:-/home/ubuntu/juken-map/.env}"
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
GO_UPSTREAM_CONF="${GO_UPSTREAM_CONF:-/etc/nginx/conf.d/juken-map-go-upstream.conf}"
# Go へ振り分けるパスの置き場。サイト設定（443 の server）がこれを include する。
GO_ROUTES_CONF="${GO_ROUTES_CONF:-/etc/nginx/juken-map/go-routes.conf}"
SITE_CONF="${SITE_CONF:-/etc/nginx/sites-available/default}"
# 毎日の通知のタイマーの置き場と、タイマーが読む共有トークンのファイル（root だけが読める）。
SYSTEMD_DIR="${SYSTEMD_DIR:-/etc/systemd/system}"
NOTIFY_ENV_FILE="${NOTIFY_ENV_FILE:-/etc/juken-map/daily-notification.env}"
NOTIFY_TIMERS="juken-map-daily-notification-morning.timer juken-map-daily-notification-evening.timer"
PORT_A=3000
PORT_B=3001
GO_PORT_A=8080
GO_PORT_B=8081

# current_port upstreamファイル 既定のポート → いま nginx が向いている先。
# ファイルがまだ無い初回は既定のポート（Node はこれまで通り3000）とみなす。
#
# ⚠️ ここで `sed ... "$1" | head -1` と書くと、ファイルが無い初回に sed が
# exit 2 を返し、pipefail と set -e でスクリプトが無言で死ぬ（2026-09-23に本番で踏んだ）。
# 読む前に必ず存在を確かめること。
current_port() {
  local port="$2" found
  if [ -f "$1" ]; then
    found="$(sed -nE 's/.*127\.0\.0\.1:([0-9]+).*/\1/p' "$1" | head -1)"
    [ -n "$found" ] && port="$found"
  fi
  echo "$port"
}

CURRENT_PORT="$(current_port "$UPSTREAM_CONF" "$PORT_A")"
if [ "$CURRENT_PORT" = "$PORT_A" ]; then NEW_PORT="$PORT_B"; else NEW_PORT="$PORT_A"; fi
GO_CURRENT_PORT="$(current_port "$GO_UPSTREAM_CONF" "$GO_PORT_A")"
if [ "$GO_CURRENT_PORT" = "$GO_PORT_A" ]; then GO_NEW_PORT="$GO_PORT_B"; else GO_NEW_PORT="$GO_PORT_A"; fi

if [ "${DEPLOY_PORTS_ONLY:-}" = "1" ]; then
  echo "CURRENT_PORT=$CURRENT_PORT NEW_PORT=$NEW_PORT GO_CURRENT_PORT=$GO_CURRENT_PORT GO_NEW_PORT=$GO_NEW_PORT"
  exit 0
fi

# Secrets Managerから取得した値はコンテナ作成時だけ一時ファイルに置く。
# 既存.envから移行対象キーを除外し、同じ環境変数が重複しない状態でDockerへ渡す。
RUNTIME_ENV_FILE="$(mktemp)"
GO_ENV_FILE="$(mktemp)"
ALLOY_ENV_FILE="$(mktemp)"
MIGRATE_ENV_FILE="$(mktemp)"
trap 'rm -f "$RUNTIME_ENV_FILE" "$GO_ENV_FILE" "$ALLOY_ENV_FILE" "$MIGRATE_ENV_FILE"' EXIT
chmod 600 "$RUNTIME_ENV_FILE" "$GO_ENV_FILE" "$ALLOY_ENV_FILE" "$MIGRATE_ENV_FILE"

secret_json="$(aws secretsmanager get-secret-value \
  --secret-id "$RUNTIME_SECRET_ID" \
  --region ap-northeast-1 \
  --query SecretString \
  --output text)"

# DB の接続先。アプリ用（DML だけ）とマイグレーション用（テーブル定義も変えられる）の2つを
# シークレットに置く（権限は apps/api/src/infra/dbUsers.ts）。マイグレーション用はアプリの
# コンテナに渡さず、起動前に1回きりのコンテナで使うだけにする。
# 2つともまだ無い間は、これまで通り .env の DATABASE_URL で繋ぎ、起動時にマイグレーションを当てる。
APP_DATABASE_URL="$(jq -r '.DATABASE_URL // empty' <<<"$secret_json")"
MIGRATION_DATABASE_URL="$(jq -r '.MIGRATION_DATABASE_URL // empty' <<<"$secret_json")"
if [ -n "$APP_DATABASE_URL" ] && [ -n "$MIGRATION_DATABASE_URL" ]; then
  SEPARATE_DB_USERS=true
elif [ -z "$APP_DATABASE_URL" ] && [ -z "$MIGRATION_DATABASE_URL" ]; then
  SEPARATE_DB_USERS=false
else
  # 片方だけだと、アプリがマイグレーション用の権限で動くか、マイグレーションが当たらないかになる。
  echo "シークレットの DATABASE_URL と MIGRATION_DATABASE_URL は2つそろえて置く必要がある" >&2
  exit 1
fi

# 可観測性の2つは、下で Grafana Cloud の接続情報が揃ったときだけ付け直す。
# DATABASE_URL はシークレットにあればそちらを使うので、.env の値は渡さない。
EXCLUDED_KEYS='LINE_CHANNEL_SECRET|LINE_CHANNEL_ACCESS_TOKEN|LINE_LOGIN_CHANNEL_ID|LINE_LOGIN_CHANNEL_SECRET|METRICS_PORT|OTEL_EXPORTER_OTLP_ENDPOINT|SKIP_MIGRATIONS|MIGRATION_DATABASE_URL'
[ "$SEPARATE_DB_USERS" = true ] && EXCLUDED_KEYS="$EXCLUDED_KEYS|DATABASE_URL"
grep -Ev "^($EXCLUDED_KEYS)=" "$ENV_FILE" > "$RUNTIME_ENV_FILE"

if [ "$SEPARATE_DB_USERS" = true ]; then
  printf 'DATABASE_URL=%s\n' "$APP_DATABASE_URL" >> "$RUNTIME_ENV_FILE"
  # アプリのユーザーはテーブルを作れないので、起動時のマイグレーションを飛ばす（docker-entrypoint.sh）。
  printf 'SKIP_MIGRATIONS=1\n' >> "$RUNTIME_ENV_FILE"
  printf 'MIGRATION_DATABASE_URL=%s\n' "$MIGRATION_DATABASE_URL" >> "$MIGRATE_ENV_FILE"
fi

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

# Go に渡すのは Go が読む値だけ（apps/api-go/README.md の環境変数の表）。
# DATABASE_URL は Node と同じ接続先（シークレットがあればアプリ用＝DML だけのユーザー）。
# 毎日の通知（JUK-74）を Go が送るので、送信に使う3つ（Resend のキー・LINE の送信用トークン・cron の共有トークン）も渡す。
# シミュレーションの API（JUK-80）も Go が受けるので、有効にするかと共有トークンの2つも渡す（.env に無ければ渡らず、404 のまま）。
# LINE の Webhook の署名用（LINE_CHANNEL_SECRET）や LINE ログインの秘密は、Go がまだ使わないので渡さない。
grep -E '^(DATABASE_URL|BETTER_AUTH_SECRET|METRICS_PORT|RESEND_API_KEY|LINE_CHANNEL_ACCESS_TOKEN|DAILY_NOTIFICATION_SECRET|SIMULATION_ENABLED|SIMULATION_SECRET)=' \
  "$RUNTIME_ENV_FILE" > "$GO_ENV_FILE" || true
for key in DATABASE_URL BETTER_AUTH_SECRET; do
  grep -q "^$key=" "$GO_ENV_FILE" || { echo "Go に渡す $key が見つからない" >&2; exit 1; }
done
# NODE_ENV=production は reqId を UUID のまま出すため（Node と揃える）。
# GOMEMLIMIT はコンテナの上限（下の --memory 128m）に近づいたら GC を早めさせる。
printf 'NODE_ENV=production\nGOMEMLIMIT=96MiB\n' >> "$GO_ENV_FILE"

unset secret_json LINE_CHANNEL_SECRET LINE_CHANNEL_ACCESS_TOKEN LINE_LOGIN_CHANNEL_ID LINE_LOGIN_CHANNEL_SECRET \
  APP_DATABASE_URL MIGRATION_DATABASE_URL

aws ecr get-login-password --region ap-northeast-1 \
  | docker login --username AWS --password-stdin 961457613174.dkr.ecr.ap-northeast-1.amazonaws.com

# pull の前に不要イメージを掃除して空き容量を確保する。
# この時点では旧コンテナが稼働中なので、そのイメージは使用中として保護される。
docker image prune -a -f
df -h / | tail -1

# 既定の bridge は名前で引けないので、自分で作ったネットワークに載せる。
docker network inspect "$NETWORK" >/dev/null 2>&1 || docker network create "$NETWORK"

docker pull "$REPO:$IMAGE_TAG"
docker pull "$REPO_GO:$IMAGE_TAG"

# ---------------- 無停止デプロイ ----------------
# 以前は同じ3000番で stop → rm → run としていたため、新しいコンテナが listen するまで
# nginx の転送先が無人になり 502 が出ていた（全期間の502のうち16/19がデプロイ直後）。
#
# 新しいコンテナを空いている方のポートで起こし、health が通ってから nginx を向け替える。
# 切り替わるまで古いコンテナが応え続けるので、無人の時間が生まれない。
# おまけに、起動に失敗しても切り替えないだけで済む＝本番には何も起きない。

# write_upstream ファイル upstream名 ポート
write_upstream() {
  cat > "$1" <<EOF
# .github/scripts/deploy-ec2.sh がデプロイのたびに書き換える。手で編集しない。
upstream $2 {
    server 127.0.0.1:$3;
}
EOF
}

host_port_of() {
  docker inspect --format '{{range $p, $conf := .NetworkSettings.Ports}}{{range $conf}}{{.HostPort}}{{"\n"}}{{end}}{{end}}' "$1" 2>/dev/null | head -1
}

# 初回だけ: nginx を upstream 経由にする。中身は今のポートのままなので挙動は変わらない。
[ -f "$UPSTREAM_CONF" ] || write_upstream "$UPSTREAM_CONF" juken_map_app "$CURRENT_PORT"
if grep -q 'proxy_pass http://localhost:3000;' "$SITE_CONF"; then
  cp "$SITE_CONF" "$SITE_CONF.bak-$(date +%Y%m%d%H%M%S)"
  sed -i 's|proxy_pass http://localhost:3000;|proxy_pass http://juken_map_app;|' "$SITE_CONF"
  echo "nginx: proxy_pass を upstream 経由へ切り替えた（初回のみ）"
fi

# 初回だけ: Go の upstream と、Go へ振り分けるパスの置き場を作り、サイト設定から include する。
# 置き場は空で作るので、この時点ではまだ全部 Node へ行く（挙動は変わらない）。
# Go のコンテナがまだ無くても、upstream を名前で使うパスが無いので 502 は出ない。
[ -f "$GO_UPSTREAM_CONF" ] || write_upstream "$GO_UPSTREAM_CONF" juken_map_go "$GO_CURRENT_PORT"
mkdir -p "$(dirname "$GO_ROUTES_CONF")"
[ -f "$GO_ROUTES_CONF" ] || : > "$GO_ROUTES_CONF"
if ! grep -qF "include $GO_ROUTES_CONF;" "$SITE_CONF"; then
  # location / {（全部を Node へ送るところ）の直前に差し込む。= の完全一致は / より優先されるので
  # 順番に意味は無いが、読む人が「ここだけ Go」と分かる位置に置く。
  # location / が1つでないとき（雛形のコメント以外に2つある等）は、どこに入れるべきか決められないので止める。
  count="$(grep -cE '^[[:space:]]*location / \{' "$SITE_CONF" || true)"
  if [ "$count" != "1" ]; then
    echo "nginx: サイト設定の location / が $count 個ある。Go の振り分けを差し込めないので中止する" >&2
    exit 1
  fi
  cp "$SITE_CONF" "$SITE_CONF.bak-$(date +%Y%m%d%H%M%S)"
  # sed -i は GNU と BSD（手元の Mac）で書き方が違うので、awk で書き出してから上書きする。
  # cat > で上書きすると、ファイルの持ち主・権限はそのまま残る。
  site_tmp="$(mktemp)"
  awk -v inc="include $GO_ROUTES_CONF;" '
    /^[[:space:]]*location \/ \{/ { match($0, /^[[:space:]]*/); print substr($0, 1, RLENGTH) inc }
    { print }
  ' "$SITE_CONF" > "$site_tmp"
  cat "$site_tmp" > "$SITE_CONF"
  rm -f "$site_tmp"
  echo "nginx: Go の振り分け（${GO_ROUTES_CONF}）を include した（初回のみ）"
fi
nginx -t
systemctl reload nginx

# ここを確かめずに進むと、ポートを入れ替えても nginx が古い方を見続け、
# 静かに502を出し続けることになる。気づけない壊れ方なので先に止める。
grep -q 'proxy_pass http://juken_map_app;' "$SITE_CONF" || {
  echo "nginx が upstream を見ていない。切り替えを中止する" >&2
  exit 1
}

# 前回の付け替え失敗で <名前>-next が残っていることがある。
# それが現役（nginx の向き先）なら消してはいけないので、名前を戻して直す。
recover_next() {  # $1=コンテナ名 $2=nginx が向いているポート
  docker inspect "$1-next" >/dev/null 2>&1 || return 0
  if [ "$(host_port_of "$1-next")" = "$2" ]; then
    echo "$1-next が現役（${2}）。名前の付け替えに失敗した形跡があるので直す"
    docker rm -f "$1" >/dev/null 2>&1 || true
    docker rename "$1-next" "$1"
  else
    docker rm -f "$1-next" >/dev/null 2>&1 || true
  fi
}
recover_next juken-map "$CURRENT_PORT"
recover_next juken-map-go "$GO_CURRENT_PORT"

# マイグレーションを、新しいイメージの1回きりのコンテナで先に当てる。失敗したら set -e でここで
# 止まり、新しいコンテナは起動しない（nginx は古いコンテナを向いたままなので、本番は無傷）。
if [ "$SEPARATE_DB_USERS" = true ]; then
  echo "deploy: マイグレーションを当てる"
  docker run --rm \
    --network "$NETWORK" \
    --env-file "$MIGRATE_ENV_FILE" \
    "$REPO:$IMAGE_TAG" \
    migrate
fi

echo "deploy: nginx は Node ${CURRENT_PORT}・Go $GO_CURRENT_PORT を向いている -> 新しいコンテナを Node ${NEW_PORT}・Go $GO_NEW_PORT で起こす"

docker run -d \
  --name juken-map-next \
  --restart always \
  --network "$NETWORK" \
  --env-file "$RUNTIME_ENV_FILE" \
  -p "$NEW_PORT":3000 \
  "$REPO:$IMAGE_TAG"

# Go はメモリを数十MB しか使わないが、t3.micro（1GB）で Node と同居するので上限を付ける。
# ポートはホストの中（nginx）からだけ届けばよいので 127.0.0.1 に開く。
docker run -d \
  --name juken-map-go-next \
  --restart always \
  --network "$NETWORK" \
  --memory 128m \
  --env-file "$GO_ENV_FILE" \
  -p "127.0.0.1:$GO_NEW_PORT":8080 \
  "$REPO_GO:$IMAGE_TAG"

# スモークテスト: 最大45秒待つ。見るのは新しいコンテナのポート。この間ずっと、利用者には古いコンテナが応えている。
# - Node: 画面（/login）と DB 接続（/api/health）。/login は静的な SPA なので DB に繋がらなくても 200 になる
# - Go: DB 接続（/api/health）と、ダッシュボードのルートがあること（Cookie 無しなので 401）
ok=false
code="not-requested"
for _ in $(seq 1 15); do
  login="$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$NEW_PORT/login" || true)"
  health="$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$NEW_PORT/api/health" || true)"
  go_health="$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$GO_NEW_PORT/api/health" || true)"
  go_dashboard="$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$GO_NEW_PORT/api/dashboard" || true)"
  code="login=$login health=$health go_health=$go_health go_dashboard=$go_dashboard"
  if [ "$login" = "200" ] && [ "$health" = "200" ] && [ "$go_health" = "200" ] && [ "$go_dashboard" = "401" ]; then
    ok=true
    break
  fi
  sleep 3
done

discard_next() {
  docker rm -f juken-map-next juken-map-go-next >/dev/null 2>&1 || true
}

# 失敗なら新しい方を捨てるだけ。nginx は古いコンテナを向いたままなので、本番は無傷。
# Node と Go のどちらかが駄目なら、両方とも切り替えない（同じコミットの組でしか動かさない）。
if [ "$ok" != "true" ]; then
  echo "smoke test failed (last code: $code) -> 新しいコンテナを捨てる（本番は古い方が応え続ける）" >&2
  docker logs --tail 50 juken-map-next 2>&1 || true
  docker logs --tail 50 juken-map-go-next 2>&1 || true
  discard_next
  exit 1
fi

echo "smoke test passed -> nginx を Node ${NEW_PORT}・Go $GO_NEW_PORT へ向け、Go の振り分けを置く"
cp "$GO_ROUTES_CONF" "$GO_ROUTES_CONF.prev"
write_upstream "$UPSTREAM_CONF" juken_map_app "$NEW_PORT"
write_upstream "$GO_UPSTREAM_CONF" juken_map_go "$GO_NEW_PORT"
printf '%s' "$GO_ROUTES_B64" | base64 -d > "$GO_ROUTES_CONF"
if ! nginx -t; then
  echo "nginx -t が通らない。向き先と振り分けを元に戻す" >&2
  write_upstream "$UPSTREAM_CONF" juken_map_app "$CURRENT_PORT"
  write_upstream "$GO_UPSTREAM_CONF" juken_map_go "$GO_CURRENT_PORT"
  mv "$GO_ROUTES_CONF.prev" "$GO_ROUTES_CONF"
  discard_next
  exit 1
fi
rm -f "$GO_ROUTES_CONF.prev"
# reload は既存の接続を処理し終えてから古いワーカーを終わらせる（接続は切れない）。
systemctl reload nginx

# ⚠️ reload はすぐ返るが、古いワーカーは処理中の接続を終えるまで「旧設定」で動き続ける＝
# まだ古いコンテナへ転送している。待たずに止めると、その最中のリクエストだけが502になる。
# 手元のリハーサル（scripts/rehearse-zero-downtime.sh）で実際に1件出たので待つ。
sleep 5

# 切り替わったので古い方を片付け、名前を juken-map・juken-map-go に戻す。
# Alloy はコンテナ名でメトリクス（juken-map:9464・juken-map-go:9464）とログを拾うので、
# 名前を元に戻しておかないと監視が止まる。docker rename は Docker の DNS も追随する。
for name in juken-map juken-map-go; do
  docker stop "$name" >/dev/null 2>&1 || true
  docker rm "$name" >/dev/null 2>&1 || true
  docker rename "$name-next" "$name"
done
echo "deploy: Node ${NEW_PORT}・Go $GO_NEW_PORT へ切り替え完了（旧コンテナを停止）"

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

# ---- 毎日の通知のタイマー（JUK-85）----
# 朝7時・夜21時（日本時間）に、EC2 の systemd timer が通知の入口（Go）を呼ぶ。GitHub Actions の schedule は
# 毎回2〜4時間遅れ、夜の分が翌日の未明に届いていたので移した。定義は infra/systemd/ が正。
# アプリの切り替えが済んでから入れる。ここで失敗してもアプリは動き続け、デプロイだけが失敗になる。
if [ -n "$SYSTEMD_UNITS_B64" ]; then
  notify_secret="$(sed -n 's/^DAILY_NOTIFICATION_SECRET=//p' "$RUNTIME_ENV_FILE" | tail -1)"
  if [ -z "$notify_secret" ]; then
    # 共有トークンが無いと、呼んでも 401 になるだけなので止めておく。
    echo "timers: DAILY_NOTIFICATION_SECRET が無いので、毎日の通知のタイマーを止める" >&2
    # shellcheck disable=SC2086
    systemctl disable --now $NOTIFY_TIMERS >/dev/null 2>&1 || true
  else
    units_dir="$(mktemp -d)"
    printf '%s' "$SYSTEMD_UNITS_B64" | base64 -d | tar -xz -C "$units_dir"
    install -d -m 755 "$SYSTEMD_DIR"
    install -m 644 "$units_dir"/juken-map-daily-notification* "$SYSTEMD_DIR"/
    rm -rf "$units_dir"

    # 書き込み途中のファイルを読まれないよう、同じ場所に作ってから差し替える。最初から root だけが読める。
    install -d -m 700 "$(dirname "$NOTIFY_ENV_FILE")"
    (umask 077; printf 'DAILY_NOTIFICATION_SECRET=%s\n' "$notify_secret" > "$NOTIFY_ENV_FILE.tmp")
    mv "$NOTIFY_ENV_FILE.tmp" "$NOTIFY_ENV_FILE"

    systemctl daemon-reload
    # shellcheck disable=SC2086
    systemctl enable --now $NOTIFY_TIMERS
    echo "timers: 毎日の通知のタイマーを入れた（朝7時・夜21時）"
  fi
  unset notify_secret
fi
