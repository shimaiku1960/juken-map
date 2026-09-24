#!/usr/bin/env bash
# 本番の AMI から複製した EC2 を、試験用の設定で動かし直す。
# 手元から ssm-run.sh で送る（root で動く）。
#
#   app-prepare.sh [--init-disk] [--overload-max N]
#
#   --init-disk       AMI から作ったディスクは、初めて読むブロックを S3 から取り寄せるので遅い。
#                     そのまま測ると結果が悪く出るので、最初の1回だけ全体を読んでおく（数分〜十数分）。
#   --overload-max N  OVERLOAD_MAX_IN_FLIGHT（混雑時に断る同時処理数の上限、JUK-7）を N にする。
#                     付けなければ本番と同じ（アプリの既定値）。
#
# イメージ・ネットワーク・ポート・再起動の設定は本番のコンテナをそのまま引き継ぎ、
# 環境変数だけを差し替える。Alloy は本番と同じくメモリを使うので残す（外へ出られないので送信は失敗するだけ）。
set -euo pipefail

PARAM_NAME="/juken-map-loadtest/env"
REGION="ap-northeast-1"
EXPECTED_VPC_CIDR="10.50.0.0/16"
UPSTREAM_CONF="/etc/nginx/conf.d/juken-map-upstream.conf"
# 試験に持ち込まない外部サービスの鍵。値を捨ててダミーにする（消すと起動時に要求するものがあるため）。
DUMMY_KEYS='RESEND_API_KEY|AUTH_GOOGLE_SECRET|AUTH_GITHUB_SECRET|LINE_CHANNEL_SECRET|LINE_CHANNEL_ACCESS_TOKEN|LINE_LOGIN_CHANNEL_SECRET|MICROCMS_API_KEY|DAILY_NOTIFICATION_SECRET|SIMULATION_SECRET'
# 丸ごと外すもの。DB と認証の秘密は試験用の値を後から足す。
DROP_KEYS='DATABASE_URL|BETTER_AUTH_SECRET|SIMULATION_ENABLED|FARO_COLLECTOR_URL|OVERLOAD_MAX_IN_FLIGHT|PATH|NODE_VERSION|YARN_VERSION'

init_disk=off
overload_max=""
while [ $# -gt 0 ]; do
  case "$1" in
    --init-disk) init_disk=on ;;
    --overload-max) overload_max="${2:?}"; shift ;;
    *) echo "知らない引数: $1" >&2; exit 1 ;;
  esac
  shift
done

# ---- 0. ここが本当に試験用の VPC か確かめる（本番 EC2 に誤って送ったときに何もせず止まる）----
imds() {
  local token
  token="$(curl -fsS -X PUT http://169.254.169.254/latest/api/token -H 'X-aws-ec2-metadata-token-ttl-seconds: 60')"
  curl -fsS -H "X-aws-ec2-metadata-token: $token" "http://169.254.169.254/latest/meta-data/$1"
}
mac="$(imds mac)"
vpc_cidr="$(imds "network/interfaces/macs/$mac/vpc-ipv4-cidr-block")"
if [ "$vpc_cidr" != "$EXPECTED_VPC_CIDR" ]; then
  echo "試験用の VPC（${EXPECTED_VPC_CIDR}）ではありません（${vpc_cidr}）。何もせず止めます" >&2
  exit 1
fi

# ---- 1. 外へ出られないことを確かめる ----
if curl -s -m 5 -o /dev/null https://api.resend.com; then
  echo "インターネットへ出られてしまいます。SG と経路を確かめるまで止めます" >&2
  exit 1
fi
echo "ok: 外へは出られない"

# ---- 2. ディスクを一度読み切る ----
if [ "$init_disk" = on ]; then
  root_disk="/dev/$(lsblk -no PKNAME "$(findmnt -no SOURCE /)")"
  echo "ディスク $root_disk を読み切ります"
  start="$(date +%s)"
  dd if="$root_disk" of=/dev/null bs=1M iflag=direct status=none
  echo "ok: 読み切り $(( $(date +%s) - start ))秒"
fi

# ---- 3. 試験の邪魔になる定期処理を止める（どれも外へ出られず失敗するだけだが、CPU とディスクを使う）----
systemctl stop certbot.timer apt-daily.timer apt-daily-upgrade.timer man-db.timer \
  update-notifier-download.timer fwupd-refresh.timer motd-news.timer 2>/dev/null || true

# ---- 4. アプリのコンテナを試験用の設定で作り直す ----
image="$(docker inspect juken-map --format '{{.Config.Image}}')"
port="$(sed -nE 's/.*127\.0\.0\.1:([0-9]+).*/\1/p' "$UPSTREAM_CONF" | head -1)"
[ -n "$port" ] || { echo "nginx の向き先ポートが読めません（${UPSTREAM_CONF}）" >&2; exit 1; }

env_file="$(mktemp)"
chmod 600 "$env_file"
trap 'rm -f "$env_file"' EXIT
docker inspect juken-map --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | grep -v -E "^($DROP_KEYS)=" \
  | sed -E "s/^($DUMMY_KEYS)=.*/\1=loadtest-dummy/" \
  | sed -E 's/^ADMIN_NOTIFICATION_EMAIL=.*/ADMIN_NOTIFICATION_EMAIL=loadtest@example.invalid/' \
  | grep -v '^$' > "$env_file"
aws ssm get-parameter --region "$REGION" --name "$PARAM_NAME" --with-decryption \
  --query Parameter.Value --output text | grep -v '^$' >> "$env_file"
[ -n "$overload_max" ] && echo "OVERLOAD_MAX_IN_FLIGHT=$overload_max" >> "$env_file"

docker rm -f juken-map-next >/dev/null 2>&1 || true
docker rm -f juken-map >/dev/null
docker run -d \
  --name juken-map \
  --restart always \
  --network juken-map \
  --env-file "$env_file" \
  -p "$port":3000 \
  "$image" >/dev/null

# nginx 越しに、DB まで繋がるか（/api/health）を見る。
ok=false
for _ in $(seq 1 30); do
  code="$(curl -sk -o /dev/null -w '%{http_code}' https://127.0.0.1/api/health || true)"
  if [ "$code" = 200 ]; then ok=true; break; fi
  sleep 3
done
if [ "$ok" != true ]; then
  echo "アプリが起動しません（最後の /api/health: ${code}）" >&2
  docker logs --tail 40 juken-map >&2 || true
  exit 1
fi

# ---- 5. 今の状態 ----
echo "ok: アプリ起動（イメージ ${image##*:}、ポート ${port}、OVERLOAD_MAX_IN_FLIGHT=${overload_max:-既定}）"
echo "差し替えたキー（値は出さない）:"
cut -d= -f1 "$env_file" | sort | tr '\n' ' '; echo
docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}'
free -m
swapon --show
