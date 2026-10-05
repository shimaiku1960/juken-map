#!/usr/bin/env bash
# デプロイスクリプトが nginx の Go への振り分け（JUK-72）をどう扱うかを手元で試す。
#
# aws・docker・nginx などを偽物に差し替えて deploy-ec2.sh を最後まで流し、
# nginx の設定ファイルがどうなったかと、docker に何をさせたかを見る。確かめたいのは次のこと。
# - 初回はサイト設定の location / の直前に include を1行だけ差し込み、2回目は足さない
# - 振り分けファイルは deploy.yml が渡した中身になり、サイト設定の upstream（juken_map_app）も Go と同じ先へ向く（JUK-109）
# - 動いている Node のアプリのコンテナ（juken-map）は、切り替えたあとに止めて消す（JUK-109）
# - location / が1つでないサイト設定には手を出さず、何も起動せずに止まる
# - Go のスモークテストが落ちたら切り替えない（画面を配れない Go のイメージも、JUK-111）
# - nginx -t が通らなければ、向き先と振り分けを元に戻す
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEPLOY="$ROOT/.github/scripts/deploy-ec2.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

mkdir "$WORK/bin"
cat > "$WORK/bin/aws" <<'EOF'
#!/usr/bin/env bash
if [ "$1 $2" = "secretsmanager get-secret-value" ]; then
  echo '{"LINE_CHANNEL_SECRET":"l","LINE_CHANNEL_ACCESS_TOKEN":"t"}'
fi
EOF
# docker は run・rm・rename だけを記録する（run は --name の値）。NODE_RUNNING=1 なら、
# Node のアプリのコンテナ（juken-map）が動いている（inspect が成功する）ことにする。
cat > "$WORK/bin/docker" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  inspect) [ "${NODE_RUNNING:-}" = "1" ] && [ "$2" = "juken-map" ] && exit 0; exit 1 ;;
  run)
    prev=""
    for arg in "$@"; do [ "$prev" = "--name" ] && echo "run $arg" >> "$LOG"; prev="$arg"; done ;;
  rm) if [ "$2" = "-f" ]; then echo "rm -f ${*:3}" >> "$LOG"; else echo "rm $2" >> "$LOG"; fi ;;
  rename) echo "rename $2 $3" >> "$LOG" ;;
esac
exit 0
EOF
# GO_HEALTH・GO_LOGIN で Go の /api/health・/login の応答を変えられる。Go のダッシュボードは Cookie 無しなので 401。
cat > "$WORK/bin/curl" <<'EOF'
#!/usr/bin/env bash
case "${*: -1}" in
  *:808[01]/api/health) echo "${GO_HEALTH:-200}" ;;
  *:808[01]/login) echo "${GO_LOGIN:-200}" ;;
  */api/dashboard) echo 401 ;;
  *) echo 200 ;;
esac
EOF
# NGINX_BROKEN_ROUTES=1 なら、振り分けファイルに中身があるときだけ nginx -t を落とす
# （振り分けを置いた直後の確かめで失敗する場面）。
cat > "$WORK/bin/nginx" <<'EOF'
#!/usr/bin/env bash
if [ "${NGINX_BROKEN_ROUTES:-}" = "1" ] && [ -s "$GO_ROUTES_CONF" ]; then exit 1; fi
exit 0
EOF
for c in systemctl sleep; do printf '#!/usr/bin/env bash\n' > "$WORK/bin/$c"; done
chmod +x "$WORK/bin/"*

printf 'DATABASE_URL=mysql://app@rds/juken_map\nBETTER_AUTH_SECRET=s\n' > "$WORK/env"

# 本番のサイト設定（infra/nginx/README.md）を縮めたもの。雛形のコメントの location / は数えない。
SITE='server {
    #location / {
    #    try_files $uri $uri/ =404;
    #}
    server_name juken-map.com;

    location / {
        proxy_pass http://juken_map_app;
    }
}'
ROUTES='location = /api/dashboard {
    proxy_pass http://juken_map_go;
}'
ROUTES_B64="$(printf '%s\n' "$ROUTES" | base64 | tr -d '\n')"

