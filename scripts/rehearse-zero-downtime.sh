#!/usr/bin/env bash
# 無停止デプロイのリハーサル。本番の deploy-ec2.sh に入れた切り替え方が、本当に
# 転送先の空白を作らないかを手元で確かめる。
#
# 本番と同じ形を小さく再現する：
#   curl ──▶ [nginx] ──▶ ホストのポート ──▶ [アプリのコンテナ]
#            upstream ファイルを書き換えて reload すると向き先が変わる
#
# 旧方式（同じポートで stop → run）と新方式（別ポートで起こして向け替え）を、
# 同じ起動待ち時間で順に流し、その間の応答を数える。
set -euo pipefail

NET="zdt-rehearsal"
NGINX="zdt-nginx"
PROXY_PORT=18080
PORT_A=13000
PORT_B=13001
# アプリが listen するまでの時間。本番も起動に数秒かかるので、そこを再現する。
BOOT_SECONDS="${BOOT_SECONDS:-5}"
# reload 後、古いワーカーが処理中の接続を終えるのを待つ時間。
DRAIN_SECONDS="${DRAIN_SECONDS:-5}"
WORK="$(mktemp -d)"

cleanup() {
  docker rm -f "$NGINX" zdt-app-a zdt-app-b >/dev/null 2>&1 || true
  docker network rm "$NET" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

write_upstream() {
  cat > "$WORK/conf.d/upstream.conf" <<EOF
upstream juken_map_app {
    server host.docker.internal:$1;
}
EOF
}

start_app() {  # $1=コンテナ名 $2=ホストのポート $3=返す中身
  docker run -d --name "$1" -p "$2":80 busybox sh -c \
    "mkdir -p /www && echo $3 > /www/index.html && sleep $BOOT_SECONDS && httpd -f -p 80 -h /www" >/dev/null
}

wait_ok() {  # $1=ポート。listen するまで待つ（本番のスモークテストにあたる）
  for _ in $(seq 1 30); do
    [ "$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:$1/" || true)" = "200" ] && return 0
    sleep 1
  done
  return 1
}

# nginx は接続に失敗した転送先を一定時間「死んでいる」と覚える（fail_timeout、既定10秒）。
# そのため旧方式で一度502を出すと、アプリが復帰しても数秒は502が続き、次の計測に混ざる。
# 落ち着くまで待ってから次を測る。
settle_proxy() {
  local ok=0
  for _ in $(seq 1 60); do
    if [ "$(curl -s -m 2 -o /dev/null -w '%{http_code}' "http://localhost:$PROXY_PORT/" || true)" = "200" ]; then
      ok=$((ok + 1)); [ "$ok" -ge 5 ] && return 0
    else
      ok=0
    fi
    sleep 0.5
  done
  echo "proxy が安定しない" >&2; return 1
}

# 切り替えのあいだ、利用者の立場で叩き続ける。数えるのは 200 以外。
watch_start() {
  rm -f "$WORK/fail" "$WORK/total"; : > "$WORK/fail"; : > "$WORK/total"
  # 標準出力を閉じておく。開けたままだと、この関数を $( ) で受けた側が
  # バックグラウンドのループが終わるまで待ち続けて止まる。
  ( while :; do
      code="$(curl -s -m 2 -o /dev/null -w '%{http_code}' "http://localhost:$PROXY_PORT/" || echo 000)"
      echo x >> "$WORK/total"
      [ "$code" = "200" ] || echo "$(date +%s.%N) $code" >> "$WORK/fail"
      sleep 0.1
    done ) >/dev/null 2>&1 &
  echo $!
}

report() {  # $1=見出し $2=watcher の pid
  kill "$2" 2>/dev/null || true; wait "$2" 2>/dev/null || true
  local total fail
  total="$(wc -l < "$WORK/total" | tr -d ' ')"
  fail="$(wc -l < "$WORK/fail" | tr -d ' ')"
  printf '%-28s リクエスト %4s 件中 失敗 %3s 件' "$1" "$total" "$fail"
  if [ "$fail" = "0" ]; then
    echo "  ✅ 無停止"
  else
    echo "  ❌"
    echo "     最初の失敗: $(head -1 "$WORK/fail")  最後の失敗: $(tail -1 "$WORK/fail")  基準時刻: $PHASE_START"
  fi
}

# ---- 準備 ----
docker network create "$NET" >/dev/null 2>&1 || true
mkdir -p "$WORK/conf.d"
write_upstream "$PORT_A"
cat > "$WORK/nginx.conf" <<'EOF'
events {}
http {
  include /etc/nginx/conf.d/*.conf;
  server {
    listen 80;
    location / { proxy_pass http://juken_map_app; }
  }
}
EOF

start_app zdt-app-a "$PORT_A" A
wait_ok "$PORT_A" || { echo "アプリAが起動しない"; exit 1; }

docker run -d --name "$NGINX" --network "$NET" \
  --add-host host.docker.internal:host-gateway \
  -p "$PROXY_PORT":80 \
  -v "$WORK/nginx.conf:/etc/nginx/nginx.conf:ro" \
  -v "$WORK/conf.d:/etc/nginx/conf.d:ro" \
  nginx:alpine >/dev/null
wait_ok "$PROXY_PORT" || { echo "nginx が立たない"; exit 1; }

echo "起動待ち時間 ${BOOT_SECONDS}秒 で比較する"
echo

# ---- 旧方式: 同じポートで止めてから起こす ----
settle_proxy
PHASE_START="$(date +%s.%N)"; w="$(watch_start)"; sleep 1
docker rm -f zdt-app-a >/dev/null
start_app zdt-app-a "$PORT_A" A2
wait_ok "$PORT_A" || true
sleep 1
report "旧（同じポートで入れ替え）" "$w"

# ---- 新方式: 別ポートで起こしてから向け替える ----
settle_proxy
PHASE_START="$(date +%s.%N)"; w="$(watch_start)"; sleep 1
start_app zdt-app-b "$PORT_B" B
wait_ok "$PORT_B" || { echo "アプリBが起動しない"; exit 1; }   # ここまで利用者はAを見ている
write_upstream "$PORT_B"
docker exec "$NGINX" nginx -t >/dev/null 2>&1
docker exec "$NGINX" nginx -s reload
# reload はすぐ返るが、古いワーカーは処理中の接続を終えるまで「旧設定」で動き続ける。
# 待たずに古いコンテナを止めると、その最中のリクエストだけが502になる（実際に1件出た）。
sleep "$DRAIN_SECONDS"
docker rm -f zdt-app-a >/dev/null
sleep 1
report "新（別ポート→向け替え）" "$w"