fail=0
ok() { printf '  ✅ %s\n' "$1"; }
ng() { printf '  ❌ %s\n%s\n--- 出力\n' "$1" "$2"; cat "$WORK/out"; fail=1; }

# run_deploy [振り分けの base64] → deploy-ec2.sh を本番と同じ配送（base64 → パイプ → bash -s）で流す。
run_deploy() {
  : > "$WORK/log"
  status=0
  base64 < "$DEPLOY" | tr -d '\n' | base64 -d \
    | PATH="$WORK/bin:$PATH" LOG="$WORK/log" ENV_FILE="$WORK/env" \
      UPSTREAM_CONF="$WORK/upstream.conf" GO_UPSTREAM_CONF="$WORK/go-upstream.conf" \
      GO_ROUTES_CONF="$WORK/routes/go-routes.conf" SITE_CONF="$WORK/site" \
      bash -s -- dummy-tag "" "${1:-}" > "$WORK/out" 2>&1 || status=$?
}

# fresh → Go を足す前の本番（サイト設定の upstream は Node の3000、Go の upstream と振り分けはまだ無い）。
fresh() {
  rm -rf "$WORK/routes" "$WORK/go-upstream.conf" "$WORK"/site.bak-*
  printf '%s\n' "$SITE" > "$WORK/site"
  printf 'upstream juken_map_app {\n    server 127.0.0.1:3000;\n}\n' > "$WORK/upstream.conf"
}

port_of() { sed -nE 's/.*127\.0\.0\.1:([0-9]+).*/\1/p' "$1"; }

echo "deploy-ec2.sh の Go への振り分け:"

fresh
run_deploy "$ROUTES_B64"
expected_site='server {
    #location / {
    #    try_files $uri $uri/ =404;
    #}
    server_name juken-map.com;

    include '"$WORK"'/routes/go-routes.conf;
    location / {
        proxy_pass http://juken_map_app;
    }
}'
if [ "$status" = 0 ] && [ "$(cat "$WORK/site")" = "$expected_site" ]; then
  ok "初回は location / の直前に include を1行差し込む（コメントの location / には触らない）"
else
  ng "初回の include" "exit=$status
$(cat "$WORK/site")"
fi
if [ "$(cat "$WORK/routes/go-routes.conf")" = "$ROUTES" ] \
  && [ "$(port_of "$WORK/upstream.conf")" = 8081 ] && [ "$(port_of "$WORK/go-upstream.conf")" = 8081 ]; then
  ok "振り分けは渡した中身になり、Go は8081へ切り替わる。サイト設定の upstream も Node から Go の8081へ"
else
  ng "振り分けと向き先" "$(cat "$WORK/routes/go-routes.conf"; cat "$WORK/upstream.conf" "$WORK/go-upstream.conf")"
fi
if [ "$(cat "$WORK/log")" = "rm -f juken-map-next
run juken-map-go-next
rm juken-map-go
rename juken-map-go-next juken-map-go
rm juken-map-alloy" ]; then
  ok "Go の新しいコンテナだけを起こし、切り替え後に名前を戻す（Node のアプリのコンテナは起こさない。可観測性の設定が無いので Alloy は消す）"
else
  ng "コンテナの操作" "$(cat "$WORK/log")"
fi

run_deploy "$ROUTES_B64"
if [ "$status" = 0 ] && [ "$(grep -c 'include ' "$WORK/site")" = 1 ] \
  && [ "$(port_of "$WORK/upstream.conf")" = 8080 ] && [ "$(port_of "$WORK/go-upstream.conf")" = 8080 ]; then
  ok "2回目は include を足さず、Go は8080へ戻る（サイト設定の upstream も一緒に）"
else
  ng "2回目のデプロイ" "exit=$status
$(cat "$WORK/site" "$WORK/upstream.conf" "$WORK/go-upstream.conf")"
fi

run_deploy ""
if [ "$status" = 0 ] && [ ! -s "$WORK/routes/go-routes.conf" ]; then
  ok "振り分けを渡さなければ空になる（サイト設定の upstream が Go を向くので、全部 Go が返す）"
else
  ng "振り分け無し" "exit=$status"
fi

fresh
printf '%s\n' "$SITE" "server {" "    location / {" "    }" "}" > "$WORK/site"
before="$(cat "$WORK/site")"
run_deploy "$ROUTES_B64"
if [ "$status" != 0 ] && [ "$(cat "$WORK/site")" = "$before" ] && ! grep -q '^run' "$WORK/log"; then
  ok "location / が2つあれば、サイト設定に触らず何も起動せずに止まる"
else
  ng "location / が2つ" "exit=$status
$(cat "$WORK/log")"
fi

fresh
run_deploy "$ROUTES_B64"
printf '%s\n' "$ROUTES" "# 前回の振り分け" > "$WORK/routes/go-routes.conf"
before="$(cat "$WORK/routes/go-routes.conf")"
GO_HEALTH=500 run_deploy "$ROUTES_B64"
if [ "$status" != 0 ] && [ "$(port_of "$WORK/upstream.conf")" = 8081 ] && [ "$(port_of "$WORK/go-upstream.conf")" = 8081 ] \
  && [ "$(cat "$WORK/routes/go-routes.conf")" = "$before" ] \
  && grep -q '^rm -f juken-map-go-next$' "$WORK/log"; then
  ok "Go のスモークテストが落ちたら、切り替えずに新しいコンテナを捨てる"
else
  ng "Go のスモークテストの失敗" "exit=$status
$(cat "$WORK/log" "$WORK/upstream.conf" "$WORK/go-upstream.conf")"
fi

fresh
run_deploy "$ROUTES_B64"
GO_LOGIN=404 run_deploy "$ROUTES_B64"
if [ "$status" != 0 ] && [ "$(port_of "$WORK/upstream.conf")" = 8081 ] && [ "$(port_of "$WORK/go-upstream.conf")" = 8081 ] \
  && grep -q '^rm -f juken-map-go-next$' "$WORK/log"; then
  ok "Go が画面を配れなければ（画面の無いイメージ）、切り替えずに新しいコンテナを捨てる"
else
  ng "Go の /login の失敗" "exit=$status
$(cat "$WORK/log" "$WORK/upstream.conf" "$WORK/go-upstream.conf")"
fi

fresh
run_deploy ""
NGINX_BROKEN_ROUTES=1 run_deploy "$ROUTES_B64"
if [ "$status" != 0 ] && [ ! -s "$WORK/routes/go-routes.conf" ] \
  && [ "$(port_of "$WORK/upstream.conf")" = 8081 ] && [ "$(port_of "$WORK/go-upstream.conf")" = 8081 ] \
  && grep -q '^rm -f juken-map-go-next$' "$WORK/log"; then
  ok "nginx -t が通らなければ、向き先（サイト設定の upstream も）と振り分けを元に戻し、新しいコンテナを捨てる"
else
  ng "nginx -t の失敗" "exit=$status
$(cat "$WORK/log" "$WORK/routes/go-routes.conf" "$WORK/upstream.conf" "$WORK/go-upstream.conf")"
fi

fresh
NODE_RUNNING=1 run_deploy "$ROUTES_B64"
if [ "$status" = 0 ] && grep -q '^rm juken-map$' "$WORK/log" && grep -q 'Node のアプリのコンテナ（juken-map）を止めて消した' "$WORK/out" \
  && [ "$(grep -n '^rm juken-map$' "$WORK/log" | cut -d: -f1)" -gt "$(grep -n '^rename juken-map-go-next' "$WORK/log" | cut -d: -f1)" ]; then
  ok "動いている Node のアプリのコンテナは、Go へ切り替えたあとに止めて消す"
else
  ng "Node のコンテナの片付け" "exit=$status
$(cat "$WORK/log")"
fi

exit "$fail"
